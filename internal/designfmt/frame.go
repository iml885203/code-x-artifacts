package designfmt

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// allowedTags are the elements a frame may contain: the document skeleton,
// text, lists, tables, the sectioning elements, form controls as they look,
// pictures and the drawing elements of inline SVG. Nothing that runs, loads or
// embeds another document is here. Names are lower case, as the tokenizer gives them.
var allowedTags = setOf(`
html head body title meta style link
div span p h1 h2 h3 h4 h5 h6 a b i u s em strong small mark sub sup code pre kbd samp var abbr cite q blockquote br hr wbr del ins bdi bdo data time dfn ruby rt rp
ul ol li dl dt dd menu
table thead tbody tfoot tr th td caption col colgroup
header footer nav main section article aside figure figcaption address details summary dialog hgroup search
form fieldset legend label input button select option optgroup datalist textarea output progress meter
img picture source map area
svg g path circle ellipse rect line polyline polygon text tspan textpath defs lineargradient radialgradient stop clippath mask pattern symbol use image marker desc
filter feblend fecolormatrix fecomponenttransfer fecomposite feconvolvematrix fediffuselighting fedisplacementmap fedropshadow feflood fegaussianblur feimage femerge femergenode femorphology feoffset fespecularlighting fetile feturbulence fefunca fefuncb fefuncg fefuncr`)

// forbiddenAttributes are refused whatever their value: they run code, embed a
// document, or change how the browser treats the page.
var forbiddenAttributes = setOf("srcdoc http-equiv data-pictor-tokens")

// addressAttributes hold an address, which checkAddress holds to the frame's rules.
var addressAttributes = setOf("href src srcset poster action formaction background data cite longdesc xlink:href usemap ping archive codebase manifest")

const maxProblemsPerFrame = 12

// checkSource lists what is wrong with a frame's HTML as it was written. It
// reads tokens, not a tree, so the elements it numbers are the ones the author
// wrote, in the order they wrote them (element 1 is the first start tag), and an
// element the parser would move or drop is still seen.
func checkSource(text string) []string {
	var (
		problems []string
		element  int
		inStyle  bool
	)
	add := func(format string, args ...any) {
		if len(problems) < maxProblemsPerFrame {
			problems = append(problems, fmt.Sprintf("element %d: ", element)+fmt.Sprintf(format, args...))
		}
	}
	z := html.NewTokenizer(strings.NewReader(text))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); !errors.Is(err, io.EOF) {
				add("the HTML cannot be read: %v", err)
			}
			return problems
		case html.StartTagToken, html.SelfClosingTagToken:
			element++
			name, hasAttr := z.TagName()
			tag := string(name)
			if !allowedTags[tag] {
				add("tag %s is not allowed (%s)", tag, tagHint(tag))
			}
			checkStartAttributes(z, tag, hasAttr, add)
			inStyle = tag == "style" && tt == html.StartTagToken
		case html.TextToken:
			if inStyle {
				for _, p := range checkCSS(string(z.Text()), false) {
					add("style element: %s", p)
				}
			}
			inStyle = false
		case html.EndTagToken, html.CommentToken, html.DoctypeToken:
			inStyle = false
		}
		if len(problems) >= maxProblemsPerFrame {
			return problems
		}
	}
}

func checkStartAttributes(z *html.Tokenizer, tag string, hasAttr bool, add func(string, ...any)) {
	var attrs []html.Attribute
	kitAttrs := map[string]string{}
	pid := ""
	for hasAttr {
		var key, value []byte
		key, value, hasAttr = z.TagAttr()
		attrs = append(attrs, html.Attribute{Key: string(key), Val: string(value)})
		kitAttrs[string(key)] = string(value)
		if string(key) == "data-pid" {
			pid = string(value)
		}
	}
	if tag == "link" {
		checkKitLink(kitAttrs, add)
	}
	for _, attr := range attrs {
		checkAttribute(tag, attr.Key, attr.Val, func(format string, args ...any) {
			if pid != "" && (tag == "path" && attr.Key == "d" || attr.Key == "stroke-width") {
				add("node %q: "+format, append([]any{pid}, args...)...)
			} else {
				add(format, args...)
			}
		})
	}
}

func tagHint(tag string) string {
	switch tag {
	case "script", "noscript", "template":
		return "a frame is static markup and runs no scripts"
	case "iframe", "object", "embed", "frame", "frameset", "applet":
		return "a frame cannot embed another document"
	case "link", "base":
		return "a frame loads nothing from outside; put styles in a <style> element"
	case "video", "audio", "canvas":
		return "a frame is static; use an img, or draw with svg"
	}
	return "use plain elements such as div, span, p, img, ul, table or svg"
}

// checkAttribute checks one attribute of an element.
func checkAttribute(tag, key, value string, add func(string, ...any)) {
	if err := checkVectorAttribute(tag, key, value); err != nil {
		add("attribute %s: %v", key, err)
		return
	}
	switch {
	case strings.HasPrefix(key, "on"):
		add("attribute %s is not allowed (a frame runs no scripts)", key)
	case forbiddenAttributes[key]:
		add("attribute %s is not allowed on %s", key, tag)
	case key == "style":
		for _, p := range checkCSS(value, true) {
			add("style attribute: %s", p)
		}
	case permittedHref(tag, key, value):
		// Only the trusted kit is loaded; an outside anchor is followed by a person.
	case addressAttributes[key]:
		for _, u := range addressesOf(key, value) {
			if err := checkAddress(u); err != nil {
				add("attribute %s: %v", key, err)
			}
		}
	case strings.Contains(strings.ToLower(value), "url("):
		for _, u := range urlsIn(value) {
			if err := checkAddress(u); err != nil {
				add("attribute %s: %v", key, err)
			}
		}
	}
}

// addressesOf splits an attribute that may hold several addresses: a srcset is
// candidates "url descriptor" separated by commas, where a url runs to the next
// space (so a data: address keeps its commas).
func addressesOf(key, value string) []string {
	if key != "srcset" {
		return []string{value}
	}
	var out []string
	rest := value
	for {
		rest = strings.TrimLeft(rest, " \t\r\n\f,")
		if rest == "" {
			return out
		}
		end := strings.IndexAny(rest, " \t\r\n\f")
		if end < 0 {
			end = len(rest)
		}
		out = append(out, strings.TrimRight(rest[:end], ","))
		rest = rest[end:]
		if comma := strings.IndexByte(rest, ','); comma >= 0 {
			rest = rest[comma+1:]
		} else {
			return out
		}
	}
}
