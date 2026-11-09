package web

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// From an opaque origin every load is a cross-origin request with Origin: null
// and no cookie.
var fromTheSandbox = map[string]string{"Origin": "null"}

func TestEveryVersionOfTheKitIsServedToTheSandboxWithoutASessionAndCachedForever(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, v := range kitVersions {
		path := "/kit/" + v.dir + "/pictor.css"
		res, body := h.do(nil, "GET", path, "", fromTheSandbox)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d without a session", path, res.StatusCode)
		}
		if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
			t.Errorf("%s: Content-Type %q", path, got)
		}
		if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
			t.Errorf("%s: Cache-Control %q: a versioned path is cached for a year and never revalidated", path, cc)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: Access-Control-Allow-Origin %q: the sandbox's origin is null, so nothing narrower works", path, got)
		}
		if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options %q", path, got)
		}
		if body != string(v.css) || !strings.Contains(body, "--night") || !strings.Contains(body, "prefers-color-scheme") {
			t.Errorf("%s: what is served is not that version's stylesheet: %.80s", path, body)
		}
	}
}

func TestTheKitHasOneImmutableVersionPerDirectoryAndNoOthers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, path := range []string{"/kit/v6/pictor.css", "/kit/pictor.css", "/kit/v1/other.css", "/kit/v6/sample.html"} {
		if res, _ := h.do(nil, "GET", path, "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, res.StatusCode)
		}
	}
	if kitStylesheetPath != "/kit/"+kitVersions[len(kitVersions)-1].dir+"/pictor.css" {
		t.Errorf("the path the guides and the reference use, %s, is not the newest version", kitStylesheetPath)
	}
}

// Pages already published link /kit/v1/ and are cached for a year, so a change
// to its bytes would reach some viewers and not others. Update the sum only for
// a version that has not been deployed; otherwise add /kit/v3/.
func TestKitV1IsFrozenOnceReleased(t *testing.T) {
	t.Parallel()
	const released = "3af9346aa589f0507fb5fc2249a43ca3e448a3a555f6003fda75ce122b9c2310"
	sum := sha256.Sum256(kitV1CSS)
	if got := hex.EncodeToString(sum[:]); got != released {
		t.Errorf("kit/v1/pictor.css changed (sha256 %s). v1 is cached for a year by every viewer: ship the change as a new version, or, if v1 was never deployed, update this sum", got)
	}
}

// The same rule for v2 from the day it is deployed. Until then its sum follows
// the file; after it, a change is /kit/v3/.
func TestKitV2IsFrozenOnceReleased(t *testing.T) {
	t.Parallel()
	const released = "9a5d6ed24f2e98669c105b814e16f198e11c6f0efce418cfa6ff30076750b82e"
	sum := sha256.Sum256(kitV2CSS)
	if got := hex.EncodeToString(sum[:]); got != released {
		t.Errorf("kit/v2/pictor.css changed (sha256 %s). Once deployed, v2 is cached for a year by every viewer: ship the change as /kit/v3/, or, if v2 was never deployed, update this sum", got)
	}
}

// The same rule for v3 and v4 from the day each is deployed.
func TestKitV3IsFrozenOnceReleased(t *testing.T) {
	t.Parallel()
	const released = "00a0440a3a3cb50278038324eaaf4541017d9a627441241500ef893b78ca19e9"
	sum := sha256.Sum256(kitV3CSS)
	if got := hex.EncodeToString(sum[:]); got != released {
		t.Errorf("kit/v3/pictor.css changed (sha256 %s). Once deployed, v3 is cached for a year by every viewer: ship the change as /kit/v5/, or, if v3 was never deployed, update this sum", got)
	}
}

func TestKitV4IsFrozenOnceReleased(t *testing.T) {
	t.Parallel()
	const released = "015be789a92957690e64c853dd1c6852a54f13b96dc2bee1bd7a5882ff325d7e"
	sum := sha256.Sum256(kitV4CSS)
	if got := hex.EncodeToString(sum[:]); got != released {
		t.Errorf("kit/v4/pictor.css changed (sha256 %s). Once deployed, v4 is cached for a year by every viewer: ship the change as /kit/v5/, or, if v4 was never deployed, update this sum", got)
	}
}

func TestTheFontsTheKitNamesLoadInASandboxedPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var urls [][]string
	for _, v := range kitVersions {
		urls = append(urls, regexp.MustCompile(`url\("(/static/fonts/[^"]+)"\)`).FindAllStringSubmatch(string(v.css), -1)...)
	}
	if len(urls) == 0 {
		t.Fatal("the kit names no font: the scan is broken")
	}
	for _, m := range urls {
		res, body := h.do(nil, "GET", m[1], "", fromTheSandbox)
		if res.StatusCode != http.StatusOK || len(body) < 1000 {
			t.Errorf("%s: status %d, %d bytes", m[1], res.StatusCode, len(body))
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: Access-Control-Allow-Origin %q, so a font-face in a sandboxed page is blocked", m[1], got)
		}
	}
	// Only fonts are for other origins: the rest of /static stays as it was.
	if res, _ := h.do(nil, "GET", "/static/style.css", "", fromTheSandbox); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("/static/style.css is served to any origin: only the fonts should be")
	}
}

var (
	cssVarUse  = regexp.MustCompile(`var\((--[a-z0-9-]+)`)
	cssVarDef  = regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+)\s*:`)
	cssClass   = regexp.MustCompile(`\.(pk-[a-z0-9]+(?:-{1,2}[a-z0-9]+)*)`)
	classAttrs = regexp.MustCompile(`class="([^"]+)"`)
)

// block is the declarations inside the rule whose selector starts with sel.
func block(t *testing.T, css, sel string) string {
	t.Helper()
	i := strings.Index(css, sel)
	if i < 0 {
		t.Fatalf("no rule %q in the kit", sel)
	}
	rest := css[i:]
	open := strings.Index(rest, "{")
	end := strings.Index(rest, "\n}")
	if open < 0 || end < open {
		t.Fatalf("rule %q is malformed", sel)
	}
	return rest[open+1 : end]
}

func definitions(css string) []string {
	var names []string
	for _, m := range cssVarDef.FindAllStringSubmatch(css, -1) {
		names = append(names, m[1])
	}
	slices.Sort(names)
	return names
}

func TestEveryColourTokenExistsInBothThemesAndForcingATheme(t *testing.T) {
	t.Parallel()
	for _, v := range kitVersions {
		css := string(v.css)
		dark := definitions(block(t, css, ":root,\n[data-theme=\"dark\"]"))
		light := definitions(block(t, css, "  :root:not([data-theme=\"dark\"])"))
		forced := definitions(block(t, css, "[data-theme=\"light\"] {"))
		if len(dark) < 20 {
			t.Fatalf("%s: found %d dark tokens: the scan is broken", v.dir, len(dark))
		}
		// A token that only one theme defines is the colour that vanishes in the other.
		for name, got := range map[string][]string{"light by the system": light, "light forced": forced} {
			for _, token := range dark {
				if !slices.Contains(got, token) {
					t.Errorf("%s: %s theme lacks %s, which the dark theme defines", v.dir, name, token)
				}
			}
		}
		if !slices.Equal(light, forced) {
			t.Errorf("%s: the system's light theme and the forced one define different tokens:\n%v\n%v", v.dir, light, forced)
		}
		// Whatever the kit reads is defined by the kit.
		defined := definitions(css)
		for _, m := range cssVarUse.FindAllStringSubmatch(css, -1) {
			if !slices.Contains(defined, m[1]) && m[1] != "--series" && m[1] != "--tone" && m[1] != "--v" {
				t.Errorf("%s: the kit reads %s and never defines it", v.dir, m[1])
			}
		}
	}
}

// The newest kit is the site's look for pages agents make: every colour token it
// shares with the site's :root (style.css, written with light-dark()) has the
// site's value in each theme, so a change to the site's tokens that the kit does
// not follow fails here and asks for the next kit version. Older kits are held
// only by their sums: they are frozen, not kept in step.
func TestTheNewestKitHasTheSitesColourTokensInBothThemes(t *testing.T) {
	t.Parallel()
	css := string(kitVersions[len(kitVersions)-1].css)
	site := siteTokens(t)
	dark := block(t, css, ":root,\n[data-theme=\"dark\"]")
	themes := map[string]string{
		"dark":                dark,
		"light by the system": block(t, css, "  :root:not([data-theme=\"dark\"])"),
		"light forced":        block(t, css, "[data-theme=\"light\"] {"),
	}
	must := []string{"--night", "--plate", "--raised", "--line", "--edge", "--text", "--text-2", "--muted", "--star", "--star-edge", "--star-hover", "--star-text", "--star-wash", "--on-star", "--danger", "--kind-web-bg", "--kind-design-fg"}
	compared := map[string]bool{}
	for _, token := range definitions(dark) {
		if _, ok := site[token]; ok {
			compared[token] = true
		}
	}
	for _, token := range must {
		if !compared[token] {
			t.Errorf("the newest kit does not define %s, which the site does", token)
		}
	}
	for token := range compared {
		for name, b := range themes {
			theme := "dark"
			if strings.HasPrefix(name, "light") {
				theme = "light"
			}
			if got, want := tokenValue(t, b, token), siteValue(t, site, token, theme); got != want {
				t.Errorf("kit %s theme: %s is %s, the site's is %s", name, token, got, want)
			}
		}
	}
	if !strings.Contains(block(t, css, ".pk-btn--primary {"), "var(--star-edge)") || !strings.Contains(block(t, css, ".pk-btn--primary:hover"), "var(--star-hover)") {
		t.Error("the primary button does not use the edge and hover tokens")
	}
}

// siteTokens reads the declarations of the site's :root as written.
func siteTokens(t *testing.T) map[string]string {
	t.Helper()
	decl := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+)\s*:\s*([^;]+);`).FindAllStringSubmatch(block(t, string(mustRead(t, "static/style.css")), ":root {"), -1) {
		decl[m[1]] = strings.TrimSpace(m[2])
	}
	if len(decl) < 30 {
		t.Fatalf("found %d site tokens: the scan is broken", len(decl))
	}
	return decl
}

// siteValue is a site token's value in one theme: the light-dark() side that theme takes, with var() followed.
func siteValue(t *testing.T, site map[string]string, token, theme string) string {
	t.Helper()
	v, ok := site[token]
	if !ok {
		t.Fatalf("the site has no %s", token)
	}
	if m := regexp.MustCompile(`(?s)^light-dark\((.*)\)$`).FindStringSubmatch(v); m != nil {
		depth, from := 0, 0
		var parts []string
		for i, c := range m[1] {
			switch c {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(m[1][from:i]))
					from = i + 1
				}
			}
		}
		parts = append(parts, strings.TrimSpace(m[1][from:]))
		if len(parts) != 2 {
			t.Fatalf("%s: light-dark() without two values", token)
		}
		v = parts[0]
		if theme == "dark" {
			v = parts[1]
		}
	}
	if ref := regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`).FindStringSubmatch(v); ref != nil {
		return siteValue(t, site, ref[1], theme)
	}
	return v
}

// The primary button must be readable and findable in both themes: its label
// against its fill at 7:1 (AAA), and its edge against the surfaces it sits on at
// 3:1 (non-text contrast). Computed from the tokens, so a new colour is checked
// without anyone looking at it.
func TestKitFromV2OnPrimaryButtonContrast(t *testing.T) {
	t.Parallel()
	for _, v := range kitVersions[1:] {
		primaryButtonIsReadable(t, v)
	}
}

func primaryButtonIsReadable(t *testing.T, v kitVersion) {
	t.Helper()
	css := string(v.css)
	for name, sel := range map[string]string{"dark": ":root,\n[data-theme=\"dark\"]", "light": "[data-theme=\"light\"] {"} {
		b := block(t, css, sel)
		colour := func(token string) [3]float64 { return parseOKLCH(t, tokenValue(t, b, token)) }
		fill, label, edge := colour("--star"), colour("--on-star"), colour("--star-edge")
		if got := contrast(label, fill); got < 7 {
			t.Errorf("%s: label on the primary button is %.2f:1, want at least 7:1", name, got)
		}
		for _, surface := range []string{"--night", "--plate"} {
			if got := contrast(edge, colour(surface)); got < 3 {
				t.Errorf("%s: the button's edge on %s is %.2f:1, want at least 3:1", name, surface, got)
			}
		}
	}
	// Hover: the fill mixed toward --plate (light) or --text (dark) as the site does (v4), or a literal (before it). The label stays readable on it by the same margin.
	b := block(t, css, "[data-theme=\"light\"] {")
	if got := contrast(parseOKLCH(t, tokenValue(t, b, "--on-star")), hoverColour(t, b)); got < 7 {
		t.Errorf("light: label on the hovered primary button is %.2f:1, want at least 7:1", got)
	}
}

// hoverColour is --star-hover as a colour: a literal, or the fill mixed N% toward --text or --plate, worked out.
func hoverColour(t *testing.T, b string) [3]float64 {
	t.Helper()
	v := tokenValue(t, b, "--star-hover")
	m := regexp.MustCompile(`^color-mix\(in oklch, var\(--star\), var\((--text|--plate)\) ([0-9.]+)%\)$`).FindStringSubmatch(v)
	if m == nil {
		return parseOKLCH(t, v)
	}
	share, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		t.Fatal(err)
	}
	from, to := parseOKLCH(t, tokenValue(t, b, "--star")), parseOKLCH(t, tokenValue(t, b, m[1]))
	d := to[2] - from[2]
	if d > 180 {
		d -= 360
	} else if d < -180 {
		d += 360
	}
	k := share / 100
	return [3]float64{from[0] + (to[0]-from[0])*k, from[1] + (to[1]-from[1])*k, from[2] + d*k}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := staticFS.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var oklchValue = regexp.MustCompile(`^oklch\(([0-9.]+) ([0-9.]+) ([0-9.]+)\)$`)

// tokenValue is what a block declares for a token, following one var() to
// another token in the same block.
func tokenValue(t *testing.T, b, token string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(token) + `\s*:\s*([^;]+);`).FindStringSubmatch(b)
	if m == nil {
		t.Fatalf("no %s in the block", token)
	}
	if ref := regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`).FindStringSubmatch(m[1]); ref != nil {
		return tokenValue(t, b, ref[1])
	}
	return strings.TrimSpace(m[1])
}

func parseOKLCH(t *testing.T, v string) [3]float64 {
	t.Helper()
	m := oklchValue.FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("%q is not a plain oklch() colour", v)
	}
	var c [3]float64
	for i := range c {
		f, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			t.Fatal(err)
		}
		c[i] = f
	}
	return c
}

// luminance is the WCAG relative luminance of an OKLCH colour (clipped to sRGB).
func luminance(c [3]float64) float64 {
	a, b := c[1]*math.Cos(c[2]*math.Pi/180), c[1]*math.Sin(c[2]*math.Pi/180)
	cube := func(x float64) float64 { return x * x * x }
	l, m, s := cube(c[0]+0.3963377774*a+0.2158037573*b), cube(c[0]-0.1055613458*a-0.0638541728*b), cube(c[0]-0.0894841775*a-1.2914855480*b)
	rgb := [3]float64{4.0767416621*l - 3.3077115913*m + 0.2309699292*s, -1.2684380046*l + 2.6097574011*m - 0.3413193965*s, -0.0041960863*l - 0.7034186147*m + 1.7076147010*s}
	var y float64
	for i, w := range [3]float64{0.2126, 0.7152, 0.0722} {
		y += w * math.Min(1, math.Max(0, rgb[i]))
	}
	return y
}

func contrast(a, b [3]float64) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func TestTheSampleLinksTheKitAndShowsEveryComponentItDefines(t *testing.T) {
	t.Parallel()
	var defined []string
	for _, m := range cssClass.FindAllStringSubmatch(string(kitCSS), -1) {
		if !slices.Contains(defined, m[1]) {
			defined = append(defined, m[1])
		}
	}
	used := map[string]bool{}
	for _, m := range classAttrs.FindAllStringSubmatch(kitSample, -1) {
		for _, c := range strings.Fields(m[1]) {
			used[c] = true
		}
	}
	if len(defined) < 20 {
		t.Fatalf("found %d component classes: the scan is broken", len(defined))
	}
	for _, c := range defined {
		if !used[c] {
			t.Errorf("the kit defines .%s and the reference never shows it", c)
		}
	}
	if v1 := kitVersions[0]; !strings.Contains(v1.sample, `href="/kit/v1/pictor.css"`) {
		t.Error("the v1 sample no longer links v1")
	}
	if !strings.Contains(kitSample, `href="`+kitStylesheetPath+`"`) {
		t.Error("the sample does not link the kit the way an agent page does")
	}
	if !strings.Contains(kitSample, `name="viewport"`) {
		t.Error("the sample has no viewport meta, so it does not show the phone-width contract")
	}
}

func TestTheReferenceIsAPageOfTheAppForSignedInPeopleOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if res, _ := h.do(nil, "GET", "/kit", "", nil); res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("signed out: status %d to %q", res.StatusCode, res.Header.Get("Location"))
	}
	res, body := h.do(h.browser("alice", "pw-a"), "GET", "/kit", "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `"page":"kit"`) {
		t.Fatalf("status %d, boot block %.200q", res.StatusCode, body)
	}
	// The samples are framed with a CSP that allows frames from this origin, and
	// the sample itself may be framed by it.
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-src 'self'") {
		t.Errorf("csp %q cannot frame the samples", res.Header.Get("Content-Security-Policy"))
	}
	if csp := func() string {
		r, _ := h.do(nil, "GET", kitSamplePath, "", nil)
		return r.Header.Get("Content-Security-Policy")
	}(); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("the sample's csp %q does not let the reference frame it", csp)
	}
}

func TestTheSampleForcesOnlyAThemeThatExists(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, v := range kitVersions {
		for query, want := range map[string]string{"?theme=dark": `<html data-theme="dark" `, "?theme=light": `<html data-theme="light" `, "": `<html lang`, "?theme=x%22%3E": `<html lang`} {
			res, body := h.do(nil, "GET", "/kit/"+v.dir+"/sample.html"+query, "", nil)
			if res.StatusCode != http.StatusOK || !strings.Contains(body, want) {
				t.Errorf("%s %q: status %d, want %q", v.dir, query, res.StatusCode, want)
			}
			if csp := res.Header.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox") {
				t.Errorf("%s %q: csp %q", v.dir, query, csp)
			}
		}
	}
}

// The reading width limits lines of running text. It must not narrow what else
// a page puts inside .pk-prose (a scorecard grid, a chart, a table), which is
// what happened when agents wrapped a whole page in it (#124). Versions before
// v3 capped the wrapper; they are frozen and stay as they were.
func TestKitV3CapsRunningTextInsideProseAndNotTheWideContentAroundIt(t *testing.T) {
	t.Parallel()
	for _, v := range kitVersions[2:] {
		capsRunningTextOnly(t, v)
	}
}

func capsRunningTextOnly(t *testing.T, v kitVersion) {
	t.Helper()
	css := string(v.css)
	var capped []string
	for _, m := range regexp.MustCompile(`(?s)([^{}]+)\{[^{}]*max-width:\s*var\(--measure\)[^{}]*\}`).FindAllStringSubmatch(css, -1) {
		capped = append(capped, strings.TrimSpace(m[1]))
	}
	if len(capped) == 0 {
		t.Fatal("nothing in the kit uses --measure: the scan is broken")
	}
	text := []string{"p", "ul", "ol", "dl", "blockquote", "h2", "h3", "h4"}
	childOf := regexp.MustCompile(`^\.pk-prose :is\(([^)]*)\)$`)
	self := regexp.MustCompile(`^\.pk-prose:is\(([^)]*)\)$`)
	inside := map[string]bool{}
	for _, group := range capped {
		for _, sel := range strings.Split(group, ",\n") {
			sel = strings.Join(strings.Fields(sel), " ")
			m := childOf.FindStringSubmatch(sel)
			if m == nil {
				m = self.FindStringSubmatch(sel)
			}
			if m == nil {
				t.Errorf("%q caps its width to the measure: only running text inside .pk-prose may be capped, not the wrapper or other children", sel)
				continue
			}
			for _, tag := range strings.Split(m[1], ",") {
				tag = strings.TrimSpace(tag)
				if !slices.Contains(text, tag) {
					t.Errorf("%s is capped to the measure: it is not running text", tag)
				}
				inside[tag] = true
			}
		}
	}
	// A descendant, not only a child: a paragraph in a div, a card or a grid cell
	// inside .pk-prose gets a readable line too (max-width never widens, so a
	// narrow cell is unaffected), while the grid itself keeps the page width.
	if !strings.Contains(strings.Join(capped, "\n"), ".pk-prose :is(p,") {
		t.Error("a paragraph nested in a div or a grid cell inside .pk-prose is not capped to the measure")
	}
	for _, tag := range []string{"p", "ul", "ol", "blockquote", "h2", "h3"} {
		if !inside[tag] {
			t.Errorf("a %s inside .pk-prose is not capped to the measure", tag)
		}
	}
	if old := string(kitV2CSS); !strings.Contains(old, ".pk-prose { max-width: var(--measure); }") {
		t.Error("v2 changed: it caps the wrapper and must stay as released")
	}
}

func TestKitV5IsFrozenOnceReleased(t *testing.T) {
	t.Parallel()
	const released = "939768d654ae2e95ebbcc5dca61976972eaa73642636cd19447b21ec8b38c9cc"
	sum := sha256.Sum256(kitCSS)
	if got := hex.EncodeToString(sum[:]); got != released {
		t.Errorf("kit/v5/pictor.css changed (sha256 %s); ship new bytes under /kit/v6/", got)
	}
}
