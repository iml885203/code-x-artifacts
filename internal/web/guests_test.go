package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const (
	visitorGuest = "visitor-browser-0001"
	otherGuest   = "visitor-browser-0002"
)

// guestBeat is a signed-out browser's heartbeat: no cookie, no token, its own random guest id.
func (h *harness) guestBeat(id, guest string) (response, string) {
	h.t.Helper()
	return h.do(nil, "POST", "/api/artifacts/"+id+"/viewers", `{"guest":"`+guest+`"}`, map[string]string{"X-Pictor": "1", "Content-Type": "application/json"})
}

func guestEntries(list []service.ViewerEntry) []service.ViewerEntry {
	out := []service.ViewerEntry{}
	for _, e := range list {
		if e.Guest {
			out = append(out, e)
		}
	}
	return out
}

// onlyItself is a guest's heartbeat that must answer with that guest alone: no names, no browser id, the server's pace.
func onlyItself(t *testing.T, h *harness, id, guest string) service.ViewerEntry {
	t.Helper()
	res, body := h.guestBeat(id, guest)
	own := decode[[]service.ViewerEntry](t, body)
	if res.StatusCode != http.StatusOK || len(own) != 1 || !own[0].Guest || !own[0].Self || own[0].Agent || !strings.HasPrefix(own[0].ID, service.GuestIDPrefix) {
		t.Fatalf("the visitor's heartbeat answers %d %s; want only itself, as a guest", res.StatusCode, body)
	}
	if strings.Contains(body, "ALICE") || strings.Contains(body, "BOB") || strings.Contains(body, guest) {
		t.Errorf("the visitor's answer holds a name or its own browser id: %s", body)
	}
	if res.Header.Get("CodeXArtifacts-Heartbeat-Ms") == "" {
		t.Error("the guest's page is not told the server's pace")
	}
	return own[0]
}

func TestASignedOutVisitorOfAPublicLinkIsListedAsAGuestForSignedInReadersAndReadsOnlyItself(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	h.beat(alice, pub.ID)
	h.beat(bob, pub.ID)
	h.guestBeat(pub.ID, otherGuest)

	if res, _ := h.do(nil, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("anonymous reads the page: %d", res.StatusCode)
	}
	own := onlyItself(t, h, pub.ID, visitorGuest)
	if res, body := h.do(nil, "GET", "/api/artifacts/"+pub.ID+"/viewers", "", map[string]string{"X-Pictor": "1"}); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous GET of the viewers: %d %s, want a sign-in demand", res.StatusCode, body)
	}

	_, list := h.viewersSeenBy(alice, pub.ID)
	guests := guestEntries(list)
	if !slices.Equal(viewerNames(list)[:2], []string{"ALICE", "BOB"}) || len(guests) != 2 || !slices.ContainsFunc(guests, func(e service.ViewerEntry) bool { return e.ID == own.ID && e.Name == own.Name && !e.Self }) {
		t.Errorf("alice sees %+v; want the people, then both guests, the visitor under the name it was given", list)
	}
}

func TestAnonymousHeartbeatsCountOnlyWhereTheAnonymousMayViewAndAClosedPublicLinkDropsItsGuests(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	alice := h.browser("alice", "pw-a")
	for _, visibility := range []store.Visibility{store.VisibilityPrivate, store.VisibilityOrganisation} {
		a := h.publishAs(h.token("alice"), "Not public", "<p>x", visibility)
		res, body := h.guestBeat(a.ID, visitorGuest)
		missing, missingBody := h.guestBeat("no-such-id", visitorGuest)
		if res.StatusCode != http.StatusUnauthorized || res.StatusCode != missing.StatusCode || body != missingBody {
			t.Errorf("a guest beat on a %s page: %d %q; a missing page gives %d %q", visibility, res.StatusCode, body, missing.StatusCode, missingBody)
		}
		if _, list := h.viewersSeenBy(alice, a.ID); len(guestEntries(list)) != 0 {
			t.Errorf("a refused guest is listed on a %s page: %+v", visibility, list)
		}
	}

	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	if res, body := h.guestBeat(pub.ID, visitorGuest); res.StatusCode != http.StatusOK {
		t.Fatalf("guest beat: %d %s", res.StatusCode, body)
	}
	if res, body := h.do(nil, "PATCH", "/api/artifacts/"+pub.ID, `{"visibility":"organisation"}`, h.bearer("alice")); res.StatusCode != http.StatusOK {
		t.Fatalf("close the public link: %d %s", res.StatusCode, body)
	}
	if _, list := h.viewersSeenBy(alice, pub.ID); len(guestEntries(list)) != 0 {
		t.Errorf("the guest is still listed after the public link closed: %+v", list)
	}
	if res, body := h.guestBeat(pub.ID, visitorGuest); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a guest beat after the public link closed: %d %s", res.StatusCode, body)
	}
	if res, body := h.do(nil, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode == http.StatusOK {
		t.Errorf("the page is still served to the anonymous after the link closed: %s", body)
	}

	off := newHarness(t)
	shared := off.publishShared(off.token("alice"), "Shared", "<p>x")
	if res, _ := off.guestBeat(shared.ID, visitorGuest); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a guest beat on a deployment without public links: %d", res.StatusCode)
	}
}

// refusesMalformedGuests: what is not a bounded, URL-safe random id is refused, an account or agent id included.
func refusesMalformedGuests(t *testing.T, h *harness, id string) {
	t.Helper()
	for _, bad := range []string{"alice", "bob/claude-code", "guest:" + strings.Repeat("a", 24), strings.Repeat("a", 65), "visitor browser 0001"} {
		res, body := h.guestBeat(id, bad)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("guest id %q: %d %s, want it refused", bad, res.StatusCode, body)
		}
	}
	if res, body := h.do(nil, "POST", "/api/artifacts/"+id+"/viewers", "", map[string]string{"X-Pictor": "1"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("a signed-out beat without a guest id, as a page whose session just ended sends: %d %s", res.StatusCode, body)
	}
	if res, body := h.do(nil, "POST", "/api/artifacts/"+id+"/viewers", `{"guest":"`+strings.Repeat("a", 2<<10)+`"}`, map[string]string{"X-Pictor": "1"}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an oversized body: %d %s", res.StatusCode, body)
	}
}

func TestAGuestIDCannotPassForAnAccountOrAnAgentAndGrantsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	alice := h.browser("alice", "pw-a")
	h.beat(alice, pub.ID)

	refusesMalformedGuests(t, h, pub.ID)
	lookalike := "alice" + strings.Repeat("0", 16)
	if res, body := h.guestBeat(pub.ID, lookalike); res.StatusCode != http.StatusOK {
		t.Fatalf("a well-formed guest id: %d %s", res.StatusCode, body)
	}
	_, list := h.viewersSeenBy(alice, pub.ID)
	for _, e := range list {
		if e.Guest && (e.ID == "alice" || e.Name == "ALICE" || e.Agent || strings.Contains(e.ID, lookalike)) {
			t.Errorf("a guest passes for someone: %+v", e)
		}
	}
	if len(list) != 2 || list[0].ID != "alice" || !list[1].Guest {
		t.Errorf("alice sees %+v; want herself once and the guest", list)
	}

	guestHeaders := map[string]string{"X-Pictor": "1", "Content-Type": "application/json"}
	for _, write := range []struct{ method, path, body string }{
		{"POST", "/api/artifacts/" + pub.ID + "/comments", `{"body":"hi","guest":"` + lookalike + `"}`},
		{"PUT", "/api/artifacts/" + pub.ID, `{"title":"Demo","content":"<p>mine","baseVersion":1,"guest":"` + lookalike + `"}`},
		{"PUT", "/api/artifacts/" + pub.ID + "/db/notes/one", `{"guest":"` + lookalike + `"}`},
	} {
		if res, body := h.do(nil, write.method, write.path, write.body, guestHeaders); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("a guest's %s %s: %d %s, want a sign-in demand", write.method, write.path, res.StatusCode, body)
		}
	}
}

func TestSigningInSwitchesTheBrowserFromItsGuestToTheAccountWithoutADuplicate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	alice := h.browser("alice", "pw-a")
	h.beat(alice, pub.ID)
	h.guestBeat(pub.ID, visitorGuest)
	h.guestBeat(pub.ID, otherGuest)

	bob := h.browser("bob", "pw-b")
	res, body := h.do(bob, "POST", "/api/artifacts/"+pub.ID+"/viewers", `{"guest":"`+visitorGuest+`"}`, bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bob's first heartbeat after signing in: %d %s", res.StatusCode, body)
	}
	_, list := h.viewersSeenBy(alice, pub.ID)
	if !slices.Equal(viewerNames(list)[:2], []string{"ALICE", "BOB"}) || len(guestEntries(list)) != 1 {
		t.Errorf("alice sees %+v; want bob in place of his browser's guest, and the other browser's guest", list)
	}
}

func TestReadingOverMCPListsTheSameGuestsWithTheGuestMarkerAsHTTPForTheSameViewer(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	private := h.publishAs(h.token("alice"), "Mine", "<p>x", store.VisibilityPrivate)
	h.guestBeat(pub.ID, visitorGuest)
	h.guestBeat(pub.ID, otherGuest)
	bobTok := h.token("bob")

	res, body := h.do(nil, "GET", "/api/artifacts/"+pub.ID+"/viewers", "", map[string]string{"Authorization": "Bearer " + bobTok})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET viewers: %d %s", res.StatusCode, body)
	}
	viaHTTP := guestEntries(decode[[]service.ViewerEntry](t, body))
	read := h.agent(bobTok).ok("read", map[string]any{"id": pub.ID})
	viaMCP := []service.ViewerEntry{}
	for _, v := range read["viewers"].([]any) {
		e := v.(map[string]any)
		if e["guest"] == true {
			viaMCP = append(viaMCP, service.ViewerEntry{ID: e["id"].(string), Name: e["name"].(string), Agent: e["agent"].(bool), Guest: true, Self: e["self"].(bool)})
		}
	}
	if len(viaHTTP) != 2 || !slices.Equal(viaHTTP, viaMCP) {
		t.Errorf("guests over HTTP %+v, over MCP %+v; want the same two", viaHTTP, viaMCP)
	}
	if refusal := h.agent(bobTok).refused("read", map[string]any{"id": private.ID}); refusal == "" {
		t.Error("an agent read the viewers of a private page")
	}
	tools, err := h.agent(bobTok).session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "read" && (!strings.Contains(tool.Description, "guest: true") || !strings.Contains(tool.Description, "not a verified person")) {
			t.Errorf("the read tool does not tell an agent what a guest is: %s", tool.Description)
		}
	}
}

func TestAFullGuestListAnswersTooManyRequestsWithWhenToTryAgainAndKeepsItsGuests(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	pub := h.publishAs(h.token("alice"), "Demo", "<p>demo", store.VisibilityPublic)
	guest := func(n int) string { return fmt.Sprintf("crowd-browser-%06d", n) }
	for n := range service.MaxGuestsPerArtifact {
		if res, body := h.guestBeat(pub.ID, guest(n)); res.StatusCode != http.StatusOK {
			t.Fatalf("guest %d: %d %s", n, res.StatusCode, body)
		}
	}
	res, body := h.guestBeat(pub.ID, guest(service.MaxGuestsPerArtifact))
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" || !strings.Contains(body, "as many guests") {
		t.Errorf("one guest too many: %d Retry-After=%q %s", res.StatusCode, res.Header.Get("Retry-After"), body)
	}
	if res, body := h.guestBeat(pub.ID, guest(0)); res.StatusCode != http.StatusOK {
		t.Errorf("a listed guest is refused its next beat: %d %s", res.StatusCode, body)
	}
	if res, _ := h.do(nil, "GET", "/raw/"+pub.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("the page no longer reads while the guest list is full: %d", res.StatusCode)
	}
}
