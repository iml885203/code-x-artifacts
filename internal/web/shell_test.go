package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iml885203/code-x-artifacts/internal/kind"
)

var (
	bootBlock   = regexp.MustCompile(`<script type="application/json" id="pictor-boot">(.*?)</script>`)
	moduleEntry = regexp.MustCompile(`<script type="module" src="(/assets/[^"]+\.js)"></script>`)
	anyScript   = regexp.MustCompile(`(?s)<script\b([^>]*)>`)
)

type boot struct {
	DisplayName  string                    `json:"displayName"`
	Page         string                    `json:"page"`
	Lang         string                    `json:"lang"`
	User         string                    `json:"user"`
	Initials     string                    `json:"initials"`
	Langs        []string                  `json:"langs"`
	Here         string                    `json:"here"`
	Strings      map[string]string         `json:"strings"`
	PublicLinks  bool                      `json:"publicLinks"`
	FrameSandbox string                    `json:"frameSandbox"`
	Exports      map[string][]exportOption `json:"exports"`
	Artifact     *artifactView             `json:"artifact"`
	Login        *loginBoot                `json:"login"`
	Connect      *connectBoot              `json:"connect"`
	Status       int                       `json:"status"`
}

func bootOf(t *testing.T, body string) boot {
	t.Helper()
	m := bootBlock.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no boot block in %q", body)
	}
	var b boot
	if err := json.Unmarshal([]byte(m[1]), &b); err != nil {
		t.Fatalf("boot block is not JSON: %v", err)
	}
	return b
}

func TestTheGalleryIsTheReactShellFromTheBundleEmbeddedInTheBinary(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	res, body := h.do(alice, "GET", "/", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	if !strings.Contains(body, `<div id="root"></div>`) {
		t.Error("the shell has no element to mount the app on")
	}
	m := moduleEntry.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the shell does not load the built bundle: %s", body)
	}
	res, js := h.do(alice, "GET", m[1], "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Type"), "javascript") || len(js) < 1000 {
		t.Errorf("the bundle %s is not served: %d %q, %d bytes", m[1], res.StatusCode, res.Header.Get("Content-Type"), len(js))
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("a hashed asset must be cached for good, Cache-Control is %q", cc)
	}
}

func TestTheShellAsksNothingOfTheCSPThatTheOtherPagesDoNotAsk(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	res, body := h.do(alice, "GET", "/", "", nil)
	if got, want := res.Header.Get("Content-Security-Policy"), "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-hashes' 'sha256-z8KTzswc4v8taSUJ/JTguDMagJvum9ej3m3yXld36bE='; frame-src 'self'; frame-ancestors 'none'"; got != want {
		t.Errorf("CSP is %q, want %q", got, want)
	}
	// Every script is a file of ours or a JSON data block: nothing runs from the page's own text.
	for _, m := range anyScript.FindAllStringSubmatch(body, -1) {
		attrs := m[1]
		if !strings.Contains(attrs, "src=") && !strings.Contains(attrs, `type="application/json"`) {
			t.Errorf("an inline script would be blocked by the CSP: <script%s>", attrs)
		}
	}
	if strings.Contains(body, "style=") || strings.Contains(body, "<style") {
		t.Error("inline styles would be blocked by the CSP")
	}
	// The share preview's address is absolute because a chat app fetches it, not because this page does.
	body = regexp.MustCompile(`<meta (?:property="og:image"|name="twitter:image") content="[^"]*">`).ReplaceAllString(body, "")
	if regexp.MustCompile(`https?://`).MatchString(strings.ReplaceAll(body, "http://www.w3.org", "")) {
		t.Error("the shell loads something from another origin")
	}
}

func TestTheShellHandsTheAppTheViewerAndTheTextInTheLanguageServed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	for lang, want := range map[string]struct{ tag, title, mine string }{
		"en":    {"en", "Artifacts", "Mine"},
		"zh-TW": {"zh-Hant", "作品", "我的"},
	} {
		res, body := h.do(alice, "GET", "/", "", accept(lang))
		if got := res.Header.Get("Content-Language"); got != want.tag {
			t.Errorf("%s: Content-Language %q", lang, got)
		}
		if !strings.Contains(body, `<html lang="`+want.tag+`">`) || !strings.Contains(res.Header.Get("Vary"), "Accept-Language") {
			t.Errorf("%s: the shell does not say its language", lang)
		}
		b := bootOf(t, body)
		if b.Page != "gallery" || b.Lang != want.tag || b.User != "alice" || b.Initials != "AL" || b.DisplayName != "ALICE" {
			t.Errorf("%s: boot is %+v", lang, b)
		}
		if b.Strings["gallery.title"] != want.title || b.Strings["gallery.filter.mine"] != want.mine {
			t.Errorf("%s: the app is not given the %s catalogue", lang, want.tag)
		}
	}
}

func TestTheShellIsForSignedInPeopleOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for path, next := range map[string]string{"/": "%2F", "/recent": "%2Frecent", "/connect": "%2Fconnect"} {
		res, _ := h.do(nil, "GET", path, "", nil)
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login?next="+next {
			t.Errorf("%s: got %d %q", path, res.StatusCode, res.Header.Get("Location"))
		}
	}
}

func TestTheSignInPageIsTheShellWithWhereToGoAfterwards(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, body := h.do(nil, "GET", "/login?next=%2Fa%2Fabc", "", nil)
	b := bootOf(t, body)
	if res.StatusCode != http.StatusOK || b.Page != "login" || b.User != "" || b.Login == nil || b.Login.Next != "/a/abc" || b.Login.Error != "" {
		t.Errorf("%d %+v", res.StatusCode, b)
	}
	// A destination outside the site is dropped before the page ever carries it.
	_, body = h.do(nil, "GET", "/login?next=https%3A%2F%2Fevil.example%2F", "", nil)
	if next := bootOf(t, body).Login.Next; next != "/" {
		t.Errorf("next is %q", next)
	}
}

func TestTheOldTokensPageSendsEveryoneToTheTokensOnTheConnectPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	for name, c := range map[string]*http.Client{"signed in": alice, "anonymous": nil} {
		res, _ := h.do(c, "GET", "/tokens", "", nil)
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/connect#tokens" {
			t.Errorf("%s: got %d %q", name, res.StatusCode, res.Header.Get("Location"))
		}
	}
	// The pages that used to be posted to are gone.
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": h.srv.URL}
	for _, path := range []string{"/tokens", "/tokens/x/delete"} {
		if res, _ := h.do(alice, "POST", path, "name=x", form); res.StatusCode != http.StatusMethodNotAllowed && res.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s: %d", path, res.StatusCode)
		}
	}
}

func TestTheBundleEmbeddedInTheBinaryIsBuilt(t *testing.T) {
	t.Parallel()
	b, err := loadBundle(embeddedBundle())
	if err != nil {
		t.Fatalf("%v (run `make web`: the bundle is built, not committed)", err)
	}
	if _, err := fs.Stat(embeddedBundle(), b.Entry); err != nil {
		t.Errorf("the manifest names %s but it is not embedded: %v", b.Entry, err)
	}
}

func TestWithoutABundleTheGalleryFailsLoudlyInsteadOfShowingABlankPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.Bundle = fstest.MapFS{".gitkeep": {}} })
	res, body := h.do(h.browser("alice", "pw-a"), "GET", "/", "", nil)
	if res.StatusCode != http.StatusInternalServerError || strings.Contains(body, `id="root"`) {
		t.Errorf("got %d %q", res.StatusCode, body)
	}
}

func TestTheManifestDecidesWhichFilesTheShellLoads(t *testing.T) {
	t.Parallel()
	manifest := `{
	  "index.html": {"file": "assets/index-AAA.js", "isEntry": true, "css": ["assets/index-AAA.css"], "imports": ["_vendor.js"]},
	  "_vendor.js": {"file": "assets/vendor-BBB.js", "imports": ["_deep.js"]},
	  "_deep.js": {"file": "assets/deep-CCC.js", "css": ["assets/deep-CCC.css"]}
	}`
	b, err := loadBundle(fstest.MapFS{".vite/manifest.json": {Data: []byte(manifest)}})
	if err != nil {
		t.Fatal(err)
	}
	// A chunk the entry imports carries the styles of its modules: the page needs them from the start, as Vite's own index.html links them.
	if b.Entry != "assets/index-AAA.js" || strings.Join(b.CSS, ",") != "assets/index-AAA.css,assets/deep-CCC.css" {
		t.Errorf("entry and styles: %+v", b)
	}
	if got := strings.Join(b.Preload, ","); got != "assets/vendor-BBB.js,assets/deep-CCC.js" {
		t.Errorf("preload %q", got)
	}
	for name, fsys := range map[string]fs.FS{
		"no manifest":     fstest.MapFS{},
		"no entry":        fstest.MapFS{".vite/manifest.json": {Data: []byte(`{"a.js":{"file":"assets/a.js"}}`)}},
		"broken manifest": fstest.MapFS{".vite/manifest.json": {Data: []byte(`{`)}},
	} {
		if _, err := loadBundle(fsys); err == nil {
			t.Errorf("%s: a bundle that cannot be loaded must be an error", name)
		}
	}
}

func TestOnlyBuiltAssetsAreServedUnderAssets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, path := range []string{"/assets/nope.js", "/assets/%2e%2e/go.mod", "/assets/", "/assets/.vite/manifest.json"} {
		if res, _ := h.do(nil, "GET", path, "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d", path, res.StatusCode)
		}
	}
}

func TestTheShellFollowsTheLanguageAPersonPickedOverTheBrowsersAndSwitchingReturnsToIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": h.srv.URL}
	if res, _ := h.do(alice, "POST", "/lang", "lang=zh-Hant&next=%2F", form); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("switch: %d", res.StatusCode)
	}
	_, body := h.do(alice, "GET", "/", "", accept("en"))
	if b := bootOf(t, body); b.Lang != "zh-Hant" || b.Strings["gallery.title"] != "作品" {
		t.Errorf("the shell ignores the picked language: %+v", b.Lang)
	}
}

func TestEachKindOffersTheExportFormatsItDeclaresAndNoOthers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, body := h.do(h.browser("alice", "pw-a"), "GET", "/", "", nil)
	got := bootOf(t, body).Exports
	for _, name := range kind.Names() {
		k, err := kind.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		var want, offered []string
		if x, ok := k.(kind.Exporter); ok {
			want = x.Formats()
		}
		if x, ok := k.(kind.BrowserExporter); ok {
			want = append(want, x.BrowserFormats()...)
		}
		for _, o := range got[name] {
			offered = append(offered, o.Format)
		}
		if !slices.Equal(offered, want) {
			t.Errorf("kind %s is offered %v, declares %v", name, offered, want)
		}
	}
	if !shippedExports(got) {
		t.Errorf("exports = %v; the shipped kinds that export are board (svg from the server, png from the browser), html (html from the server, pdf from the browser), react (pdf from the browser), doc (md and docx from the server, pdf from the browser), design (zip from the server, pdf from the browser) and slides (pptx from the server, pdf from the browser)", got)
	}
}

func TestEveryExportFormatHasAMenuLabelInEveryLanguage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, lang := range []string{"en", "zh-Hant"} {
		_, body := h.do(h.browser("alice", "pw-a"), "GET", "/", "", map[string]string{"Accept-Language": lang})
		b := bootOf(t, body)
		for name, options := range b.Exports {
			for _, o := range options {
				if b.Strings["export."+o.Format] == "" {
					t.Errorf("%s: kind %s exports %q, which has no export.%s label", lang, name, o.Format, o.Format)
				}
			}
		}
	}
}

// shippedExports says whether got is what the shipped kinds export, in order.
func shippedExports(got map[string][]exportOption) bool {
	doc := got["doc"]
	design := got["design"]
	return len(got) == 6 && slices.Equal(got["html"], []exportOption{{Format: "html"}, {Format: "pdf", Browser: true}}) && slices.Equal(got["react"], []exportOption{{Format: "pdf", Browser: true}}) && slices.Equal(got["board"], []exportOption{{Format: "svg"}, {Format: "png", Browser: true}}) && slices.Equal(got["slides"], []exportOption{{Format: "pptx"}, {Format: "pdf", Browser: true}}) && slices.Equal(design, []exportOption{{Format: "zip"}, {Format: "pdf", Browser: true}}) &&
		len(doc) == 3 && doc[0] == (exportOption{Format: "md"}) && doc[1] == (exportOption{Format: "docx"}) && doc[2] == (exportOption{Format: "pdf", Browser: true})
}

// The browser aborts the transition of a navigation, and rejects its promises,
// when the page is first shown; the app is a deferred module and can run later
// than that. The code that takes those rejections is a plain script in <head>,
// so it is there before the first frame, and the CSP lets it run.
func TestTheShellTakesTheAbortedNavigationTransitionsBeforeTheAppStarts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	res, body := h.do(alice, "GET", "/", "", nil)
	if csp := res.Header.Get("Content-Security-Policy"); strings.Contains(csp, "script-src") {
		t.Fatalf("csp %q has its own script-src: the plain script is allowed by default-src 'self'", csp)
	}
	scripts := anyScript.FindAllStringSubmatch(body, -1)
	first := -1
	module := -1
	for i, m := range scripts {
		switch {
		case strings.Contains(m[1], `src="/static/transitions.js"`):
			first = i
			if rest := strings.TrimSpace(strings.Replace(m[1], `src="/static/transitions.js"`, "", 1)); rest != "" {
				t.Errorf("the transitions script must block the page's first frame, not wait for it (type, defer, async): <script%s>", m[1])
			}
		case strings.Contains(m[1], `type="module"`):
			module = i
		}
	}
	if first < 0 || module < 0 || first > module {
		t.Fatalf("the shell must load /static/transitions.js before the app's module (script %d, module %d)", first, module)
	}
	if head, _, _ := strings.Cut(body, "</head>"); !strings.Contains(head, "/static/transitions.js") {
		t.Error("the transitions script is not in <head>")
	}
	js, got := h.do(alice, "GET", "/static/transitions.js", "", nil)
	if js.StatusCode != 200 || !strings.Contains(got, "pagereveal") || !strings.Contains(got, "unhandledrejection") {
		t.Errorf("/static/transitions.js: status %d, does not take the transitions' rejections", js.StatusCode)
	}
}

// The theme a person pinned is on <html> before the first paint because a plain script file in <head> puts it
// there, ahead of the stylesheet and the app; it is a file, not inline text, so the CSP stays as it is.
func TestTheShellSetsTheThemeBeforeFirstPaintFromAFileTheCSPAllows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	for _, path := range []string{"/", "/login"} {
		who := alice
		if path == "/login" {
			who = nil
		}
		_, body := h.do(who, "GET", path, "", nil)
		script, sheet := strings.Index(body, `<script src="/static/theme.js"></script>`), strings.Index(body, `href="/static/style.css"`)
		if script < 0 || sheet < 0 || script > sheet {
			t.Errorf("%s: theme.js is not loaded in <head> ahead of the stylesheet (script at %d, stylesheet at %d)", path, script, sheet)
		}
		if strings.Contains(body, "<script>") {
			t.Errorf("%s: an inline script would be blocked by the CSP", path)
		}
	}
	res, js := h.do(nil, "GET", "/static/theme.js", "", nil)
	if res.StatusCode != 200 || !strings.Contains(js, "data-theme") {
		t.Errorf("theme.js: status %d, body %q", res.StatusCode, js)
	}
}
