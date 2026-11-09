package docfmt

import (
	"fmt"
	"regexp"
	"strings"
)

// ToMarkdown writes the doc as Markdown, the inverse of FromMarkdown: what
// Markdown has no syntax for degrades to plain text, and a comment at the end
// names what was lost (underline and highlight, empty paragraphs, a table without
// a header or with mixed alignment in a column, a line break inside a table cell).
func ToMarkdown(d Doc) string {
	w := &mdWriter{lost: map[string]bool{}}
	var parts []string
	for _, t := range d.Tabs {
		if len(d.Tabs) > 1 || t.Title != DefaultTabTitle {
			parts = append(parts, "<!-- pictor:tab "+t.Title+" -->")
		}
		parts = append(parts, w.blocks(t.Blocks)...)
	}
	out := strings.Join(parts, "\n\n")
	if note := w.note(); note != "" {
		out += "\n\n" + note
	}
	return out + "\n"
}

type mdWriter struct{ lost map[string]bool }

// order of the notes, so the same doc always gives the same text.
var lostOrder = []string{"underline", "highlight", "empty paragraphs", "tables without a header row", "table columns with mixed alignment", "line breaks inside table cells", "line breaks inside code spans"}

func (w *mdWriter) note() string {
	var names []string
	for _, n := range lostOrder {
		if w.lost[n] {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "<!-- pictor: Markdown cannot hold " + strings.Join(names, ", ") + "; they are plain text or left out here -->"
}

func (w *mdWriter) blocks(blocks []Block) []string {
	var out []string
	for _, b := range blocks {
		if s, ok := w.block(b); ok {
			out = append(out, s)
		}
	}
	return out
}

func (w *mdWriter) block(b Block) (string, bool) {
	switch b.Type {
	case Paragraph:
		if len(b.Content) == 0 {
			w.lost["empty paragraphs"] = true
			return "", false
		}
		return w.inlines(b.Content, true, false), true
	case Heading:
		text := w.inlines(b.Content, false, false)
		if strings.HasSuffix(text, "#") && !strings.HasSuffix(text, `\#`) {
			text = text[:len(text)-1] + `\#`
		}
		return strings.TrimRight(strings.Repeat("#", b.Level)+" "+text, " "), true
	case BulletList, NumberedList, TaskList:
		return strings.Join(w.items(b.Type, b.Start, b.Items, 0), "\n"), true
	case Quote:
		inner := strings.Join(w.blocks(b.Blocks), "\n\n")
		lines := strings.Split(inner, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight("> "+l, " ")
		}
		return strings.Join(lines, "\n"), true
	case Code:
		return fenced(b), true
	case Rule:
		return "---", true
	case Table:
		return w.table(b), true
	case Image:
		title := ""
		if b.Size != "" {
			title = ` "` + b.Size + `"`
		}
		return "![" + escapeLabel(b.Alt) + "](" + destination(b.Src) + title + ")", true
	case Pending:
		return "<!-- pictor:pending " + b.Intent + " -->", true
	}
	return "", false
}

var backticks = regexp.MustCompile("`+")

func longestRun(s string) int {
	longest := 0
	for _, m := range backticks.FindAllString(s, -1) {
		longest = max(longest, len(m))
	}
	return longest
}

func fenced(b Block) string {
	fence := strings.Repeat("`", max(3, longestRun(b.Text)+1))
	return fence + b.Lang + "\n" + b.Text + "\n" + fence
}

// destination is a link or image address, in angle brackets when it holds
// characters that would end it.
func destination(u string) string {
	if strings.ContainsAny(u, "()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(u) + ">"
	}
	return u
}

func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(s)
}

func (w *mdWriter) items(typ string, start int, items []Item, depth int) []string {
	var out []string
	for i, it := range items {
		marker := "- "
		switch typ {
		case NumberedList:
			n := start
			if n == 0 {
				n = 1
			}
			marker = fmt.Sprintf("%d. ", n+i)
		case TaskList:
			marker = "- [ ] "
			if it.Checked != nil && *it.Checked {
				marker = "- [x] "
			}
		}
		pad := strings.Repeat(" ", markerWidth(typ, start, len(items)))
		text := w.inlines(it.Content, true, false)
		lines := strings.Split(text, "\n")
		line := marker + lines[0]
		for _, l := range lines[1:] {
			line += "\n" + pad + l
		}
		out = append(out, line)
		for _, nested := range w.items(typ, 1, it.Items, depth+1) {
			out = append(out, indent(nested, pad))
		}
	}
	return out
}

// markerWidth is how far the content of an item is indented, so what follows
// it belongs to the item: wide enough for the widest number of the list.
func markerWidth(typ string, start, count int) int {
	if typ != NumberedList {
		return 2
	}
	if start == 0 {
		start = 1
	}
	return len(fmt.Sprint(start+count-1)) + 2
}

func indent(block, pad string) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func (w *mdWriter) table(b Block) string {
	if len(b.Rows) == 0 {
		return ""
	}
	cols := len(b.Rows[0].Cells)
	aligns := make([]string, cols)
	for c := 0; c < cols; c++ {
		aligns[c] = b.Rows[0].Cells[c].Align
		for _, r := range b.Rows[1:] {
			if c < len(r.Cells) && r.Cells[c].Align != aligns[c] {
				w.lost["table columns with mixed alignment"] = true
			}
		}
	}
	var lines []string
	rows := b.Rows
	if b.Header {
		lines = append(lines, w.row(rows[0]))
		rows = rows[1:]
	} else {
		w.lost["tables without a header row"] = true
		lines = append(lines, w.row(Row{Cells: make([]Cell, cols)}))
	}
	delim := make([]string, cols)
	for c, a := range aligns {
		delim[c] = map[string]string{"left": ":--", "center": ":-:", "right": "--:"}[a]
		if delim[c] == "" {
			delim[c] = "---"
		}
	}
	lines = append(lines, "| "+strings.Join(delim, " | ")+" |")
	for _, r := range rows {
		lines = append(lines, w.row(r))
	}
	return strings.Join(lines, "\n")
}

func (w *mdWriter) row(r Row) string {
	cells := make([]string, len(r.Cells))
	for i, c := range r.Cells {
		text := w.inlines(c.Content, false, true)
		if strings.Contains(text, "\n") {
			w.lost["line breaks inside table cells"] = true
			text = strings.ReplaceAll(text, "\\\n", " ")
			text = strings.ReplaceAll(text, "\n", " ")
		}
		cells[i] = text
	}
	return strings.TrimRight("| "+strings.Join(cells, " | ")+" |", " ")
}

// inlines writes runs as Markdown. start says the text begins a block, where a
// leading #, -, + or number would start a different block. cell escapes the |
// that ends a table cell.
func (w *mdWriter) inlines(runs []Inline, start, cell bool) string {
	var b strings.Builder
	for i, r := range runs {
		text := w.run(r, start && i == 0, cell)
		// A ! right before a link would make it a picture.
		if i+1 < len(runs) && (contains(runs[i+1].Marks, MarkLink) || runs[i+1].Mention()) && !contains(r.Marks, MarkLink) && strings.HasSuffix(text, "!") && !strings.HasSuffix(text, `\!`) {
			text = text[:len(text)-1] + `\!`
		}
		b.WriteString(text)
	}
	return b.String()
}

var (
	bareURL  = regexp.MustCompile(`(?i)\b(https?|ftp)://`)
	bareWWW  = regexp.MustCompile(`(?i)\b(www)\.`)
	digitEnd = regexp.MustCompile(`^(\s*\d+)([.)])`)
)

// escapeText makes text come back as the same text: every character Markdown
// would read as syntax is escaped, and the two things GitHub Markdown turns
// into links by itself (an address and www.) are broken so they stay text.
func escapeText(s string, start, cell bool) string {
	s = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "~", `\~`, "&", `\&`).Replace(s)
	if cell {
		s = strings.ReplaceAll(s, "|", `\|`)
	}
	s = bareURL.ReplaceAllString(s, `$1\://`)
	s = bareWWW.ReplaceAllString(s, `$1\.`)
	if start {
		s = escapeStart(s)
	}
	return s
}

// escapeStart escapes what would make the start of a paragraph a heading, a
// list, a rule or a quote.
func escapeStart(s string) string {
	t := strings.TrimLeft(s, " ")
	pad := s[:len(s)-len(t)]
	switch {
	case strings.HasPrefix(t, "#"), strings.HasPrefix(t, "+"), strings.HasPrefix(t, "-"), strings.HasPrefix(t, "="):
		return pad + `\` + t
	case digitEnd.MatchString(t):
		return pad + digitEnd.ReplaceAllString(t, `$1\$2`)
	}
	return s
}

func (w *mdWriter) run(r Inline, start, cell bool) string {
	if r.Dropdown != nil {
		return escapeText(r.Dropdown.Value, start, cell)
	}
	if r.Date != "" {
		return escapeText(r.Date, start, cell)
	}
	if r.Mention() {
		return "[" + escapeText(MentionText(r), false, cell) + "](" + MentionScheme + r.User + ")"
	}
	text := r.Text
	linked := contains(r.Marks, MarkLink)
	coded := contains(r.Marks, MarkCode)
	if contains(r.Marks, MarkUnderline) {
		w.lost["underline"] = true
	}
	if contains(r.Marks, MarkHighlight) {
		w.lost["highlight"] = true
	}
	lead, core, trail := splitSpace(text)
	if core == "" {
		return w.lines(escapeText(text, start, cell))
	}
	var body string
	if coded {
		body = codeSpan(core)
		if strings.Contains(core, "\n") {
			w.lost["line breaks inside code spans"] = true
			body = codeSpan(strings.ReplaceAll(core, "\n", " "))
		}
		if cell {
			body = strings.ReplaceAll(body, "|", `\|`)
		}
	} else {
		body = w.lines(escapeText(core, start && lead == "", cell))
	}
	if contains(r.Marks, MarkStrike) {
		body = "~~" + body + "~~"
	}
	if contains(r.Marks, MarkItalic) {
		body = "*" + body + "*"
	}
	if contains(r.Marks, MarkBold) {
		body = "**" + body + "**"
	}
	if linked {
		body = "[" + body + "](" + destination(r.Href) + ")"
	}
	return w.lines(escapeText(lead, false, cell)) + body + w.lines(escapeText(trail, false, cell))
}

// lines turns a line break of the text into a Markdown hard break.
func (w *mdWriter) lines(s string) string {
	return strings.ReplaceAll(s, "\n", "\\\n")
}

// splitSpace separates the spaces and line breaks at the ends of text, which
// cannot sit inside emphasis markers.
func splitSpace(s string) (lead, core, trail string) {
	core = strings.TrimLeft(s, " \t\n")
	lead = s[:len(s)-len(core)]
	t := strings.TrimRight(core, " \t\n")
	trail = core[len(t):]
	return lead, t, trail
}

func codeSpan(s string) string {
	fence := strings.Repeat("`", longestRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || (strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.TrimSpace(s) != "") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

// AddressFiles writes each picture uploaded to the doc (a /files/ address) by
// its full address under origin, for a copy of the doc that leaves the server:
// the relative address means nothing anywhere else. It changes d, so it is for
// a doc just parsed. A doc accepts only https: addresses for pictures, so an
// origin that is not https (a development server) leaves the addresses alone
// rather than write ones that cannot be published again.
func AddressFiles(d *Doc, origin string) {
	if !strings.HasPrefix(origin, "https://") {
		return
	}
	var walk func(blocks []Block)
	walk = func(blocks []Block) {
		for i := range blocks {
			b := &blocks[i]
			if b.Type == Image && strings.HasPrefix(b.Src, "/files/") {
				b.Src = strings.TrimSuffix(origin, "/") + b.Src
			}
			walk(b.Blocks)
		}
	}
	for i := range d.Tabs {
		walk(d.Tabs[i].Blocks)
	}
}
