package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

type deletedCommentView struct {
	commentView
	Deleted   bool           `json:"deleted"`
	CanDelete bool           `json:"canDelete"`
	Anchor    map[string]any `json:"anchor"`
}

func TestOwnCommentDeletionKeepsOtherRepliesAndScrubsTheBodyAcrossHTTPMCPAndRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "comments.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := newHarnessOn(t, st)
	a := h.publishShared(h.token("alice"), "Review", "<p>page")
	bob := h.browser("bob", "pw-b")
	_, body := h.commentRaw(bob, a.ID, `{"body":"Remove my original words","anchor":{"type":"design","frameId":"hero","pid":"stroke","x":10,"y":20}}`)
	root := decode[deletedCommentView](t, body)
	if !root.CanDelete {
		t.Fatal("the comment author is not offered deletion")
	}
	_, body = h.comment(h.browser("alice", "pw-a"), a.ID, "Keep the other person's reply", root.ID)
	reply := decode[commentView](t, body)
	for range 2 {
		res, body := h.do(bob, "DELETE", "/api/artifacts/"+a.ID+"/comments/"+root.ID, "", bridgeHeaders(h))
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("own deletion and its repeat: %d %s", res.StatusCode, body)
		}
	}
	if summary := h.summary(h.browser("alice", "pw-a"), a.ID); summary.Count != 1 || summary.Unread != 0 {
		t.Fatalf("deleted content still contributes to counts: %+v", summary)
	}
	h.srv.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	h = newHarnessOn(t, reopened)
	bob = h.browser("bob", "pw-b")
	_, body = h.do(bob, "GET", "/api/artifacts/"+a.ID+"/comments", "", bridgeHeaders(h))
	assertSurvivingCommentThread(t, body, root.ID)
	assertCommentBodyRemoved(t, reopened, a.ID, root.ID)
	alice := h.agent(h.token("alice"))
	read := alice.ok("comments", map[string]any{"id": a.ID})
	if strings.Contains(string(mustJSON(t, read)), "Remove my original words") {
		t.Fatal("MCP retained deleted content")
	}
	for range 2 {
		if got := alice.ok("delete_comment", map[string]any{"id": a.ID, "comment_id": reply.ID}); got["deleted"] != true {
			t.Fatal("own reply deletion did not succeed")
		}
	}
	if got := h.comments(bob, a.ID); len(got) != 0 {
		t.Fatalf("an empty deleted thread is still listed: %+v", got)
	}
	assertCommentBodyRemoved(t, reopened, a.ID, reply.ID)
	changes := alice.ok("changes_since", map[string]any{"id": a.ID, "since_version": 1})
	if raw := string(mustJSON(t, changes)); strings.Contains(raw, "Remove my original words") || strings.Contains(raw, "Keep the other person's reply") {
		t.Fatal("incremental MCP read retained deleted bodies")
	}
}

func TestCommentDeletionRefusesOtherAuthorsAnonymousAndInvisibleArtifactsThroughHTTPAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	a := h.publishAs(h.token("alice"), "Public review", "<p>page", store.VisibilityPublic)
	_, body := h.comment(h.browser("bob", "pw-b"), a.ID, "Bob's words", "")
	c := decode[commentView](t, body)
	path := "/api/artifacts/" + a.ID + "/comments/" + c.ID
	if res, _ := h.do(h.browser("alice", "pw-a"), "DELETE", path, "", bridgeHeaders(h)); res.StatusCode != http.StatusForbidden {
		t.Fatalf("artifact ownership grants no deletion of another person's comment: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "DELETE", path, "", bridgeHeaders(h)); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous deletion: %d", res.StatusCode)
	}
	alice, bob := h.agent(h.token("alice")), h.agent(h.token("bob"))
	if got := alice.refused("delete_comment", map[string]any{"id": a.ID, "comment_id": c.ID}); !strings.Contains(got, "own comment") {
		t.Fatalf("MCP ownership refusal: %s", got)
	}
	if res, body := h.do(h.browser("alice", "pw-a"), "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("close access: %d %s", res.StatusCode, body)
	}
	for _, id := range []string{a.ID, "missing-artifact"} {
		res, body := h.do(h.browser("bob", "pw-b"), "DELETE", "/api/artifacts/"+id+"/comments/"+c.ID, "", bridgeHeaders(h))
		if res.StatusCode != http.StatusNotFound || strings.Contains(body, "Bob's words") {
			t.Fatalf("invisible or missing artifact deletion: %d %s", res.StatusCode, body)
		}
		if got := bob.refused("delete_comment", map[string]any{"id": id, "comment_id": c.ID}); !strings.Contains(got, "no artifact") {
			t.Fatalf("MCP invisible or missing artifact refusal: %s", got)
		}
	}
	if got := h.comments(h.browser("alice", "pw-a"), a.ID); len(got) != 1 || got[0].Body != "Bob's words" {
		t.Fatal("a refusal changed the discussion")
	}
}

func assertSurvivingCommentThread(t *testing.T, body, rootID string) {
	t.Helper()
	list := decode[[]deletedCommentView](t, body)
	if len(list) != 2 || list[0].ID != rootID || !list[0].Deleted || list[0].Body != "" || list[0].CanDelete || list[0].Anchor["pid"] != "stroke" || list[1].Body != "Keep the other person's reply" {
		t.Fatalf("the surviving thread changed: %s", body)
	}
}

func assertCommentBodyRemoved(t *testing.T, st *store.Store, artifactID, commentID string) {
	t.Helper()
	if stored, err := st.Comment(t.Context(), artifactID, commentID); err != nil || stored.Body != "" {
		t.Fatal("deleted content survived in persistence")
	}
}
