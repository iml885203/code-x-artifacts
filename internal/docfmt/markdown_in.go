package docfmt

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

var (
	pendingLine = regexp.MustCompile(`^\s*<!--\s*pictor:pending\s+(.*?)\s*-->\s*$`)
	tabLine     = regexp.MustCompile(`^\s*<!--\s*pictor:tab\s+(.*?)\s*-->\s*$`)
)

// FromMarkdown converts the Markdown a publisher wrote into a doc: CommonMark
// with the GitHub extras, plus two comment lines of its own, <!-- pictor:pending
// intent --> for a placeholder and <!-- pictor:tab Title --> to start a tab.
// Any other raw HTML is left out, as it is in a format 1 page. A link or an image
// whose address breaks the rules of Validate refuses the whole conversion and
// the message names the line. previous, when it is not nil, is the doc being
// replaced: blocks whose content did not change keep their ids (see KeepIDs).
func FromMarkdown(src string, previous *Doc, newID func(prefix string) string) (Doc, error) {
	source := []byte(src)
	root := markdown.Parser().Parse(text.NewReader(source))
	c := &converter{src: source}
	tabs, err := c.tabs(root)
	if err != nil {
		return Doc{}, err
	}
	doc := Doc{Tabs: tabs}
	used := map[string]bool{}
	if previous != nil {
		collectIDs(*previous, used)
	}
	assignIDs(&doc, newID, used)
	if previous != nil {
		KeepIDs(&doc, *previous)
	}
	return doc, nil
}

// converter turns a Markdown syntax tree into blocks. legacy is the conversion
// of a document written before blocks existed: its comment lines are raw HTML
// like any other, and a link or an image that breaks the rules is left out
// (the text of a link stays) instead of refusing, since nobody can fix it now.
type converter struct {
	src    []byte
	legacy bool
}

// refuse is an error that names the line of the node.
func (c *converter) refuse(n ast.Node, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", c.line(n), fmt.Sprintf(format, args...))
}

// line is the 1-based line a node starts on, from its text or its nearest
// ancestor that has lines.
func (c *converter) line(n ast.Node) int {
	start := -1
	_ = ast.Walk(n, func(m ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := m.(*ast.Text); ok && entering {
			start = t.Segment.Start
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	for p := n; start < 0 && p != nil; p = p.Parent() {
		if p.Type() == ast.TypeBlock && p.Lines().Len() > 0 {
			start = p.Lines().At(0).Start
		}
	}
	if start < 0 {
		return 1
	}
	return 1 + bytes.Count(c.src[:min(start, len(c.src))], []byte("\n"))
}

// tabs splits the top level at the tab marker lines.
func (c *converter) tabs(root ast.Node) ([]Tab, error) {
	tabs := []Tab{{Title: DefaultTabTitle, Blocks: []Block{}}}
	started := false
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if title, ok := c.marker(n, tabLine); ok {
			if started || len(tabs[0].Blocks) > 0 {
				tabs = append(tabs, Tab{Blocks: []Block{}})
			}
			started = true
			tabs[len(tabs)-1].Title = title
			continue
		}
		blocks, err := c.block(n, false)
		if err != nil {
			return nil, err
		}
		last := &tabs[len(tabs)-1]
		last.Blocks = append(last.Blocks, blocks...)
	}
	return tabs, nil
}

// marker reports whether n is an HTML comment block that matches re, with the
// text it captured.
func (c *converter) marker(n ast.Node, re *regexp.Regexp) (string, bool) {
	h, ok := n.(*ast.HTMLBlock)
	if !ok || c.legacy {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < h.Lines().Len(); i++ {
		seg := h.Lines().At(i)
		b.Write(seg.Value(c.src))
	}
	if h.HasClosure() {
		b.Write(h.ClosureLine.Value(c.src))
	}
	m := re.FindStringSubmatch(strings.TrimRight(b.String(), "\r\n"))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// block converts one block node; inQuote says it sits inside a quote.
func (c *converter) block(n ast.Node, inQuote bool) ([]Block, error) {
	switch n := n.(type) {
	case *ast.Heading:
		runs, err := c.inlines(n, false)
		return []Block{{Type: Heading, Level: n.Level, Content: runs}}, err
	case *ast.Paragraph:
		return c.paragraph(n)
	case *ast.TextBlock:
		return c.paragraph(n)
	case *ast.ThematicBreak:
		return []Block{{Type: Rule}}, nil
	case *ast.FencedCodeBlock:
		return []Block{{Type: Code, Lang: codeLang(n, c.src), Text: codeText(n, c.src)}}, nil
	case *ast.CodeBlock:
		return []Block{{Type: Code, Text: codeText(n, c.src)}}, nil
	case *ast.Blockquote:
		return c.quote(n)
	case *ast.List:
		list, err := c.list(n)
		return []Block{list}, err
	case *east.Table:
		t, err := c.table(n)
		return []Block{t}, err
	case *ast.HTMLBlock:
		return c.htmlBlock(n, inQuote)
	}
	return nil, nil
}

func (c *converter) htmlBlock(n *ast.HTMLBlock, inQuote bool) ([]Block, error) {
	intent, ok := c.marker(n, pendingLine)
	switch {
	case !ok:
		return nil, nil
	case inQuote:
		return nil, c.refuse(n, "a pending placeholder cannot be inside a quote")
	case strings.TrimSpace(intent) == "":
		return nil, c.refuse(n, "<!-- pictor:pending --> needs the intent after it: what this part will hold")
	}
	return []Block{{Type: Pending, Intent: intent}}, nil
}

func codeLang(n *ast.FencedCodeBlock, src []byte) string {
	info := strings.Fields(string(n.Language(src)))
	if len(info) == 0 {
		return ""
	}
	return info[0]
}

// codeText is the source of a code block without the line break that ends its
// last line, which the fence supplies.
func codeText(n ast.Node, src []byte) string {
	var b bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		b.Write(seg.Value(src))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// paragraph is a paragraph, or the paragraphs and image blocks it splits into
// when it holds pictures.
func (c *converter) paragraph(n ast.Node) ([]Block, error) {
	var out []Block
	var runs []Inline
	var walkErr error
	flush := func() {
		if len(runs) > 0 {
			out = append(out, Block{Type: Paragraph, Content: mergeRuns(runs)})
			runs = nil
		}
	}
	err := c.walkInlines(n, nil, "", func(in Inline) { runs = append(runs, in) }, func(img *ast.Image) {
		if c.leftOut(img) {
			return
		}
		flush()
		if err := c.checkImage(img); err != nil {
			walkErr = err
			return
		}
		out = append(out, Block{Type: Image, Src: string(util.URLEscape(img.Destination, true)), Alt: c.plain(img), Size: sizeFromTitle(img.Title)})
	})
	if err == nil {
		err = walkErr
	}
	flush()
	return out, err
}

// leftOut says a legacy document's picture is dropped: its address breaks the
// rules.
func (c *converter) leftOut(img *ast.Image) bool {
	return c.legacy && imageSource(string(util.URLEscape(img.Destination, true))) != nil
}

func (c *converter) checkImage(img *ast.Image) error {
	src := string(util.URLEscape(img.Destination, true))
	if err := imageSource(src); err != nil {
		return c.refuse(img, "image %q: %s", src, err)
	}
	return nil
}

// plain is the text inside a node, with no marks.
func (c *converter) plain(n ast.Node) string {
	var runs []Inline
	_ = c.walkInlines(n, nil, "", func(in Inline) { runs = append(runs, in) }, nil)
	return plainText(runs)
}

// inlines is the text of a heading, an item or a cell. Pictures in it become
// their description, because those places cannot hold a picture block.
func (c *converter) inlines(n ast.Node, _ bool) ([]Inline, error) {
	var runs []Inline
	var firstErr error
	err := c.walkInlines(n, nil, "", func(in Inline) { runs = append(runs, in) }, func(img *ast.Image) {
		if c.leftOut(img) {
			return
		}
		if err := c.checkImage(img); err != nil && firstErr == nil {
			firstErr = err
		}
		if alt := c.plain(img); alt != "" {
			runs = append(runs, Inline{Text: alt})
		}
	})
	if err == nil {
		err = firstErr
	}
	return mergeRuns(runs), err
}

// walkInlines emits the runs of the inline children of n. marks and href are
// what the enclosing nodes contribute. An image is handed to image, or ignored
// when image is nil.
func (c *converter) walkInlines(n ast.Node, marks []string, href string, emit func(Inline), image func(*ast.Image)) error {
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if err := c.inline(ch, marks, href, emit, image); err != nil {
			return err
		}
	}
	return nil
}

func with(marks []string, m string) []string {
	return append(append([]string{}, marks...), m)
}

func (c *converter) inline(n ast.Node, marks []string, href string, emit func(Inline), image func(*ast.Image)) error {
	switch n := n.(type) {
	case *ast.Text:
		emit(run(c.textOf(n), marks, href))
		switch {
		case n.HardLineBreak():
			emit(run("\n", marks, href))
		case n.SoftLineBreak():
			emit(run(" ", marks, href))
		}
	case *ast.String:
		emit(run(string(n.Value), marks, href))
	case *ast.CodeSpan:
		emit(run(strings.ReplaceAll(c.codeSpan(n), "\n", " "), with(marks, MarkCode), href))
	case *ast.Emphasis:
		m := MarkItalic
		if n.Level >= 2 {
			m = MarkBold
		}
		return c.walkInlines(n, with(marks, m), href, emit, image)
	case *east.Strikethrough:
		return c.walkInlines(n, with(marks, MarkStrike), href, emit, image)
	case *ast.Link:
		return c.link(n, marks, string(util.URLEscape(n.Destination, true)), emit, image)
	case *ast.AutoLink:
		return c.autoLink(n, marks, emit)
	case *ast.Image:
		if image != nil {
			image(n)
		}
	case *east.TaskCheckBox:
	}
	return nil
}

func (c *converter) autoLink(n *ast.AutoLink, marks []string, emit func(Inline)) error {
	label := string(n.Label(c.src))
	href := string(n.URL(c.src))
	switch {
	case n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(href), "mailto:"):
		href = "mailto:" + href
	case strings.HasPrefix(strings.ToLower(href), "www."):
		href = "http://" + href
	}
	if err := linkTarget(href); err != nil {
		if c.legacy {
			emit(run(label, marks, ""))
			return nil
		}
		return c.refuse(n, "link %q: %s", href, err)
	}
	emit(run(label, with(marks, MarkLink), href))
	return nil
}

func (c *converter) codeSpan(n *ast.CodeSpan) string {
	var b bytes.Buffer
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if t, ok := ch.(*ast.Text); ok {
			b.Write(t.Segment.Value(c.src))
		}
	}
	return b.String()
}

// textOf is the text of a Text node with backslash escapes and entities
// resolved, as the HTML renderer shows it.
func (c *converter) textOf(n *ast.Text) string {
	var buf bytes.Buffer
	w := ghtml.DefaultWriter
	bw := &bufWriter{Buffer: &buf}
	w.Write(bw, n.Segment.Value(c.src))
	return html.UnescapeString(buf.String())
}

// bufWriter adapts a bytes.Buffer to util.BufWriter.
type bufWriter struct{ *bytes.Buffer }

func (b *bufWriter) Available() int { return 1 << 30 }
func (b *bufWriter) Buffered() int  { return b.Len() }
func (b *bufWriter) Flush() error   { return nil }

// run is one run of text; the marks are put in the fixed order.
func run(text string, marks []string, href string) Inline {
	in := Inline{Text: text, Href: href}
	for _, m := range []string{MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkHighlight, MarkCode, MarkLink} {
		if contains(marks, m) {
			in.Marks = append(in.Marks, m)
		}
	}
	return in
}

// mergeRuns joins neighbouring runs that carry the same marks and address and
// drops empty ones.
func mergeRuns(runs []Inline) []Inline {
	var out []Inline
	for _, r := range runs {
		if r.Text == "" && !r.Mention() {
			continue
		}
		if n := len(out); n > 0 && !r.Mention() && !out[n-1].Mention() && out[n-1].Href == r.Href && equalStrings(out[n-1].Marks, r.Marks) {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *converter) quote(n *ast.Blockquote) ([]Block, error) {
	var kids []Block
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		blocks, err := c.block(ch, true)
		if err != nil {
			return nil, err
		}
		kids = append(kids, blocks...)
	}
	if len(kids) == 0 {
		return nil, nil
	}
	return []Block{{Type: Quote, Blocks: kids}}, nil
}

// list converts a list; it is a task list when any item has a checkbox.
func (c *converter) list(n *ast.List) (Block, error) {
	typ := BulletList
	if n.IsOrdered() {
		typ = NumberedList
	}
	if hasCheckbox(n) {
		typ = TaskList
	}
	items, err := c.items(n, typ)
	b := Block{Type: typ, Items: items}
	if typ == NumberedList && n.Start != 1 {
		b.Start = n.Start
	}
	return b, err
}

func hasCheckbox(n *ast.List) bool {
	for it := n.FirstChild(); it != nil; it = it.NextSibling() {
		if first := it.FirstChild(); first != nil {
			if c := first.FirstChild(); c != nil {
				if _, ok := c.(*east.TaskCheckBox); ok {
					return true
				}
			}
		}
	}
	return false
}

func (c *converter) items(n *ast.List, typ string) ([]Item, error) {
	var items []Item
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		it, err := c.item(li, typ)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// item is one list item. Its first paragraph is the item's text; further
// paragraphs follow it after a line break, a nested list becomes the item's own
// items, and any other block is shown as its text, because an item holds text.
func (c *converter) item(li ast.Node, typ string) (Item, error) {
	var it Item
	if typ == TaskList {
		unchecked := false
		it.Checked = &unchecked
	}
	var runs []Inline
	for ch := li.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch ch := ch.(type) {
		case *ast.List:
			nested, err := c.items(ch, typ)
			if err != nil {
				return it, err
			}
			it.Items = append(it.Items, nested...)
		default:
			if checkbox, ok := firstCheckbox(ch); ok {
				checked := checkbox.IsChecked
				it.Checked = &checked
			}
			part, err := c.itemText(ch)
			if err != nil {
				return it, err
			}
			if len(runs) > 0 && len(part) > 0 {
				runs = append(runs, Inline{Text: "\n"})
			}
			runs = append(runs, part...)
		}
	}
	it.Content = mergeRuns(runs)
	return it, nil
}

func firstCheckbox(n ast.Node) (*east.TaskCheckBox, bool) {
	if f := n.FirstChild(); f != nil {
		cb, ok := f.(*east.TaskCheckBox)
		return cb, ok
	}
	return nil, false
}

func (c *converter) table(n *east.Table) (Block, error) {
	b := Block{Type: Table, Header: true}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []Cell
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			runs, err := c.inlines(cell, false)
			if err != nil {
				return b, err
			}
			cells = append(cells, Cell{Align: alignName(cell), Content: runs})
		}
		b.Rows = append(b.Rows, Row{Cells: cells})
	}
	return b, nil
}

func alignName(cell ast.Node) string {
	tc, ok := cell.(*east.TableCell)
	if !ok {
		return ""
	}
	switch tc.Alignment {
	case east.AlignLeft:
		return "left"
	case east.AlignCenter:
		return "center"
	case east.AlignRight:
		return "right"
	case east.AlignNone:
	}
	return ""
}

// assignIDs gives every tab and block an id that is not in used.
func assignIDs(d *Doc, newID func(prefix string) string, used map[string]bool) {
	fresh := func(prefix string) string {
		for {
			if id := newID(prefix); !used[id] {
				used[id] = true
				return id
			}
		}
	}
	for i := range d.Tabs {
		d.Tabs[i].ID = fresh("t_")
		assignBlockIDs(d.Tabs[i].Blocks, fresh)
	}
}

func assignBlockIDs(blocks []Block, fresh func(prefix string) string) {
	for i := range blocks {
		blocks[i].ID = fresh("b_")
		assignBlockIDs(blocks[i].Blocks, fresh)
	}
}

func collectIDs(d Doc, used map[string]bool) {
	for _, t := range d.Tabs {
		used[t.ID] = true
		collectBlockIDs(t.Blocks, used)
	}
}

func collectBlockIDs(blocks []Block, used map[string]bool) {
	for _, b := range blocks {
		used[b.ID] = true
		collectBlockIDs(b.Blocks, used)
	}
}

// itemText is the text a block inside a list item contributes to the item.
func (c *converter) itemText(n ast.Node) ([]Inline, error) {
	switch n := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		if t := codeText(n, c.src); t != "" {
			return []Inline{run(t, []string{MarkCode}, "")}, nil
		}
		return nil, nil
	case *ast.Blockquote:
		var runs []Inline
		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			part, err := c.itemText(ch)
			if err != nil {
				return nil, err
			}
			if len(runs) > 0 && len(part) > 0 {
				runs = append(runs, Inline{Text: "\n"})
			}
			runs = append(runs, part...)
		}
		return runs, nil
	case *ast.ThematicBreak, *ast.HTMLBlock, *east.Table:
		return nil, nil
	}
	return c.inlines(n, false)
}
