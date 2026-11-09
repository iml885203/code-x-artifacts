package designfmt

import (
	"strings"
	"testing"
)

func TestTokensAreOptionalNamedCSSValuesAndUnsafeValuesAreRefused(t *testing.T) {
	base := `{"version":1,"pages":[{"id":"p","name":"P","rows":[[{"id":"f","name":"F","file":"frames/f.html","w":1,"h":1}]]}]`
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{``, true},
		{`,"tokens":{}`, true},
		{`,"tokens":{"--accent":"#336699","--font":"'Noto Sans', sans-serif","--space":"calc(2 * 8px)","--radius":"var(--space)","--shadow":"0 2px 4px rgba(0,0,0,.2)"}`, true},
		{`,"tokens":null`, false},
		{`,"tokens":[]`, false},
		{`,"tokens":{"accent":"red"}`, false},
		{`,"tokens":{"--a":3}`, false},
		{`,"tokens":{"--a":""}`, false},
		{`,"tokens":{"--a":"red; color:blue"}`, false},
		{`,"tokens":{"--a":"</style><script>x</script>"}`, false},
		{`,"tokens":{"--a":"url(/files/x/a)"}`, false},
		{`,"tokens":{"--a":"uRl ('x')"}`, false},
		{`,"tokens":{"--a":"var(--b"}`, false},
		{`,"tokens":{"--a":"'unclosed"}`, false},
		{`,"tokens":{"--a":"red!important"}`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			_, problems := Parse(base + tc.raw + `}`)
			if (len(problems) == 0) != tc.valid {
				t.Fatalf("problems: %v", problems)
			}
			if !tc.valid && !strings.Contains(strings.Join(problems, " "), "tokens") {
				t.Fatal(problems)
			}
		})
	}
}

func TestAFrameCanLinkOnlyAReleasedCodeXArtifactsKitStylesheet(t *testing.T) {
	for _, tc := range []struct {
		link  string
		valid bool
	}{
		{`<link rel="stylesheet" href="/kit/v4/pictor.css">`, true},
		{`<link href="/kit/v1/pictor.css" rel="stylesheet" media="print">`, true},
		{`<link rel="preload" href="/kit/v4/pictor.css">`, false},
		{`<link rel="stylesheet" href="/kit/v5/pictor.css">`, true},
		{`<link rel="stylesheet" href="/kit/v6/pictor.css">`, false},
		{`<link rel="stylesheet" href="https://other.test/kit/v4/pictor.css">`, false},
		{`<link rel="stylesheet" href="/kit/v4/pictor.css" onload="alert(1)">`, false},
		{`<link rel="stylesheet" href="/files/art/a.css">`, false},
	} {
		if err := validateHero(tc.link + `<p>Hi</p>`); (err == nil) != tc.valid {
			t.Errorf("%s: %v", tc.link, err)
		}
	}
}

func TestTokenNamesAndValuesHaveBoundedLengths(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"--" + strings.Repeat("a", 80), strings.Repeat("a", 1000), true},
		{"--" + strings.Repeat("a", 81), "red", false},
		{"--a", strings.Repeat("a", 1001), false},
		{"--a", "\u0085", false},
		{"--9", "red", false},
		{"--A_9", "red", true},
	} {
		if err := ValidateToken(tc.name, tc.value); (err == nil) != tc.valid {
			t.Errorf("%q (%d bytes): %v", tc.name, len(tc.value), err)
		}
	}
	if err := validateHero(`<style data-pictor-tokens>:root{--accent:red}</style>`); err == nil {
		t.Fatal("accepted platform-owned stylesheet marker")
	}
}
