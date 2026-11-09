package web

import (
	"net/http"
	"testing"
)

func TestDesignCommentsKeepTheirSourceAnchorForReadersAndAgentsAfterAnElementIsRemoved(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	fs := designFrames()
	fs["frames/hero.html"] = `<h1 data-pid="title">Hero</h1>`
	a := h.publishDesign(tok, fs, "organisation")
	bob := h.browser("bob", "pw-b")
	anchor := map[string]any{"type": "design", "frameId": "hero", "pid": "title", "name": "Hero", "x": 120, "y": 80}
	request := string(mustJSON(t, map[string]any{"body": "Look at this heading", "anchor": anchor}))
	if res, body := h.commentRaw(bob, a.ID, request); res.StatusCode != http.StatusCreated {
		t.Fatalf("reader comment: %d %s", res.StatusCode, body)
	}
	readerAgent := h.agent(h.token("bob"))
	readerAgent.ok("comment", map[string]any{"id": a.ID, "body": "Frame feedback", "anchor": map[string]any{"type": "design", "frameId": "hero", "x": 20, "y": 20}})
	if res, body := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": `<p>Replacement</p>`})}); res.StatusCode != http.StatusOK {
		t.Fatalf("remove original element: %d %s", res.StatusCode, body)
	}
	seen := h.commentsAnchored(bob, a.ID)
	if len(seen) != 2 || string(seen[0].Anchor) != string(mustJSON(t, anchor)) {
		t.Fatalf("reader lost original thread anchor: %+v", seen)
	}
	read := readerAgent.ok("comments", map[string]any{"id": a.ID})["comments"].([]any)
	if len(read) != 2 || string(mustJSON(t, read[0].(map[string]any)["anchor"])) != string(mustJSON(t, anchor)) {
		t.Fatalf("agent lost original thread anchor: %v", read)
	}
}
