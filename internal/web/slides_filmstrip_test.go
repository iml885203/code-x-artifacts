package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// fromPage calls the API as the viewer page does: the session cookie and X-Pictor.
func fromPage(h *harness, c *http.Client, method, path, body string) (response, string) {
	h.t.Helper()
	return h.do(c, method, path, body, map[string]string{"X-Pictor": "1", "Content-Type": "application/json"})
}

func TestADeckPreviewIsTheDeckWithoutItsPlayerAndAllowsNoScriptButThePreviewStub(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishSlides(h.token("alice"), threeSlides)
	bob := h.browser("bob", "pw-b")

	full, _ := h.do(bob, "GET", "/raw/"+a.ID, "", nil)
	card, body := h.do(bob, "GET", "/raw/"+a.ID+"?preview", "", nil)
	if card.StatusCode != http.StatusOK {
		t.Fatalf("preview: %d %.200s", card.StatusCode, body)
	}
	csp := card.Header.Get("Content-Security-Policy")
	if sandboxOf(csp) != sandboxOf(full.Header.Get("Content-Security-Policy")) || strings.Contains(csp, "allow-same-origin") {
		t.Errorf("preview sandbox %q differs from the page's", sandboxOf(csp))
	}
	m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)
	if m == nil || strings.TrimSpace(m[1]) != scriptHash(previewStubJS+"\n"+testThumbnailScript(t)) {
		t.Errorf("preview csp %q: want a script-src that allows the preview stub alone", csp)
	}
	if strings.Contains(body, bridgeJS) || !strings.Contains(body, previewCaptureJS) || strings.Contains(body, `id="bar"`) {
		t.Error("a deck preview lost capture or carries the API bridge or player controls")
	}
	if !strings.Contains(body, `id="slide-3"`) {
		t.Error("a deck preview does not hold the slide a thumbnail's address names")
	}
}

func TestTheSlideListOfADeckIsWhatGetSlideListsAndFollowsEachEdit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishSlides(tok, threeSlides)
	alice := h.browser("alice", "pw-a")

	res, body := fromPage(h, alice, "GET", "/api/artifacts/"+a.ID+"/slides", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", res.StatusCode, body)
	}
	list := decode[map[string]any](t, body)
	if list["version"] != float64(1) || slideTitles(t, list) != "1 Q3,2 Two,3 Three" {
		t.Errorf("the list is %s", body)
	}
	one := h.agent(tok).ok("get_slide", map[string]any{"id": a.ID, "number": 1})
	if slideTitles(t, one) != slideTitles(t, list) {
		t.Errorf("the list %s differs from get_slide's %s", slideTitles(t, list), slideTitles(t, one))
	}

	if res, body := fromPage(h, alice, "POST", "/api/artifacts/"+a.ID+"/slides/3/move", `{"to":1,"baseVersion":1}`); res.StatusCode != http.StatusOK {
		t.Fatalf("move: %d %s", res.StatusCode, body)
	}
	_, body = fromPage(h, alice, "GET", "/api/artifacts/"+a.ID+"/slides", "")
	if list := decode[map[string]any](t, body); list["version"] != float64(2) || slideTitles(t, list) != "1 Three,2 Q3,3 Two" {
		t.Errorf("after a move the list is %s", body)
	}
	page := h.publishShared(tok, "Page", "<p>hi</p>")
	if res, body := fromPage(h, alice, "GET", "/api/artifacts/"+page.ID+"/slides", ""); res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "not a deck") {
		t.Errorf("the slide list of a page that is not a deck: %d %s", res.StatusCode, body)
	}
}

func TestTheSlideRoutesTheFilmstripUsesRefuseAReaderAndHideTheDeckFromWhoCannotSeeIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	shared := h.publishSlides(tok, threeSlides).ID
	res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Private","kind":"slides","content":`+jsonString(threeSlides)+`}`, tok)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	private := decode[artifactView](t, out).ID
	bob := h.browser("bob", "pw-b")
	slide := `{"title":"New","layout":"title"}`

	type route struct{ method, path, body string }
	reads := []route{{"GET", "/slides", ""}, {"GET", "/slides/2", ""}}
	edits := []route{
		{"POST", "/slides", `{"after":2,"slide":` + slide + `,"baseVersion":1}`},
		{"POST", "/slides/1/move", `{"to":3,"baseVersion":1}`},
		{"DELETE", "/slides/2?baseVersion=1", ""},
	}
	for _, r := range reads {
		if res, body := fromPage(h, bob, r.method, "/api/artifacts/"+shared+r.path, ""); res.StatusCode != http.StatusOK {
			t.Errorf("a reader %s %s: %d %s, want to read the deck they can see", r.method, r.path, res.StatusCode, body)
		}
	}
	for _, e := range edits {
		if res, body := fromPage(h, bob, e.method, "/api/artifacts/"+shared+e.path, e.body); res.StatusCode != http.StatusForbidden {
			t.Errorf("a reader %s %s: %d %s, want 403", e.method, e.path, res.StatusCode, body)
		}
	}
	_, unseen := fromPage(h, bob, "GET", "/api/artifacts/"+private, "")
	for _, r := range append(reads, edits...) {
		res, body := fromPage(h, bob, r.method, "/api/artifacts/"+private+r.path, r.body)
		if res.StatusCode != http.StatusNotFound || body != unseen {
			t.Errorf("%s %s of a deck bob cannot see: %d %s, want the 404 of the deck itself (%s)", r.method, r.path, res.StatusCode, body, unseen)
		}
	}
	for _, id := range []string{shared, private} {
		if titles, v := deckTitles(t, h.agent(tok), id); titles != "Q3,Two,Three" || v != 1 {
			t.Errorf("a refused edit changed the deck: %s at version %v", titles, v)
		}
	}
}

func TestAnAnonymousViewerOfAPublicDeckReadsItsSlidesButNeverEditsAndLearnsNothingOfAPrivateOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	tok := h.token("alice")
	publish := func(visibility string) string {
		res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Deck","kind":"slides","visibility":"`+visibility+`","content":`+jsonString(threeSlides)+`}`, tok)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("publish %s: %d %s", visibility, res.StatusCode, out)
		}
		return decode[artifactView](t, out).ID
	}
	public, private := publish("public"), publish("private")

	res, body := h.do(nil, "GET", "/api/artifacts/"+public+"/slides", "", nil)
	if res.StatusCode != http.StatusOK || slideTitles(t, decode[map[string]any](t, body)) != "1 Q3,2 Two,3 Three" {
		t.Errorf("anonymous slide list of a public deck: %d %s", res.StatusCode, body)
	}
	res, body = h.do(nil, "GET", "/api/artifacts/"+public+"/slides/3", "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `"notes":"bye"`) {
		t.Errorf("anonymous slide of a public deck: %d %s", res.StatusCode, body)
	}
	// The same answer as any open read of an artifact the anonymous viewer cannot see: nothing says it exists.
	_, unseen := h.do(nil, "GET", "/api/artifacts/"+private+"/me", "", nil)
	for _, path := range []string{"/slides", "/slides/1"} {
		if res, body := h.do(nil, "GET", "/api/artifacts/"+private+path, "", nil); res.StatusCode != http.StatusUnauthorized || body != unseen {
			t.Errorf("anonymous %s of a private deck: %d %s, want %s", path, res.StatusCode, body, unseen)
		}
	}
	for _, e := range []struct{ method, path, body string }{
		{"POST", "/slides", `{"after":1,"slide":{"title":"x","layout":"title"}}`},
		{"POST", "/slides/1/move", `{"to":2}`},
		{"DELETE", "/slides/1", ""},
		{"PUT", "/slides/1", `{"slide":{"title":"x","layout":"title"}}`},
	} {
		if res, body := h.do(nil, e.method, "/api/artifacts/"+public+e.path, e.body, map[string]string{"Content-Type": "application/json"}); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s of a public deck: %d %s, want 401", e.method, e.path, res.StatusCode, body)
		}
	}
	if titles, v := deckTitles(t, h.agent(tok), public); titles != "Q3,Two,Three" || v != 1 {
		t.Errorf("an anonymous edit changed the deck: %s at version %v", titles, v)
	}
}
