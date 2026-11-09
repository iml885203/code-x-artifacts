package docx

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

type rel struct {
	id, typ, target string
	external        bool
}

type picture struct {
	path string
	data []byte
}

// numbering instances: a numbered list is its own so that it counts from its
// own start; bullets and to-do lists share one.
const (
	numBullet = 1
	numTask   = 2
	numFirst  = 3 // the first numbered list
)

type writer struct {
	images   Images
	rels     []rel
	pictures []picture
	media    map[string]int
	starts   []int // start of each numbered list, in order of numFirst
	drawings int
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s)) // a strings.Builder does not fail
	return b.String()
}

func (w *writer) body(d docfmt.Doc) string {
	var sb strings.Builder
	for i, t := range d.Tabs {
		if len(d.Tabs) > 1 {
			// Each tab starts a page, under its title.
			pageBreak := ""
			if i > 0 {
				pageBreak = `<w:pageBreakBefore/>`
			}
			sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/>` + pageBreak + `</w:pPr>` + plainRun(t.Title) + `</w:p>`)
		}
		w.blocks(&sb, t.Blocks, 0)
	}
	if sb.Len() == 0 {
		sb.WriteString(`<w:p/>`)
	}
	return sb.String()
}

func plainRun(s string) string { return `<w:r>` + text(s) + `</w:r>` }

// text is the content of a run: a newline is a break, a tab a tab.
func text(s string) string {
	var sb strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			sb.WriteString(`<w:br/>`)
		}
		for j, part := range strings.Split(line, "\t") {
			if j > 0 {
				sb.WriteString(`<w:tab/>`)
			}
			if part != "" {
				sb.WriteString(`<w:t xml:space="preserve">` + esc(part) + `</w:t>`)
			}
		}
	}
	return sb.String()
}

// blocks writes blocks; quote is how many quotes deep they are.
func (w *writer) blocks(sb *strings.Builder, blocks []docfmt.Block, quote int) {
	for _, b := range blocks {
		w.block(sb, b, quote)
	}
}

func para(style, props, content string) string {
	pPr := ""
	if style != "" {
		pPr = `<w:pStyle w:val="` + style + `"/>`
	}
	pPr += props
	if pPr != "" {
		pPr = `<w:pPr>` + pPr + `</w:pPr>`
	}
	return `<w:p>` + pPr + content + `</w:p>`
}

// quoteIndent is the extra left indent of what sits inside quotes.
func quoteIndent(quote int) string {
	if quote == 0 {
		return ""
	}
	return fmt.Sprintf(`<w:ind w:left="%d"/>`, 567*quote)
}

func (w *writer) block(sb *strings.Builder, b docfmt.Block, quote int) {
	style := ""
	if quote > 0 {
		style = "Quote"
	}
	switch b.Type {
	case docfmt.Paragraph:
		sb.WriteString(para(style, quoteIndent(quote), w.runs(b.Content, false)))
	case docfmt.Heading:
		sb.WriteString(para(fmt.Sprintf("Heading%d", b.Level), quoteIndent(quote), w.runs(b.Content, false)))
	case docfmt.BulletList:
		w.items(sb, b.Items, 0, numBullet, quote, "")
	case docfmt.TaskList:
		w.items(sb, b.Items, 0, numTask, quote, "task")
	case docfmt.NumberedList:
		start := b.Start
		if start == 0 {
			start = 1
		}
		w.starts = append(w.starts, start)
		w.items(sb, b.Items, 0, numFirst+len(w.starts)-1, quote, "")
	case docfmt.Quote:
		w.blocks(sb, b.Blocks, quote+1)
	case docfmt.Code:
		for _, line := range strings.Split(strings.TrimSuffix(b.Text, "\n"), "\n") {
			sb.WriteString(para("CodeBlock", quoteIndent(quote), `<w:r>`+text(line)+`</w:r>`))
		}
	case docfmt.Rule:
		sb.WriteString(para("", `<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="999999"/></w:pBdr>`, ""))
	case docfmt.Table:
		w.table(sb, b)
	case docfmt.Image:
		sb.WriteString(para(style, quoteIndent(quote), w.image(b)))
	case docfmt.Pending:
		sb.WriteString(para("Placeholder", quoteIndent(quote), plainRun("To be written: "+b.Intent)))
	}
}

// items writes list items, nested ones one level deeper.
func (w *writer) items(sb *strings.Builder, items []docfmt.Item, level, numID, quote int, kind string) {
	for _, it := range items {
		content := w.runs(it.Content, false)
		if kind == "task" {
			box := "☐ "
			if it.Checked != nil && *it.Checked {
				box = "☑ "
			}
			content = plainRun(box) + content
		}
		left := 720*(level+1) + 567*quote
		props := fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr><w:spacing w:after="40"/><w:ind w:left="%d" w:hanging="360"/>`, level, numID, left)
		sb.WriteString(para("", props, content))
		w.items(sb, it.Items, level+1, numID, quote, kind)
	}
}

func (w *writer) table(sb *strings.Builder, b docfmt.Block) {
	cols := len(b.Rows[0].Cells)
	width := 9026 / cols
	sb.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9026" w:type="dxa"/><w:tblBorders>`)
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		sb.WriteString(`<w:` + edge + ` w:val="single" w:sz="4" w:space="0" w:color="AAAAAA"/>`)
	}
	sb.WriteString(`</w:tblBorders><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for range cols {
		fmt.Fprintf(sb, `<w:gridCol w:w="%d"/>`, width)
	}
	sb.WriteString(`</w:tblGrid>`)
	for i, row := range b.Rows {
		header := b.Header && i == 0
		sb.WriteString(`<w:tr>`)
		if header {
			sb.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for _, c := range row.Cells {
			props := ""
			if c.Align != "" {
				props = `<w:jc w:val="` + c.Align + `"/>`
			}
			fmt.Fprintf(sb, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, width)
			if header {
				sb.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F0F0F0"/>`)
			}
			sb.WriteString(`</w:tcPr>` + para("", `<w:spacing w:after="0"/>`+props, w.runs(c.Content, header)) + `</w:tc>`)
		}
		sb.WriteString(`</w:tr>`)
	}
	// Word wants a paragraph after a table.
	sb.WriteString(`</w:tbl><w:p/>`)
}

// runs writes inline content; bold forces every run bold (a table header).
func (w *writer) runs(content []docfmt.Inline, bold bool) string {
	var sb strings.Builder
	for _, in := range content {
		run := w.run(in, bold)
		if href, ok := w.external(in); ok {
			id := w.relate("http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", href, true)
			sb.WriteString(`<w:hyperlink r:id="` + id + `" w:history="1">` + run + `</w:hyperlink>`)
			continue
		}
		sb.WriteString(run)
	}
	return sb.String()
}

// external is the address of a link that leaves the file; a link to a place in
// the doc (#anchor) has nothing to point at in the file and stays plain text.
func (w *writer) external(in docfmt.Inline) (string, bool) {
	if in.Href == "" || strings.HasPrefix(in.Href, "#") {
		return "", false
	}
	return in.Href, true
}

func (w *writer) run(in docfmt.Inline, bold bool) string {
	in = visibleInline(in)
	has := map[string]bool{}
	for _, m := range in.Marks {
		has[m] = true
	}
	var rPr strings.Builder
	switch {
	case has[docfmt.MarkLink] && in.Href != "" && !strings.HasPrefix(in.Href, "#"):
		rPr.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	case has[docfmt.MarkCode]:
		rPr.WriteString(`<w:rStyle w:val="InlineCode"/>`)
	}
	if has[docfmt.MarkBold] || bold {
		rPr.WriteString(`<w:b/>`)
	}
	if has[docfmt.MarkItalic] {
		rPr.WriteString(`<w:i/>`)
	}
	if has[docfmt.MarkStrike] {
		rPr.WriteString(`<w:strike/>`)
	}
	if has[docfmt.MarkHighlight] {
		rPr.WriteString(`<w:highlight w:val="yellow"/>`)
	}
	if has[docfmt.MarkUnderline] {
		rPr.WriteString(`<w:u w:val="single"/>`)
	}
	pr := ""
	if rPr.Len() > 0 {
		pr = `<w:rPr>` + rPr.String() + `</w:rPr>`
	}
	return `<w:r>` + pr + text(in.Text) + `</w:r>`
}

func visibleInline(in docfmt.Inline) docfmt.Inline {
	if in.Dropdown != nil {
		in = docfmt.Inline{Text: in.Dropdown.Value}
	}
	if in.Date != "" {
		in = docfmt.Inline{Text: in.Date}
	}
	if in.Mention() {
		in = docfmt.Inline{Text: docfmt.MentionText(in)}
	}
	return in
}

// relate adds a relationship of the document and returns its id.
func (w *writer) relate(typ, target string, external bool) string {
	for _, r := range w.rels {
		if r.typ == typ && r.target == target {
			return r.id
		}
	}
	id := fmt.Sprintf("rId%d", len(w.rels)+3) // 1 and 2 are the styles and the numbering
	w.rels = append(w.rels, rel{id: id, typ: typ, target: target, external: external})
	return id
}

func (w *writer) relsXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>
`)
	for _, r := range w.rels {
		mode := ""
		if r.external {
			mode = ` TargetMode="External"`
		}
		sb.WriteString(`<Relationship Id="` + r.id + `" Type="` + r.typ + `" Target="` + esc(r.target) + `"` + mode + `/>` + "\n")
	}
	sb.WriteString(`</Relationships>`)
	return sb.String()
}

func (w *writer) contentTypes() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Default Extension="png" ContentType="image/png"/>
<Default Extension="jpeg" ContentType="image/jpeg"/>
<Default Extension="gif" ContentType="image/gif"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`)
	return sb.String()
}

// numberingXML defines the bullets, the to-do lists and one numbered list per
// numbered list of the body.
func (w *writer) numberingXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	levels := func(format, textFor func(l int) string) {
		for l := range 9 {
			fmt.Fprintf(&sb, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="%s"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`, l, format(l), textFor(l), 720*(l+1))
		}
	}
	sb.WriteString(`<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>`)
	levels(func(int) string { return "bullet" }, func(l int) string { return []string{"•", "◦", "▪"}[l%3] })
	sb.WriteString(`</w:abstractNum><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="hybridMultilevel"/>`)
	levels(func(l int) string { return []string{"decimal", "lowerLetter", "lowerRoman"}[l%3] }, func(l int) string { return fmt.Sprintf("%%%d.", l+1) })
	sb.WriteString(`</w:abstractNum><w:abstractNum w:abstractNumId="2"><w:multiLevelType w:val="hybridMultilevel"/>`)
	levels(func(int) string { return "none" }, func(int) string { return "" })
	sb.WriteString(`</w:abstractNum>`)
	fmt.Fprintf(&sb, `<w:num w:numId="%d"><w:abstractNumId w:val="0"/></w:num><w:num w:numId="%d"><w:abstractNumId w:val="2"/></w:num>`, numBullet, numTask)
	for i, start := range w.starts {
		fmt.Fprintf(&sb, `<w:num w:numId="%d"><w:abstractNumId w:val="1"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="%d"/></w:lvlOverride></w:num>`, numFirst+i, start)
	}
	sb.WriteString(`</w:numbering>`)
	return sb.String()
}
