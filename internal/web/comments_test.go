package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// commentView is a comment as the API shows it.
type commentView struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	ParentID  string `json:"parentId"`
	Unread    bool   `json:"unread"`
	CreatedAt string `json:"createdAt"`
}

func (h *harness) comments(c *http.Client, id string) []commentView {
	h.t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts/"+id+"/comments", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("list comments: %d %s", res.StatusCode, body)
	}
	return decode[[]commentView](h.t, body)
}

func (h *harness) comment(c *http.Client, id, text, parent string) (response, string) {
	h.t.Helper()
	body := `{"body":` + string(mustJSON(h.t, text))
	if parent != "" {
		body += `,"parentId":"` + parent + `"`
	}
	return h.do(c, "POST", "/api/artifacts/"+id+"/comments", body+"}", bridgeHeaders(h))
}

func (h *harness) summary(c *http.Client, id string) store.CommentSummary {
	h.t.Helper()
	_, body := h.do(c, "GET", "/api/artifacts/"+id, "", bridgeHeaders(h))
	return decode[artifactView](h.t, body).Comments
}

func TestWhoCannotSeeAnArtifactCannotReadOrWriteItsComments(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	secret := h.publish(h.token("alice"), "Secret plan", "<p>secret")
	alice := h.browser("alice", "pw-a")
	if res, body := h.comment(alice, secret.ID, "a note to self", ""); res.StatusCode != http.StatusCreated {
		t.Fatalf("the author comments on their own private artifact: %d %s", res.StatusCode, body)
	}
	bob := h.browser("bob", "pw-b")

	type call struct{ method, path, body string }
	routes := func(id string) []call {
		api := "/api/artifacts/" + id + "/comments"
		return []call{
			{"GET", api, ""},
			{"POST", api, `{"body":"hello"}`},
			{"POST", api + "/seen", `{"upTo":"x"}`},
		}
	}
	missing, hidden := routes("nosuchartifact0000"), routes(secret.ID)
	for i := range hidden {
		wantRes, wantBody := h.do(bob, hidden[i].method, missing[i].path, missing[i].body, bridgeHeaders(h))
		res, body := h.do(bob, hidden[i].method, hidden[i].path, hidden[i].body, bridgeHeaders(h))
		wantBody = strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID")
		body = strings.ReplaceAll(body, secret.ID, "ID")
		if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || body != wantBody {
			t.Errorf("%s %s: got %d %q, want %d %q", hidden[i].method, hidden[i].path, res.StatusCode, body, wantRes.StatusCode, wantBody)
		}
	}

	agent := h.agent(h.token("bob"))
	for tool, args := range map[string]map[string]any{
		"comments": {"id": secret.ID},
		"comment":  {"id": secret.ID, "body": "hello"},
	} {
		if got := agent.refused(tool, args); !strings.Contains(got, "no artifact with that id is visible to you") {
			t.Errorf("%s on a hidden artifact: %q", tool, got)
		}
	}
	if n := len(h.comments(alice, secret.ID)); n != 1 {
		t.Errorf("the stranger's attempts changed the comments: %d", n)
	}
}

func TestAnonymousReadersOfAPublicArtifactCannotReadOrWriteComments(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	a := h.publishAs(h.token("alice"), "Open page", "<p>x", store.VisibilityPublic)
	for _, method := range []string{"GET", "POST"} {
		res, _ := h.do(nil, method, "/api/artifacts/"+a.ID+"/comments", `{"body":"hi"}`, bridgeHeaders(h))
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s comments: %d, want 401", method, res.StatusCode)
		}
	}
	_, page := h.do(nil, "GET", "/a/"+a.ID, "", nil)
	if bootOf(t, page).Artifact.CanComment {
		t.Error("the page tells an anonymous reader they may comment")
	}
	_, page = h.do(h.browser("bob", "pw-b"), "GET", "/a/"+a.ID, "", nil)
	if !bootOf(t, page).Artifact.CanComment {
		t.Error("the page does not tell a signed-in reader they may comment")
	}
	if n := len(h.comments(h.browser("bob", "pw-b"), a.ID)); n != 0 {
		t.Errorf("comments on a fresh artifact: %d", n)
	}
}

func TestAColleagueCommentsAndTheAuthorsAgentReadsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Q3 report", "content": page, "visibility": "organisation"})["id"].(string)

	res, body := h.comment(h.browser("bob", "pw-b"), id, "The revenue chart is missing October.", "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("a colleague comments: %d %s", res.StatusCode, body)
	}
	first := decode[commentView](t, body)
	if first.Author != "bob" || first.Body != "The revenue chart is missing October." || first.ID == "" || first.CreatedAt == "" {
		t.Fatalf("the new comment = %+v", first)
	}

	// The agent finds the page by what the list says about it, then reads.
	found := alice.ok("list", nil)["artifacts"].([]any)[0].(map[string]any)["comments"].(map[string]any)
	if found["count"] != float64(1) || found["unread"] != float64(1) {
		t.Fatalf("list says %v about the comments", found)
	}
	read := alice.ok("comments", map[string]any{"id": id})["comments"].([]any)
	if len(read) != 1 || read[0].(map[string]any)["body"] != first.Body || read[0].(map[string]any)["author"] != "bob" || read[0].(map[string]any)["unread"] != true {
		t.Fatalf("comments = %v", read)
	}
}

func TestTheAuthorsAgentRepliesAndTheColleagueSeesItInTheThread(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Q3 report", "content": page, "visibility": "organisation"})["id"].(string)
	bob := h.browser("bob", "pw-b")
	_, body := h.comment(bob, id, "The revenue chart is missing October.", "")
	first := decode[commentView](t, body)

	reply := alice.ok("comment", map[string]any{"id": id, "body": "Added October, thanks.", "reply_to": first.ID})
	if reply["author"] != "alice" || reply["parentId"] != first.ID {
		t.Fatalf("comment = %v", reply)
	}
	seen := h.comments(bob, id)
	if len(seen) != 2 || seen[1].Body != "Added October, thanks." || seen[1].ParentID != first.ID || seen[1].Author != "alice" {
		t.Errorf("the colleague sees %+v", seen)
	}
	// Replying to a reply lands in the same thread.
	res, body := h.comment(bob, id, "Looks right now.", seen[1].ID)
	if res.StatusCode != http.StatusCreated || decode[commentView](t, body).ParentID != first.ID {
		t.Errorf("a reply to a reply: %d %s", res.StatusCode, body)
	}
}

func TestTheAuthorIsToldOfNewCommentsUntilTheyHaveSeenThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Notes", "<p>x")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	agent := h.agent(h.token("alice"))

	if got := h.summary(alice, a.ID); got != (store.CommentSummary{}) {
		t.Fatalf("before any comment: %+v", got)
	}
	h.comment(bob, a.ID, "one", "")
	_, two := h.comment(bob, a.ID, "two", "")
	h.comment(alice, a.ID, "my own words", "")

	if got := h.summary(alice, a.ID); got != (store.CommentSummary{Count: 3, Unread: 2}) {
		t.Errorf("the author's artifact says %+v, want 3 comments and 2 unread (their own is not new)", got)
	}
	if got := h.summary(bob, a.ID); got != (store.CommentSummary{Count: 3}) {
		t.Errorf("a colleague's view says %+v: only the author is told of new comments", got)
	}
	_, list := h.do(alice, "GET", "/api/artifacts", "", bridgeHeaders(h))
	if got := decode[[]artifactView](t, list)[0].Comments; got.Unread != 2 {
		t.Errorf("the gallery's list says %+v", got)
	}
	_, pg := h.do(alice, "GET", "/a/"+a.ID, "", nil)
	if got := bootOf(t, pg).Artifact.Comments; got.Unread != 2 {
		t.Errorf("the viewer page says %+v", got)
	}

	// Seen up to the second comment, as displayed: a comment that arrived after the list was read stays unread.
	res, _ := h.do(alice, "POST", "/api/artifacts/"+a.ID+"/comments/seen", `{"upTo":"`+decode[commentView](t, two).ID+`"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("mark seen: %d", res.StatusCode)
	}
	if got := h.summary(alice, a.ID).Unread; got != 0 {
		t.Errorf("after seeing them: %d unread", got)
	}
	h.comment(bob, a.ID, "three", "")
	if got := h.summary(alice, a.ID).Unread; got != 1 {
		t.Errorf("a new comment after that: %d unread", got)
	}
	// Seeing an older comment never un-sees a newer one.
	h.do(alice, "POST", "/api/artifacts/"+a.ID+"/comments/seen", `{"upTo":"`+decode[commentView](t, two).ID+`"}`, bridgeHeaders(h))
	if got := h.summary(alice, a.ID).Unread; got != 1 {
		t.Errorf("marking an older comment moved the mark back: %d unread", got)
	}

	// An agent that reads for the author can say it has: that is the same mark.
	agent.ok("comments", map[string]any{"id": a.ID})
	if got := h.summary(alice, a.ID).Unread; got != 1 {
		t.Errorf("reading alone marked comments as seen: %d unread", got)
	}
	agent.ok("comments", map[string]any{"id": a.ID, "mark_read": true})
	if got := h.summary(alice, a.ID).Unread; got != 0 {
		t.Errorf("after mark_read: %d unread", got)
	}
}

func TestACommentIsPlainTextOfBoundedSizeOnARealThread(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Notes", "<p>x")
	bob := h.browser("bob", "pw-b")
	other := h.publishShared(h.token("alice"), "Other", "<p>y")
	_, elsewhere := h.comment(bob, other.ID, "on the other page", "")

	for label, c := range map[string]struct{ text, parent, want string }{
		"nothing":                    {"", "", "write something"},
		"only spaces":                {"  \n ", "", "write something"},
		"too long":                   {strings.Repeat("x", 4001), "", "4000"},
		"a NUL":                      {"a\x00b", "", "text"},
		"a parent that is not there": {"hi", "nosuchcomment", "reply_to"},
		"a parent on another page":   {"hi", decode[commentView](t, elsewhere).ID, "reply_to"},
	} {
		res, body := h.comment(bob, a.ID, c.text, c.parent)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s, want a 400 that mentions %q", label, res.StatusCode, body, c.want)
		}
	}
	agent := h.agent(h.token("bob"))
	if got := agent.refused("comment", map[string]any{"id": a.ID, "body": "   "}); !strings.Contains(got, "write something") {
		t.Errorf("an empty comment through MCP: %q", got)
	}

	// Markup is kept as the words it is; showing it as text is the front end's job.
	res, body := h.comment(bob, a.ID, "<script>alert(1)</script> & more", "")
	if res.StatusCode != http.StatusCreated || decode[commentView](t, body).Body != "<script>alert(1)</script> & more" {
		t.Errorf("markup in a comment: %d %s", res.StatusCode, body)
	}
	if n := len(h.comments(bob, a.ID)); n != 1 {
		t.Errorf("refused comments were stored: %d", n)
	}
}

func TestDeletingAnArtifactDeletesItsComments(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Notes", "<p>x")
	h.comment(h.browser("bob", "pw-b"), a.ID, "hello", "")
	if res, body := h.do(h.browser("alice", "pw-a"), "DELETE", "/api/artifacts/"+a.ID, "", bridgeHeaders(h)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", res.StatusCode, body)
	}
	left, err := h.st.Comments(t.Context(), a.ID)
	if err != nil || len(left) != 0 {
		t.Errorf("comments of a deleted artifact: %v, %v", left, err)
	}
}

func TestEveryCommentToolIsDescribedForTheAgent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tools, err := h.agent(h.token("alice")).session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range tools.Tools {
		if tool.Name != "comments" && tool.Name != "comment" {
			continue
		}
		found++
		for _, want := range []string{"comment"} {
			if !strings.Contains(tool.Description, want) {
				t.Errorf("%s: description never says %q", tool.Name, want)
			}
		}
		if tool.Name == "comment" && (!strings.Contains(tool.Description, "reply_to") || !strings.Contains(tool.Description, "4000")) {
			t.Errorf("comment's description lacks reply_to or the size limit: %s", tool.Description)
		}
		if tool.Name == "comments" && !strings.Contains(tool.Description, "mark_read") {
			t.Errorf("comments' description never says mark_read: %s", tool.Description)
		}
	}
	if found != 2 {
		t.Errorf("found %d comment tools, want 2", found)
	}
}
