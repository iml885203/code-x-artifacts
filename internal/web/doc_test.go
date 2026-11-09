package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// docHostileRaw is Markdown with every way a document could carry script as raw
// HTML, which is left out.
const docHostileRaw = "# Report\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(2)>\n\nSome **text**.\n"

// publishDoc publishes Markdown the way a new doc is: format 2 (blocks).
func (h *harness) publishDoc(tok, markdown string) artifactView {
	h.t.Helper()
	return h.publishDocAs(tok, "", markdown)
}

func (h *harness) publishDocAs(tok, format, markdown string) artifactView {
	h.t.Helper()
	body := `{"title":"Doc","kind":"doc","visibility":"organisation","content":` + jsonString(markdown)
	if format != "" {
		body += `,"format":"` + format + `"`
	}
	res, out := putJSON(h, "POST", "/api/artifacts", body+"}", tok)
	if res.StatusCode != http.StatusCreated {
		h.t.Fatalf("publish doc: %d %s", res.StatusCode, out)
	}
	return decode[artifactView](h.t, out)
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func TestAnInvalidOrOversizedDocIsRefusedAtPublishWithAReadableError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	for name, tc := range map[string]struct{ content, want string }{
		"empty":     {"", "needs content"},
		"oversized": {strings.Repeat("a", 2<<20+1), "at most 2 MB"},
		"html page": {"<!doctype html><html><body>x</body></html>", "kind html"},
	} {
		res, out := putJSON(h, "POST", "/api/artifacts", `{"title":"Doc","kind":"doc","content":`+jsonString(tc.content)+`}`, tok)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(out, tc.want) {
			t.Errorf("%s: %d %.200s, want 400 mentioning %q", name, res.StatusCode, out, tc.want)
		}
	}
}

func TestMarkdownRawHTMLAndScriptURLsDoNotReachTheRenderedPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.assertHostileMarkdownIsInert(h.publishDoc(h.token("alice"), docHostileRaw), "doc")
}

func (h *harness) assertHostileMarkdownIsInert(a artifactView, name string) {
	t := h.t
	t.Helper()
	_, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	_, main, found := strings.Cut(body, `<main class="doc">`)
	if !found {
		t.Fatalf("no document in the page: %s", body)
	}
	for _, bad := range []string{"<script", "<img", "onerror", `href="javascript`} {
		if strings.Contains(main, bad) {
			t.Errorf("%s: the document body contains %q:\n%s", name, bad, main)
		}
	}
	if !strings.Contains(main, "<strong>text</strong>") {
		t.Errorf("%s: the Markdown itself was not rendered:\n%s", name, main)
	}
}

func TestADocPageKeepsTheOpaqueOriginSandboxAndTheBridge(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishDoc(h.token("alice"), "# Hello\n")

	res, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("csp %q", csp)
	}
	if !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "<head><script>") || !strings.Contains(body, "window.pictor") {
		t.Fatalf("bridge not injected after the doctype: %.80s", body)
	}
	if a.Kind != "doc" || a.Format != "2" {
		t.Errorf("artifact is kind %q format %q, want doc 2", a.Kind, a.Format)
	}
}

func TestADocPageAllowsOnlyTheScriptThePlatformInjectsByHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	a := h.publishDoc(tok, "# Hello\n\n<script>window.pwned=1</script>\n")
	if res, out := api(h, "PUT", "/api/artifacts/"+a.ID, input(t, artifactInput{Title: "Doc", Content: "# Hello again\n"}), tok); res.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", res.StatusCode, out)
	}
	alice := h.browser("alice", "pw-a")
	for path, injected := range map[string]string{
		"/raw/" + a.ID:                bridgeJS,
		"/raw/" + a.ID + "?version=1": bridgeJS,
		"/raw/" + a.ID + "?preview":   previewJS,
	} {
		res, body := h.do(alice, "GET", path, "", nil)
		csp := res.Header.Get("Content-Security-Policy")
		if !strings.HasPrefix(csp, artifactSandbox+"; ") || !strings.HasSuffix(csp, "; frame-ancestors 'self'") {
			t.Errorf("%s: csp %q: the sandbox and frame-ancestors must be every kind's", path, csp)
		}
		m := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)
		if m == nil || m[1] != scriptHash(injected) {
			t.Errorf("%s: csp %q: want a script-src of the injected script's hash alone, no 'self', host or 'unsafe-'", path, csp)
		}
		inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(body, -1)
		if len(inline) != 1 || inline[0][1] != injected {
			t.Errorf("%s: %d inline scripts, want the one the server injects", path, len(inline))
		}
	}
}

func TestAnAgentLearnsTheDocKindFromKindsAndPublishesIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ag := h.agent(h.token("alice"))

	kinds := ag.ok("kinds", nil)
	var found bool
	for _, k := range kinds["kinds"].([]any) {
		m := k.(map[string]any)
		if m["name"] == "doc" {
			found = true
			if !strings.Contains(m["guide"].(string), "Markdown") || !strings.Contains(m["guide"].(string), "pictor:pending") || m["currentFormat"] != "2" {
				t.Errorf("the doc entry does not teach Markdown and the pending line or name format 2: %v", m)
			}
		}
	}
	if !found {
		t.Fatalf("kinds does not list doc: %v", kinds)
	}

	out := ag.ok("publish", map[string]any{"title": "Notes", "kind": "doc", "content": "# Notes\n"})
	if out["kind"] != "doc" {
		t.Errorf("publish result: %v", out)
	}
	if _, refusal := ag.call("publish", map[string]any{"title": "Notes", "kind": "doc", "content": " "}); !strings.Contains(refusal, "needs content") {
		t.Errorf("refusal %q does not say what to send", refusal)
	}
}

// A doc page draws with the fonts DESIGN.md names, and every one of them has to
// be a @font-face whose file the sandboxed page can load.
func TestADocPageNamesOnlyFontsCodeXArtifactsServes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.publishDoc(h.token("alice"), "# Hello\n\n```\ncode\n```\n")
	_, body := h.do(h.browser("bob", "pw-b"), "GET", "/raw/"+a.ID, "", nil)

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`@font-face\s*\{[^}]*font-family:\s*"([^"]+)"[^}]*url\("(/static/fonts/[^"]+)"\)`).FindAllStringSubmatch(body, -1) {
		declared[m[1]] = true
		if res, font := h.do(nil, "GET", m[2], "", fromTheSandbox); res.StatusCode != http.StatusOK || len(font) < 1000 || res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: status %d, %d bytes, ACAO %q", m[2], res.StatusCode, len(font), res.Header.Get("Access-Control-Allow-Origin"))
		}
	}
	for _, name := range []string{"Familjen Grotesk", "Martian Mono"} {
		if !declared[name] {
			t.Errorf("the doc page does not declare %q", name)
		}
	}
	// Every family a --font-* token names first must be one of the declared ones.
	for _, m := range regexp.MustCompile(`--font-[a-z]+:\s*"([^"]+)"`).FindAllStringSubmatch(body, -1) {
		if !declared[m[1]] {
			t.Errorf("the doc page asks for %q, which pictor does not serve", m[1])
		}
	}
	if strings.Contains(body, "IBM Plex") {
		t.Error("the doc page still names IBM Plex Mono")
	}
}

func TestThereIsNoServerPDFAndTheExportRouteSaysWhatItDoesOffer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tok := h.token("alice")
	for _, c := range []struct {
		a     artifactView
		offer string
	}{
		{h.publishDoc(tok, "# Plan\n"), "available formats: md"},
		{h.publish(tok, "Report", "<!doctype html><h1>Report</h1>"), "available formats: html"},
	} {
		res, body := putJSON(h, "GET", "/api/artifacts/"+c.a.ID+"/export/pdf", "", tok)
		if res.StatusCode != http.StatusNotFound || !strings.Contains(body, c.offer) {
			t.Errorf("%s: export pdf: %d %q", c.a.Kind, res.StatusCode, body)
		}
	}
}

func TestEveryFontSliceTheChineseStylesheetNamesIsServedToAPageInTheSandbox(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, css := h.do(nil, "GET", "/static/fonts/noto-sans-tc.css", "", nil)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("stylesheet: %d %q cors %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Access-Control-Allow-Origin"))
	}
	files := regexp.MustCompile(`url\(\./(noto-sans-tc/[^)]+\.woff2)\)`).FindAllStringSubmatch(css, -1)
	if len(files) < 100 {
		t.Fatalf("the stylesheet names only %d font files", len(files))
	}
	for _, f := range files {
		res, _ := h.do(nil, "GET", "/static/fonts/"+f[1], "", nil)
		if res.StatusCode != http.StatusOK || res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: %d cors %q", f[1], res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
		}
	}
}
