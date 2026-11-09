package docfmt

import (
	_ "embed" // blocks.css
	"fmt"
	"html"
	"strings"
	"unicode"
)

//go:embed blocks.css
var blocksCSS string

// Stylesheet is the CSS the doc page needs besides the shared reading style:
// the pending placeholder, task lists, table alignment and tabs.
func Stylesheet() string { return blocksCSS }

// RenderPage draws the doc as a complete read-only page, with the shared
// reading style first and the format 2 additions after it.
func RenderPage(d Doc, shared string) string {
	body := RenderBody(d)
	var page strings.Builder
	page.WriteString("<!doctype html>\n<html lang=\"" + language(Title(d)+body) + "\">\n<head>\n<meta charset=\"utf-8\">\n")
	page.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	page.WriteString("<title>" + html.EscapeString(Title(d)) + "</title>\n<style>\n")
	page.WriteString(shared)
	page.WriteString(blocksCSS)
	page.WriteString("</style>\n</head>\n<body>\n<main class=\"doc\">\n")
	page.WriteString(body)
	page.WriteString("</main>\n</body>\n</html>\n")
	return page.String()
}

// RenderBody is the markup inside the page's main element. Every string of the
// doc goes through html escaping and the only attributes written are ones this
// package chooses, so no content can add markup, script or an event handler.
func RenderBody(d Doc) string {
	r := &renderer{slugs: map[string]int{}}
	if len(d.Tabs) == 1 {
		r.blocks(d.Tabs[0].Blocks)
		return r.b.String()
	}
	for _, t := range d.Tabs {
		r.w(`<section class="tab" data-tab="` + attr(t.ID) + `">` + "\n")
		r.w(`<p class="tab-title">` + html.EscapeString(t.Title) + "</p>\n")
		r.blocks(t.Blocks)
		r.w("</section>\n")
	}
	return r.b.String()
}

type renderer struct {
	b     strings.Builder
	slugs map[string]int
}

func (r *renderer) w(s string) { _, _ = r.b.WriteString(s) }

func attr(s string) string { return html.EscapeString(s) }

func (r *renderer) blocks(blocks []Block) {
	for _, b := range blocks {
		r.block(b)
		r.w("\n")
	}
}

func (r *renderer) block(b Block) {
	id := ` data-block="` + attr(b.ID) + `"`
	switch b.Type {
	case Paragraph:
		r.w("<p" + id + ">" + runs(b.Content) + "</p>")
	case Heading:
		level := min(max(b.Level, 1), 6)
		r.w(fmt.Sprintf(`<h%d id="%s"%s>%s</h%d>`, level, attr(r.slug(b.Content)), id, runs(b.Content), level))
	case BulletList:
		r.w("<ul" + id + ">" + items(BulletList, b.Items) + "</ul>")
	case NumberedList:
		start := ""
		if b.Start != 0 && b.Start != 1 {
			start = fmt.Sprintf(` start="%d"`, b.Start)
		}
		r.w("<ol" + start + id + ">" + items(NumberedList, b.Items) + "</ol>")
	case TaskList:
		r.w(`<ul class="task-list"` + id + ">" + items(TaskList, b.Items) + "</ul>")
	case Quote:
		r.w("<blockquote" + id + ">")
		r.blocks(b.Blocks)
		r.w("</blockquote>")
	case Code:
		class := ""
		if b.Lang != "" {
			class = ` class="language-` + attr(b.Lang) + `"`
		}
		r.w("<pre" + id + "><code" + class + ">" + html.EscapeString(b.Text) + "</code></pre>")
	case Rule:
		r.w("<hr" + id + ">")
	case Table:
		r.w(tableMarkup(b, id))
	case Image:
		r.w(figure(b, id))
	case Pending:
		r.w(`<div class="pending"` + id + `><span class="pending-intent">` + html.EscapeString(b.Intent) + "</span></div>")
	}
}

func figure(b Block, id string) string {
	if imageSource(b.Src) != nil {
		return `<p class="missing"` + id + ">" + html.EscapeString(b.Alt) + "</p>"
	}
	width := ""
	if px := ImageSizes[b.Size]; px > 0 {
		width = fmt.Sprintf(` width="%d"`, px)
	}
	return `<p class="image"` + id + `><img src="` + attr(b.Src) + `" alt="` + attr(b.Alt) + `"` + width + ` loading="lazy"></p>`
}

func tableMarkup(b Block, id string) string {
	var s strings.Builder
	s.WriteString("<table" + id + ">")
	rows := b.Rows
	if b.Header && len(rows) > 0 {
		s.WriteString("<thead>" + rowMarkup(rows[0], "th") + "</thead>")
		rows = rows[1:]
	}
	if len(rows) > 0 {
		s.WriteString("<tbody>")
		for _, row := range rows {
			s.WriteString(rowMarkup(row, "td"))
		}
		s.WriteString("</tbody>")
	}
	s.WriteString("</table>")
	return s.String()
}

func rowMarkup(row Row, tag string) string {
	var s strings.Builder
	s.WriteString("<tr>")
	for _, c := range row.Cells {
		class := ""
		if c.Align != "" {
			class = ` class="align-` + attr(c.Align) + `"`
		}
		s.WriteString("<" + tag + class + ">" + runs(c.Content) + "</" + tag + ">")
	}
	s.WriteString("</tr>")
	return s.String()
}

func listMarkup(typ string, list []Item) string {
	switch typ {
	case NumberedList:
		return "<ol>" + items(typ, list) + "</ol>"
	case TaskList:
		return `<ul class="task-list">` + items(typ, list) + "</ul>"
	}
	return "<ul>" + items(typ, list) + "</ul>"
}

func items(typ string, list []Item) string {
	var s strings.Builder
	for _, it := range list {
		s.WriteString("<li>")
		if typ == TaskList {
			s.WriteString(`<input type="checkbox" disabled`)
			if it.Checked != nil && *it.Checked {
				s.WriteString(" checked")
			}
			s.WriteString("> ")
		}
		s.WriteString(runs(it.Content))
		if len(it.Items) > 0 {
			s.WriteString(listMarkup(typ, it.Items))
		}
		s.WriteString("</li>")
	}
	return s.String()
}

// markOrder is the order marks nest in, outermost first, so the same run always
// gives the same markup.
var markTags = []struct{ mark, tag string }{
	{MarkBold, "strong"},
	{MarkItalic, "em"},
	{MarkUnderline, "u"},
	{MarkStrike, "s"},
	{MarkHighlight, "mark"},
	{MarkCode, "code"},
}

func runs(list []Inline) string {
	var s strings.Builder
	for _, in := range list {
		s.WriteString(runMarkup(in))
	}
	return s.String()
}

func runMarkup(in Inline) string {
	if in.Dropdown != nil {
		return `<span class="dropdown">` + html.EscapeString(in.Dropdown.Value) + `</span>`
	}
	if in.Date != "" {
		return `<time datetime="` + attr(in.Date) + `">` + html.EscapeString(in.Date) + `</time>`
	}
	if in.Mention() {
		return `<span class="mention">` + html.EscapeString(MentionText(in)) + `</span>`
	}
	out := strings.ReplaceAll(html.EscapeString(in.Text), "\n", "<br>")
	for i := len(markTags) - 1; i >= 0; i-- {
		if contains(in.Marks, markTags[i].mark) {
			out = "<" + markTags[i].tag + ">" + out + "</" + markTags[i].tag + ">"
		}
	}
	if contains(in.Marks, MarkLink) && linkTarget(in.Href) == nil {
		extra := ""
		if !strings.HasPrefix(in.Href, "#") {
			extra = ` target="_blank" rel="noopener noreferrer"`
		}
		out = `<a href="` + attr(in.Href) + `"` + extra + ">" + out + "</a>"
	}
	return out
}

// slug is the anchor of a heading: its text in lowercase with words joined by
// dashes, made unique within the page.
func (r *renderer) slug(content []Inline) string {
	var b strings.Builder
	dash := false
	for _, ch := range strings.ToLower(plainText(content)) {
		switch {
		case unicode.IsLetter(ch) || unicode.IsDigit(ch):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(ch)
		case unicode.IsSpace(ch) || ch == '-' || ch == '_':
			dash = true
		}
	}
	base := b.String()
	if base == "" {
		base = "heading"
	}
	n := r.slugs[base]
	r.slugs[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n)
}

func plainText(content []Inline) string {
	var b strings.Builder
	for _, in := range content {
		if in.Dropdown != nil {
			b.WriteString(in.Dropdown.Value)
			continue
		}
		if in.Date != "" {
			b.WriteString(in.Date)
			continue
		}
		if in.Mention() {
			b.WriteString(MentionText(in))
			continue
		}
		b.WriteString(in.Text)
	}
	return b.String()
}

// MentionText is how a mention reads as plain text: the name it had, after an @.
func MentionText(in Inline) string { return "@" + in.Name }

// PlainText is the text of runs with each mention as @name, with no marks.
func PlainText(runs []Inline) string { return plainText(runs) }

// Title is the text of the first heading, which is what the doc calls itself;
// a doc without one is "Document".
func Title(d Doc) string {
	for _, t := range d.Tabs {
		if h, ok := firstHeading(t.Blocks); ok {
			return h
		}
	}
	return "Document"
}

func firstHeading(blocks []Block) (string, bool) {
	for _, b := range blocks {
		if b.Type == Heading {
			if t := strings.TrimSpace(plainText(b.Content)); t != "" {
				return t, true
			}
		}
		if b.Type == Quote {
			if t, ok := firstHeading(b.Blocks); ok {
				return t, true
			}
		}
	}
	return "", false
}

// language lets the browser pick the right CJK glyph variants: any Han
// character makes the document Traditional Chinese, the house default.
func language(markup string) string {
	for _, ch := range markup {
		if unicode.Is(unicode.Han, ch) {
			return "zh-Hant"
		}
	}
	return "en"
}
