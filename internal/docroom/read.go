package docroom

import (
	"fmt"
	"slices"
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// reader turns the elements of a room back into blocks. It only reads.
type reader struct{}

func (reader) doc(frag *crdt.YXmlFragment) (docfmt.Doc, error) {
	d := docfmt.Doc{Tabs: []docfmt.Tab{}}
	for i, child := range frag.Children() {
		path := fmt.Sprintf("tabs[%d]", i)
		e, ok := child.(*crdt.YXmlElement)
		if !ok || e.NodeName != nodeTab {
			return docfmt.Doc{}, unexpected(path, child)
		}
		t, err := reader{}.tab(e, path)
		if err != nil {
			return docfmt.Doc{}, err
		}
		d.Tabs = append(d.Tabs, t)
	}
	return d, nil
}

func (r reader) tab(e *crdt.YXmlElement, path string) (docfmt.Tab, error) {
	id, err := requiredString(e, attrID, path)
	if err != nil {
		return docfmt.Tab{}, err
	}
	title, _ := stringAttr(e, attrTitle)
	blocks, err := r.blocks(e, path+".blocks")
	return docfmt.Tab{ID: id, Title: title, Blocks: blocks}, err
}

func (r reader) blocks(parent *crdt.YXmlElement, path string) ([]docfmt.Block, error) {
	blocks := []docfmt.Block{}
	for i, child := range parent.Children() {
		p := fmt.Sprintf("%s[%d]", path, i)
		e, ok := child.(*crdt.YXmlElement)
		if !ok {
			return nil, unexpected(p, child)
		}
		b, err := r.block(e, p)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

func (r reader) block(e *crdt.YXmlElement, path string) (docfmt.Block, error) {
	typ := e.NodeName
	if typ == nodeCode {
		typ = docfmt.Code
	}
	b := docfmt.Block{Type: typ}
	if !blockTypes[typ] || e.NodeName == docfmt.Code {
		return b, fmt.Errorf("%s: <%s> is not a block of a doc", path, e.NodeName)
	}
	var err error
	if b.ID, err = requiredString(e, attrID, path); err != nil {
		return b, err
	}
	switch typ {
	case docfmt.Paragraph:
		b.Content, err = r.inlines(e, path)
	case docfmt.Heading:
		b.Level = intAttr(e, attrLevel)
		b.Content, err = r.inlines(e, path)
	case docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList:
		if typ == docfmt.NumberedList {
			if b.Start = intAttr(e, attrStart); b.Start == 1 {
				b.Start = 0
			}
		}
		b.Items, err = r.items(e, typ, path+".items")
	case docfmt.Quote:
		b.Blocks, err = r.blocks(e, path+".blocks")
	case docfmt.Code:
		b.Lang, _ = stringAttr(e, attrLang)
		b.Text, err = r.plainText(e, path)
	case docfmt.Table:
		b.Header = boolAttr(e, attrHeader)
		b.Rows, err = r.rows(e, path+".rows")
	case docfmt.Image:
		b.Src, _ = stringAttr(e, attrSrc)
		b.Alt, _ = stringAttr(e, attrAlt)
		b.Size, _ = stringAttr(e, attrSize)
	case docfmt.Pending:
		b.Intent, _ = stringAttr(e, attrIntent)
	}
	return b, err
}

var blockTypes = map[string]bool{
	docfmt.Paragraph: true, docfmt.Heading: true, docfmt.BulletList: true, docfmt.NumberedList: true,
	docfmt.TaskList: true, docfmt.Quote: true, docfmt.Code: true, docfmt.Rule: true,
	docfmt.Table: true, docfmt.Image: true, docfmt.Pending: true,
}

func (r reader) items(list *crdt.YXmlElement, typ, path string) ([]docfmt.Item, error) {
	var items []docfmt.Item
	for i, child := range list.Children() {
		p := fmt.Sprintf("%s[%d]", path, i)
		li, ok := child.(*crdt.YXmlElement)
		if !ok || li.NodeName != nodeListItem {
			return nil, unexpected(p, child)
		}
		it := docfmt.Item{}
		if typ == docfmt.TaskList {
			checked := boolAttr(li, attrChecked)
			it.Checked = &checked
		}
		for j, part := range li.Children() {
			e, ok := part.(*crdt.YXmlElement)
			var err error
			switch {
			case ok && e.NodeName == docfmt.Paragraph && j == 0:
				it.Content, err = r.inlines(e, p)
			case ok && e.NodeName == typ && j == 1:
				it.Items, err = r.items(e, typ, p+".items")
			default:
				err = unexpected(fmt.Sprintf("%s[%d]", p, j), part)
			}
			if err != nil {
				return nil, err
			}
		}
		items = append(items, it)
	}
	return items, nil
}

func (r reader) rows(table *crdt.YXmlElement, path string) ([]docfmt.Row, error) {
	var rows []docfmt.Row
	for i, child := range table.Children() {
		p := fmt.Sprintf("%s[%d]", path, i)
		tr, ok := child.(*crdt.YXmlElement)
		if !ok || tr.NodeName != nodeRow {
			return nil, unexpected(p, child)
		}
		row := docfmt.Row{Cells: []docfmt.Cell{}}
		for j, c := range tr.Children() {
			cp := fmt.Sprintf("%s.cells[%d]", p, j)
			td, ok := c.(*crdt.YXmlElement)
			if !ok || td.NodeName != nodeCell {
				return nil, unexpected(cp, c)
			}
			cell := docfmt.Cell{}
			cell.Align, _ = stringAttr(td, attrAlign)
			for _, part := range td.Children() {
				e, ok := part.(*crdt.YXmlElement)
				if !ok || e.NodeName != docfmt.Paragraph {
					return nil, unexpected(cp, part)
				}
				var err error
				if cell.Content, err = r.inlines(e, cp); err != nil {
					return nil, err
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// plainText is the text of a code block: its text children with no marks.
func (reader) plainText(e *crdt.YXmlElement, path string) (string, error) {
	var sb strings.Builder
	for _, child := range e.Children() {
		text, ok := child.(*crdt.YXmlText)
		if !ok {
			return "", unexpected(path, child)
		}
		for _, d := range text.ToDelta() {
			s, ok := d.Insert.(string)
			if !ok {
				return "", fmt.Errorf("%s: the text holds something that is not text", path)
			}
			sb.WriteString(s)
		}
	}
	return sb.String(), nil
}

// inlines reads the text of a paragraph or heading. A mention element is a
// mention run, which has no marks and is never merged. A hard_break reads as a
// newline that takes the marks of the text before it (the text after it when it
// starts the line): a break has no glyph, so no one can see its marks, and the
// room does not store any for it. Neighbouring runs with the same marks are one.
func (reader) inlines(e *crdt.YXmlElement, path string) ([]docfmt.Inline, error) {
	var runs []docfmt.Inline
	var breaks []int
	for _, child := range e.Children() {
		switch n := child.(type) {
		case *crdt.YXmlText:
			for _, d := range n.ToDelta() {
				s, ok := d.Insert.(string)
				if !ok {
					return nil, fmt.Errorf("%s: the text holds something that is not text", path)
				}
				run, err := runOf(s, d.Attributes, path)
				if err != nil {
					return nil, err
				}
				runs = append(runs, run)
			}
		case *crdt.YXmlElement:
			switch n.NodeName {
			case nodeBreak:
				breaks = append(breaks, len(runs))
				runs = append(runs, docfmt.Inline{Text: "\n"})
			case nodeDropdown:
				run, err := dropdownRun(n, path)
				if err != nil {
					return nil, err
				}
				runs = append(runs, run)
			case nodeDate:
				value, err := requiredString(n, attrDate, path)
				if err != nil {
					return nil, err
				}
				runs = append(runs, docfmt.Inline{Date: value})
			case nodeMention:
				user, err := requiredString(n, attrUser, path)
				if err != nil {
					return nil, err
				}
				name, err := requiredString(n, attrName, path)
				if err != nil {
					return nil, err
				}
				runs = append(runs, docfmt.Inline{User: user, Name: name})
			default:
				return nil, unexpected(path, child)
			}
		default:
			return nil, unexpected(path, child)
		}
	}
	return settle(runs, breaks), nil
}

func dropdownRun(e *crdt.YXmlElement, path string) (docfmt.Inline, error) {
	config, err := requiredString(e, attrConfig, path)
	if err != nil {
		return docfmt.Inline{}, err
	}
	value, err := docfmt.ParseDropdown(config)
	if err != nil {
		return docfmt.Inline{}, fmt.Errorf("%s: %w", path, err)
	}
	return docfmt.Inline{Dropdown: value}, nil
}

// settle gives the line breaks (the runs at the indexes in breaks) their marks
// and merges the runs that end up with the same ones.
func settle(runs []docfmt.Inline, breaks []int) []docfmt.Inline {
	for _, i := range breaks {
		switch {
		case i > 0:
			runs[i].Marks, runs[i].Href = runs[i-1].Marks, runs[i-1].Href
		case i+1 < len(runs) && !slices.Contains(breaks, i+1):
			runs[i].Marks, runs[i].Href = runs[i+1].Marks, runs[i+1].Href
		}
	}
	return mergeRuns(runs)
}

// runOf is a piece of text with the marks it carries in the room.
func runOf(text string, attrs crdt.Attributes, path string) (docfmt.Inline, error) {
	run := docfmt.Inline{Text: text}
	present := map[string]bool{}
	for key, value := range attrs {
		name, _, _ := strings.Cut(key, markSeparator)
		if name == ignoredMark || value == nil {
			continue
		}
		if !slices.Contains(markOrder, name) {
			return run, fmt.Errorf("%s: %q is not a mark of a doc", path, name)
		}
		present[name] = true
		if name == docfmt.MarkLink {
			obj, _ := value.(map[string]any)
			href, ok := obj[attrHref].(string)
			if !ok {
				return run, fmt.Errorf("%s: a link has no href", path)
			}
			run.Href = href
		}
	}
	for _, m := range markOrder {
		if present[m] {
			run.Marks = append(run.Marks, m)
		}
	}
	return run, nil
}

func mergeRuns(runs []docfmt.Inline) []docfmt.Inline {
	var out []docfmt.Inline
	for _, r := range runs {
		if r.Text == "" && !r.Mention() && r.Date == "" && r.Dropdown == nil {
			continue
		}
		if n := len(out); n > 0 && !r.Mention() && !out[n-1].Mention() && r.Date == "" && out[n-1].Date == "" && r.Dropdown == nil && out[n-1].Dropdown == nil && slices.Equal(out[n-1].Marks, r.Marks) && out[n-1].Href == r.Href {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

func unexpected(path string, node any) error {
	switch n := node.(type) {
	case *crdt.YXmlElement:
		return fmt.Errorf("%s: <%s> is not allowed here", path, n.NodeName)
	case *crdt.YXmlText:
		return fmt.Errorf("%s: text is not allowed here", path)
	default:
		return fmt.Errorf("%s: this node is not allowed here", path)
	}
}

func requiredString(e *crdt.YXmlElement, key, path string) (string, error) {
	s, ok := stringAttr(e, key)
	if !ok || s == "" {
		return "", fmt.Errorf("%s: <%s> has no %s", path, e.NodeName, key)
	}
	return s, nil
}
