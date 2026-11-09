package web

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// threadOn starts a comment thread anchored on a block of a doc, with a reply,
// and gives the id of the thread.
func threadOn(t *testing.T, a *agent, id, block, quote string) string {
	t.Helper()
	thread := a.ok("comment", map[string]any{"id": id, "body": "Is this right?", "anchor": map[string]any{"block": block, "quote": quote}})["id"].(string)
	a.ok("comment", map[string]any{"id": id, "body": "I think so.", "reply_to": thread})
	return thread
}

// editOverHTTP sends a doc_edit over HTTP and gives its result.
func (h *harness) editOverHTTP(tok, id, ops string) map[string]any {
	h.t.Helper()
	res, body := api(h, "POST", "/api/artifacts/"+id+"/edit", `{"ops":`+ops+`}`, tok)
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("edit over HTTP: %d %s", res.StatusCode, body)
	}
	return decode[map[string]any](h.t, body)
}

func editOverMCP(t *testing.T, a *agent, id, ops string) map[string]any {
	t.Helper()
	var list []any
	if err := jsonUnmarshal(ops, &list); err != nil {
		t.Fatal(err)
	}
	return a.ok("doc_edit", map[string]any{"id": id, "ops": list})
}

func TestDeletingABlockWithACommentThreadWarnsWithTheThreadAndDeletingOneWithoutCommentsDoesNot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	alice := h.agent(tok)
	viaHTTP, viaMCP := h.editableDoc(tok), h.editableDoc(tok)
	threads := map[string]string{}
	for _, id := range []string{viaHTTP, viaMCP} {
		threads[id] = threadOn(t, alice, id, "p2", "second")
		alice.ok("comment", map[string]any{"id": id, "body": "On the whole page."})
	}

	results := map[string]map[string]any{
		viaHTTP: h.editOverHTTP(tok, viaHTTP, `[{"op":"delete","id":"p2"}]`),
		viaMCP:  editOverMCP(t, alice, viaMCP, `[{"op":"delete","id":"p2"}]`),
	}
	for id, res := range results {
		warned, _ := res["commentsOnDeletedBlocks"].([]any)
		want := `[{"block":"p2","id":"` + threads[id] + `","quote":"second"}]`
		if string(mustJSON(t, warned)) != want {
			t.Errorf("the warning = %s, want %s (result %v)", mustJSON(t, warned), want, res)
		}
		review, _ := res["review"].([]any)
		if len(review) != 1 || !strings.Contains(review[0].(string), "commentsOnDeletedBlocks") {
			t.Errorf("the review does not point at the warning: %v", review)
		}
	}
	if next, _ := results[viaMCP]["next"].(string); !strings.Contains(next, "Review before you tell the person") {
		t.Errorf("the next step does not ask the agent to review: %q", next)
	}

	quiet := map[string]map[string]any{
		viaHTTP: h.editOverHTTP(tok, viaHTTP, `[{"op":"delete","id":"p1"}]`),
		viaMCP:  editOverMCP(t, alice, viaMCP, `[{"op":"delete","id":"p1"}]`),
	}
	for _, res := range quiet {
		if _, has := res["commentsOnDeletedBlocks"]; has || res["review"] != nil {
			t.Errorf("deleting a block nobody commented on warned: %v", res)
		}
	}
}

func TestADeleteThatWarnsStillAppliesAndTheThreadIsStillReturnedWithItsQuote(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	alice := h.agent(tok)
	id := h.editableDoc(tok)
	thread := threadOn(t, alice, id, "p2", "second")

	res := editOverMCP(t, alice, id, `[{"op":"delete","id":"p2"}]`)
	if res["saved"] != true || string(mustJSON(t, res["deleted"])) != `["p2"]` {
		t.Fatalf("the edit = %v", res)
	}
	blocks := alice.ok("read", map[string]any{"id": id, "view": "blocks"})["tabs"].([]any)[0].(map[string]any)["blocks"].([]any)
	for _, b := range blocks {
		if b.(map[string]any)["id"] == "p2" {
			t.Errorf("the block is still in the doc: %v", blocks)
		}
	}
	list := alice.ok("comments", map[string]any{"id": id})["comments"].([]any)
	if len(list) != 2 {
		t.Fatalf("comments = %v", list)
	}
	first := list[0].(map[string]any)
	if first["id"] != thread || string(mustJSON(t, first["anchor"])) != `{"block":"p2","quote":"second"}` {
		t.Errorf("the thread after the edit = %v", first)
	}
	if got := h.commentsAnchored(h.browser("alice", "pw-a"), id); len(got) != 2 || string(got[0].Anchor) != `{"block":"p2","quote":"second"}` {
		t.Errorf("the thread over HTTP after the edit = %+v", got)
	}
}

func TestTheDocEditToolAndItsDocDescribeTheWarningForCommentsOnDeletedBlocks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tools, err := h.agent(h.token("alice")).session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var desc string
	for _, tool := range tools.Tools {
		if tool.Name == "doc_edit" {
			desc = tool.Description
		}
	}
	for _, want := range []string{"commentsOnDeletedBlocks", "not blocked", "comments"} {
		if !strings.Contains(desc, want) {
			t.Errorf("the doc_edit description never says %q:\n%s", want, desc)
		}
	}
	raw, err := os.ReadFile("../../docs/doc-edit.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc := string(raw); !strings.Contains(doc, "commentsOnDeletedBlocks") || strings.Contains(doc, "還沒有「刪掉了有留言的區塊」") {
		t.Errorf("docs/doc-edit.md does not describe the warning as it is now")
	}
}
