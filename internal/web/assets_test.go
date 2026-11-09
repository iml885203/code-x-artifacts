package web

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A 1x1 PNG, a PDF header and an SVG: real enough for the type check, which
// reads bytes.
var (
	pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa75\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82"
	svgBytes = `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><script>alert(1)</script></svg>`
)

// upload stores a file the way a script does: the file is the request body.
func (h *harness) upload(tok, id, name, body string) (response, string) {
	h.t.Helper()
	return h.do(nil, "PUT", "/api/artifacts/"+id+"/assets/"+name, body, map[string]string{"Authorization": "Bearer " + tok})
}

func (h *harness) mustUpload(tok, id, name, body string) assetView {
	h.t.Helper()
	res, out := h.upload(tok, id, name, body)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		h.t.Fatalf("upload %s: %d %s", name, res.StatusCode, out)
	}
	return decode[assetView](h.t, out)
}

func (h *harness) fileStatus(c *http.Client, path string) (int, string) {
	h.t.Helper()
	res, body := h.do(c, "GET", path, "", nil)
	return res.StatusCode, body
}

func TestAnAssetFollowsItsArtifactsVisibility(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	aliceTok := h.token("alice")
	a := h.publish(aliceTok, "Report", "<p>x")
	asset := h.mustUpload(aliceTok, a.ID, "chart.png", pngBytes)
	path := "/files/" + a.ID + "/chart.png"
	if !strings.HasSuffix(asset.URL, path) {
		t.Fatalf("url = %q, want it to end in %q", asset.URL, path)
	}
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	bobTok := h.bearer("bob")

	type reader struct {
		name   string
		client *http.Client
		header map[string]string
	}
	readers := []reader{{"alice's browser", alice, nil}, {"bob's browser", bob, nil}, {"bob's token", nil, bobTok}, {"nobody", nil, nil}}
	status := func(r reader) int {
		res, _ := h.do(r.client, "GET", path, "", r.header)
		return res.StatusCode
	}
	steps := []struct {
		visibility store.Visibility
		want       map[string]int
	}{
		{store.VisibilityPrivate, map[string]int{"alice's browser": 200, "bob's browser": 404, "bob's token": 404, "nobody": 404}},
		{store.VisibilityOrganisation, map[string]int{"alice's browser": 200, "bob's browser": 200, "bob's token": 200, "nobody": 404}},
		{store.VisibilityPublic, map[string]int{"alice's browser": 200, "bob's browser": 200, "bob's token": 200, "nobody": 200}},
		{store.VisibilityPrivate, map[string]int{"alice's browser": 200, "bob's browser": 404, "bob's token": 404, "nobody": 404}},
	}
	for _, step := range steps {
		body := `{"visibility":"` + string(step.visibility) + `"}`
		if res, out := h.do(nil, "PATCH", "/api/artifacts/"+a.ID, body, h.bearer("alice")); res.StatusCode != http.StatusOK {
			t.Fatalf("set %s: %d %s", step.visibility, res.StatusCode, out)
		}
		for _, r := range readers {
			if got := status(r); got != step.want[r.name] {
				t.Errorf("%s, artifact %s: %s got %d, want %d", r.name, step.visibility, path, got, step.want[r.name])
			}
		}
	}

	// Whoever may not see the artifact gets the answer for a file that is not
	// there, so a 404 does not say there is something to find.
	_, hidden := h.fileStatus(bob, path)
	_, missing := h.fileStatus(bob, "/files/"+a.ID+"/nothing.png")
	if hidden != missing {
		t.Errorf("hidden file answers %q, missing file answers %q", hidden, missing)
	}
}

func TestPublicFilesStayClosedWhenThePublicLinksSwitchIsOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")
	h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	if err := h.st.SetSharing(t.Context(), a.ID, store.VisibilityPublic); err != nil {
		t.Fatal(err)
	}
	path := "/files/" + a.ID + "/chart.png"
	if got, _ := h.fileStatus(nil, path); got != http.StatusNotFound {
		t.Errorf("anonymous read of a public artifact's file with public links off: %d", got)
	}
	if got, _ := h.fileStatus(h.browser("bob", "pw-b"), path); got != http.StatusOK {
		t.Errorf("a signed-in reader of the same: %d, want it to behave as organisation", got)
	}
}

func TestDisallowedTypesAndOversizeFilesAreRefusedWithAReadableError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")

	cases := []struct {
		label, name, body, want string
		status                  int
	}{
		{"HTML passed off as a picture", "chart.png", "<html><script>alert(1)</script></html>", "not an accepted type", http.StatusBadRequest},
		{"an HTML file", "page.html", "<!doctype html><p>hi", "not an accepted type", http.StatusBadRequest},
		{"a picture under the wrong name", "chart.pdf", pngBytes, "image/png", http.StatusBadRequest},
		{"an empty file", "chart.png", "", "empty", http.StatusBadRequest},
		{"no extension", "chart", pngBytes, "send something like chart.png", http.StatusBadRequest},
		{"too large", "big.png", pngBytes + strings.Repeat("0", assets.MaxBytes), "at most 15 MB", http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		res, body := h.upload(tok, a.ID, c.name, c.body)
		if res.StatusCode != c.status || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s, want %d mentioning %q", c.label, res.StatusCode, body, c.status, c.want)
		}
	}
	// The claimed type is not what decides: a PNG sent as text/html is stored as a PNG.
	req := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "text/html"}
	if res, body := h.do(nil, "PUT", "/api/artifacts/"+a.ID+"/assets/chart.png", pngBytes, req); res.StatusCode != http.StatusCreated || !strings.Contains(body, `"image/png"`) {
		t.Errorf("a png sent as text/html: %d %s", res.StatusCode, body)
	}
	_, list := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/assets", "", h.bearer("alice"))
	if got := decode[[]assetView](t, list); len(got) != 1 || got[0].Name != "chart.png" {
		t.Errorf("a refused upload left something behind: %s", list)
	}
}

func TestAnArtifactHoldsAtMostTheFilesAndBytesItIsGiven(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")
	for i := range assets.MaxFiles {
		h.mustUpload(tok, a.ID, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".png", pngBytes)
	}
	res, body := h.upload(tok, a.ID, "one-too-many.png", pngBytes)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "at most 50 files") {
		t.Errorf("file 51: %d %s", res.StatusCode, body)
	}
	if res, body := h.upload(tok, a.ID, "faa.png", pngBytes); res.StatusCode != http.StatusOK {
		t.Errorf("replacing a file at the limit: %d %s", res.StatusCode, body)
	}
}

func TestOnlyTheAuthorAndEditorsUploadOrDeleteAssets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	a := h.publishShared(aliceTok, "Report", "<p>x")
	h.mustUpload(aliceTok, a.ID, "chart.png", pngBytes)

	if res, _ := h.upload(bobTok, a.ID, "mine.png", pngBytes); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader uploading: %d, want 403", res.StatusCode)
	}
	if res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID+"/assets/chart.png", "", h.bearer("bob")); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader deleting: %d, want 403", res.StatusCode)
	}
	if res, body := h.do(nil, "GET", "/api/artifacts/"+a.ID+"/assets", "", h.bearer("bob")); res.StatusCode != http.StatusOK || !strings.Contains(body, "chart.png") {
		t.Errorf("a reader listing: %d %s", res.StatusCode, body)
	}
	if res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID+"/assets/chart.png", "", h.bearer("alice")); res.StatusCode != http.StatusNoContent {
		t.Errorf("the author deleting: %d", res.StatusCode)
	}
	if got, _ := h.fileStatus(h.browser("alice", "pw-a"), "/files/"+a.ID+"/chart.png"); got != http.StatusNotFound {
		t.Errorf("a deleted file: %d", got)
	}
	if res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID+"/assets/chart.png", "", h.bearer("alice")); res.StatusCode != http.StatusNotFound {
		t.Errorf("deleting a file twice: %d", res.StatusCode)
	}
}

func TestANameIsCleanedAndTheUrlUsesTheCleanName(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")
	got := h.mustUpload(tok, a.ID, "Q3 Revenue (final).png", pngBytes)
	if got.Name != "Q3-Revenue-final-.png" || !strings.HasSuffix(got.URL, "/files/"+a.ID+"/Q3-Revenue-final-.png") {
		t.Errorf("asset = %+v", got)
	}
	got = h.mustUpload(tok, a.ID, "..%2F..%2Fetc%2Fpasswd.png", pngBytes)
	if got.Name != "etc-passwd.png" && !strings.HasSuffix(got.Name, "passwd.png") {
		t.Errorf("a traversal name became %q", got.Name)
	}
	if res, _ := h.do(nil, "GET", "/files/"+a.ID+"/..%2F..%2Fetc%2Fpasswd.png", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a traversal path: %d", res.StatusCode)
	}
}

func TestFilesAreServedAsWhatTheyAreAndCannotRunAsThisSite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishAs(tok, "Report", "<p>x", store.VisibilityOrganisation)
	h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	h.mustUpload(tok, a.ID, "logo.svg", svgBytes)
	bob := h.browser("bob", "pw-b")

	res, body := h.do(bob, "GET", "/files/"+a.ID+"/chart.png", "", nil)
	if body != pngBytes || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(res.Header.Get("Cache-Control"), "private, max-age=") || res.Header.Get("ETag") == "" {
		t.Errorf("png: %d %v", res.StatusCode, res.Header)
	}
	if res.Header.Get("Content-Security-Policy") != "" {
		t.Errorf("a PNG carries a CSP that would break nothing and mean nothing: %q", res.Header.Get("Content-Security-Policy"))
	}
	if res, _ := h.do(bob, "GET", "/files/"+a.ID+"/chart.png", "", map[string]string{"If-None-Match": res.Header.Get("ETag")}); res.StatusCode != http.StatusNotModified {
		t.Errorf("a revalidation with the ETag: %d, want 304", res.StatusCode)
	}

	res, _ = h.do(bob, "GET", "/files/"+a.ID+"/logo.svg", "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if res.Header.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(csp, "sandbox;") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("an SVG opened on its own must be sandboxed and script-free: %v", res.Header)
	}
}

// sandboxedPage publishes a page that references two of its own files, one
// as a plain path and one as an absolute url, and returns the two urls the page was given.
func sandboxedPage(t *testing.T, h *harness) (a artifactView, imageURL, csvURL string) {
	t.Helper()
	tok := h.token("alice")
	a = h.publish(tok, "Report", "<p>x")
	page := `<!doctype html><html><head></head><body><img src="/files/` + a.ID + `/chart.png"><script>fetch("` + h.srv.URL + `/files/` + a.ID + `/data.csv")</script></body></html>`
	if res, out := h.do(nil, "PUT", "/api/artifacts/"+a.ID, input(t, artifactInput{Title: "Report", HTML: page}), h.bearer("alice")); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	h.mustUpload(tok, a.ID, "data.csv", "x,y\n1,2\n")

	res, raw := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("the page's csp changed: %q", csp)
	}
	img := regexp.MustCompile(`src="(/files/` + a.ID + `/[^/"]+/chart\.png)"`).FindStringSubmatch(raw)
	if img == nil {
		t.Fatalf("the page was not given a way to load its own file:\n%s", raw)
	}
	csv := regexp.MustCompile(`"` + regexp.QuoteMeta(h.srv.URL) + `(/files/` + a.ID + `/[^/"]+/data\.csv)"`).FindStringSubmatch(raw)
	if csv == nil {
		t.Fatalf("an absolute url to the file was not given the grant either:\n%s", raw)
	}
	return a, img[1], csv[1]
}

func TestAnArtifactsFilesLoadFromInsideItsSandbox(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, imageURL, csvURL := sandboxedPage(t, h)

	// What the sandboxed page sends: no cookie, no token, Origin: null.
	fromThePage := map[string]string{"Origin": "null"}
	res, body := h.do(nil, "GET", imageURL, "", fromThePage)
	if res.StatusCode != http.StatusOK || body != pngBytes || res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Errorf("the page's own request: %d %v", res.StatusCode, res.Header)
	}
	if res, body := h.do(nil, "GET", csvURL, "", fromThePage); res.StatusCode != http.StatusOK || body != "x,y\n1,2\n" {
		t.Errorf("the page's fetch of its CSV: %d %q", res.StatusCode, body)
	}
	if got, _ := h.fileStatus(nil, "/files/"+a.ID+"/chart.png"); got != http.StatusNotFound {
		t.Errorf("the plain url without credentials: %d, want 404 for a private artifact", got)
	}
}

func TestAGrantOpensOnlyItsOwnArtifactsFilesAndRefusesForgeries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, imageURL, _ := sandboxedPage(t, h)
	other := h.publish(h.token("alice"), "Other", "<p>y")
	h.mustUpload(h.token("alice"), other.ID, "chart.png", pngBytes)

	grant := strings.Split(imageURL, "/")[3]
	for label, path := range map[string]string{
		"another artifact":  "/files/" + other.ID + "/" + grant + "/chart.png",
		"a tampered grant":  strings.Replace(imageURL, grant, grant[:len(grant)-2]+"xx", 1),
		"a malformed grant": "/files/" + a.ID + "/nonsense/chart.png",
	} {
		if got, _ := h.fileStatus(nil, path); got != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", label, got)
		}
	}
}

func TestAGrantDiesWithTheAudienceItWasMadeForAndTheNextPageGetsAFreshOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, imageURL, _ := sandboxedPage(t, h)
	alice := h.browser("alice", "pw-a")

	if res, _ := h.do(alice, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"organisation"}`, bridgeHeaders(h)); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d", res.StatusCode)
	}
	if got, _ := h.fileStatus(nil, imageURL); got != http.StatusNotFound {
		t.Errorf("a grant outlived the audience it was made for: %d", got)
	}
	res, raw := h.do(alice, "GET", "/raw/"+a.ID, "", nil)
	fresh := regexp.MustCompile(`src="(/files/` + a.ID + `/[^/"]+/chart\.png)"`).FindStringSubmatch(raw)
	switch {
	case res.StatusCode != http.StatusOK || strings.Contains(raw, imageURL):
		t.Error("the page served after sharing still carries the old grant")
	case fresh == nil:
		t.Error("no fresh grant after sharing")
	default:
		if got, _ := h.fileStatus(nil, fresh[1]); got != http.StatusOK {
			t.Errorf("the fresh grant: %d", got)
		}
	}
}

func TestADocShowsAnUploadedImage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	doc := input(t, artifactInput{Title: "Doc", Kind: "doc", Content: "# Doc\n"})
	res, out := h.do(nil, "POST", "/api/artifacts", doc, map[string]string{"Authorization": "Bearer " + tok})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	up := h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	update := input(t, artifactInput{Title: "Doc", Content: "# Doc\n\n![Revenue](" + up.URL + ")\n"})
	if res, out := h.do(nil, "PUT", "/api/artifacts/"+a.ID, update, h.bearer("alice")); res.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	_, raw := h.do(h.browser("alice", "pw-a"), "GET", "/raw/"+a.ID, "", nil)
	m := regexp.MustCompile(`<img src="[^"]*(/files/` + a.ID + `/[^/"]+/chart\.png)"`).FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("the document's image was not given a grant:\n%s", raw)
	}
	if got, _ := h.fileStatus(nil, m[1]); got != http.StatusOK {
		t.Errorf("the document's image: %d", got)
	}
}

func TestDeletingAnArtifactRemovesItsAssetsFromTheVolume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files, err := assets.OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, func(c *Config) { c.Assets = files })
	tok := h.token("alice")
	a, keep := h.publish(tok, "Report", "<p>x"), h.publish(tok, "Keep", "<p>y")
	h.mustUpload(tok, a.ID, "one.png", pngBytes)
	h.mustUpload(tok, a.ID, "two.png", pngBytes)
	h.mustUpload(tok, keep.ID, "one.png", pngBytes)
	if n := fileCount(t, filepath.Join(dir, a.ID)); n != 2 {
		t.Fatalf("files on the volume before the delete: %d", n)
	}
	if res, _ := h.do(nil, "DELETE", "/api/artifacts/"+a.ID, "", h.bearer("alice")); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, a.ID)); !os.IsNotExist(err) {
		t.Errorf("the deleted artifact's directory is still on the volume: %v", err)
	}
	if n := fileCount(t, filepath.Join(dir, keep.ID)); n != 1 {
		t.Errorf("another artifact's files: %d, want 1", n)
	}
	if recs, err := h.st.Assets(t.Context(), a.ID); err != nil || len(recs) != 0 {
		t.Errorf("asset records of a deleted artifact: %v %v", recs, err)
	}
}

func TestReplacingAFileRemovesTheOldBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files, err := assets.OpenDisk(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, func(c *Config) { c.Assets = files })
	tok := h.token("alice")
	a := h.publish(tok, "Report", "<p>x")
	h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	res, out := h.upload(tok, a.ID, "chart.png", pngBytes+"more")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("replace: %d %s", res.StatusCode, out)
	}
	if n := fileCount(t, filepath.Join(dir, a.ID)); n != 1 {
		t.Errorf("files on the volume after a replace: %d, want 1", n)
	}
	if _, body := h.do(h.browser("alice", "pw-a"), "GET", "/files/"+a.ID+"/chart.png", "", nil); !bytes.HasSuffix([]byte(body), []byte("more")) {
		t.Error("the replaced file still serves the old bytes")
	}
}

func fileCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func input(t *testing.T, in artifactInput) string {
	t.Helper()
	return string(mustJSON(t, in))
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestAGrantIsStableForAnHourExpiresAfterSixAndOnlyFitsItsArtifact(t *testing.T) {
	t.Parallel()
	s := sessions{key: []byte("k")}
	at := time.Date(2026, 10, 19, 10, 20, 0, 0, time.UTC)
	g := s.grant("art1", store.VisibilityPrivate, at)

	if again := s.grant("art1", store.VisibilityPrivate, at.Add(30*time.Minute)); again != g {
		t.Error("the url changed within the hour, so the browser cannot cache the file")
	}
	if s.grant("art1", store.VisibilityPrivate, at.Add(time.Hour)) == g {
		t.Error("the grant never rolls over")
	}
	if v, ok := s.openGrant(g, "art1", at.Add(5*time.Hour)); !ok || v != store.VisibilityPrivate {
		t.Errorf("a grant five hours on: %v %v", v, ok)
	}
	if _, ok := s.openGrant(g, "art1", at.Add(6*time.Hour)); ok {
		t.Error("a grant valid beyond six hours")
	}
	if _, ok := s.openGrant(g, "art2", at); ok {
		t.Error("a grant opened another artifact")
	}
	if _, ok := (sessions{key: []byte("other")}).openGrant(g, "art1", at); ok {
		t.Error("a grant signed with another key was accepted")
	}
	if _, ok := s.verify(g, at); ok {
		t.Error("a grant was accepted as a session cookie")
	}
}
