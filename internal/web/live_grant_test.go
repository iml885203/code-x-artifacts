package web

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// typeIntoHero changes the hero frame's heading in the design's room, as a
// person typing would, without any version holding it.
func typeIntoHero(t *testing.T, h *liveHarness, id, text string) {
	t.Helper()
	err := h.app.live.hub.Edit(t.Context(), id, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		room := designroom.Open(doc)
		d, err := room.Read()
		if err != nil {
			return nil, err
		}
		d.Frames["frames/hero.html"] = strings.Replace(d.Frames["frames/hero.html"], ">Hero<", ">"+text+"<", 1)
		return room.Write(d)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func liveAddress(t *testing.T, h *liveHarness, id, tok string) string {
	t.Helper()
	res, out := api(h.harness, "GET", "/api/artifacts/"+id+"/design", "", tok)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("design data: %d %s", res.StatusCode, out)
	}
	return decode[designAnswer](t, out).Live
}

// readingTheFileSees checks that the person reads the hero frame with the text over HTTP and over MCP.
func readingTheFileSees(t *testing.T, h *liveHarness, id, tok, text string) {
	t.Helper()
	if _, read := api(h.harness, "GET", "/api/artifacts/"+id+"/files/frames/hero.html", "", tok); !strings.Contains(read, text) {
		t.Errorf("reading the file over HTTP: %s", read)
	}
	if read := rawActivityMCP(t, h.harness, tok, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read","arguments":{"id":"`+id+`","file":"frames/hero.html"}}}`); !strings.Contains(read, text) {
		t.Errorf("reading the file over MCP: %s", read)
	}
}

// Someone who may join a design's room is given a second address for its frames:
// the frames as the room holds them now, so what is not saved yet is there, in the
// same sandbox as a version's frame. The address does not name a version; the
// version address still serves the version, and an older version gets none.
// What the frame shows is what reading the file gives the same person over HTTP
// and MCP.
func TestSomeoneWhoMayJoinTheRoomGetsAnAddressThatServesItsFramesAsTheRoomHoldsThem(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	a := h.publishDesign(h.token("alice"), designFrames(), store.VisibilityOrganisation)
	bob := h.token("bob")
	_, out := api(h.harness, "GET", "/api/artifacts/"+a.ID+"/design", "", bob)
	d := decode[designAnswer](t, out)
	if !strings.HasPrefix(d.Live, "/raw/"+a.ID+"/live.") || !strings.HasSuffix(d.Live, "/") || d.Live == d.Base {
		t.Fatalf("live = %q, base = %q", d.Live, d.Base)
	}
	typeIntoHero(t, h, a.ID, "Typed in the room")

	res, body := h.do(nil, "GET", d.Live+"frames/hero.html", "", map[string]string{"Origin": "null"})
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "Typed in the room") {
		t.Fatalf("the live frame: %d %s", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") || res.Header.Get("Referrer-Policy") != "no-referrer" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("the live frame's headers: %v", res.Header)
	}
	if _, saved := h.do(nil, "GET", d.Base+"frames/hero.html", "", nil); strings.Contains(saved, "Typed in the room") || !strings.Contains(saved, ">Hero<") {
		t.Errorf("the version address serves what the room holds: %s", saved)
	}
	readingTheFileSees(t, h, a.ID, bob, "Typed in the room")
	if again := liveAddress(t, h, a.ID, bob); again != d.Live {
		t.Errorf("the live address changed without a new hour: %q then %q", d.Live, again)
	}
	if _, older := api(h.harness, "GET", "/api/artifacts/"+a.ID+"/design?version=1", "", h.token("alice")); decode[designAnswer](t, older).Live != "" {
		t.Errorf("a version asked for by number has a live address: %s", older)
	}
}

// The live address is only for those who may join the room, checked on every
// request for the account it was made for: the anonymous reader of a public link
// gets none, and a forged, expired, empty-account or other artifact's grant, an
// account that may not see the design, a path that is not a frame and an account
// that has just lost the design all get the same 404 as an artifact that does
// not exist.
func TestTheLiveAddressIsForThoseWhoMayJoinTheRoomAndAnythingElseIsTheSame404(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, withPublicLinks)
	alice := h.token("alice")
	private := h.publishDesign(alice, designFrames(), "")
	public := h.publishDesign(alice, designFrames(), store.VisibilityPublic)
	shared := h.publishDesign(alice, designFrames(), store.VisibilityOrganisation)

	if res, out := h.do(nil, "GET", "/api/artifacts/"+public.ID+"/design", "", nil); res.StatusCode != http.StatusOK || decode[designAnswer](t, out).Live != "" {
		t.Errorf("the anonymous reader of a public design: %d %s", res.StatusCode, out)
	}
	if res, _ := h.do(nil, "GET", "/api/artifacts/"+private.ID+"/design", "", map[string]string{"Authorization": "Bearer " + h.token("bob")}); res.StatusCode != http.StatusNotFound {
		t.Errorf("someone who cannot see the design: %d", res.StatusCode)
	}
	live := liveAddress(t, h, private.ID, alice)
	if res, _ := h.do(nil, "GET", live+"frames/hero.html", "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("the author's own live frame: %d", res.StatusCode)
	}
	_, missing := h.do(nil, "GET", "/raw/nosuchartifact/live.1.YWxpY2U.x/frames/hero.html", "", nil)
	grant := strings.Split(live, "/")[3]
	now := time.Now()
	refused := map[string]string{
		"a forged grant":              "/raw/" + private.ID + "/" + flipped(grant) + "/frames/hero.html",
		"a grant cut short":           "/raw/" + private.ID + "/" + grant[:len(grant)-4] + "/frames/hero.html",
		"an account that may not see": "/raw/" + private.ID + "/" + h.app.sessions.liveGrant(private.ID, "bob", now) + "/frames/hero.html",
		"an expired grant":            "/raw/" + private.ID + "/" + h.app.sessions.liveGrant(private.ID, "alice", now.Add(-7*time.Hour)) + "/frames/hero.html",
		"a grant for nobody":          "/raw/" + private.ID + "/" + h.app.sessions.liveGrant(private.ID, "", now) + "/frames/hero.html",
		"another artifact's grant":    "/raw/" + public.ID + "/" + grant + "/frames/hero.html",
		"the entry page":              live,
		"a path that is not a frame":  live + "design.json",
		"a frame the design lacks":    live + "frames/nowhere.html",
	}
	for why, path := range refused {
		res, body := h.do(nil, "GET", path, "", nil)
		if res.StatusCode != http.StatusNotFound || body != missing {
			t.Errorf("%s: %d %s, want the 404 of a missing artifact %s", why, res.StatusCode, body, missing)
		}
	}

	bobLive := liveAddress(t, h, shared.ID, h.token("bob"))
	if res, _ := h.do(nil, "GET", bobLive+"frames/hero.html", "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("a reader's live frame: %d", res.StatusCode)
	}
	if res, out := api(h.harness, "PATCH", "/api/artifacts/"+shared.ID, `{"visibility":"private"}`, alice); res.StatusCode != http.StatusOK {
		t.Fatalf("make it private: %d %s", res.StatusCode, out)
	}
	if res, body := h.do(nil, "GET", bobLive+"frames/hero.html", "", nil); res.StatusCode != http.StatusNotFound || body != missing {
		t.Errorf("after losing the design: %d %s", res.StatusCode, body)
	}
}

// flipped is a live grant with one character of its sealed part changed.
func flipped(grant string) string {
	i := len(grant) / 2
	c := byte('A')
	if grant[i] == 'A' {
		c = 'B'
	}
	return grant[:i] + string(c) + grant[i+1:]
}

// The account a live grant stands for is sealed inside it: frame code can read
// its own address, so the address must not tell it whose it is.
func TestALiveAddressDoesNotTellWhoseItIs(t *testing.T) {
	t.Parallel()
	s := sessions{key: []byte("k")}
	g := s.liveGrant("art", "alice.wong", time.Now())
	for _, part := range append([]string{g}, strings.Split(g, ".")...) {
		for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
			if raw, err := enc.DecodeString(part); err == nil && strings.Contains(string(raw), "alice") {
				t.Errorf("the grant %q decodes to %q", g, raw)
			}
		}
		if strings.Contains(part, "alice") || strings.Contains(part, base64.RawURLEncoding.EncodeToString([]byte("alice"))[:5]) {
			t.Errorf("the grant %q names the account", g)
		}
	}
	if who, ok := s.openLiveGrant(flipped(g), "art", time.Now()); ok {
		t.Errorf("a changed grant opened as %q", who)
	}
}

func TestALiveGrantNamesItsArtifactAndAccountAndIsNeverAPageGrant(t *testing.T) {
	t.Parallel()
	s := sessions{key: []byte("k")}
	now := time.Now()
	g := s.liveGrant("art", "alice", now)
	if who, ok := s.openLiveGrant(g, "art", now); !ok || who != "alice" {
		t.Fatalf("open = %q %v", who, ok)
	}
	if _, ok := s.openLiveGrant(g, "other", now); ok {
		t.Error("opened for another artifact")
	}
	if _, ok := s.openLiveGrant(g, "art", now.Add(grantLife+grantRound)); ok {
		t.Error("opened after it expired")
	}
	if _, _, ok := s.openPageGrant(g, "art", now); ok {
		t.Error("a live grant opened as a page grant")
	}
	if _, ok := s.openLiveGrant(s.pageGrant("art", store.VisibilityPrivate, 1, now), "art", now); ok {
		t.Error("a page grant opened as a live grant")
	}
	if g != s.liveGrant("art", "alice", now.Truncate(grantRound).Add(30*time.Minute)) {
		t.Error("the grant is not the same all hour")
	}
}

// A frame someone deleted in the room, before any version leaves it out, is gone
// for every way a person reads the design now: the live address, reading the
// file over HTTP and over MCP agree for a reader, and an editor is told the file
// is not present, with nothing of the saved version in its place.
func TestAFrameDeletedInTheRoomIsGoneAlikeFromTheLiveAddressAndFromReadingIt(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	alice := h.token("alice")
	a := h.publishDesign(alice, designFrames(), store.VisibilityOrganisation)
	bob := h.token("bob")
	live := liveAddress(t, h, a.ID, bob)
	err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		room := designroom.Open(doc)
		d, err := room.Read()
		if err != nil {
			return nil, err
		}
		without := strings.Replace(d.Manifest, `,{"id":"mobile","name":"Mobile","file":"frames/mobile.html","w":390,"h":844}`, "", 1)
		if without == d.Manifest {
			return nil, fmt.Errorf("the room's manifest has no mobile frame to remove: %s", d.Manifest)
		}
		d.Manifest = without
		delete(d.Frames, "frames/mobile.html")
		return room.Write(d)
	})
	if err != nil {
		t.Fatal(err)
	}
	if res, body := h.do(nil, "GET", live+"frames/mobile.html", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the live address: %d %s", res.StatusCode, body)
	}
	if res, body := api(h.harness, "GET", "/api/artifacts/"+a.ID+"/files/frames/mobile.html", "", bob); res.StatusCode != http.StatusNotFound || strings.Contains(body, "Mobile<") {
		t.Errorf("a reader reading the file over HTTP: %d %s", res.StatusCode, body)
	}
	if read := rawActivityMCP(t, h.harness, bob, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read","arguments":{"id":"`+a.ID+`","file":"frames/mobile.html"}}}`); !strings.Contains(read, `"isError":true`) || strings.Contains(read, "Mobile<") {
		t.Errorf("a reader reading the file over MCP: %s", read)
	}
	if res, body := api(h.harness, "GET", "/api/artifacts/"+a.ID+"/files/frames/mobile.html", "", alice); res.StatusCode != http.StatusOK || !strings.Contains(body, `"present":false`) {
		t.Errorf("an editor reading the file over HTTP: %d %s", res.StatusCode, body)
	}
}
