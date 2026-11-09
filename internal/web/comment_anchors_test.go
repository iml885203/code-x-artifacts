package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// anchoredView is a comment as the API shows it, with what the anchor adds.
type anchoredView struct {
	commentView
	Anchor   json.RawMessage `json:"anchor"`
	ForAgent bool            `json:"forAgent"`
}

func (h *harness) commentRaw(c *http.Client, id, body string) (response, string) {
	h.t.Helper()
	return h.do(c, "POST", "/api/artifacts/"+id+"/comments", body, bridgeHeaders(h))
}

func TestACommentCanPointAtAPlaceAndSaysWhetherTheAgentShouldActOnIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.agent(h.token("alice"))
	id := alice.ok("publish", map[string]any{"title": "Q3 report", "content": page, "visibility": "organisation"})["id"].(string)
	bob := h.browser("bob", "pw-b")

	res, body := h.commentRaw(bob, id, `{"body":"Which quarter?","anchor":{ "block": "b3", "quote": "revenue" }}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("an anchored comment: %d %s", res.StatusCode, body)
	}
	viaHTTP := decode[anchoredView](t, body)
	if string(viaHTTP.Anchor) != `{"block":"b3","quote":"revenue"}` || !viaHTTP.ForAgent {
		t.Errorf("the new comment = %s, want its anchor kept as compact JSON and forAgent true by default", body)
	}
	_, body = h.commentRaw(bob, id, `{"body":"Lunch?","forAgent":false}`)
	if got := decode[anchoredView](t, body); got.ForAgent || len(got.Anchor) != 0 || strings.Contains(body, `"anchor"`) {
		t.Errorf("a comment for people only, on the whole page = %s", body)
	}

	// The agent reads the same two comments, and writes one of its own the same way.
	read := alice.ok("comments", map[string]any{"id": id})["comments"].([]any)
	first, second := read[0].(map[string]any), read[1].(map[string]any)
	if string(mustJSON(t, first["anchor"])) != `{"block":"b3","quote":"revenue"}` || first["forAgent"] != true {
		t.Errorf("comments through MCP: %v", first)
	}
	if _, has := second["anchor"]; has || second["forAgent"] != false {
		t.Errorf("an unanchored comment through MCP: %v", second)
	}
	h.checkAgentCommentIsSeenByAColleague(t, alice, bob, id)

	h.checkChangesCarryAnchors(t, alice, id)
}

// What changed since carries the anchor too, so an agent catching up sees where.
func (h *harness) checkChangesCarryAnchors(t *testing.T, alice *agent, id string) {
	t.Helper()
	changes := alice.ok("changes_since", map[string]any{"id": id, "since_version": 1})["comments"].([]any)
	if len(changes) != 3 || string(mustJSON(t, changes[0].(map[string]any)["anchor"])) != `{"block":"b3","quote":"revenue"}` {
		t.Errorf("changes_since comments = %v", changes)
	}
}

func (h *harness) checkAgentCommentIsSeenByAColleague(t *testing.T, alice *agent, bob *http.Client, id string) {
	t.Helper()
	made := alice.ok("comment", map[string]any{"id": id, "body": "Look here.", "anchor": map[string]any{"frame": "f1", "element": "e2"}, "for_agent": false})
	if string(mustJSON(t, made["anchor"])) != `{"element":"e2","frame":"f1"}` || made["forAgent"] != false {
		t.Errorf("comment through MCP = %v", made)
	}
	seen := h.commentsAnchored(bob, id)
	if len(seen) != 3 || string(seen[2].Anchor) != string(mustJSON(t, made["anchor"])) || seen[2].ForAgent {
		t.Errorf("what a colleague sees of the agent's comment: %+v", seen)
	}
}

func (h *harness) commentsAnchored(c *http.Client, id string) []anchoredView {
	h.t.Helper()
	_, body := h.do(c, "GET", "/api/artifacts/"+id+"/comments", "", bridgeHeaders(h))
	return decode[[]anchoredView](h.t, body)
}

func TestACommentWhoseTargetIsGoneIsStillReturnedWithItsAnchor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishShared(tok, "Notes", "<p>the old paragraph")
	bob := h.browser("bob", "pw-b")
	if res, body := h.commentRaw(bob, a.ID, `{"body":"Reword this","anchor":{"block":"gone","quote":"the old paragraph"}}`); res.StatusCode != http.StatusCreated {
		t.Fatalf("comment: %d %s", res.StatusCode, body)
	}
	if res, body := putJSON(h, "PUT", "/api/artifacts/"+a.ID, `{"title":"Notes","content":"<p>something else entirely"}`, tok); res.StatusCode != http.StatusOK {
		t.Fatalf("republish: %d %s", res.StatusCode, body)
	}
	got := h.commentsAnchored(bob, a.ID)
	if len(got) != 1 || string(got[0].Anchor) != `{"block":"gone","quote":"the old paragraph"}` {
		t.Errorf("the thread whose target is gone = %+v", got)
	}
	read := h.agent(tok).ok("comments", map[string]any{"id": a.ID})["comments"].([]any)
	if len(read) != 1 || string(mustJSON(t, read[0].(map[string]any)["anchor"])) != `{"block":"gone","quote":"the old paragraph"}` {
		t.Errorf("the same thread through MCP = %v", read)
	}
}

func TestAnAnchorIsABoundedJSONObjectAndOnlyStartsAThread(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Notes", "<p>x")
	bob := h.browser("bob", "pw-b")
	agent := h.agent(h.token("bob"))
	_, first := h.commentRaw(bob, a.ID, `{"body":"start"}`)
	parent := decode[commentView](t, first).ID

	for label, c := range map[string]struct {
		anchor any
		reply  bool
		want   string
	}{
		"a string":        {anchor: "b3", want: "JSON object"},
		"a list":          {anchor: []any{"b3"}, want: "JSON object"},
		"an empty object": {anchor: map[string]any{}, want: "JSON object"},
		"too big":         {anchor: map[string]any{"quote": strings.Repeat("x", 2001)}, want: "2000"},
		"on a reply":      {anchor: map[string]any{"block": "b3"}, reply: true, want: "reply"},
	} {
		httpBody := map[string]any{"body": "hi", "anchor": c.anchor}
		toolArgs := map[string]any{"id": a.ID, "body": "hi", "anchor": c.anchor}
		if c.reply {
			httpBody["parentId"], toolArgs["reply_to"] = parent, parent
		}
		res, body := h.commentRaw(bob, a.ID, string(mustJSON(t, httpBody)))
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Errorf("%s over HTTP: %d %s, want a 400 that mentions %q", label, res.StatusCode, body, c.want)
		}
		// The tool's schema may stop a value of the wrong type before the rule sees it; either way it is refused and nothing is stored.
		if got := agent.refused("comment", toolArgs); got == "" {
			t.Errorf("%s through MCP was not refused", label)
		}
	}
	if n := len(h.comments(bob, a.ID)); n != 1 {
		t.Errorf("refused anchors stored comments: %d", n)
	}
	// null is the same as leaving it out.
	if res, body := h.commentRaw(bob, a.ID, `{"body":"hi","anchor":null}`); res.StatusCode != http.StatusCreated || strings.Contains(body, `"anchor"`) {
		t.Errorf("a null anchor: %d %s", res.StatusCode, body)
	}
}

func TestAStrangerCannotTellAnAnchoredCommentWasRefusedOrThatTheArtifactExists(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	secret := h.publish(h.token("alice"), "Secret plan", "<p>secret")
	bob := h.browser("bob", "pw-b")
	body := `{"body":"hi","anchor":{"block":"b1"},"forAgent":false}`
	wantRes, wantBody := h.commentRaw(bob, "nosuchartifact0000", body)
	res, got := h.commentRaw(bob, secret.ID, body)
	if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || strings.ReplaceAll(got, secret.ID, "ID") != strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID") {
		t.Errorf("anchored comment on a hidden artifact: %d %q, want what a missing one says: %d %q", res.StatusCode, got, wantRes.StatusCode, wantBody)
	}
	// A malformed anchor does not turn the 404 into a 400 that proves the artifact exists.
	res, got = h.commentRaw(bob, secret.ID, `{"body":"hi","anchor":"x"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("bad anchor on a hidden artifact: %d %s", res.StatusCode, got)
	}
}
