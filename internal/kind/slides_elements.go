package kind

import (
	"fmt"
	"html"
	"math"
	"strings"
	"unicode/utf8"
)

// Every string a deck contains reaches the page through esc or inline, never
// as markup. inline is the only place text gains tags, and the tags are the
// two it writes itself.
func esc(s string) string { return html.EscapeString(s) }

// inline writes text with **bold** and `code` as markup and everything else
// as text. An unmatched marker is shown as it was typed.
func inline(s string) string {
	var b strings.Builder
	for _, sp := range spans(s) {
		switch {
		case sp.code:
			b.WriteString("<code>" + esc(sp.text) + "</code>")
		case sp.bold:
			b.WriteString("<strong>" + inline(sp.text) + "</strong>")
		default:
			b.WriteString(esc(sp.text))
		}
	}
	return b.String()
}

// span is a piece of inline text: literal, `code`, or **bold**, whose text is
// inline text again.
type span struct {
	text       string
	code, bold bool
}

// spans reads the inline markers of s: the one grammar the page and the
// PowerPoint export share.
func spans(s string) []span {
	var out []span
	for s != "" {
		switch {
		case s[0] == '`':
			if end := strings.IndexByte(s[1:], '`'); end > 0 {
				out = append(out, span{text: s[1 : 1+end], code: true})
				s = s[end+2:]
				continue
			}
		case strings.HasPrefix(s, "**"):
			if end := strings.Index(s[2:], "**"); end > 0 {
				out = append(out, span{text: s[2 : 2+end], bold: true})
				s = s[end+4:]
				continue
			}
		}
		// A literal run: at least one byte, so a marker that did not match is
		// written as text and the loop moves on.
		next := strings.IndexAny(s[1:], "`*")
		if next < 0 {
			next = len(s) - 1
		}
		out = append(out, span{text: s[:next+1]})
		s = s[next+1:]
	}
	return out
}

// editable marks an element as the place of a string in the slide, a path such
// as elements/1/rows/0/2, so the viewer can put an editing field over it. The
// path is built from field names and indexes, never from the deck's text.
func editable(path string) string { return ` data-edit="` + esc(path) + `"` }

// child is the path of an entry of the list or object at path.
func child(path string, steps ...any) string {
	for _, s := range steps {
		path += "/" + fmt.Sprint(s)
	}
	return path
}

// plain is text without its inline markers, which is what takes up room.
func plain(s string) string { return strings.NewReplacer("**", "", "`", "").Replace(s) }

// ---- measuring ----
//
// The estimates below describe the stylesheet (slides.css): a 1920 x 1080
// canvas, type sizes in canvas pixels, scaled together by the slide's k. They
// only have to be right enough to tell an agent that a slide is too full,
// because the agent cannot see the screen.

// advance is the width of one character as a fraction of the font size, as
// measured for Familjen Grotesk (the kit's sans) in Chrome; CJK glyphs come
// from a system font and are square.
func advance(r rune) float64 {
	switch {
	case r >= 0x2E80: // CJK, kana, hangul, full-width forms
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

// lines is how many lines the text takes in a box w pixels wide, honouring
// explicit line breaks. Wrapping at word boundaries wastes a little of each
// line, which the 1.07 stands for.
func lines(s string, w, px float64) float64 {
	n := 0.0
	for _, line := range strings.Split(plain(s), "\n") {
		n += math.Max(1, math.Ceil(textWidth(line, px)*1.07/math.Max(w, px)))
	}
	return n
}

// monoLines is lines for preformatted text, where every character of Martian
// Mono is 0.7 of the font size wide and breaks happen anywhere.
func monoLines(s string, w, px float64) float64 {
	per := math.Max(1, math.Floor(w/(0.7*px)))
	n := 0.0
	for _, line := range strings.Split(s, "\n") {
		n += math.Max(1, math.Ceil(float64(utf8.RuneCountInString(line))/per))
	}
	return n
}

// gap is the space between two elements of a slide or a column.
func gap(k float64) float64 { return 36 * k }

// stack is the height of elements laid out one under the other, w wide.
func stack(els []element, w, k, between float64) float64 {
	t := 0.0
	for i, e := range els {
		if i > 0 {
			t += between
		}
		t += e.height(w, k)
	}
	return t
}

// ---- text ----

type textEl struct{ text string }

func (e textEl) render(b *strings.Builder, at string) {
	b.WriteString(`<p class="el text"` + editable(child(at, "text")) + `>` + inline(e.text) + "</p>")
}

func (e textEl) height(w, k float64) float64 { return lines(e.text, w, 40*k) * 58 * k }

// ---- bullets and numbered ----

type listEl struct {
	ordered bool
	items   []string
}

func (e listEl) render(b *strings.Builder, at string) {
	tag, class := "ul", "el list"
	if e.ordered {
		tag, class = "ol", "el list numbered"
	}
	b.WriteString("<" + tag + ` class="` + class + `">`)
	for i, it := range e.items {
		b.WriteString("<li" + editable(child(at, "items", i)) + ">" + inline(it) + "</li>")
	}
	b.WriteString("</" + tag + ">")
}

func (e listEl) height(w, k float64) float64 {
	indent := 56.0
	t := 0.0
	for i, it := range e.items {
		if i > 0 {
			t += 14 * k
		}
		t += lines(it, w-indent*k, 40*k) * 56 * k
	}
	return t
}

// ---- code ----

type codeEl struct{ code, lang string }

func (e codeEl) render(b *strings.Builder, _ string) {
	b.WriteString(`<figure class="el code"><pre><code>` + esc(e.code) + "</code></pre>")
	if e.lang != "" {
		b.WriteString("<figcaption>" + esc(e.lang) + "</figcaption>")
	}
	b.WriteString("</figure>")
}

func (e codeEl) height(w, k float64) float64 {
	return monoLines(e.code, w-64*k, 28*k)*42*k + 56*k
}

// ---- table ----

type cell struct{ text, tone string }

type tableEl struct {
	columns []string
	rows    [][]cell
}

func (e tableEl) render(b *strings.Builder, at string) {
	b.WriteString(`<table class="el table"><thead><tr>`)
	for i, c := range e.columns {
		b.WriteString("<th" + editable(child(at, "columns", i)) + ">" + inline(c) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for r, row := range e.rows {
		b.WriteString("<tr>")
		for i, c := range row {
			if c.tone != "" {
				b.WriteString(`<td class="tone-` + esc(c.tone) + `"` + editable(child(at, "rows", r, i)) + `>`)
			} else {
				b.WriteString("<td" + editable(child(at, "rows", r, i)) + ">")
			}
			b.WriteString(inline(c.text) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
}

func (e tableEl) height(w, k float64) float64 {
	colW := w / float64(max(1, len(e.columns))) // padding is taken off below
	inner := colW - 48*k
	head := 1.0
	for _, c := range e.columns {
		head = math.Max(head, lines(c, inner, 30*k))
	}
	t := head*40*k + 30*k
	for _, row := range e.rows {
		n := 1.0
		for _, c := range row {
			n = math.Max(n, lines(c.text, inner, 32*k))
		}
		t += n*44*k + 28*k
	}
	return t
}

// ---- stats ----

type stat struct{ value, label string }

type statsEl struct{ items []stat }

func (e statsEl) render(b *strings.Builder, at string) {
	fmt.Fprintf(b, `<div class="el stats n%d">`, len(e.items))
	for i, s := range e.items {
		b.WriteString(`<div class="stat"><div class="value"` + editable(child(at, "items", i, "value")) + `>` + inline(s.value) +
			`</div><div class="label"` + editable(child(at, "items", i, "label")) + `>` + inline(s.label) + "</div></div>")
	}
	b.WriteString("</div>")
}

func (e statsEl) height(w, k float64) float64 {
	n := float64(max(1, len(e.items)))
	itemW := (w - (n-1)*32*k) / n
	h := 0.0
	for _, s := range e.items {
		v := lines(s.value, itemW-80*k, 112*k) * 112 * k
		h = math.Max(h, v+lines(s.label, itemW-80*k, 32*k)*44*k)
	}
	return h + 80*k + 12*k
}

// ---- quote ----

type quoteEl struct{ text, by string }

func (e quoteEl) render(b *strings.Builder, at string) {
	b.WriteString(`<blockquote class="el quote"><p` + editable(child(at, "text")) + `>` + inline(e.text) + "</p>")
	if e.by != "" {
		b.WriteString("<footer" + editable(child(at, "by")) + ">" + inline(e.by) + "</footer>")
	}
	b.WriteString("</blockquote>")
}

func (e quoteEl) height(w, k float64) float64 {
	h := lines(e.text, w-64*k, 56*k) * 74 * k
	if e.by != "" {
		h += lines(e.by, w-64*k, 34*k)*46*k + 20*k
	}
	return h
}

// ---- steps ----

type step struct{ title, text string }

type stepsEl struct{ items []step }

func (e stepsEl) render(b *strings.Builder, at string) {
	fmt.Fprintf(b, `<ol class="el steps n%d">`, len(e.items))
	for i, s := range e.items {
		b.WriteString(fmt.Sprintf(`<li><span class="num">%d</span><div class="stitle"`, i+1) + editable(child(at, "items", i, "title")) + `>` + inline(s.title) + "</div>")
		if s.text != "" {
			b.WriteString(`<div class="stext"` + editable(child(at, "items", i, "text")) + `>` + inline(s.text) + "</div>")
		}
		b.WriteString("</li>")
	}
	b.WriteString("</ol>")
}

func (e stepsEl) height(w, k float64) float64 {
	n := float64(max(1, len(e.items)))
	itemW := (w - (n-1)*28*k) / n
	h := 0.0
	for _, s := range e.items {
		t := lines(s.title, itemW, 36*k) * 46 * k
		if s.text != "" {
			t += lines(s.text, itemW, 30*k)*42*k + 8*k
		}
		h = math.Max(h, t)
	}
	return 64*k + 20*k + h
}

// ---- columns ----

type column struct {
	title string
	els   []element
}

type columnsEl struct{ cols []column }

func (e columnsEl) render(b *strings.Builder, at string) {
	fmt.Fprintf(b, `<div class="el columns n%d">`, len(e.cols))
	for i, c := range e.cols {
		b.WriteString(`<div class="col">`)
		if c.title != "" {
			b.WriteString("<h3" + editable(child(at, "columns", i, "title")) + ">" + inline(c.title) + "</h3>")
		}
		for j, el := range c.els {
			el.render(b, child(at, "columns", i, "elements", j))
		}
		b.WriteString("</div>")
	}
	b.WriteString("</div>")
}

func (e columnsEl) height(w, k float64) float64 {
	n := float64(max(1, len(e.cols)))
	colW := (w - (n-1)*56*k) / n
	h := 0.0
	for _, c := range e.cols {
		t := stack(c.els, colW, k*0.85, 28*k)
		if c.title != "" {
			t += lines(c.title, colW, 44*k)*54*k + 20*k
		}
		h = math.Max(h, t)
	}
	return h
}

// ---- cards ----

type card struct {
	title, text string
	items       []string
	highlight   bool
}

type cardsEl struct{ items []card }

func (e cardsEl) render(b *strings.Builder, at string) {
	fmt.Fprintf(b, `<div class="el cards n%d">`, len(e.items))
	for i, c := range e.items {
		class := "card"
		if c.highlight {
			class += " highlight"
		}
		b.WriteString(`<div class="` + class + `"><h3` + editable(child(at, "items", i, "title")) + `>` + inline(c.title) + "</h3>")
		if c.text != "" {
			b.WriteString("<p" + editable(child(at, "items", i, "text")) + ">" + inline(c.text) + "</p>")
		}
		if len(c.items) > 0 {
			b.WriteString("<ul>")
			for j, it := range c.items {
				b.WriteString("<li" + editable(child(at, "items", i, "items", j)) + ">" + inline(it) + "</li>")
			}
			b.WriteString("</ul>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("</div>")
}

func (e cardsEl) height(w, k float64) float64 {
	n := float64(max(1, len(e.items)))
	cardW := (w - (n-1)*32*k) / n
	inner := cardW - 72*k
	h := 0.0
	for _, c := range e.items {
		t := lines(c.title, inner, 40*k) * 50 * k
		if c.text != "" {
			t += 12*k + lines(c.text, inner, 32*k)*45*k
		}
		for _, it := range c.items {
			t += 10*k + lines(it, inner-36*k, 30*k)*42*k
		}
		h = math.Max(h, t)
	}
	return h + 72*k
}

// ---- progress ----

type bar struct {
	label, text string
	value       float64
}

type progressEl struct{ items []bar }

func (e progressEl) render(b *strings.Builder, at string) {
	b.WriteString(`<div class="el progress">`)
	for i, it := range e.items {
		text := it.text
		if text == "" {
			text = fmt.Sprintf("%g%%", it.value)
		}
		b.WriteString(`<div class="pbar"><div class="row"><span` + editable(child(at, "items", i, "label")) + `>` + inline(it.label) + "</span><span>" + inline(text) + `</span></div><div class="track"><div class="fill" style="width:` +
			fmt.Sprintf("%.1f", it.value) + `%"></div></div></div>`)
	}
	b.WriteString("</div>")
}

func (e progressEl) height(_, k float64) float64 {
	return float64(len(e.items))*(50+16+20)*k + float64(max(0, len(e.items)-1))*28*k
}
