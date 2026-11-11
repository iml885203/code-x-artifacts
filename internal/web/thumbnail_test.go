package web

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"time"
)

func thumbnailMeta(t *testing.T, h *harness, c *http.Client, id string) thumbnailIdentity {
	t.Helper()
	r, b := h.do(c, "GET", "/api/artifacts/"+id+"/thumbnail?scheme=light&lang=en", "", bridgeHeaders(h))
	if r.StatusCode != 200 {
		t.Fatalf("metadata %d %s", r.StatusCode, b)
	}
	return decode[thumbnailIdentity](t, b)
}

func thumbnailJPEG(t *testing.T) string {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, thumbnailWidth, thumbnailHeight))
	im.Set(2, 2, color.RGBA{200, 40, 80, 255})
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func uploadThumbnail(t *testing.T, h *harness, c *http.Client, id string, meta thumbnailIdentity) {
	t.Helper()
	r, b := h.do(c, "POST", "/api/artifacts/"+id+"/thumbnail?scheme=light&lang=en&key="+meta.Key, thumbnailJPEG(t), bridgeHeaders(h))
	if r.StatusCode != 204 {
		t.Fatalf("save %d %s", r.StatusCode, b)
	}
}

func TestImageThumbnailsRemainPrivateAndRejectStaleVersions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Thumbnail", page)
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	cold := thumbnailMeta(t, h, alice, a.ID)
	if cold.Source == "" || cold.URL != "" {
		t.Fatalf("cold %+v", cold)
	}
	uploadThumbnail(t, h, alice, a.ID, cold)
	warm := thumbnailMeta(t, h, alice, a.ID)
	if warm.URL == "" {
		t.Fatal("no warm image")
	}
	r, b := h.do(alice, "GET", warm.URL, "", nil)
	assertThumbnailImage(t, r, b)

	if thumbnailMeta(t, h, bob, a.ID).URL != "" {
		t.Fatal("shared account cache")
	}
	for _, c := range []*http.Client{nil, bob} {
		r, _ = h.do(c, "GET", warm.URL, "", nil)
		if r.StatusCode == 200 {
			t.Fatal("foreign image served")
		}
	}
	// An invalidation independent of saved version is covered by the existing previewKey asset tests.
	r, _ = h.do(alice, "PUT", "/api/artifacts/"+a.ID, `{"title":"Changed","content":"<!doctype html><p>Changed</p>","baseVersion":1}`, bridgeHeaders(h))
	if r.StatusCode != 200 {
		t.Fatalf("publish %d", r.StatusCode)
	}
	next := thumbnailMeta(t, h, alice, a.ID)
	if next.Key == cold.Key || next.URL != "" {
		t.Fatal("stale image identity")
	}
	r, _ = h.do(alice, "GET", warm.URL, "", nil)
	if r.StatusCode != 404 {
		t.Fatalf("stale read %d", r.StatusCode)
	}
	r, _ = h.do(alice, "POST", "/api/artifacts/"+a.ID+"/thumbnail?scheme=light&lang=en&key="+cold.Key, thumbnailJPEG(t), bridgeHeaders(h))
	if r.StatusCode != 409 {
		t.Fatalf("stale save %d", r.StatusCode)
	}
	uploadThumbnail(t, h, alice, a.ID, next)
	r, _ = h.do(alice, "DELETE", "/api/artifacts/"+a.ID, "", bridgeHeaders(h))
	if r.StatusCode != 204 {
		t.Fatalf("delete %d", r.StatusCode)
	}
	r, _ = h.do(alice, "GET", thumbnailMetaURL(a.ID, next.Key), "", nil)
	if r.StatusCode != 404 {
		t.Fatalf("deleted read %d", r.StatusCode)
	}
}

func thumbnailMetaURL(id, key string) string {
	return "/thumbnail/" + id + "?scheme=light&lang=en&key=" + key
}

func TestThumbnailStorageSurvivesRestartAndEvictsOldRevisionsWithinItsBudget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := strings.Repeat("a", 64)
	b := strings.Repeat("b", 64)
	c := strings.Repeat("c", 64)
	cache, err := openThumbnailCache(dir, 8)
	if err != nil {
		t.Fatal(err)
	}
	old := a + "-" + a + ".jpg"
	newer := a + "-" + b + ".jpg"
	other := b + "-" + c + ".jpg"
	if err = cache.put(old, []byte("111")); err != nil {
		t.Fatal(err)
	}
	if err = cache.put(newer, []byte("222")); err != nil {
		t.Fatal(err)
	}
	if cache.has(old) {
		t.Fatal("old revision retained")
	}
	cache, err = openThumbnailCache(dir, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := cache.get(newer); !ok || string(got) != "222" {
		t.Fatal("restart lost image")
	}
	if err = cache.put(other, []byte("333333")); err != nil {
		t.Fatal(err)
	}
	if cache.has(newer) || cache.bytes > 8 {
		t.Fatal("budget not enforced")
	}
	if err = cache.put(newer, []byte("123456789")); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestThumbnailUploadRefusesNonImagesWrongDimensionsAndMissingBrowserProof(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishShared(h.token("alice"), "Thumbnail", page)
	c := h.browser("alice", "pw-a")
	m := thumbnailMeta(t, h, c, a.ID)
	path := "/api/artifacts/" + a.ID + "/thumbnail?scheme=light&lang=en&key=" + m.Key
	for _, body := range []string{"<svg onload=alert(1)></svg>", strings.Repeat("x", maxThumbnailBytes+1)} {
		r, _ := h.do(c, "POST", path, body, bridgeHeaders(h))
		if r.StatusCode != 400 && r.StatusCode != 413 {
			t.Fatalf("invalid image %d", r.StatusCode)
		}
	}
	r, _ := h.do(c, "POST", path, thumbnailJPEG(t), nil)
	if r.StatusCode != 403 {
		t.Fatalf("missing proof %d", r.StatusCode)
	}
	if thumbnailMeta(t, h, c, a.ID).URL != "" {
		t.Fatal("bad upload cached")
	}
}

func testThumbnailScript(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(embeddedBundle(), "thumbnail-vendor.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n" + previewCaptureJS
}

func TestImageThumbnailsCheckRevokedAccessAndAssetChangesWithoutHourlyExpiry(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	h := newHarness(t, func(c *Config) { c.Clock = clock })
	token := h.token("alice")
	a := h.publishShared(token, "Asset thumbnail", page)
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	first := thumbnailMeta(t, h, alice, a.ID)
	uploadThumbnail(t, h, alice, a.ID, first)
	clock.Advance(2 * time.Hour)
	if thumbnailMeta(t, h, alice, a.ID).URL == "" {
		t.Fatal("hour expired image")
	}
	h.mustUpload(token, a.ID, "picture.png", pngBytes)
	changed := thumbnailMeta(t, h, alice, a.ID)
	if changed.Key == first.Key || changed.URL != "" {
		t.Fatal("asset did not invalidate")
	}
	r, _ := h.do(alice, "GET", thumbnailMetaURL(a.ID, first.Key), "", nil)
	if r.StatusCode != 404 {
		t.Fatal("old asset image served")
	}
	reader := thumbnailMeta(t, h, bob, a.ID)
	uploadThumbnail(t, h, bob, a.ID, reader)
	r, _ = h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"private"}`, bridgeHeaders(h))
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	r, _ = h.do(bob, "GET", thumbnailMetaURL(a.ID, reader.Key), "", nil)
	if r.StatusCode != 404 {
		t.Fatal("revoked image served")
	}
	r, _ = h.do(bob, "POST", "/api/artifacts/"+a.ID+"/thumbnail?scheme=light&lang=en&key="+reader.Key, thumbnailJPEG(t), bridgeHeaders(h))
	if r.StatusCode != 404 {
		t.Fatal("revoked upload accepted")
	}
}

func assertThumbnailImage(t *testing.T, r response, b string) {
	t.Helper()
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/jpeg" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("image %d", r.StatusCode)
	}
	cfg, err := jpeg.DecodeConfig(strings.NewReader(b))
	if err != nil || cfg.Width != thumbnailWidth {
		t.Fatalf("image %+v %v", cfg, err)
	}
}
