package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

type designAnswer struct {
	Version  int            `json:"version"`
	Manifest map[string]any `json:"manifest"`
	Base     string         `json:"base"`
	Files    string         `json:"files"`
	Live     string         `json:"live"`
}

func TestTheCanvasIsGivenTheManifestAndTheAddressOfItsFrames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishDesign(h.token("alice"), designFrames(), store.VisibilityOrganisation)
	bob := h.browser("bob", "pw-b")
	res, out := h.do(bob, "GET", "/api/artifacts/"+a.ID+"/design", "", bridgeHeaders(h))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("design data: %d %s", res.StatusCode, out)
	}
	d := decode[designAnswer](t, out)
	if d.Version != 1 || d.Manifest["version"] != float64(1) || !strings.HasPrefix(d.Base, "/raw/"+a.ID+"/") || !strings.HasSuffix(d.Base, "/") {
		t.Fatalf("design data = %+v", d)
	}
	// The address is the grant address the frames are served at.
	if res, _ := h.do(nil, "GET", d.Base+"frames/hero.html", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("a frame at the address the canvas was given: %d", res.StatusCode)
	}
}

func TestADesignsDataFollowsTheSameAccessRulesAsItsFrames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), "") // private
	bobTok := h.token("bob")
	_, missing := api(h, "GET", "/api/artifacts/nosuchartifact/design", "", bobTok)
	if res, body := api(h, "GET", "/api/artifacts/"+a.ID+"/design", "", bobTok); res.StatusCode != http.StatusNotFound || body != missing {
		t.Errorf("a stranger: %d %q, want what a missing artifact gives", res.StatusCode, body)
	}
	html := h.publish(tok, "page", "<p>x")
	if res, _ := api(h, "GET", "/api/artifacts/"+html.ID+"/design", "", tok); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an artifact that is not a design: %d", res.StatusCode)
	}
}

func TestAnOlderVersionsDataIsForEditorsOnlyAndPointsAtThatVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": twoFrameManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/footer.html": remove})}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	_, out := api(h, "GET", "/api/artifacts/"+a.ID+"/design?version=1", "", tok)
	old := decode[designAnswer](t, out)
	_, out = api(h, "GET", "/api/artifacts/"+a.ID+"/design", "", tok)
	now := decode[designAnswer](t, out)
	if old.Version != 1 || now.Version != 2 || len(old.Manifest["pages"].([]any)[0].(map[string]any)["rows"].([]any)) != 2 || len(now.Manifest["pages"].([]any)[0].(map[string]any)["rows"].([]any)) != 1 {
		t.Errorf("version 1 %+v, version 2 %+v", old, now)
	}
	if res, _ := h.do(nil, "GET", old.Base+"frames/footer.html", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("the footer of version 1: %d", res.StatusCode)
	}
	if res, _ := h.do(nil, "GET", now.Base+"frames/footer.html", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the footer is gone from version 2: %d", res.StatusCode)
	}
	if res, _ := api(h, "GET", "/api/artifacts/"+a.ID+"/design?version=1", "", h.token("bob")); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader asking for an old version: %d", res.StatusCode)
	}
	if res, _ := api(h, "GET", "/api/artifacts/"+a.ID+"/design?version=x", "", tok); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a version that is not a number: %d", res.StatusCode)
	}
}

func TestAnAnonymousViewerOfAPublicDesignGetsItsData(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	a := h.publishDesign(h.token("alice"), designFrames(), store.VisibilityPublic)
	res, out := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/design", "", nil)
	if res.StatusCode != http.StatusOK || decode[designAnswer](t, out).Base == "" {
		t.Errorf("anonymous: %d %s", res.StatusCode, out)
	}
}

func TestTheCanvasIsGivenTheAddressAFrameLoadsTheArtifactsFilesFromAndOnlyItsOwn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.token("alice")
	a := h.publishDesign(alice, designFrames(), "") // private
	h.mustUpload(alice, a.ID, "chart.png", pngBytes)
	other := h.publish(alice, "Other", "<p>y")
	h.mustUpload(alice, other.ID, "chart.png", pngBytes)

	_, out := api(h, "GET", "/api/artifacts/"+a.ID+"/design", "", alice)
	d := decode[designAnswer](t, out)
	if !strings.HasPrefix(d.Files, "/files/"+a.ID+"/") || !strings.HasSuffix(d.Files, "/") || d.Files == "/files/"+a.ID+"/" {
		t.Fatalf("files = %q, want the artifact's files with a grant in it", d.Files)
	}
	// What a frame sends: no cookie, no token, Origin: null.
	res, body := h.do(nil, "GET", d.Files+"chart.png", "", map[string]string{"Origin": "null"})
	if res.StatusCode != http.StatusOK || body != pngBytes {
		t.Errorf("a file at the address the canvas was given: %d", res.StatusCode)
	}
	grant := strings.Split(d.Files, "/")[3]
	if got, _ := h.fileStatus(nil, "/files/"+other.ID+"/"+grant+"/chart.png"); got != http.StatusNotFound {
		t.Errorf("the grant opened another artifact's file: %d", got)
	}
}
