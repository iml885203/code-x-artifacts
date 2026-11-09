package web

import (
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/i18n"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const (
	enSignIn = "Username"
	zhSignIn = "帳號"
)

func accept(lang string) map[string]string {
	if lang == "" {
		return nil
	}
	return map[string]string{"Accept-Language": lang}
}

func TestThePageLanguageFollowsAcceptLanguage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, c := range []struct {
		header, tag, text string
	}{
		{"zh-TW", "zh-Hant", zhSignIn},
		{"zh-CN", "zh-Hant", zhSignIn},
		{"zh", "zh-Hant", zhSignIn},
		{"en-US", "en", enSignIn},
		{"ja", "en", enSignIn},
		{"fr", "en", enSignIn},
		{"", "en", enSignIn},
		{"ja,zh;q=0.4", "zh-Hant", zhSignIn},
		{"en;q=0.5,zh-TW;q=0.9", "zh-Hant", zhSignIn},
	} {
		res, body := h.do(nil, "GET", "/login", "", accept(c.header))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("Accept-Language %q: status %d", c.header, res.StatusCode)
		}
		if !strings.Contains(body, `<html lang="`+c.tag+`">`) || !strings.Contains(body, c.text) {
			t.Errorf("Accept-Language %q: want %s with %q", c.header, c.tag, c.text)
		}
		if got := res.Header.Get("Content-Language"); got != c.tag {
			t.Errorf("Accept-Language %q: Content-Language %q, want %s", c.header, got, c.tag)
		}
		if !strings.Contains(res.Header.Get("Vary"), "Accept-Language") {
			t.Errorf("Accept-Language %q: Vary is %q", c.header, res.Header.Get("Vary"))
		}
	}
}

func TestEveryPageIsTheShellWithTheCatalogueOfTheLanguageServed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	a := h.publish(h.token("alice"), "Notes", "<p>hi")
	// Every page is the same shell; what makes it a page, and the text it shows in
	// the language served, is its boot block. The app's own rendering of that text
	// is tested as components (web/src/*.test.tsx).
	for _, c := range []struct {
		page, path, who string
		// key is one entry the page's own interface needs, and what it says per language.
		key, en, zh string
	}{
		{"gallery", "/", "alice", "gallery.filter.shared", "Shared with me", "分享給我的"},
		{"viewer", "/a/" + a.ID, "alice", "share.title", "Who can open this", "誰可以開啟"},
		{"connect", "/connect", "alice", "connect.tokens.title", "Your tokens", "你的 token"},
		{"login", "/login", "", "login.username", "Username", "帳號"},
		{"error", "/a/nosuchartifact0000", "alice", "error.notFound.title", "This page isn't here", "找不到這個頁面"},
	} {
		client := alice
		if c.who == "" {
			client = nil
		}
		for _, l := range []struct{ accept, tag, text string }{{"en", "en", c.en}, {"zh-TW", "zh-Hant", c.zh}} {
			res, body := h.do(client, "GET", c.path, "", accept(l.accept))
			if got := res.Header.Get("Content-Language"); got != l.tag {
				t.Errorf("%s in %s: Content-Language %q", c.page, l.accept, got)
			}
			b := bootOf(t, body)
			if b.Page != c.page || b.Lang != l.tag || b.Strings[c.key] != l.text {
				t.Errorf("%s in %s: boot is page %q lang %q, %s = %q, want %q", c.page, l.accept, b.Page, b.Lang, c.key, b.Strings[c.key], l.text)
			}
		}
	}
}

func TestSignInErrorsAreTranslatedAndKeepTheirHeaders(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	for lang, want := range map[string]string{"en": "Wrong username or password", "zh-TW": "帳號或密碼錯誤"} {
		header := accept(lang)
		for k, v := range form {
			header[k] = v
		}
		res, body := h.do(nil, "POST", "/login", "username=alice&password=nope", header)
		b := bootOf(t, body)
		if res.StatusCode != http.StatusUnauthorized || b.Page != "login" || b.Login == nil || b.Login.Error != "login.error.invalid" || b.Strings[b.Login.Error] != want {
			t.Errorf("%s: got %d, boot %+v", lang, res.StatusCode, b)
		}
		if b.Login != nil && b.Login.Username != "alice" {
			t.Errorf("%s: the page does not give back what was typed: %q", lang, b.Login.Username)
		}
		if res.Header.Get("Content-Security-Policy") == "" || res.Header.Get("Content-Language") == "" {
			t.Errorf("%s: the 401 page lost its headers: %v", lang, res.Header)
		}
	}
}

func TestAPIErrorsStayEnglishWhateverTheBrowserAsks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, body := h.do(nil, "GET", "/api/artifacts", "", accept("zh-TW"))
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "sign in or send a token") {
		t.Fatalf("got %d %s", res.StatusCode, body)
	}
	if res.Header.Get("Content-Language") != "" {
		t.Error("an API response claims a page language")
	}
}

// literalKey finds a quoted catalogue key in source. Its namespaces are the
// areas of the English catalogue (the first segment of every key), so a new
// area needs no change here.
var literalKey = sync.OnceValue(func() *regexp.Regexp {
	langs, err := i18n.Load()
	if err != nil {
		panic(err)
	}
	areas := langs.Get(i18n.Fallback).Areas()
	for i, a := range areas {
		areas[i] = regexp.QuoteMeta(a)
	}
	return regexp.MustCompile(`"((?:` + strings.Join(areas, "|") + `)\.[A-Za-z0-9.]+)"`)
})

// sources reads every file under dir of the embedded assets whose name ends in suffix.
func sources(t *testing.T, fsys fs.FS, suffix string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		out[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// appKeys is every catalogue key the React app asks for by its literal name
// (src/i18n.tsx makes a key that is not in en.json a type error). Its tests are
// not interface.
func appKeys(t *testing.T) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for _, suffix := range []string{".ts", ".tsx"} {
		for name, src := range sources(t, os.DirFS("../../web/src"), suffix) {
			if strings.Contains(name, ".test.") || name == "test-support.tsx" {
				continue
			}
			if h := han.FindString(src); h != "" {
				t.Errorf("web/src/%s has interface text outside the catalogues: %q", name, h)
			}
			for _, m := range literalKey().FindAllStringSubmatch(src, -1) {
				keys[m[1]] = true
			}
		}
	}
	return keys
}

var han = regexp.MustCompile(`\p{Han}`)

// keysInUse is every catalogue key the React app and server.go ask for. Text
// written straight into the shell or the app is a failure: it would show in one
// language only.
func keysInUse(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	for name, src := range sources(t, templateFS, ".html") {
		if h := han.FindString(src); h != "" {
			t.Errorf("%s has interface text outside the catalogues: %q", name, h)
		}
	}
	for name, src := range sources(t, staticFS, ".css") {
		if h := han.FindString(src); h != "" {
			t.Errorf("%s has interface text outside the catalogues: %q", name, h)
		}
	}
	// The switcher offers every language the build has, by the key lang.<tag>.
	langs, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range langs.Tags() {
		used["lang."+tag] = true
		used["shell.language."+tag] = true // the avatar menu names each language the build has
	}
	for key := range appKeys(t) {
		used[key] = true
	}
	here := os.DirFS(".")
	sources, err := fs.Glob(here, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		goSource, err := fs.ReadFile(here, source)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range literalKey().FindAllStringSubmatch(string(goSource), -1) {
			used[m[1]] = true
		}
	}
	return used
}

func TestEveryTextTheInterfaceAsksForExistsInEveryLanguage(t *testing.T) {
	t.Parallel()
	langs, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	used := keysInUse(t)
	if len(used) < 20 {
		t.Fatalf("found only %d keys in use: the scan is broken", len(used))
	}
	for _, tag := range langs.Tags() {
		l := langs.Get(tag)
		for key := range used {
			if l.T(key) == key {
				t.Errorf("%s lacks %q, which the interface asks for", tag, key)
			}
		}
	}
	// No dead text: a key nobody asks for is a translation someone paid for.
	for _, key := range langs.Get(i18n.Fallback).Keys() {
		if !used[key] {
			t.Errorf("%q is in the catalogues but nothing uses it", key)
		}
	}
}

func TestTheViewerKnowsWhetherPublicLinksExistAndWhatAnArtifactIsSoItCanSayWhyAnOptionIsUnavailable(t *testing.T) {
	t.Parallel()
	publicOn := func(c *Config) { c.PublicLinks = true }
	for name, c := range map[string]struct {
		options []func(*Config)
		public  bool // the artifact is already public
		links   bool
	}{
		"public links off":                          {nil, false, false},
		"public links on":                           {[]func(*Config){publicOn}, false, true},
		"public artifact after the switch went off": {nil, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, c.options...)
			alice := h.browser("alice", "pw-a")
			id := h.publish(h.token("alice"), "Notes", "<p>x").ID
			if c.public {
				if err := h.st.SetSharing(t.Context(), id, store.VisibilityPublic); err != nil {
					t.Fatal(err)
				}
			}
			for lang, want := range map[string]map[string]string{
				"en":    {"visibility.public.unavailable": "This server has no public links", "visibility.public.hint": "without signing in", "visibility.public.hintOff": "for now it works like Organisation"},
				"zh-TW": {"visibility.public.unavailable": "此伺服器沒有開放公開連結", "visibility.public.hint": "不需要登入", "visibility.public.hintOff": "目前等同組織內所有人"},
			} {
				_, body := h.do(alice, "GET", "/a/"+id, "", accept(lang))
				b := bootOf(t, body)
				if b.PublicLinks != c.links || b.Artifact == nil || (b.Artifact.Visibility == store.VisibilityPublic) != c.public {
					t.Errorf("%s: boot says publicLinks %v and %+v", lang, b.PublicLinks, b.Artifact)
				}
				for key, text := range want {
					if !strings.Contains(b.Strings[key], text) {
						t.Errorf("%s: %s is %q, want it to say %q", lang, key, b.Strings[key], text)
					}
				}
			}
		})
	}
}

var formPost = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}

// chooseLanguage posts the switcher's form from this site's own pages and
// returns the cookie jar-less response plus the cookie it set.
func (h *harness) chooseLanguage(c *http.Client, lang, next string) (response, string) {
	h.t.Helper()
	header := map[string]string{"Origin": h.srv.URL}
	for k, v := range formPost {
		header[k] = v
	}
	res, _ := h.do(c, "POST", "/lang", url.Values{"lang": {lang}, "next": {next}}.Encode(), header)
	for _, ck := range res.cookies {
		if ck.Name == "lang" {
			return res, ck.Value
		}
	}
	return res, ""
}

func TestThePickedLanguageOverridesAcceptLanguageInBothDirections(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, c := range []struct{ pick, header, tag, text string }{
		{"en", "zh-TW", "en", enSignIn},
		{"zh-Hant", "en-US", "zh-Hant", zhSignIn},
		{"zh-Hant", "", "zh-Hant", zhSignIn},
		{"en", "", "en", enSignIn},
	} {
		browser := h.client(newJar(t))
		if res, _ := h.chooseLanguage(browser, c.pick, "/login"); res.StatusCode != http.StatusSeeOther {
			t.Fatalf("pick %s: status %d", c.pick, res.StatusCode)
		}
		res, body := h.do(browser, "GET", "/login", "", accept(c.header))
		if !strings.Contains(body, `<html lang="`+c.tag+`">`) || !strings.Contains(body, c.text) {
			t.Errorf("picked %s with Accept-Language %q: want %s", c.pick, c.header, c.tag)
		}
		if got := res.Header.Get("Content-Language"); got != c.tag {
			t.Errorf("picked %s: Content-Language %q", c.pick, got)
		}
		if vary := res.Header.Get("Vary"); !strings.Contains(vary, "Cookie") || !strings.Contains(vary, "Accept-Language") {
			t.Errorf("a page that depends on the cookie must Vary on it: %q", vary)
		}
	}
	// No cookie, or one this build does not know: the browser decides.
	for _, cookie := range []string{"", "lang=ja", "lang=zh"} {
		header := accept("zh-TW")
		if cookie != "" {
			header["Cookie"] = cookie
		}
		if _, body := h.do(nil, "GET", "/login", "", header); !strings.Contains(body, zhSignIn) {
			t.Errorf("cookie %q should leave Accept-Language in charge", cookie)
		}
	}
}

func TestSwitchingLanguageReturnsToTheSamePageAndOnlyWithinTheSite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, c := range []struct{ next, want string }{
		{"/", "/"},
		{"/tokens", "/tokens"},
		{"/a/abc?x=1", "/a/abc?x=1"},
		{"/login?next=%2Ftokens", "/login?next=%2Ftokens"},
		{"", "/"},
		{"https://evil.example/", "/"},
		{"//evil.example/", "/"},
		{`/\evil.example/`, "/"},
	} {
		res, cookie := h.chooseLanguage(nil, "zh-Hant", c.next)
		if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != c.want {
			t.Errorf("next %q: %d to %q, want %q", c.next, res.StatusCode, loc, c.want)
		}
		if cookie != "zh-Hant" {
			t.Errorf("next %q: cookie %q", c.next, cookie)
		}
	}
}

func TestSwitchingLanguageRefusesAnUnknownLanguageAndCrossOriginPosts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if res, cookie := h.chooseLanguage(nil, "ja", "/"); res.StatusCode != http.StatusBadRequest || cookie != "" {
		t.Errorf("unknown language: %d, cookie %q", res.StatusCode, cookie)
	}
	body := url.Values{"lang": {"en"}, "next": {"/"}}.Encode()
	for _, origin := range []string{"null", "https://evil.example", ""} {
		header := map[string]string{"Content-Type": formPost["Content-Type"]}
		if origin != "" {
			header["Origin"] = origin
		}
		res, _ := h.do(nil, "POST", "/lang", body, header)
		if res.StatusCode != http.StatusForbidden || len(res.cookies) != 0 {
			t.Errorf("Origin %q: %d, cookies %v", origin, res.StatusCode, res.cookies)
		}
	}
}

func TestEveryPageHandsTheAppTheLanguagesToOfferAndWhereTheSwitchReturnsTo(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.PublicLinks = true })
	alice := h.browser("alice", "pw-a")
	a := h.publish(h.token("alice"), "Notes", "<p>hi")
	if err := h.st.SetSharing(t.Context(), a.ID, store.VisibilityPublic); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path, here string
		client           *http.Client
	}{
		{"sign-in", "/login", "/login?next=%2F", nil},
		{"gallery", "/", "/", alice},
		{"viewer", "/a/" + a.ID, "/a/" + a.ID, alice},
		{"anonymous viewer", "/a/" + a.ID, "/a/" + a.ID, nil},
		{"connect", "/connect", "/connect", alice},
		{"not found", "/a/nosuchartifact0000", "/a/nosuchartifact0000", alice},
	} {
		for lang, tag := range map[string]string{"en": "en", "zh-TW": "zh-Hant"} {
			_, body := h.do(c.client, "GET", c.path, "", accept(lang))
			// The app draws the switch (tested in web/src as components); the
			// server's part is the active language, every language, and the page the
			// switch returns to.
			b := bootOf(t, body)
			if b.Lang != tag || !slices.Equal(b.Langs, []string{"en", "zh-Hant"}) || b.Here != c.here {
				t.Errorf("%s in %s: boot says lang %q, langs %v, here %q", c.name, lang, b.Lang, b.Langs, b.Here)
			}
		}
	}
}

func newJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}
