package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A page that comes with a script, a stylesheet and some data.
const (
	multiPage = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="styles/site.css"></head>` +
		`<body><p id="out">no</p><script src="app.js"></script><a href="other.html">other</a></body></html>`
	appJS = `document.getElementById("out").textContent = "js ran";`
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// publishWith creates an html artifact that comes with files.
func (h *harness) publishWith(tok, page string, fs map[string]string, visibility store.Visibility) artifactView {
	h.t.Helper()
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Multi", "content": page, "visibility": visibility, "files": fileMap(fs)})
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish with files: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

// fileMap turns "" values into JSON null, which removes a file.
func fileMap(fs map[string]string) map[string]*string {
	m := make(map[string]*string, len(fs))
	for p, text := range fs {
		if text == "\x00remove" {
			m[p] = nil
			continue
		}
		m[p] = &text
	}
	return m
}

const remove = "\x00remove"

func (h *harness) sendFiles(method, path, tok string, body map[string]any) (response, string) {
	h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	return api(h, method, path, string(raw), tok)
}

// update publishes a new version of an artifact with files changed.
func (h *harness) update(tok, id string, base int, page string, fs map[string]string) (response, string) {
	h.t.Helper()
	body := map[string]any{"title": "Multi", "content": page}
	if base != 0 {
		body["baseVersion"] = base
	}
	if fs != nil {
		body["files"] = fileMap(fs)
	}
	return h.sendFiles("PUT", "/api/artifacts/"+id, tok, body)
}

// pageAddress follows what a browser does with /raw/{id}: the redirect to the
// address that carries the grant, returning that address (path only).
func (h *harness) pageAddress(c *http.Client, id string, query string) string {
	h.t.Helper()
	res, body := h.do(c, "GET", "/raw/"+id+query, "", nil)
	if res.StatusCode != http.StatusFound {
		h.t.Fatalf("/raw/%s%s: %d %s, want a redirect to the grant address", id, query, res.StatusCode, body)
	}
	return res.Header.Get("Location")
}

func (h *harness) fileText(tok, id, path string, version int) (response, string) {
	h.t.Helper()
	q := ""
	if version > 0 {
		q = "?version=" + string(mustJSON(h.t, version))
	}
	return api(h, "GET", "/api/artifacts/"+id+"/files/"+path+q, "", tok)
}

func TestAPublishWithFilesSavesPageAndFilesAsOneVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{color:red}", "data/rows.json": `[1,2]`}, "")
	if a.Version != 1 || len(a.Files) != 3 {
		t.Fatalf("created = version %d with %d files", a.Version, len(a.Files))
	}
	want := map[string]struct{ ct, sum string }{
		"app.js":          {"text/javascript; charset=utf-8", sha(appJS)},
		"styles/site.css": {"text/css; charset=utf-8", sha("p{color:red}")},
		"data/rows.json":  {"application/json; charset=utf-8", sha(`[1,2]`)},
	}
	for _, f := range a.Files {
		if w := want[f.Path]; f.ContentType != w.ct || f.SHA256 != w.sum || f.Size == 0 {
			t.Errorf("file %s = %+v, want type %s hash %s", f.Path, f, w.ct, w.sum)
		}
	}
	// The files are part of what is read back, without their content in the listing.
	res, out := api(h, "GET", "/api/artifacts/"+a.ID, "", tok)
	got := decode[artifactView](t, out)
	if res.StatusCode != http.StatusOK || len(got.Files) != 3 || strings.Contains(out, "js ran") {
		t.Errorf("get: %d %s", res.StatusCode, out)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 0); text != appJS {
		t.Errorf("app.js reads back as %q", text)
	}
	v := decode[versionView](t, func() string { _, o := api(h, "GET", "/api/artifacts/"+a.ID+"/versions/1", "", tok); return o }())
	if len(v.Files) != 3 {
		t.Errorf("version 1 lists %d files", len(v.Files))
	}
}

func TestANewVersionWithChangedFilesKeepsTheOldVersionsFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	if res, out := h.update(tok, a.ID, 1, multiPage, map[string]string{"app.js": `document.title = "two";`}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 1); text != appJS {
		t.Errorf("version 1's app.js is %q after version 2 changed it", text)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 0); text != `document.title = "two";` {
		t.Errorf("the newest app.js is %q", text)
	}
	list := versionsOf(t, h, tok, a.ID)
	if len(list) != 2 {
		t.Fatalf("history = %+v", list)
	}
	if res, out := api(h, "GET", "/api/artifacts/"+a.ID+"/versions", "", tok); res.StatusCode != http.StatusOK || strings.Count(out, `"fileCount":1`) != 2 {
		t.Errorf("each version in the history says how many files it has: %s", out)
	}
}

func TestOmittingFilesCarriesTheFilesOverAndNullRemovesOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{}"}, "")
	// A page-only update carries every file.
	res, out := h.update(tok, a.ID, 1, multiPage+"<!-- v2 -->", nil)
	if got := decode[artifactView](t, out); res.StatusCode != http.StatusOK || len(got.Files) != 2 || got.Version != 2 {
		t.Fatalf("page-only update: %d %s", res.StatusCode, out)
	}
	// Naming one file changes that file and carries the other.
	res, out = h.update(tok, a.ID, 2, multiPage, map[string]string{"app.js": "1", "styles/site.css": remove, "data/x.json": "{}"})
	got := decode[artifactView](t, out)
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	if res.StatusCode != http.StatusOK || strings.Join(paths, ",") != "app.js,data/x.json" {
		t.Fatalf("patch: %d %s; paths %v", res.StatusCode, out, paths)
	}
	// Removing what is not there says what is.
	res, out = h.update(tok, a.ID, 3, multiPage, map[string]string{"nope.js": remove})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "nope.js") || !strings.Contains(out, "app.js") {
		t.Errorf("removing a file that is not there: %d %s", res.StatusCode, out)
	}
	// Version 1 still has what it had.
	if _, text := h.fileText(tok, a.ID, "styles/site.css", 1); text != "p{}" {
		t.Errorf("version 1's stylesheet = %q", text)
	}
}

func TestAStaleUpdateWithFilesSavesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	h.update(tok, a.ID, 1, multiPage, map[string]string{"app.js": "theirs"})
	res, out := h.update(tok, a.ID, 1, multiPage, map[string]string{"app.js": "mine", "extra.js": "x"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale update with files: %d %s", res.StatusCode, out)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 0); text != "theirs" {
		t.Errorf("the newest app.js = %q after a refused update", text)
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 2 {
		t.Errorf("history = %+v", got)
	}
	if res, _ := h.fileText(tok, a.ID, "extra.js", 0); res.StatusCode != http.StatusNotFound {
		t.Errorf("a file of a refused update exists: %d", res.StatusCode)
	}
}

func TestRestoreBringsBackTheFilesOfThatVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{}"}, "")
	h.update(tok, a.ID, 1, "<p>v2", map[string]string{"app.js": "two", "styles/site.css": remove})
	res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok)
	got := decode[artifactView](t, out)
	if res.StatusCode != http.StatusOK || got.Version != 3 || len(got.Files) != 2 {
		t.Fatalf("restore: %d %s", res.StatusCode, out)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 0); text != appJS {
		t.Errorf("restored app.js = %q", text)
	}
	if _, text := h.fileText(tok, a.ID, "styles/site.css", 0); text != "p{}" {
		t.Errorf("restored styles/site.css = %q", text)
	}
	// Version 2 is still there, without the stylesheet.
	if res, _ := h.fileText(tok, a.ID, "styles/site.css", 2); res.StatusCode != http.StatusNotFound {
		t.Errorf("version 2 gained a file: %d", res.StatusCode)
	}
}

func TestRestoreRefusesAVersionWhoseFilesNoLongerValidate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	// A version written when a type was still accepted: a file the rules of
	// today refuse, in the manifest of version 2.
	data := "MZ binary"
	if err := h.disk.WriteBlob(a.ID, sha(data), []byte(data)); err != nil {
		t.Fatal(err)
	}
	rev := store.Revision{Title: "Multi", Content: multiPage, Author: "alice", Files: []store.FileRef{{Path: "tool.exe", ContentType: "application/octet-stream", Size: int64(len(data)), SHA256: sha(data)}}}
	if err := h.st.UpdateArtifact(t.Context(), a.ID, 1, rev, ""); err != nil {
		t.Fatal(err)
	}
	h.update(tok, a.ID, 2, multiPage, map[string]string{"tool.exe": remove})
	res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/2/restore", "", tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "version 2 can no longer be restored") || !strings.Contains(out, "tool.exe") {
		t.Errorf("restore of a version with a file that is no longer accepted: %d %s", res.StatusCode, out)
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 3 {
		t.Errorf("a refused restore saved a version: %+v", got)
	}
}

func TestARefusedPublishLeavesNoFilesBehind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Multi", "content": multiPage, "files": fileMap(map[string]string{"app.js": appJS, "data.json": "{broken"})})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "data.json") {
		t.Fatalf("publish with a broken JSON file: %d %s", res.StatusCode, out)
	}
	if res, body := api(h, "GET", "/api/artifacts", "", tok); res.StatusCode != http.StatusOK || strings.TrimSpace(body) != "[]" {
		t.Errorf("a refused publish left an artifact: %s", body)
	}
	if refs, err := h.st.BlobRefs(t.Context()); err != nil || len(refs) != 0 {
		t.Errorf("manifest rows after a refused publish: %v, %v", refs, err)
	}
	// And an update that is refused leaves the files of the version it built on.
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	if res, _ := h.update(tok, a.ID, 1, multiPage, map[string]string{"data.json": "{broken", "app.js": "changed"}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("update with a broken JSON file: %d", res.StatusCode)
	}
	if _, text := h.fileText(tok, a.ID, "app.js", 0); text != appJS {
		t.Errorf("app.js = %q after a refused update", text)
	}
	entries, _ := os.ReadDir(filepath.Join(h.assetsDir, a.ID, "blobs"))
	if len(entries) != 1 {
		t.Errorf("blobs after a refused update = %d, want only app.js's", len(entries))
	}
}

func TestFilesOfOtherTypesAreRefusedWithTheTypesThatAreAccepted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	cases := []struct{ path, content, want string }{
		{"logo.png", "x", ".js"},
		{"notes.txt", "x", ".json"},
		{"index.html", "<p>", "content"},
		{"../evil.js", "x", ".."},
		{"/abs.js", "x", "relative"},
		{"a b.js", "x", "letters, digits"},
		{"a/b/c/d/e/f.js", "x", "at most 5"},
		{"data.json", "{", "JSON"},
		{"app.js", "a\x00b", "NUL"},
	}
	for _, c := range cases {
		res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "t", "content": "<p>x", "files": map[string]string{c.path: c.content}})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, c.want) {
			t.Errorf("file %q: %d %s, want a 400 mentioning %q", c.path, res.StatusCode, out, c.want)
		}
	}
	res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "t", "content": "<p>x", "files": map[string]string{"A.js": "1", "a.JS": "2"}})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "case") {
		t.Errorf("paths that differ only in case: %d %s", res.StatusCode, out)
	}
}

func TestTheLimitsOnFilesAreEnforcedAndNameTheLimitHit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) {
		c.FileLimits = &files.Limits{FileBytes: 20, PerPublish: 2, PerVersion: 3, TotalBytes: 40}
	})
	tok := h.token("alice")
	cases := []struct {
		label string
		fs    map[string]string
		want  string
	}{
		{"one file over its limit", map[string]string{"a.js": strings.Repeat("x", 21)}, "at most 20 bytes"},
		{"too many in one publish", map[string]string{"a.js": "1", "b.js": "1", "c.js": "1"}, "at most 2 files in one publish"},
		{"too many bytes in all", map[string]string{"a.js": strings.Repeat("x", 20), "b.js": strings.Repeat("y", 20), "c.json": "[1]"}, "at most 2 files in one publish"},
	}
	for _, c := range cases {
		res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "t", "content": "<p>x", "files": c.fs})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, c.want) {
			t.Errorf("%s: %d %s, want mentioning %q", c.label, res.StatusCode, out, c.want)
		}
	}
	a := h.publishWith(tok, "<p>x", map[string]string{"a.js": "1", "b.js": "2"}, "")
	res, out := h.update(tok, a.ID, 1, "<p>x", map[string]string{"c.js": "3", "d.js": "4"})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "at most 3 files") {
		t.Errorf("too many in a version: %d %s", res.StatusCode, out)
	}
	res, out = h.update(tok, a.ID, 1, "<p>x", map[string]string{"c.js": strings.Repeat("z", 20), "a.js": strings.Repeat("z", 20)})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "bytes in all") {
		t.Errorf("too many bytes in a version: %d %s", res.StatusCode, out)
	}
}

func TestAKindThatTakesNoFilesRefusesThemRatherThanIgnoringThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	contents := map[string]string{"doc": "# x", "slides": threeSlides}
	for kind, content := range contents {
		res, out := h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "t", "kind": kind, "content": content, "files": map[string]string{"a.js": "1"}})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "takes no files") || !strings.Contains(out, kind) {
			t.Errorf("%s with files: %d %s", kind, res.StatusCode, out)
		}
	}
}

func TestAnArtifactWithoutFilesIsServedExactlyAsBefore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishShared(tok, "Plain", "<!doctype html><p>plain")
	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "<p>plain") || res.Header.Get("Location") != "" {
		t.Errorf("a page without files: %d %q, want it served in place", res.StatusCode, res.Header.Get("Location"))
	}
	// And one that had files but whose newest version has none is in place again.
	b := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, store.VisibilityOrganisation)
	h.update(tok, b.ID, 1, "<p>none", map[string]string{"app.js": remove})
	if res, _ := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+b.ID, "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("a page whose files were all removed: %d", res.StatusCode)
	}
}

func TestAPageAndItsRelativeFilesAreServedByTheGrantInTheirPathAlone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "styles/site.css": "p{color:red}", "data/rows.json": `[1,2]`, "other.html": "<!doctype html><p>other page"}, store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	if !regexp.MustCompile(`^/raw/` + a.ID + `/[0-9]+\.[0-9]+\.[A-Za-z0-9_-]+/$`).MatchString(address) {
		t.Fatalf("redirected to %q", address)
	}
	// From here on no cookie is sent: a page in an opaque origin cannot send one.
	res, body := h.do(nil, "GET", address, "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `<script src="app.js">`) || !strings.Contains(body, "window.pictor") {
		t.Fatalf("the page at its grant address: %d %.200s", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Errorf("the page at its grant address is not in the sandbox: %q", csp)
	}
	// index.html is the same page as the directory.
	if _, same := h.do(nil, "GET", address+"index.html", "", nil); same != body {
		t.Error("index.html and the directory are not the same page")
	}
	for path, want := range map[string]struct{ ct, body string }{
		"app.js":          {"text/javascript; charset=utf-8", appJS},
		"styles/site.css": {"text/css; charset=utf-8", "p{color:red}"},
		"data/rows.json":  {"application/json; charset=utf-8", `[1,2]`},
	} {
		res, body := h.do(nil, "GET", address+path, "", nil)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != want.ct || body != want.body {
			t.Errorf("%s: %d %q %q", path, res.StatusCode, res.Header.Get("Content-Type"), body)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Cross-Origin-Resource-Policy") != "cross-origin" {
			t.Errorf("%s is not readable by a page in an opaque origin: %v", path, res.Header)
		}
	}
}

func TestEveryHTMLFileGetsTheSandboxAndTheBridgeOfTheEntryPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"other.html": "<!doctype html><html><head><title>o</title></head><p>other", "sub/page.html": "<p>no doctype"}, store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	entry, _ := h.do(nil, "GET", address, "", nil)
	for _, p := range []string{"other.html", "sub/page.html"} {
		res, body := h.do(nil, "GET", address+p, "", nil)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Security-Policy") != entry.Header.Get("Content-Security-Policy") {
			t.Errorf("%s: %d, csp %q, want the entry page's %q", p, res.StatusCode, res.Header.Get("Content-Security-Policy"), entry.Header.Get("Content-Security-Policy"))
		}
		if !strings.Contains(body, "window.pictor") || res.Header.Get("Content-Type") != "text/html; charset=utf-8" || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s has no bridge or the wrong headers: %v", p, res.Header)
		}
	}
	if _, body := h.do(nil, "GET", address+"other.html", "", nil); !strings.HasPrefix(body, "<!doctype html><html><head><script>") {
		t.Errorf("the bridge is not first in the head, after the doctype: %.80s", body)
	}
}

func TestFilesAreServedWithTheirOwnTypeNosniffAndNoReferrer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "m.mjs": "export {}", "data.json": "{}"}, "")
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	for _, p := range []string{"", "app.js", "m.mjs", "data.json"} {
		res, _ := h.do(nil, "GET", address+p, "", nil)
		if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%q: nosniff %q, referrer policy %q", p, res.Header.Get("X-Content-Type-Options"), res.Header.Get("Referrer-Policy"))
		}
	}
	if res, _ := h.do(nil, "GET", address+"m.mjs", "", nil); res.Header.Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Errorf("an .mjs file is served as %q", res.Header.Get("Content-Type"))
	}
}

func TestAScriptFileOpenedOnItsOwnIsNotADocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": "alert(1)", "data.json": `{"a":"<script>alert(1)</script>"}`, "site.css": "a{}"}, "")
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	for _, p := range []string{"app.js", "data.json", "site.css"} {
		res, _ := h.do(nil, "GET", address+p, "", nil)
		if csp := res.Header.Get("Content-Security-Policy"); csp != "sandbox; default-src 'none'" {
			t.Errorf("%s opened on its own has csp %q, want a sandbox that allows nothing", p, csp)
		}
		if strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s is served as a document", p)
		}
	}
}

func TestAReaderWhoCannotSeeTheArtifactGetsTheSame404ForEveryFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "") // private
	bob := h.browser("bob", "pw-b")
	_, missing := h.do(bob, "GET", "/raw/nosuchartifact", "", nil)
	if res, body := h.do(bob, "GET", "/raw/"+a.ID, "", nil); res.StatusCode != http.StatusNotFound || body != missing {
		t.Errorf("the page of a private artifact for a stranger: %d %q, want what a missing artifact gives", res.StatusCode, body)
	}
	_, ghost := h.do(nil, "GET", "/raw/nosuchartifact/1.1.x/app.js", "", nil)
	for _, p := range []string{"", "app.js", "styles/site.css", "nothing.js"} {
		for _, c := range []*http.Client{bob, nil} {
			// The cookie is no credential here, and a made-up grant is none either.
			res, body := h.do(c, "GET", "/raw/"+a.ID+"/1.9999999999.forged/"+p, "", nil)
			if res.StatusCode != http.StatusNotFound || body != ghost {
				t.Errorf("file %q with a forged grant: %d %q, want the answer for a missing artifact %q", p, res.StatusCode, body, ghost)
			}
		}
	}
	// A file that is not there, a file of a version that is not there and a bad
	// grant are one answer.
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	if res, body := h.do(nil, "GET", address+"nothing.js", "", nil); res.StatusCode != http.StatusNotFound || body != ghost {
		t.Errorf("a path that is not there: %d %q", res.StatusCode, body)
	}
}

func TestAGrantOfAnotherArtifactOrAnotherAudienceReadsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": "A"}, store.VisibilityOrganisation)
	b := h.publishWith(tok, multiPage, map[string]string{"app.js": "B"}, store.VisibilityOrganisation)
	alice := h.browser("alice", "pw-a")
	addrA := h.pageAddress(alice, a.ID, "")
	grantA := strings.Split(strings.TrimPrefix(addrA, "/raw/"+a.ID+"/"), "/")[0]
	if res, _ := h.do(nil, "GET", "/raw/"+b.ID+"/"+grantA+"/app.js", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a grant of another artifact read a file: %d", res.StatusCode)
	}
	// The same grant with its version changed is not a grant.
	parts := strings.SplitN(grantA, ".", 2)
	if res, _ := h.do(nil, "GET", "/raw/"+a.ID+"/7."+parts[1]+"/app.js", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a grant with another version written into it read a file: %d", res.StatusCode)
	}
	// An asset's grant is not a page grant.
	assetGrant := assetGrantOf(t, h, tok, a.ID)
	if res, _ := h.do(nil, "GET", "/raw/"+a.ID+"/"+assetGrant+"/app.js", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("an asset's grant read a page file: %d", res.StatusCode)
	}
}

func assetGrantOf(t *testing.T, h *harness, tok, id string) string {
	t.Helper()
	up := h.mustUpload(tok, id, "chart.png", pngBytes)
	body := `<img src="` + up.URL + `">`
	h.update(tok, id, 0, body, nil)
	_, page := h.do(h.browser("alice", "pw-a"), "GET", h.pageAddress(h.browser("alice", "pw-a"), id, ""), "", nil)
	m := regexp.MustCompile(`/files/` + id + `/([^/]+)/chart\.png`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no asset grant in the page: %.300s", page)
	}
	return m[1]
}

func TestAGrantStopsWorkingWhenTheAudienceChanges(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withPublicLinks)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	if res, _ := h.do(nil, "GET", address+"app.js", "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("before: %d", res.StatusCode)
	}
	for _, v := range []string{"private", "public", "organisation"} {
		if res, out := api(h, "PATCH", "/api/artifacts/"+a.ID, `{"visibility":"`+v+`"}`, tok); res.StatusCode != http.StatusOK {
			t.Fatalf("set %s: %d %s", v, res.StatusCode, out)
		}
		want := http.StatusNotFound
		if v != "private" {
			want = http.StatusOK // the audience the grant was made for, or everyone
		}
		if res, _ := h.do(nil, "GET", address+"app.js", "", nil); res.StatusCode != want {
			t.Errorf("after the audience became %s: %d, want %d", v, res.StatusCode, want)
		}
	}
}

func TestAnOpenPageKeepsLoadingItsOwnVersionsFilesAfterANewVersionIsPublished(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": "ONE", "lazy.js": "lazy one"}, store.VisibilityOrganisation)
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	h.update(tok, a.ID, 1, "<p>two", map[string]string{"app.js": "TWO", "lazy.js": remove})
	if _, body := h.do(nil, "GET", address+"app.js", "", nil); body != "ONE" {
		t.Errorf("an open page's app.js after version 2 = %q, want its own version's", body)
	}
	if res, body := h.do(nil, "GET", address+"lazy.js", "", nil); res.StatusCode != http.StatusOK || body != "lazy one" {
		t.Errorf("a file loaded late that version 2 removed: %d %q", res.StatusCode, body)
	}
	// A reload gets version 2.
	fresh := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	if _, body := h.do(nil, "GET", fresh+"app.js", "", nil); body != "TWO" {
		t.Errorf("after a reload app.js = %q", body)
	}
}

func TestOpeningAnOlderVersionRedirectsToAGrantForThatVersionForItsAuthorOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": "ONE"}, store.VisibilityOrganisation)
	h.update(tok, a.ID, 1, "<p>two", map[string]string{"app.js": "TWO"})
	alice, bob := h.browser("alice", "pw-a"), h.browser("bob", "pw-b")
	old := h.pageAddress(alice, a.ID, "?version=1")
	if _, body := h.do(nil, "GET", old+"app.js", "", nil); body != "ONE" {
		t.Errorf("version 1's app.js = %q", body)
	}
	if res, _ := h.do(bob, "GET", "/raw/"+a.ID+"?version=1", "", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader opening version 1: %d, want 403", res.StatusCode)
	}
	if res, _ := h.do(alice, "GET", "/raw/"+a.ID+"?version=9", "", nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a version that does not exist: %d, want 400", res.StatusCode)
	}
	// The reader's own address is for the newest version only.
	if _, body := h.do(nil, "GET", h.pageAddress(bob, a.ID, "")+"app.js", "", nil); body != "TWO" {
		t.Errorf("the reader's app.js = %q", body)
	}
}

func TestAnAssetUrlInsideAFileCarriesTheGrantAsInThePage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": `x`, "data.json": `"/files/ID/chart.png"`}, store.VisibilityOrganisation)
	up := h.mustUpload(tok, a.ID, "chart.png", pngBytes)
	if res, out := h.update(tok, a.ID, 0, multiPage, map[string]string{"app.js": `const u = "` + up.URL + `";`, "data.json": `"` + up.URL + `"`}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	address := h.pageAddress(h.browser("bob", "pw-b"), a.ID, "")
	_, js := h.do(nil, "GET", address+"app.js", "", nil)
	m := regexp.MustCompile(`/files/` + a.ID + `/[^/"]+/chart\.png`).FindString(js)
	if m == "" {
		t.Fatalf("the script's asset url has no grant: %q", js)
	}
	if got, _ := h.fileStatus(nil, m); got != http.StatusOK {
		t.Errorf("the script's asset url: %d", got)
	}
	// JSON is data, never rewritten.
	if _, data := h.do(nil, "GET", address+"data.json", "", nil); !strings.HasSuffix(data, `/files/`+a.ID+`/chart.png"`) {
		t.Errorf("a JSON file was rewritten: %q", data)
	}
}

func TestAMissingBlobIsAnErrorNotAnEmptyFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	if err := os.Remove(filepath.Join(h.assetsDir, a.ID, "blobs", sha(appJS))); err != nil {
		t.Fatal(err)
	}
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	res, body := h.do(nil, "GET", address+"app.js", "", nil)
	if res.StatusCode != http.StatusInternalServerError || body == "" || strings.Contains(body, sha(appJS)) {
		t.Errorf("a file whose bytes are gone: %d %q, want an internal error that does not leak paths", res.StatusCode, body)
	}
	if res, _ := h.fileText(tok, a.ID, "app.js", 0); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("reading it over the API: %d", res.StatusCode)
	}
}

func TestTheSameContentInManyVersionsIsStoredOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS, "copy.js": appJS}, "")
	for v := 1; v <= 3; v++ {
		h.update(tok, a.ID, v, multiPage+strings.Repeat("!", v), map[string]string{"app.js": appJS})
	}
	entries, err := os.ReadDir(filepath.Join(h.assetsDir, a.ID, "blobs"))
	if err != nil || len(entries) != 1 {
		t.Errorf("blobs for four versions of one content = %d, %v; want 1", len(entries), err)
	}
}

func TestDeletingAnArtifactDeletesItsFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	if res, out := api(h, "DELETE", "/api/artifacts/"+a.ID, "", tok); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", res.StatusCode, out)
	}
	if _, err := os.Stat(filepath.Join(h.assetsDir, a.ID)); !os.IsNotExist(err) {
		t.Errorf("the artifact's directory is still there: %v", err)
	}
	if refs, _ := h.st.BlobRefs(t.Context()); len(refs) != 0 {
		t.Errorf("manifest rows survive the artifact: %v", refs)
	}
}

func TestExportAsHTMLIsRefusedWhenTheVersionHasFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	res, out := api(h, "GET", "/api/artifacts/"+a.ID+"/export/html", "", tok)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "1 files") || !strings.Contains(out, "broken") {
		t.Errorf("export of a page with files: %d %s, want a refusal that says why", res.StatusCode, out)
	}
	plain := h.publish(tok, "Plain", "<p>x")
	if res, _ := api(h, "GET", "/api/artifacts/"+plain.ID+"/export/html", "", tok); res.StatusCode != http.StatusOK {
		t.Errorf("export of a page without files: %d", res.StatusCode)
	}
}

func TestTheGrantSegmentIsNotLogged(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	h := newHarness(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	address := h.pageAddress(h.browser("alice", "pw-a"), a.ID, "")
	grant := strings.Split(strings.TrimPrefix(address, "/raw/"+a.ID+"/"), "/")[0]
	// A request that fails on the server, the case that logs the most.
	if err := os.Remove(filepath.Join(h.assetsDir, a.ID, "blobs", sha(appJS))); err != nil {
		t.Fatal(err)
	}
	h.do(nil, "GET", address+"app.js", "", nil)
	h.do(nil, "GET", address+"nothing.js", "", nil)
	if out := logs.String(); !strings.Contains(out, "level=ERROR") || strings.Contains(out, grant) {
		t.Errorf("the log should hold the failure and never the grant:\n%s", out)
	}
}

// syncBuffer is a log destination that is safe to write from the server's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestTheServerSweepsOldUnreferencedBlobsWhenItStartsAndStopsWithItsContext(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"app.js": appJS}, "")
	orphan := "orphan"
	if err := h.disk.WriteBlob(a.ID, sha(orphan), []byte(orphan)); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	orphanPath := filepath.Join(h.assetsDir, a.ID, "blobs", sha(orphan))
	if err := os.Chtimes(orphanPath, old, old); err != nil {
		t.Fatal(err)
	}
	var logs syncBuffer
	srv := New(Config{Store: h.st, Assets: h.disk, Authenticator: h.dir, SessionKey: []byte("k"), Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { srv.Maintain(ctx, time.Hour); close(done) }()
	// Wait for the sweep that runs at the start, not for a clock.
	for deadline := time.Now().Add(10 * time.Second); ; {
		if _, err := os.Stat(orphanPath); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the orphan blob was not swept at start")
		}
		runtime.Gosched()
	}
	if _, err := os.Stat(filepath.Join(h.assetsDir, a.ID, "blobs", sha(appJS))); err != nil {
		t.Errorf("the sweep removed a file a version names: %v", err)
	}
	stop()
	<-done
	if out := logs.String(); !strings.Contains(out, `level=INFO msg="unreferenced file blobs deleted" blobs=1`) {
		t.Errorf("maintenance lost its audit: %s", out)
	}
}

func TestReadingAFileOverTheAPIIsPlainTextThatRunsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishWith(tok, multiPage, map[string]string{"other.html": "<!doctype html><script>alert(1)</script>"}, "")
	res, body := h.fileText(tok, a.ID, "other.html", 0)
	if res.StatusCode != http.StatusOK || body != "<!doctype html><script>alert(1)</script>" {
		t.Fatalf("read: %d %q", res.StatusCode, body)
	}
	if res.Header.Get("Content-Type") != "text/plain; charset=utf-8" || res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Content-Security-Policy") != "sandbox; default-src 'none'" {
		t.Errorf("a file read over the API can be opened as a document: %v", res.Header)
	}
	// Whoever cannot see the artifact is told what a missing artifact is told.
	_, missing := h.fileText(tok, "nosuchartifact", "other.html", 0)
	if res, body := h.fileText(h.token("bob"), a.ID, "other.html", 0); res.StatusCode != http.StatusNotFound || body != missing {
		t.Errorf("a stranger reading a file: %d %q, want %q", res.StatusCode, body, missing)
	}
}

func TestAPageGrantIsStableForAnHourExpiresAfterSixAndOnlyFitsItsArtifactAndVersion(t *testing.T) {
	t.Parallel()
	s := sessions{key: []byte("k")}
	at := time.Date(2026, 10, 19, 10, 20, 0, 0, time.UTC)
	g := s.pageGrant("art1", store.VisibilityPrivate, 3, at)

	if s.pageGrant("art1", store.VisibilityPrivate, 3, at.Add(30*time.Minute)) != g {
		t.Error("the address changed within the hour, so the browser cannot cache the file")
	}
	if v, version, ok := s.openPageGrant(g, "art1", at.Add(5*time.Hour)); !ok || v != store.VisibilityPrivate || version != 3 {
		t.Errorf("a grant five hours on: %v %d %v", v, version, ok)
	}
	if _, _, ok := s.openPageGrant(g, "art1", at.Add(6*time.Hour)); ok {
		t.Error("a grant valid beyond six hours")
	}
	if _, _, ok := s.openPageGrant(g, "art2", at); ok {
		t.Error("a grant opened another artifact")
	}
	if _, _, ok := s.openPageGrant("4"+strings.TrimPrefix(g, "3"), "art1", at); ok {
		t.Error("a grant with its version rewritten was accepted: the address must not be able to choose a version")
	}
	if _, _, ok := (sessions{key: []byte("other")}).openPageGrant(g, "art1", at); ok {
		t.Error("a grant signed with another key was accepted")
	}
	if _, ok := s.openGrant(g, "art1", at); ok {
		t.Error("a page grant opened an uploaded asset")
	}
	if _, _, ok := s.openPageGrant(s.grant("art1", store.VisibilityPrivate, at), "art1", at); ok {
		t.Error("an asset's grant opened a page")
	}
	if _, ok := s.verify(g, at); ok {
		t.Error("a page grant was accepted as a session cookie")
	}
}

func TestAPageRendererPageAllowsOnlyItsPathsItsHashesAndTheBridgeNeverSelf(t *testing.T) {
	t.Parallel()
	csp := artifactCSP("https://pictor.test", []string{"inline one", "inline two"}, []string{"/vendor/"})
	want := "script-src https://pictor.test/vendor/ " + scriptHash(bridgeJS) + " " + scriptHash("inline one") + " " + scriptHash("inline two") + ";"
	if !strings.Contains(csp, want) {
		t.Errorf("csp = %q, want it to contain %q", csp, want)
	}
	scriptSrc := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)[1]
	for _, bad := range []string{"'self'", "unsafe-inline", "unsafe-eval", "*"} {
		if strings.Contains(scriptSrc, bad) {
			t.Errorf("script-src %q contains %s", scriptSrc, bad)
		}
	}
	// A page the author wrote gets no script-src at all.
	if csp := artifactCSP("https://pictor.test", nil, nil); strings.Contains(csp, "script-src") {
		t.Errorf("csp of an author's page = %q", csp)
	}
}
