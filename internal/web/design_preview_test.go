package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// publishFrames publishes a design of n frames in one row, frame i named "Frame i".
func (h *harness) publishFrames(tok string, n int, title string) artifactView {
	h.t.Helper()
	var row []string
	files := map[string]string{}
	for i := 1; i <= n; i++ {
		file := fmt.Sprintf("frames/f%d.html", i)
		row = append(row, fmt.Sprintf(`{"id":"f%d","name":"Frame %d","file":"%s","w":%d,"h":600}`, i, i, file, 400+i))
		files[file] = fmt.Sprintf(`<!doctype html><html><body>%s %d</body></html>`, title, i)
	}
	manifest := `{"version":1,"pages":[{"id":"p","name":"Page","rows":[[` + strings.Join(row, ",") + `]]}]}`
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": title, "kind": "design", "content": manifest, "visibility": "organisation", "files": fileMap(files)})
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func TestADesignCardLoadsOneFrameWhileTheOpenedPageKeepsAllEightWithTheSameGuards(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishFrames(h.token("alice"), 8, "Wall")
	bob := h.browser("bob", "pw-b")

	full, fullBody := h.do(bob, "GET", h.pageAddress(bob, a.ID, ""), "", nil)
	card, cardBody := h.do(bob, "GET", h.pageAddress(bob, a.ID, "?preview"), "", nil)
	if full.StatusCode != http.StatusOK || card.StatusCode != http.StatusOK {
		t.Fatalf("full %d, card %d", full.StatusCode, card.StatusCode)
	}
	if n := strings.Count(fullBody, "<iframe"); n != 8 {
		t.Errorf("the opened overview has %d frames, want 8", n)
	}
	if n := strings.Count(cardBody, "<iframe"); n != 1 || !strings.Contains(cardBody, `src="frames/f1.html?preview"`) {
		t.Errorf("the card has %d frames, want frames/f1.html only: %.400s", n, cardBody)
	}
	if strings.Contains(cardBody, bridgeJS) || !strings.Contains(cardBody, previewCaptureJS) {
		t.Error("the card must carry preview capture only, never the viewer API bridge")
	}
	if got, want := sandboxOf(card.Header.Get("Content-Security-Policy")), sandboxOf(full.Header.Get("Content-Security-Policy")); got != want {
		t.Errorf("card sandbox %q, page %q", got, want)
	}
	for _, r := range []response{full, card} {
		if r.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("cache policy changed: %q", r.Header.Get("Cache-Control"))
		}
	}
	// The frame the card names is served under the same grant as ever.
	loc := h.pageAddress(bob, a.ID, "?preview")
	if res, _ := h.do(nil, "GET", strings.TrimSuffix(loc, "?preview")+"frames/f1.html?preview", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("the card's frame: %d", res.StatusCode)
	}
}

func TestADesignCardOfAnOlderVersionShowsThatVersionsFirstFrameAndIsRefusedLikeThePage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishFrames(tok, 3, "Old")
	alice := h.browser("alice", "pw-a")
	// Version 2 is a design whose first frame is another file.
	manifest := `{"version":1,"pages":[{"id":"p","name":"Page","rows":[[{"id":"n","name":"New","file":"frames/new.html","w":500,"h":500}]]}]}`
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "New", "content": manifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/new.html": "<!doctype html><body>new</body>"})}); res.StatusCode != http.StatusOK {
		t.Fatalf("publish v2: %d %s", res.StatusCode, out)
	}
	_, v1 := h.do(alice, "GET", h.pageAddress(alice, a.ID, "?version=1&preview"), "", nil)
	if strings.Count(v1, "<iframe") != 1 || !strings.Contains(v1, `src="frames/f1.html?preview"`) {
		t.Errorf("version 1 card: %.300s", v1)
	}
	_, latest := h.do(alice, "GET", h.pageAddress(alice, a.ID, "?preview"), "", nil)
	if strings.Count(latest, "<iframe") != 1 || !strings.Contains(latest, `src="frames/new.html?preview"`) {
		t.Errorf("latest card: %.300s", latest)
	}
	// Someone who may not read it, and history for someone who may not read history, get what the page gets.
	private := h.publishDesign(tok, designFrames(), "")
	bob := h.browser("bob", "pw-b")
	plain, plainBody := h.do(bob, "GET", "/raw/"+private.ID, "", nil)
	card, cardBody := h.do(bob, "GET", "/raw/"+private.ID+"?preview", "", nil)
	if card.StatusCode != plain.StatusCode || card.StatusCode != http.StatusNotFound || cardBody != plainBody {
		t.Errorf("a private design: page %d, card %d", plain.StatusCode, card.StatusCode)
	}
}
