package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

const reactIndex = `import Chart from "./Chart"; import "./styles.css";
export default function App() { return <main className="chart"><Chart /></main>; }`

func reactFileSet() map[string]string {
	return map[string]string{
		"Chart.tsx":      `import { fmt } from "./lib/format"; import rows from "./data/rows.json"; export default function Chart() { return <p id="out">{fmt(rows.length)}</p>; }`,
		"lib/format.ts":  `export const fmt = (n: number): string => "rows=" + n;`,
		"data/rows.json": `[1, 2, 3]`,
		"styles.css":     `.chart { color: rgb(1, 2, 3); }`,
		"extra.json":     `{"fetched": true}`,
	}
}

// publishReactWith creates the react artifact of reactIndex with files.
func (h *harness) publishReactWith(tok string, fs map[string]string) (response, string) {
	h.t.Helper()
	return h.sendFiles("POST", "/api/artifacts", tok, map[string]any{"title": "Chart", "kind": "react", "visibility": "organisation", "content": reactIndex, "files": fileMap(fs)})
}

func TestAReactArtifactTakesFilesAndItsPageCarriesThemBundledAndServesTheRest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	res, out := h.publishReactWith(tok, reactFileSet())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("publish react with files: %d %s", res.StatusCode, out)
	}
	a := decode[artifactView](t, out)
	if a.Kind != "react" || len(a.Files) != 5 {
		t.Fatalf("created %s with %d files", a.Kind, len(a.Files))
	}
	bob := h.browser("bob", "pw-b")
	address := h.pageAddress(bob, a.ID, "")
	res, body := h.do(nil, "GET", address, "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "rows=") || !strings.Contains(body, "#010203") {
		t.Fatalf("the react page at its grant address: %d, it must hold the bundle and the stylesheet: %.300s", res.StatusCode, body)
	}
	// The files the page can load are served like an html page's; the sources that were compiled in are not files.
	if res, body := h.do(nil, "GET", address+"extra.json", "", nil); res.StatusCode != http.StatusOK || body != `{"fetched": true}` {
		t.Errorf("extra.json: %d %q", res.StatusCode, body)
	}
	if res, _ := h.do(nil, "GET", address+"data/rows.json", "", nil); res.StatusCode != http.StatusOK {
		t.Errorf("a JSON file the page may fetch: %d", res.StatusCode)
	}
	for _, p := range []string{"Chart.tsx", "lib/format.ts"} {
		if res, _ := h.do(nil, "GET", address+p, "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("the source %s was served as a file: %d", p, res.StatusCode)
		}
		if res, text := h.fileText(tok, a.ID, p, 0); res.StatusCode != http.StatusOK || text == "" {
			t.Errorf("the author cannot read the source %s over the API: %d", p, res.StatusCode)
		}
	}
}

func TestAReactPageAllowsScriptsFromVendorItsOwnFilesAndItsOwnHashesOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	_, out := h.publishReactWith(tok, reactFileSet())
	a := decode[artifactView](t, out)
	other := h.publishWith(tok, "<p>other", map[string]string{"evil.js": "window.evil = 1"}, store.VisibilityOrganisation)
	res, body := h.do(nil, "GET", h.pageAddress(h.browser("bob", "pw-b"), a.ID, ""), "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)
	if m == nil {
		t.Fatalf("csp %q has no script-src", csp)
	}
	allowed := strings.Fields(m[1])
	if len(allowed) < 3 || allowed[0] != h.srv.URL+"/vendor/" || allowed[1] != h.srv.URL+"/raw/"+a.ID+"/" {
		t.Errorf("script-src %q: want the runtime libraries, then this artifact's own files, then hashes", m[1])
	}
	refuseSelf(t, m[1])
	if strings.Contains(m[1], other.ID) || strings.Contains(m[1], "*") {
		t.Errorf("script-src %q names another artifact or a wildcard", m[1])
	}
	for _, sc := range regexp.MustCompile(`(?s)<script(?: type="(?:importmap|module)")?>(.*?)</script>`).FindAllStringSubmatch(body, -1) {
		if !strings.Contains(m[1], scriptHash(sc[1])) {
			t.Errorf("an inline script of the page is not allowed by hash, so it would not run: %.80s", sc[1])
		}
	}
}

func TestAReactArtifactWithoutFilesStillPinsOnlyVendorAndHashes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishReact(h.token("alice"), "export default function App() { return <p>no files</p>; }")
	res, _ := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	if m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(res.Header.Get("Content-Security-Policy")); m == nil || strings.Contains(m[1], "/raw/") {
		t.Errorf("a page with no files allows paths under /raw/: %v", res.Header.Get("Content-Security-Policy"))
	}
}

func TestAReactUpdateIsCheckedAgainstTheFilesItKeepsAndItsPatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	_, out := h.publishReactWith(tok, reactFileSet())
	a := decode[artifactView](t, out)
	update := func(base int, source string, fs map[string]string) (response, string) {
		body := map[string]any{"title": "Chart", "content": source, "baseVersion": base}
		if fs != nil {
			body["files"] = fileMap(fs)
		}
		return h.sendFiles("PUT", "/api/artifacts/"+a.ID, tok, body)
	}
	// A change to the component alone is compiled with the files it carries.
	if res, out := update(1, strings.ReplaceAll(reactIndex, "<Chart />", "<Chart /><Chart />"), nil); res.StatusCode != http.StatusOK {
		t.Fatalf("component-only update: %d %s", res.StatusCode, out)
	}
	// Removing a file the component imports is refused, and says which import.
	res, out := update(2, reactIndex, map[string]string{"Chart.tsx": remove})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, `cannot import \"./Chart\"`) || !strings.Contains(out, "styles.css") {
		t.Errorf("removing an imported file: %d %s", res.StatusCode, out)
	}
	// A broken sibling is refused with its own line, and nothing is saved.
	res, out = update(2, reactIndex, map[string]string{"lib/format.ts": "export const fmt = (n: number) => {\n  return \n"})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, "does not compile") {
		t.Errorf("a broken sibling: %d %s", res.StatusCode, out)
	}
	if got := versionsOf(t, h, tok, a.ID); len(got) != 2 {
		t.Errorf("a refused update saved a version: %+v", got)
	}
	// Restore compiles the old version with its own files.
	if res, out := update(2, reactIndex, map[string]string{"lib/format.ts": `export const fmt = (n: number) => "changed " + n;`}); res.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", res.StatusCode, out)
	}
	if res, out := api(h, "POST", "/api/artifacts/"+a.ID+"/versions/1/restore", "", tok); res.StatusCode != http.StatusOK {
		t.Errorf("restore of a react version with files: %d %s", res.StatusCode, out)
	}
	_, page := h.do(nil, "GET", h.pageAddress(h.browser("alice", "pw-a"), a.ID, ""), "", nil)
	if !strings.Contains(page, "rows=") || strings.Contains(page, "changed ") {
		t.Errorf("the restored page does not use version 1's files: %.200s", page)
	}
}

func TestACopyOfAReactArtifactCompilesWithItsOwnCopiedFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	aliceTok, bobTok := h.token("alice"), h.token("bob")
	_, out := h.publishReactWith(aliceTok, reactFileSet())
	src := decode[artifactView](t, out)
	res, out := h.copyOf(bobTok, src.ID, 0)
	c := decode[artifactView](t, out)
	if res.StatusCode != http.StatusCreated || len(c.Files) != 5 || c.Kind != "react" {
		t.Fatalf("copy of a react artifact: %d %s", res.StatusCode, out)
	}
	_, page := h.do(nil, "GET", h.pageAddress(h.browser("bob", "pw-b"), c.ID, ""), "", nil)
	if !strings.Contains(page, "rows=") {
		t.Errorf("the copy's page does not hold its bundle: %.200s", page)
	}
}
