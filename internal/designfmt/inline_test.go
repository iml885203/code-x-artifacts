package designfmt

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func TestAHostileInlineAttributeIsRefusedAtPublish(t *testing.T) {
	const where = `Page "Home" frame "Hero" element `
	for name, tc := range map[string]struct{ body, want string }{
		"an event on a bold":          {`<div>Hello <b onclick="x()">bold</b><br>line2</div>`, where + `3: attribute onclick is not allowed`},
		"a script address on a link":  {`<div>Hello <a href="javascript:alert(1)">go</a></div>`, where + `3: attribute href: external address`},
		"an encoded script address":   {`<div>Hello <a href="java&#x09;script:alert(1)">go</a></div>`, where + `3: attribute href: external address`},
		"an event on a break":         {`<div>one<br onmouseover="x()">two</div>`, where + `3: attribute onmouseover is not allowed`},
		"an address in a span style":  {`<div>a <span style="background:url(https://x.test/a.png)">b</span></div>`, where + `3: style attribute: css property "background": external address`},
		"a script inside a paragraph": {`<p>text <script>alert(1)</script></p>`, where + `3: tag script is not allowed`},
	} {
		err := validateHero(tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want a problem containing %q", name, err, tc.want)
		}
	}
}

func parseInline(t *testing.T, markup string) *html.Node {
	t.Helper()
	nodes, err := html.ParseFragment(strings.NewReader(markup), &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("%q: %v (%d nodes)", markup, err, len(nodes))
	}
	return nodes[0]
}

func TestTextKeepsTheInlineElementsItEditsAndOnlyThoseThatAreText(t *testing.T) {
	for markup, want := range map[string]bool{
		`<b>x</b>`:                                               true,
		`<strong class="k">x</strong>`:                           true,
		`<a href="#x" target="_blank">x</a>`:                     true,
		`<span style="color:red;font-weight:700;--x:1">x</span>`: true,
		`<span style="position:absolute;left:5px">x</span>`:      false,
		`<span style="display:inline-block">x</span>`:            false,
		`<span style="width:10px">x</span>`:                      false,
		`<mark>x</mark>`:                                         false, // phrasing, but not edited
		`<div>x</div>`:                                           false,
		`<img alt="">`:                                           false,
	} {
		if got := EditedInline(parseInline(t, markup)); got != want {
			t.Errorf("%s: EditedInline is %v, want %v", markup, got, want)
		}
	}
	for markup, want := range map[string]bool{`<mark>x</mark>`: true, `<small style="color:red">x</small>`: true, `<small style="position:absolute">x</small>`: false, `<b>x</b>`: false} {
		if got := OpaqueInline(parseInline(t, markup)); got != want {
			t.Errorf("%s: OpaqueInline is %v, want %v", markup, got, want)
		}
	}
}

func TestAnAttributeAndAFragmentAreHeldToTheRulesOfAFrame(t *testing.T) {
	if got := CheckAttribute("a", "href", "#top"); len(got) != 0 {
		t.Errorf("a fragment link: %v", got)
	}
	for _, c := range [][3]string{{"a", "href", "javascript:alert(1)"}, {"b", "onclick", "x()"}, {"span", "style", "background:url(https://x.test/a.png)"}} {
		if got := CheckAttribute(c[0], c[1], c[2]); len(got) == 0 {
			t.Errorf("%v was accepted", c)
		}
	}
	if got := CheckFragment(`<mark class="m">x</mark> <b>y</b>`); len(got) != 0 {
		t.Errorf("a fragment of text: %v", got)
	}
	if got := CheckFragment(`<script>1</script>`); len(got) == 0 {
		t.Error("a script was accepted")
	}
}
