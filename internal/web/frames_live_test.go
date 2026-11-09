package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A frame someone adds in the room is drawn from the room at once for the people
// in the room: the live address the canvas was given serves it, with the same
// sandbox as any frame, before any version holds it; a frame the room does not
// have is still not found, and the version's address does not serve it.
func TestAFrameAddedInTheRoomIsServedBeforeAnyVersionHoldsIt(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "")
	_, out := api(h.harness, "GET", "/api/artifacts/"+a.ID+"/design", "", tok)
	d := decode[designAnswer](t, out)
	base, live := d.Base, d.Live

	err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		room := designroom.Open(doc)
		d, err := room.Read()
		if err != nil {
			return nil, err
		}
		d.Manifest = strings.Replace(d.Manifest, `"rows":[[`, `"rows":[[{"id":"fresh","name":"Fresh","file":"frames/fresh.html","w":300,"h":200},`, 1)
		d.Frames["frames/fresh.html"] = `<!doctype html><html><head><meta charset="utf-8"></head><body><p>made in the room</p></body></html>`
		return room.Write(d)
	})
	if err != nil {
		t.Fatal(err)
	}
	res, body := h.do(nil, "GET", live+"frames/fresh.html", "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "made in the room") {
		t.Fatalf("the new frame: %d %s", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Errorf("the frame's policy is %q", csp)
	}
	if res, _ := h.do(nil, "GET", live+"frames/nowhere.html", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a frame the room does not have: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "GET", base+"frames/fresh.html", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the version's address serves a frame only the room has: %d", res.StatusCode)
	}
	// A saved frame is still the saved frame's file.
	if res, _ := h.do(nil, "GET", base+"frames/hero.html", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("a saved frame: %d", res.StatusCode)
	}
}

// A page's grant reads its version and nothing the room holds: the anonymous
// reader of a public design gets no frame that only the room has, and the saved
// bytes of a frame the room has changed.
func TestAPageGrantNeverServesWhatOnlyTheRoomHolds(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, withPublicLinks)
	a := h.publishDesign(h.token("alice"), designFrames(), store.VisibilityPublic)
	_, out := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/design", "", nil)
	base := decode[designAnswer](t, out).Base
	err := h.app.live.hub.Edit(t.Context(), a.ID, realtime.Identity{Username: "alice"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		room := designroom.Open(doc)
		d, err := room.Read()
		if err != nil {
			return nil, err
		}
		d.Manifest = strings.Replace(d.Manifest, `"rows":[[`, `"rows":[[{"id":"secret","name":"Secret","file":"frames/secret.html","w":300,"h":200},`, 1)
		d.Frames["frames/secret.html"] = `<!doctype html><html><head><meta charset="utf-8"></head><body><p>not saved yet</p></body></html>`
		d.Frames["frames/hero.html"] = strings.Replace(d.Frames["frames/hero.html"], ">Hero<", ">Unsaved hero<", 1)
		return room.Write(d)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, missing := h.do(nil, "GET", "/raw/nosuchartifact/1.1.x/frames/hero.html", "", nil)
	if res, body := h.do(nil, "GET", base+"frames/secret.html", "", nil); res.StatusCode != http.StatusNotFound || body != missing {
		t.Errorf("a frame only the room has, through a page grant: %d %s", res.StatusCode, body)
	}
	res, body := h.do(nil, "GET", base+"frames/hero.html", "", nil)
	if res.StatusCode != http.StatusOK || strings.Contains(body, "Unsaved hero") || !strings.Contains(body, ">Hero<") {
		t.Errorf("a frame the room changed, through a page grant: %d %s", res.StatusCode, body)
	}
}
