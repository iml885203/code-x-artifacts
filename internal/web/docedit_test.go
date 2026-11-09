package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const editableDoc = `{"title":"Doc","kind":"doc","blocks":[
 {"id":"h","type":"heading","level":1,"content":[{"text":"Plan"}]},
 {"id":"p1","type":"paragraph","content":[{"text":"first"}]},
 {"id":"p2","type":"paragraph","content":[{"text":"second"}]}]}`

func (h *harness) editableDoc(tok string) string {
	h.t.Helper()
	res, out := api(h, "POST", "/api/artifacts", editableDoc, tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish a doc: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out).ID
}

func TestDocEditGivesTheSameResultAndTheSameRefusalOverHTTPAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	viaHTTP, viaMCP := h.editableDoc(tok), h.editableDoc(tok)

	ops := `[{"op":"replace","id":"p1","markdown":"changed 你好 😀"},{"op":"insert","after":"h","blocks":[{"id":"img","type":"image","src":"https://example.com/a.png"}]},{"op":"delete","id":"p2"}]`
	res, body := api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+ops+`}`, tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit over HTTP: %d %s", res.StatusCode, body)
	}
	got := decode[map[string]any](t, body)
	var opsValue []any
	if err := jsonUnmarshal(ops, &opsValue); err != nil {
		t.Fatal(err)
	}
	want := agent.ok("doc_edit", map[string]any{"id": viaMCP, "ops": opsValue})
	delete(want, "next")
	if string(mustJSON(t, got)) != string(mustJSON(t, want)) {
		t.Errorf("the result differs:\n http: %s\n  mcp: %s", mustJSON(t, got), mustJSON(t, want))
	}

	// The same refusal, for a block that is not there.
	bad := `[{"op":"delete","id":"nope"}]`
	res, body = api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":`+bad+`}`, tok)
	var badValue []any
	_ = jsonUnmarshal(bad, &badValue)
	refusal := agent.refused("doc_edit", map[string]any{"id": viaMCP, "ops": badValue})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, `no block \"nope\"`) || !strings.Contains(refusal, `no block "nope"`) {
		t.Errorf("refusals: http %d %s, mcp %s", res.StatusCode, body, refusal)
	}

	// A stale hash is a 409 that carries the block as it is.
	res, body = api(h, "POST", "/api/artifacts/"+viaHTTP+"/edit", `{"ops":[{"op":"delete","id":"h","ifHash":"000000000000"}]}`, tok)
	if res.StatusCode != http.StatusConflict || !strings.Contains(body, `"stale"`) || !strings.Contains(body, `"hash"`) {
		t.Errorf("a stale hash: %d %s", res.StatusCode, body)
	}
	if refusal := agent.refused("doc_edit", map[string]any{"id": viaMCP, "ops": []any{map[string]any{"op": "delete", "id": "h", "ifHash": "000000000000"}}}); !strings.Contains(refusal, "Current state of those blocks") {
		t.Errorf("mcp: %s", refusal)
	}
}

func TestADocIsReadAsBlocksWithHashesOverHTTPAndMCPTheSame(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	agent := h.agent(tok)
	id := h.editableDoc(tok)

	res, body := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("blocks: %d %s", res.StatusCode, body)
	}
	httpRead := decode[map[string]any](t, body)
	mcpRead := agent.ok("read", map[string]any{"id": id, "view": "blocks"})
	tabs, ok := mcpRead["tabs"].([]any)
	if !ok || string(mustJSON(t, tabs)) != string(mustJSON(t, httpRead["tabs"])) {
		t.Errorf("tabs differ:\n http: %s\n  mcp: %s", mustJSON(t, httpRead["tabs"]), mustJSON(t, mcpRead["tabs"]))
	}
	if _, has := mcpRead["content"]; has {
		t.Error("the blocks view also carried the Markdown")
	}
	first := tabs[0].(map[string]any)["blocks"].([]any)[1].(map[string]any)
	if first["id"] != "p1" || first["type"] != "paragraph" || first["hash"] == "" {
		t.Errorf("a block reads as %v", first)
	}

	md := agent.ok("read", map[string]any{"id": id})
	outline, _ := md["outline"].([]any)
	if len(outline) != 3 || outline[1].(map[string]any)["id"] != "p1" || md["content"] == "" {
		t.Errorf("the Markdown view: %v", md)
	}
	if refusal := agent.refused("read", map[string]any{"id": id, "view": "xml"}); !strings.Contains(refusal, "markdown") {
		t.Errorf("view xml: %s", refusal)
	}
	// A page has no blocks.
	page := h.publish(tok, "Page", "<p>x")
	if refusal := agent.refused("read", map[string]any{"id": page.ID, "view": "blocks"}); !strings.Contains(refusal, "not a doc") {
		t.Errorf("blocks of a page: %s", refusal)
	}
}

func TestOnlyWhoMayEditCanEditADocAndAReaderCanReadItsBlocks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	id := h.editableDoc(tok)
	api(h, "PATCH", "/api/artifacts/"+id, `{"visibility":"organisation"}`, tok)
	bob := h.token("bob")
	if res, _ := api(h, "GET", "/api/artifacts/"+id+"/blocks", "", bob); res.StatusCode != http.StatusOK {
		t.Errorf("a reader reads the blocks: %d", res.StatusCode)
	}
	if res, body := api(h, "POST", "/api/artifacts/"+id+"/edit", `{"ops":[{"op":"delete","id":"p1"}]}`, bob); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader edits: %d %s", res.StatusCode, body)
	}
	other := h.token("carol")
	hidden := h.editableDoc(tok)
	if res, _ := api(h, "POST", "/api/artifacts/"+hidden+"/edit", `{"ops":[{"op":"delete","id":"p1"}]}`, other); res.StatusCode != http.StatusNotFound {
		t.Errorf("a stranger edits a private doc: %d", res.StatusCode)
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
