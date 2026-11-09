package designfmt

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// The inline text model (docs/design-room.md, "Formatted text"): text inside an
// element may hold a few inline elements that the editor keeps and edits, and
// phrasing elements it keeps as they are. These are the definitions both the
// publish rules and the room use, so what a frame may say inside text and what
// the room may hold there is one list.

// inlineKept are the inline elements text keeps and edits: bold, italic,
// underline, code, a link, and a span for a colour or a face.
var inlineKept = setOf("b strong i em u a span code")

// inlineOpaque are phrasing elements that stay in text as they were written:
// they are not edited, and nothing about them is lost.
var inlineOpaque = setOf("s small mark sub sup abbr cite q kbd samp var time del ins data dfn bdi bdo wbr ruby rt rp")

// textStyleProperties are the declarations an inline element may have and still
// be text. A span that is placed, sized or laid out (position, width, display…)
// is a box of the design, not a stretch of text, and stays an element of the tree.
var textStyleProperties = setOf(`
color background background-color background-image background-clip opacity
font font-family font-size font-size-adjust font-stretch font-style font-variant font-variant-caps font-variant-ligatures font-variant-numeric
font-weight font-feature-settings font-kerning font-variation-settings letter-spacing word-spacing line-height
text-decoration text-decoration-color text-decoration-line text-decoration-style text-decoration-thickness text-emphasis text-shadow text-transform
text-underline-offset text-underline-position vertical-align white-space word-break overflow-wrap word-wrap hyphens tab-size
-webkit-text-fill-color -webkit-text-stroke -webkit-text-stroke-color -webkit-text-stroke-width -webkit-background-clip`)

// InlineKept reports whether tag is an inline element text keeps and edits.
func InlineKept(tag string) bool { return inlineKept[tag] }

// EditedInline reports whether n is an inline element the editor edits as
// text: one of the kept elements whose style, if any, says only how text looks.
func EditedInline(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && inlineKept[n.Data] && TextLevelStyle(attr(n, "style"))
}

// OpaqueInline reports whether n is a phrasing element that stays in text as
// written, with a style that is only about how text looks.
func OpaqueInline(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && inlineOpaque[n.Data] && TextLevelStyle(attr(n, "style"))
}

// TextLevelStyle reports whether every declaration of a style attribute's value
// is about how text looks.
func TextLevelStyle(style string) bool {
	for _, name := range declaredProperties(style, true) {
		if !textStyleProperties[name] && !strings.HasPrefix(name, "--") {
			return false
		}
	}
	return true
}

// CheckAttribute lists what is wrong with one attribute of an element: the rule
// a frame's attribute is held to when it is published, for a caller that holds
// an element's attributes itself (an inline element in the room's text).
func CheckAttribute(tag, key, value string) []string {
	var problems []string
	checkAttribute(tag, key, value, func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) })
	return problems
}

// CheckFragment lists what is wrong with a piece of markup under the rules of a
// frame: its tags, attributes, addresses and styles.
func CheckFragment(markup string) []string { return checkSource(markup) }
