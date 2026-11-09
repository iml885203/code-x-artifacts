package designfmt

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// This file is the part of Review that reads styles: text that is likely to
// overflow its box, and properties that are kept as written but do nothing. It
// reads inline styles only (the ones the editor and the agent both see on the
// element), so a frame styled by classes in a <style> element is not judged
// for overflow; the notes say "likely", never "certainly".

// Defaults a browser uses when nothing says otherwise.
const (
	defaultFontPx     = 16.0
	normalLineHeight  = 1.2
	overflowTolerance = 1.15 // an estimate this much over the room is a note; less is within what the estimate can get wrong
)

// inertProperties are accepted and kept, and no current browser applies them:
// the prefixes of engines that no longer exist, and one that was only ever a
// hint for old iOS. -moz- and -webkit- properties are not here, because
// Firefox and Safari still apply some of them.
func inertProperty(name string) bool {
	return strings.HasPrefix(name, "-ms-") || strings.HasPrefix(name, "-o-") || name == "-webkit-overflow-scrolling"
}

// styleReview collects the notes of one walk over a frame's body.
type styleReview struct {
	frame Frame
	notes []string
	seen  map[string]bool // a note is given once for each place and property
}

// reviewStyles walks the body and returns the notes about text that probably
// does not fit its box and about properties that do nothing.
func reviewStyles(body *html.Node, f Frame) []string {
	r := &styleReview{frame: f, seen: map[string]bool{}}
	r.walk(body, defaultFontPx, "")
	return r.notes
}

func (r *styleReview) walk(n *html.Node, font float64, lineHeight string) {
	if n.Type != html.ElementNode {
		return
	}
	switch n.Data {
	case "style":
		r.inert("a <style> rule", declaredProperties(textOf(n), false))
		return
	case "svg", "script":
		return // the text of a drawing is placed by the drawing, not flowed in a box
	}
	r.inert(describe(n), declaredProperties(attr(n, "style"), true))
	props := inlineStyle(attr(n, "style"))
	font = fontSizeOf(props, font)
	if v, set := props["line-height"]; set && v != "inherit" {
		lineHeight = v
	}
	if note := overflowNote(n, props, font, lineHeight, r.frame); note != "" {
		r.notes = append(r.notes, note)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c, font, lineHeight)
	}
}

// inert notes each property that nothing applies, once for each place.
func (r *styleReview) inert(where string, names []string) {
	for _, name := range names {
		if !inertProperty(name) || r.seen[where+" "+name] {
			continue
		}
		r.seen[where+" "+name] = true
		r.notes = append(r.notes, fmt.Sprintf("%s sets %s, which no current browser applies; it is kept as written and does nothing", where, name))
	}
}

// fontSizeOf is the font size in pixels an element has: its own when it gives
// one in pixels, the inherited one otherwise, and unknown (0) when it gives one
// in em, % or a keyword, which means something only with the stylesheet.
func fontSizeOf(props map[string]string, inherited float64) float64 {
	v, set := props["font-size"]
	if !set || v == "inherit" {
		return inherited
	}
	if px, ok := pixelsOf(v); ok && px > 0 {
		return px
	}
	return 0
}

// textBox is what an element says about the room for its text.
type textBox struct {
	text       string
	font       float64
	lineHeight string
	width      float64
	hasWidth   bool
	height     float64
	hasHeight  bool
	wrap       bool
	scrolls    bool
	ellipsis   bool
	clamped    bool
}

func textBoxOf(n *html.Node, props map[string]string, font float64, lineHeight string, f Frame) (textBox, bool) {
	text, ok := flowText(n, props["white-space"])
	if !ok || strings.TrimSpace(text) == "" {
		return textBox{}, false
	}
	b := textBox{text: text, font: font, lineHeight: lineHeight}
	ws := props["white-space"]
	b.wrap = !strings.Contains(ws, "nowrap") && !strings.HasPrefix(ws, "pre")
	b.scrolls = strings.Contains(props["overflow"], "auto") || strings.Contains(props["overflow"], "scroll") ||
		strings.Contains(props["overflow-y"], "auto") || strings.Contains(props["overflow-y"], "scroll")
	b.ellipsis = strings.Contains(props["text-overflow"], "ellipsis")
	b.clamped = props["-webkit-line-clamp"] != "" || props["line-clamp"] != ""
	b.width, b.hasWidth = contentBox(props, "width", "padding-left", "padding-right")
	if !b.hasWidth && !b.wrap {
		b.width, b.hasWidth = float64(f.W), true // one line that is wider than the frame is wider than the frame
	}
	b.height, b.hasHeight = contentBox(props, "height", "padding-top", "padding-bottom")
	return b, true
}

// overflowNote says that the text of n is estimated not to fit the box n gives
// itself in pixels: more lines than its height holds, or a line that is meant
// to stay on one line and is wider than its width.
func overflowNote(n *html.Node, props map[string]string, font float64, lineHeight string, f Frame) string {
	if font <= 0 {
		return ""
	}
	box, ok := textBoxOf(n, props, font, lineHeight, f)
	if !ok || box.scrolls || !box.hasWidth {
		return ""
	}
	label := fmt.Sprintf("%s (%q)", describe(n), snippet(box.text))
	if !box.wrap {
		longest := 0.0
		for _, line := range strings.Split(box.text, "\n") {
			longest = math.Max(longest, textWidth(line, box.font))
		}
		if longest > box.width*overflowTolerance && !box.ellipsis {
			return fmt.Sprintf("%s stays on one line and is about %.0f px wide for %.0f px of room; let it wrap, or make the box wider", label, longest, box.width)
		}
		return ""
	}
	if !box.hasHeight || box.clamped {
		return ""
	}
	need := wrappedLines(box.text, box.width, box.font) * lineHeightPx(box.lineHeight, box.font)
	if need > box.height*overflowTolerance+1 {
		return fmt.Sprintf("%s is likely to overflow its box: about %.0f px of text for %.0f px of height; shorten it, make the box taller, or let the height follow the text", label, need, box.height)
	}
	return ""
}

// contentBox is the pixel size the element gives its content on one axis: the
// declared size, less its padding when padding is inside that size.
func contentBox(props map[string]string, size, padStart, padEnd string) (float64, bool) {
	px, ok := pixelsOf(props[size])
	if !ok {
		return 0, false
	}
	if props["box-sizing"] == "border-box" {
		px -= paddingPx(props, padStart, padEnd, size)
	}
	return math.Max(px, 1), true
}

// paddingPx is the padding at both ends of an axis, from the shorthand and then
// the longhands; a value that is not in pixels counts as none. size says the
// axis: "height" is top and bottom, anything else left and right.
func paddingPx(props map[string]string, start, end, size string) float64 {
	var top, right, bottom, left float64
	if parts := strings.Fields(props["padding"]); len(parts) > 0 && len(parts) <= 4 {
		px := make([]float64, len(parts))
		for i, p := range parts {
			px[i], _ = pixelsOf(p)
		}
		switch len(px) {
		case 1:
			top, right, bottom, left = px[0], px[0], px[0], px[0]
		case 2:
			top, right, bottom, left = px[0], px[1], px[0], px[1]
		case 3:
			top, right, bottom, left = px[0], px[1], px[2], px[1]
		default:
			top, right, bottom, left = px[0], px[1], px[2], px[3]
		}
	}
	first, second := left, right
	if size == "height" {
		first, second = top, bottom
	}
	if px, ok := pixelsOf(props[start]); ok {
		first = px
	}
	if px, ok := pixelsOf(props[end]); ok {
		second = px
	}
	return first + second
}

// lineHeightPx is the height of a line: a pixel value, a multiple of the font
// size, or the browser's normal.
func lineHeightPx(spec string, font float64) float64 {
	spec = strings.TrimSpace(strings.TrimSuffix(spec, "!important"))
	if px, ok := pixelsOf(spec); ok && strings.HasSuffix(spec, "px") {
		return px
	}
	if k, err := strconv.ParseFloat(spec, 64); err == nil && k > 0 {
		return k * font
	}
	return normalLineHeight * font
}

// pixelsOf reads a number of pixels: "48px", "0", "12.5px".
func pixelsOf(v string) (float64, bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
	v = strings.TrimSpace(strings.TrimSuffix(v, "px"))
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// inlineTags are the elements whose text flows in the line of the element that
// holds them. An element with anything else inside is a container: its own
// children are judged, not its text as a whole.
var inlineTags = func() map[string]bool {
	out := setOf("br")
	for tag := range inlineKept {
		out[tag] = true
	}
	for tag := range inlineOpaque {
		out[tag] = true
	}
	return out
}()

// flowText is the text an element sets in its own lines: its text, with the
// text of the inline elements inside it, a <br> as a line break. It is not
// text when the element holds anything that is not inline.
func flowText(n *html.Node, whiteSpace string) (string, bool) {
	var b strings.Builder
	pre := strings.HasPrefix(whiteSpace, "pre")
	var walk func(*html.Node) bool
	walk = func(p *html.Node) bool {
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode:
				b.WriteString(c.Data)
			case c.Type == html.ElementNode && c.Data == "br":
				b.WriteString("\n")
			case c.Type == html.ElementNode && inlineTags[c.Data]:
				if !walk(c) {
					return false
				}
			case c.Type == html.CommentNode:
			default:
				return false
			}
		}
		return true
	}
	if !walk(n) {
		return "", false
	}
	text := b.String()
	if pre {
		return text, true
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Join(strings.Fields(line), " "))
	}
	return strings.Join(lines, "\n"), true
}

// textOf is the text inside an element, as written.
func textOf(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return s
}

// The width of text is estimated per character, in units of the font size: a
// character of Han, kana or hangul is one, a capital or a digit a little over a
// half, anything else a little under. It is the estimate the slides review
// uses, so the two kinds say the same about the same text.
func advance(r rune) float64 {
	switch {
	case r >= 0x2E80:
		return 1
	case r == ' ':
		return 0.2
	case r >= 'A' && r <= 'Z':
		return 0.59
	case r >= '0' && r <= '9':
		return 0.57
	default:
		return 0.46
	}
}

func textWidth(s string, px float64) float64 {
	w := 0.0
	for _, r := range s {
		w += advance(r)
	}
	return w * px
}

// wrappedLines is how many lines text takes in a box w pixels wide, honouring
// explicit line breaks; wrapping at word boundaries wastes a little of each
// line, which the 1.07 stands for.
func wrappedLines(text string, w, px float64) float64 {
	n := 0.0
	for _, line := range strings.Split(text, "\n") {
		n += math.Max(1, math.Ceil(textWidth(line, px)*1.07/math.Max(w, px)))
	}
	return n
}
