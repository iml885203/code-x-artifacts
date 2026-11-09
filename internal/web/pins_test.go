package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func (h *harness) pin(c *http.Client, method, id string) (response, string) {
	h.t.Helper()
	return h.do(c, method, "/api/artifacts/"+id+"/pin", "", bridgeHeaders(h))
}

// pinnedIn is what the list says about one artifact for the person behind c.
func (h *harness) pinnedIn(c *http.Client, id string) (pinned, listed bool) {
	h.t.Helper()
	_, body := h.do(c, "GET", "/api/artifacts", "", bridgeHeaders(h))
	for _, a := range decode[[]artifactView](h.t, body) {
		if a.ID == id {
			return a.Pinned, true
		}
	}
	return false, false
}

func TestPinsArePerPerson(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")

	for range 2 { // pinning twice is the same as once
		if res, body := h.pin(bob, "PUT", a.ID); res.StatusCode != http.StatusOK || !decode[artifactView](t, body).Pinned {
			t.Fatalf("bob pins: %d %s", res.StatusCode, body)
		}
	}
	if pinned, _ := h.pinnedIn(bob, a.ID); !pinned {
		t.Error("bob's list does not report his pin")
	}
	if pinned, listed := h.pinnedIn(alice, a.ID); !listed || pinned {
		t.Errorf("alice sees bob's pin: pinned=%v listed=%v", pinned, listed)
	}
	_, body := h.do(bob, "GET", "/api/artifacts/"+a.ID, "", bridgeHeaders(h))
	if !decode[artifactView](t, body).Pinned {
		t.Error("reading the artifact does not report the pin")
	}
	if res, body := h.pin(alice, "DELETE", a.ID); res.StatusCode != http.StatusOK || decode[artifactView](t, body).Pinned {
		t.Fatalf("alice unpins what she never pinned: %d %s", res.StatusCode, body)
	}
	if pinned, _ := h.pinnedIn(bob, a.ID); !pinned {
		t.Error("alice's unpin took bob's pin away")
	}
	if res, body := h.pin(bob, "DELETE", a.ID); res.StatusCode != http.StatusOK || decode[artifactView](t, body).Pinned {
		t.Fatalf("bob unpins: %d %s", res.StatusCode, body)
	}
	if pinned, _ := h.pinnedIn(bob, a.ID); pinned {
		t.Error("the pin survived the unpin")
	}
}

func TestAPinOfAnArtifactThatBecameInvisibleIsNotListedAndAnswersLikeAMissingOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	h.pin(bob, "PUT", a.ID)

	if res, body := h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("make private: %d %s", res.StatusCode, body)
	}
	if _, listed := h.pinnedIn(bob, a.ID); listed {
		t.Error("bob still lists an artifact he cannot see")
	}
	for _, method := range []string{"PUT", "DELETE"} {
		wantRes, wantBody := h.pin(bob, method, "nosuchartifact0000")
		res, body := h.pin(bob, method, a.ID)
		wantBody = strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID")
		body = strings.ReplaceAll(body, a.ID, "ID")
		if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || body != wantBody {
			t.Errorf("%s pin on a hidden artifact: %d %q, want %d %q", method, res.StatusCode, body, wantRes.StatusCode, wantBody)
		}
	}
}

func TestAnAnonymousViewerCannotPinAndADeletedArtifactTakesItsPinsWithIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	a := h.publishAs(h.token("alice"), "Open page", "<p>x", store.VisibilityPublic)
	if res, _ := h.do(nil, "PUT", "/api/artifacts/"+a.ID+"/pin", "", bridgeHeaders(h)); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous pin: %d, want 401", res.StatusCode)
	}
	bob := h.browser("bob", "pw-b")
	h.pin(bob, "PUT", a.ID)
	if res, _ := h.do(h.browser("alice", "pw-a"), "DELETE", "/api/artifacts/"+a.ID, "", bridgeHeaders(h)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if _, listed := h.pinnedIn(bob, a.ID); listed {
		t.Error("a deleted artifact is still listed")
	}
}

func TestPinningOverMCPShowsOverHTTPAndTheOtherWayAround(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	bobTok := h.token("bob")
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	bob := h.agent(bobTok)
	bearer := map[string]string{"Authorization": "Bearer " + bobTok}
	httpPinned := func() bool {
		_, body := h.do(nil, "GET", "/api/artifacts/"+a.ID, "", bearer)
		return decode[artifactView](t, body).Pinned
	}
	toolPinned := func() bool {
		for _, x := range bob.ok("list", nil)["artifacts"].([]any) {
			if m := x.(map[string]any); m["id"] == a.ID {
				return m["pinned"] == true
			}
		}
		t.Fatal("the artifact is not in the list tool's answer")
		return false
	}

	if httpPinned() || toolPinned() {
		t.Fatal("nothing is pinned yet")
	}
	if out := bob.ok("pin", map[string]any{"id": a.ID}); out["pinned"] != true || out["id"] != a.ID {
		t.Errorf("pin answers %v", out)
	}
	if !httpPinned() || !toolPinned() {
		t.Error("a pin made by the tool is not seen on both sides")
	}
	if res, body := h.do(nil, "DELETE", "/api/artifacts/"+a.ID+"/pin", "", bearer); res.StatusCode != http.StatusOK {
		t.Fatalf("unpin over HTTP: %d %s", res.StatusCode, body)
	}
	if httpPinned() || toolPinned() {
		t.Error("an unpin over HTTP is not seen on both sides")
	}
	if out := bob.ok("unpin", map[string]any{"id": a.ID}); out["pinned"] != false {
		t.Errorf("unpin answers %v", out)
	}

	// The same refusal as HTTP for an artifact bob cannot see.
	hidden := h.publish(h.token("alice"), "Hidden", "<p>x")
	for _, tool := range []string{"pin", "unpin"} {
		if got := bob.refused(tool, map[string]any{"id": hidden.ID}); !strings.Contains(got, "no artifact with that id is visible to you") {
			t.Errorf("%s on a hidden artifact: %q", tool, got)
		}
	}
}
