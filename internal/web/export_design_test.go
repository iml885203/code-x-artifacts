package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func zipOf(t *testing.T, body string) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatalf("the download is not a zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestADesignExportsAsAZipWithItsFramesAndTheUploadsTheyUseAndAnyoneWhoMaySeeItMayDownloadIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	h.mustUpload(tok, a.ID, "logo.png", pngBytes)
	hero := `<!doctype html><html><head><meta charset="utf-8"><style>@font-face{font-family:F;src:url("/static/fonts/FamiljenGrotesk.woff2")}</style></head>` +
		`<body><img src="/files/` + a.ID + `/logo.png" alt="logo"><img src="/files/` + a.ID + `/gone.png" alt="gone"></body></html>`
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": hero})}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}

	// A colleague who can see it gets the zip; what they download is what they may read.
	res, body := api(h, "GET", "/api/artifacts/"+a.ID+"/export/zip", "", h.token("bob"))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export: %d %s", res.StatusCode, body)
	}
	if got := res.Header.Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "Landing.zip") {
		t.Errorf("Content-Disposition = %q", got)
	}
	entries := zipOf(t, body)
	for _, want := range []string{"index.html", "design.json", "frames/hero.html", "frames/mobile.html", "frames/footer.html", "assets/logo.png", "fonts/FamiljenGrotesk.woff2"} {
		if _, ok := entries[want]; !ok {
			t.Errorf("the zip lacks %s", want)
		}
	}
	if entries["assets/logo.png"] != pngBytes || len(entries["fonts/FamiljenGrotesk.woff2"]) < 1000 {
		t.Error("the upload or the font is not in the zip as bytes")
	}
	if got := entries["frames/hero.html"]; !strings.Contains(got, `src="../assets/logo.png"`) || !strings.Contains(got, `src="data:,"`) || strings.Contains(got, "/files/") || strings.Contains(got, "/static/") {
		t.Errorf("the hero frame in the zip: %s", got)
	}
	if entries["design.json"] != designManifest {
		t.Errorf("design.json = %s", entries["design.json"])
	}
}

func TestWhoMayNotSeeADesignGetsTheAnswerForOneThatIsNotThereWhenDownloadingItsZip(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	shared := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	private := h.publishDesign(tok, designFrames(), "")
	missing, missingBody := api(h, "GET", "/api/artifacts/nosuchartifact0000/export/zip", "", h.token("bob"))
	res, body := api(h, "GET", "/api/artifacts/"+private.ID+"/export/zip", "", h.token("bob"))
	if res.StatusCode != http.StatusNotFound || res.StatusCode != missing.StatusCode || strings.ReplaceAll(body, private.ID, "ID") != strings.ReplaceAll(missingBody, "nosuchartifact0000", "ID") {
		t.Errorf("a private design's export for someone else: %d %q, want what a missing artifact says: %d %q", res.StatusCode, body, missing.StatusCode, missingBody)
	}
	if res, _ := h.do(nil, "GET", "/api/artifacts/"+shared.ID+"/export/zip", "", nil); res.StatusCode == http.StatusOK {
		t.Error("an anonymous visitor downloaded a design that is not public")
	}
	if res, body := api(h, "GET", "/api/artifacts/"+shared.ID+"/export/pdf", "", tok); res.StatusCode != http.StatusNotFound || !strings.Contains(body, "zip") {
		t.Errorf("pdf is made by the browser, not the route: %d %s", res.StatusCode, body)
	}
}

func TestAnUploadAnotherViewerMayNotOpenIsNotInTheirZip(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	mine := h.publishDesign(tok, designFrames(), store.VisibilityOrganisation)
	secret := h.publishDesign(tok, designFrames(), "")
	h.mustUpload(tok, secret.ID, "secret.png", pngBytes)
	hero := `<!doctype html><html><body><img src="/files/` + secret.ID + `/secret.png" alt="s"></body></html>`
	if res, out := h.sendFiles("PUT", "/api/artifacts/"+mine.ID, tok, map[string]any{"title": "Landing", "content": designManifest, "baseVersion": 1, "files": fileMap(map[string]string{"frames/hero.html": hero})}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	_, body := api(h, "GET", "/api/artifacts/"+mine.ID+"/export/zip", "", h.token("bob"))
	for name, text := range zipOf(t, body) {
		if strings.Contains(text, pngBytes) || strings.Contains(text, secret.ID) {
			t.Errorf("%s carries a file of another artifact", name)
		}
	}
}
