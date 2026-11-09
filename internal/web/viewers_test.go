package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func (h *harness) beat(c *http.Client, id string) (response, string) {
	h.t.Helper()
	return h.do(c, "POST", "/api/artifacts/"+id+"/viewers", "", bridgeHeaders(h))
}

func (h *harness) viewersSeenBy(c *http.Client, id string) (response, []service.ViewerEntry) {
	h.t.Helper()
	res, body := h.do(c, "GET", "/api/artifacts/"+id+"/viewers", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	return res, decode[[]service.ViewerEntry](h.t, body)
}

func viewerNames(list []service.ViewerEntry) []string {
	out := []string{}
	for _, e := range list {
		out = append(out, e.Name)
	}
	return out
}

func TestAViewersHeartbeatShowsThemToAnotherViewer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")

	if res, body := h.beat(bob, a.ID); res.StatusCode != http.StatusOK || !slices.Equal(viewerNames(decode[[]service.ViewerEntry](t, body)), []string{"BOB"}) {
		t.Fatalf("bob's heartbeat answers: %d %s", res.StatusCode, body)
	}
	h.beat(alice, a.ID)
	res, list := h.viewersSeenBy(alice, a.ID)
	if res.StatusCode != http.StatusOK || !slices.Equal(viewerNames(list), []string{"ALICE", "BOB"}) || !list[0].Self || list[1].Self || list[0].ID != "alice" || list[1].ID != "bob" {
		t.Errorf("alice reads %d %+v, want herself then bob by account", res.StatusCode, list)
	}
}

func TestWhoCannotSeeAnArtifactIsRefusedTheHeartbeatAndTheListLikeAMissingOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	private := h.publishAs(h.token("alice"), "Mine", "<p>x", store.VisibilityPrivate)
	bob := h.browser("bob", "pw-b")

	for _, method := range []string{"GET", "POST"} {
		res, body := h.do(bob, method, "/api/artifacts/"+private.ID+"/viewers", "", bridgeHeaders(h))
		missing, missingBody := h.do(bob, method, "/api/artifacts/no-such-id/viewers", "", bridgeHeaders(h))
		if res.StatusCode != http.StatusNotFound || res.StatusCode != missing.StatusCode || body != missingBody {
			t.Errorf("%s by someone who cannot see it: %d %q, a missing artifact gives %d %q", method, res.StatusCode, body, missing.StatusCode, missingBody)
		}
	}
	if _, list := h.viewersSeenBy(h.browser("alice", "pw-a"), private.ID); len(list) != 0 {
		t.Errorf("bob's refused heartbeat left an entry: %+v", list)
	}
}

func TestAPersonWhoLosesAccessLeavesTheListAndCannotReadIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	h.beat(alice, a.ID)
	h.beat(bob, a.ID)
	if _, list := h.viewersSeenBy(alice, a.ID); len(list) != 2 {
		t.Fatalf("before: %+v", list)
	}

	if res, body := h.do(nil, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, h.bearer("alice")); res.StatusCode != http.StatusOK {
		t.Fatalf("unshare: %d %s", res.StatusCode, body)
	}
	if _, list := h.viewersSeenBy(alice, a.ID); !slices.Equal(viewerNames(list), []string{"ALICE"}) {
		t.Errorf("bob is still listed after losing access: %+v", list)
	}
	if res, _ := h.viewersSeenBy(bob, a.ID); res.StatusCode != http.StatusNotFound {
		t.Errorf("bob reads the list after losing access: %d", res.StatusCode)
	}
	if res, _ := h.beat(bob, a.ID); res.StatusCode != http.StatusNotFound {
		t.Errorf("bob's heartbeat after losing access: %d", res.StatusCode)
	}
}

func TestReadingOverMCPReturnsTheViewersUnderTheSameAccessAndShowsTheAgentToThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Shared page", "<p>x")
	private := h.publishAs(h.token("alice"), "Mine", "<p>x", store.VisibilityPrivate)
	alice := h.browser("alice", "pw-a")
	h.beat(alice, a.ID)
	bobsAgent := h.agent(h.token("bob"))

	read := bobsAgent.ok("read", map[string]any{"id": a.ID})
	viewers, _ := read["viewers"].([]any)
	if len(viewers) != 1 || viewers[0].(map[string]any)["name"] != "ALICE" || viewers[0].(map[string]any)["agent"] != false {
		t.Fatalf("read.viewers = %v, want alice as a person", read["viewers"])
	}
	_, list := h.viewersSeenBy(alice, a.ID)
	if len(list) != 2 || !list[1].Agent || !strings.HasPrefix(list[1].Name, "bob 的 ") || !strings.HasPrefix(list[1].ID, "bob/") {
		t.Errorf("alice sees %+v, want bob's agent while it reads", list)
	}
	if refusal := bobsAgent.refused("read", map[string]any{"id": private.ID}); refusal == "" {
		t.Error("an agent read a private page")
	}
}
