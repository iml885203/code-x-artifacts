package web

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	titleTag = regexp.MustCompile(`<title>(.*?)</title>`)
	metaTag  = regexp.MustCompile(`<meta\s+(?:name|property)="([^"]+)"(?:\s+media="([^"]+)")?\s+content="([^"]*)">`)
)

// head reads the title and every <meta name|property> of a shell page, keyed by
// name (one with a media query is keyed "name (media)").
func head(t *testing.T, body string) (string, map[string]string) {
	t.Helper()
	m := titleTag.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no <title> in %q", body)
	}
	metas := map[string]string{}
	for _, x := range metaTag.FindAllStringSubmatch(body, -1) {
		key := x[1]
		if x[2] != "" {
			key += " " + x[2]
		}
		metas[key] = html.UnescapeString(x[3])
	}
	return html.UnescapeString(m[1]), metas
}

func TestEveryPagesTitleAndDescriptionMatchThePageAndTheLanguage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	a := h.publish(h.token("alice"), "Q3 roadmap", "<p>hi")
	for _, c := range []struct {
		page, path, who, en, zh, descEN, descZH string
	}{
		{"login", "/login", "", "Sign in · CodeXArtifacts", "登入 · CodeXArtifacts", "Sign in to CodeXArtifacts with your company account.", "用公司帳號登入 CodeXArtifacts。"},
		{"gallery", "/", "alice", "CodeXArtifacts", "CodeXArtifacts", "The artifacts you made and the ones shared with you, on CodeXArtifacts.", "你做的和別人分享給你的 artifact，都在 CodeXArtifacts。"},
		{"viewer", "/a/" + a.ID, "alice", "Q3 roadmap · CodeXArtifacts", "Q3 roadmap · CodeXArtifacts", "An artifact on CodeXArtifacts, shared by its author.", "CodeXArtifacts 上的 artifact，由作者分享。"},
		{"connect", "/connect", "alice", "Connect your agent · CodeXArtifacts", "連接你的 agent · CodeXArtifacts", "Connect your agent to CodeXArtifacts so it can publish artifacts for you.", "把你的 agent 接上 CodeXArtifacts，讓它替你發佈 artifact。"},
		{"kit", "/kit", "alice", "Kit · CodeXArtifacts", "Kit · CodeXArtifacts", "The stylesheet and components for pages published to CodeXArtifacts.", "發佈到 CodeXArtifacts 的頁面用的樣式表和元件。"},
		{"error", "/a/nosuchartifact0000", "alice", "This page isn't here · CodeXArtifacts", "找不到這個頁面 · CodeXArtifacts", "This page isn't here, or it isn't open to you.", "這個頁面不存在，或沒有開放給你。"},
	} {
		client := alice
		if c.who == "" {
			client = nil
		}
		for _, l := range []struct{ accept, title, desc string }{{"en", c.en, c.descEN}, {"zh-TW", c.zh, c.descZH}} {
			_, body := h.do(client, "GET", c.path, "", accept(l.accept))
			title, metas := head(t, body)
			if title != l.title {
				t.Errorf("%s in %s: title %q, want %q", c.page, l.accept, title, l.title)
			}
			if metas["description"] != l.desc {
				t.Errorf("%s in %s: description %q, want %q", c.page, l.accept, metas["description"], l.desc)
			}
			if metas["og:title"] != l.title || metas["og:description"] != l.desc || metas["twitter:title"] != l.title {
				t.Errorf("%s in %s: share tags say %q / %q", c.page, l.accept, metas["og:title"], metas["og:description"])
			}
		}
	}
}

var nightToken = regexp.MustCompile(`--night:\s*light-dark\(oklch\(([\d.]+) ([\d.]+) ([\d.]+)\),\s*oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)\)`)

// pageBackground is the page's background colour in each scheme as hex: the
// --night token of style.css that the body is painted with, so the theme
// colours the browser and the home screen use are read from it, not copied.
func pageBackground(t *testing.T) (light, dark string) {
	t.Helper()
	css, err := os.ReadFile(filepath.Join("static", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	m := nightToken.FindStringSubmatch(string(css))
	if m == nil {
		t.Fatal("style.css declares no --night: light-dark(oklch(…), oklch(…))")
	}
	hex := func(l, c, h string) string {
		n := func(s string) float64 {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		rgb := oklch(n(l), n(c), n(h))
		return fmt.Sprintf("#%02x%02x%02x", rgb.R, rgb.G, rgb.B)
	}
	return hex(m[1], m[2], m[3]), hex(m[4], m[5], m[6])
}

func TestTheShellCarriesTheShareThemeAndIconTags(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, body := h.do(nil, "GET", "/login", "", map[string]string{"X-Forwarded-Proto": "https"})
	_, metas := head(t, body)
	light, dark := pageBackground(t)
	for key, want := range map[string]string{
		"og:type": "website", "og:site_name": "CodeXArtifacts", "twitter:card": "summary_large_image",
		"theme-color (prefers-color-scheme: dark)":  dark,
		"theme-color (prefers-color-scheme: light)": light,
	} {
		if metas[key] != want {
			t.Errorf("%s is %q, want %q", key, metas[key], want)
		}
	}
	// The preview image is an absolute address, because a chat app fetches it from elsewhere.
	if img := metas["og:image"]; !strings.HasPrefix(img, "https://") || !strings.HasSuffix(img, "/static/og-image.png") || metas["twitter:image"] != img {
		t.Errorf("og:image %q, twitter:image %q", img, metas["twitter:image"])
	}
	for _, want := range []string{`rel="apple-touch-icon" href="/static/apple-touch-icon.png"`, `rel="manifest" href="/static/manifest.webmanifest"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the shell lacks %s", want)
		}
	}
}

func TestAnArtifactTitleOnlyReachesThePageMetadataOfWhoCanSeeIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	hidden := "Layoff plan for Q4"
	a := h.publish(h.token("alice"), hidden, "<p>hi")
	for name, c := range map[string]*http.Client{"bob (not allowed)": h.browser("bob", "pw-b"), "nobody": nil} {
		res, body := h.do(c, "GET", "/a/"+a.ID, "", nil)
		if strings.Contains(body, hidden) {
			t.Errorf("%s: the title is in the page (status %d)", name, res.StatusCode)
		}
		if c != nil {
			if title, _ := head(t, body); title != "This page isn't here · CodeXArtifacts" {
				t.Errorf("%s: title %q", name, title)
			}
		}
	}
	_, body := h.do(h.browser("alice", "pw-a"), "GET", "/a/"+a.ID, "", nil)
	if _, metas := head(t, body); metas["og:title"] != hidden+" · CodeXArtifacts" {
		t.Errorf("the author's own page says %q", metas["og:title"])
	}
	// A title with markup in it stays text.
	b := h.publish(h.token("alice"), `<b>"x"</b> & y`, "<p>hi")
	_, body = h.do(h.browser("alice", "pw-a"), "GET", "/a/"+b.ID, "", nil)
	if title, _ := head(t, body); title != `<b>"x"</b> & y · CodeXArtifacts` || strings.Contains(body, "<b>") {
		t.Errorf("the title is %q", title)
	}
}

func TestTheManifestAndIconsAreServedToAnyone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for path, typ := range map[string]string{
		"/static/manifest.webmanifest": "application/manifest+json",
		"/static/apple-touch-icon.png": "image/png",
		"/static/icon-192.png":         "image/png",
		"/static/icon-512.png":         "image/png",
		"/static/og-image.png":         "image/png",
	} {
		res, body := h.do(nil, "GET", path, "", nil)
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), typ) || len(body) == 0 {
			t.Errorf("%s: %d %q", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
	_, body := h.do(nil, "GET", "/static/manifest.webmanifest", "", nil)
	// The installed app opens on the dark page, so its colours are the dark page background.
	_, dark := pageBackground(t)
	for _, want := range []string{`"name": "CodeXArtifacts"`, `"start_url": "/"`, `"background_color": "` + dark + `"`, `"theme_color": "` + dark + `"`, `/static/icon-192.png`, `/static/icon-512.png`} {
		if !strings.Contains(body, want) {
			t.Errorf("the manifest lacks %s: %s", want, body)
		}
	}
}

func TestAnUnknownPathIsTheSitesNotFoundPageWithItsStatusTitleAndThemeForEveryone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	shell, _ := h.do(alice, "GET", "/", "", nil)
	for name, c := range map[string]*http.Client{"alice": alice, "nobody": nil} {
		for _, path := range []string{"/no/such/page", "/a/x/extra", "/settings"} {
			res, body := h.do(c, "GET", path, "", accept("en"))
			expectNotFoundPage(t, name+" "+path, res, body, shell.Header.Get("Content-Security-Policy"))
		}
	}
	// What a program asks for stays a program's answer: the API and other methods keep their plain status.
	for _, c := range []struct{ method, path string }{{"GET", "/api/no-such-endpoint"}, {"POST", "/no/such/page"}} {
		res, body := h.do(alice, c.method, c.path, "", nil)
		if res.StatusCode != http.StatusNotFound || strings.Contains(body, "<html") {
			t.Errorf("%s %s: %d %q", c.method, c.path, res.StatusCode, body)
		}
	}
}

// expectNotFoundPage checks that a response is the site's error page for
// something that is not there, under the same CSP as the shell.
func expectNotFoundPage(t *testing.T, what string, res response, body, csp string) {
	t.Helper()
	if res.StatusCode != http.StatusNotFound || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("%s: %d %q", what, res.StatusCode, res.Header.Get("Content-Type"))
	}
	if got := res.Header.Get("Content-Security-Policy"); got != csp {
		t.Errorf("%s: CSP %q, the shell's is %q", what, got, csp)
	}
	title, metas := head(t, body)
	if title != "This page isn't here · CodeXArtifacts" || metas["description"] != "This page isn't here, or it isn't open to you." || metas["og:title"] != title {
		t.Errorf("%s: title %q, description %q", what, title, metas["description"])
	}
	if metas["theme-color (prefers-color-scheme: dark)"] == "" || metas["theme-color (prefers-color-scheme: light)"] == "" {
		t.Errorf("%s: no theme colours in %v", what, metas)
	}
	if b := bootOf(t, body); b.Page != "error" || b.Status != http.StatusNotFound || b.Artifact != nil {
		t.Errorf("%s: boot %+v", what, b)
	}
}
