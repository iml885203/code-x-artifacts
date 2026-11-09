package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func (h *harness) open(c *http.Client, id string) (response, string) {
	h.t.Helper()
	return h.do(c, "POST", "/api/artifacts/"+id+"/open", "", bridgeHeaders(h))
}

// recentlyOpened is the ids of the "Recently opened" list of the person behind c, in its order.
func (h *harness) recentlyOpened(c *http.Client) []string {
	h.t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts?order=opened", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("recently opened: %d %s", res.StatusCode, body)
	}
	ids := []string{}
	for _, a := range decode[[]artifactView](h.t, body) {
		if a.OpenedAt == nil {
			h.t.Errorf("%s is in the recently opened list without the time it was opened", a.ID)
		}
		ids = append(ids, a.ID)
	}
	return ids
}

func TestAfterBobOpensAThenBHisRecentListIsBThenAAndAlicesHasNeither(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok := h.token("alice")
	a := h.publishShared(aliceTok, "Page A", "<p>a")
	b := h.publishShared(aliceTok, "Page B", "<p>b")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")

	for _, id := range []string{a.ID, b.ID} {
		if res, body := h.open(bob, id); res.StatusCode != http.StatusOK || decode[artifactView](t, body).OpenedAt == nil {
			t.Fatalf("bob opens %s: %d %s", id, res.StatusCode, body)
		}
	}
	if got := h.recentlyOpened(bob); !slices.Equal(got, []string{b.ID, a.ID}) {
		t.Errorf("bob's recent list = %v, want B then A %v", got, []string{b.ID, a.ID})
	}
	if got := h.recentlyOpened(alice); len(got) != 0 {
		t.Errorf("alice's recent list holds bob's opens: %v", got)
	}
	_, body := h.do(alice, "GET", "/api/artifacts", "", bridgeHeaders(h))
	for _, x := range decode[[]artifactView](t, body) {
		if x.OpenedAt != nil {
			t.Errorf("alice is told when %s was opened, by someone else", x.ID)
		}
	}

	h.open(bob, a.ID)
	if got := h.recentlyOpened(bob); !slices.Equal(got, []string{a.ID, b.ID}) {
		t.Errorf("opening A again does not bring it first: %v", got)
	}
	if res, body := h.do(bob, "GET", "/api/artifacts?order=sideways", "", bridgeHeaders(h)); res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "opened") {
		t.Errorf("an unknown order: %d %s", res.StatusCode, body)
	}
	if res, page := h.do(bob, "GET", "/recent", "", nil); res.StatusCode != http.StatusOK || bootOf(t, page).Page != "gallery" {
		t.Errorf("the Recently opened page is not the library: %d", res.StatusCode)
	}
}

func TestAnArtifactBobCanNoLongerSeeIsNotInHisRecentList(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	aliceTok := h.token("alice")
	hidden := h.publishShared(aliceTok, "Soon private", "<p>x")
	gone := h.publishShared(aliceTok, "Soon deleted", "<p>x")
	stays := h.publishAs(aliceTok, "Public", "<p>x", store.VisibilityPublic)
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	for _, id := range []string{hidden.ID, gone.ID, stays.ID} {
		h.open(bob, id)
	}

	if res, body := h.do(alice, "PATCH", "/api/artifacts/"+hidden.ID, `{"visibility":"private"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("make private: %d %s", res.StatusCode, body)
	}
	if res, _ := h.do(alice, "DELETE", "/api/artifacts/"+gone.ID, "", bridgeHeaders(h)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if got := h.recentlyOpened(bob); !slices.Equal(got, []string{stays.ID}) {
		t.Errorf("bob's recent list = %v, want only %s", got, stays.ID)
	}

	wantRes, wantBody := h.open(bob, "nosuchartifact0000")
	res, body := h.open(bob, hidden.ID)
	if res.StatusCode != http.StatusNotFound || res.StatusCode != wantRes.StatusCode || strings.ReplaceAll(body, hidden.ID, "ID") != strings.ReplaceAll(wantBody, "nosuchartifact0000", "ID") {
		t.Errorf("opening a hidden artifact: %d %q, want %d %q", res.StatusCode, body, wantRes.StatusCode, wantBody)
	}
	// An anonymous reader of a public link is counted in the usage, but keeps no list.
	if res, body := h.do(nil, "POST", "/api/artifacts/"+stays.ID+"/open", "", bridgeHeaders(h)); res.StatusCode != http.StatusOK || decode[artifactView](t, body).OpenedAt != nil {
		t.Errorf("an anonymous open of a public link: %d %s, want 200 without openedAt", res.StatusCode, body)
	}
	if res, _ := h.do(nil, "POST", "/api/artifacts/"+stays.ID+"/open", "", map[string]string{"Authorization": "Bearer " + h.token("bob")}); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("an open with a token, not the viewer: %d, want 401", res.StatusCode)
	}
}

func TestTheListToolInOpenedOrderReturnsTheSameArtifactsAsTheRecentlyOpenedPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	a := h.publishShared(aliceTok, "Page A", "<p>a")
	b := h.publishShared(aliceTok, "Page B", "<p>b")
	h.publishShared(aliceTok, "Never opened", "<p>c")
	bob := h.browser("bob", "pw-b")
	h.open(bob, a.ID)
	h.open(bob, b.ID)
	agent := h.agent(bobTok)

	var tool []string
	for _, x := range agent.ok("list", map[string]any{"order": "opened"})["artifacts"].([]any) {
		m := x.(map[string]any)
		if m["openedAt"] == nil {
			t.Errorf("the tool lists %v without when it was opened", m["id"])
		}
		tool = append(tool, m["id"].(string))
	}
	if page := h.recentlyOpened(bob); !slices.Equal(tool, page) || !slices.Equal(tool, []string{b.ID, a.ID}) {
		t.Errorf("list order=opened = %v, the page = %v, want B then A", tool, page)
	}
	if got := agent.refused("list", map[string]any{"order": "sideways"}); !strings.Contains(got, `"opened"`) {
		t.Errorf("an unknown order over MCP: %q", got)
	}
	if out := agent.ok("read", map[string]any{"id": b.ID}); out["openedAt"] == nil {
		t.Error("read does not tell the agent when its person opened the artifact")
	}
	if got := h.agent(aliceTok).ok("list", map[string]any{"order": "opened"})["artifacts"].([]any); len(got) != 0 {
		t.Errorf("alice's agent sees bob's opens: %v", got)
	}
}
