package docroom

import (
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// writer builds the elements of a document inside one transaction. Everything
// it does is a write: it never looks a shared type up and never reads one
// (docs/doc-room.md, "Reading and writing").
type writer struct{ txn *crdt.Transaction }

func (w writer) tab(t docfmt.Tab) *crdt.YXmlElement {
	e := crdt.NewYXmlElement(nodeTab)
	e.SetAttribute(w.txn, attrID, t.ID)
	e.SetAttribute(w.txn, attrTitle, t.Title)
	w.blocks(e, t.Blocks)
	return e
}

func (w writer) blocks(parent *crdt.YXmlElement, blocks []docfmt.Block) {
	for _, b := range blocks {
		parent.InsertElement(w.txn, parent.Len(), w.block(b))
	}
}

func (w writer) block(b docfmt.Block) *crdt.YXmlElement {
	name := b.Type
	if name == docfmt.Code {
		name = nodeCode
	}
	e := crdt.NewYXmlElement(name)
	e.SetAttribute(w.txn, attrID, b.ID)
	switch b.Type {
	case docfmt.Paragraph:
		w.inlines(e, b.Content)
	case docfmt.Heading:
		e.SetAttributeValue(w.txn, attrLevel, b.Level)
		w.inlines(e, b.Content)
	case docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList:
		if b.Type == docfmt.NumberedList && b.Start != 0 {
			e.SetAttributeValue(w.txn, attrStart, b.Start)
		}
		w.items(e, b.Type, b.Items)
	case docfmt.Quote:
		w.blocks(e, b.Blocks)
	case docfmt.Code:
		w.setString(e, attrLang, b.Lang)
		if b.Text != "" {
			text := crdt.NewYXmlText()
			e.InsertText(w.txn, 0, text)
			text.Insert(w.txn, 0, b.Text, nil)
		}
	case docfmt.Table:
		if b.Header {
			e.SetAttributeValue(w.txn, attrHeader, true)
		}
		w.rows(e, b.Rows)
	case docfmt.Image:
		e.SetAttribute(w.txn, attrSrc, b.Src)
		w.setString(e, attrAlt, b.Alt)
		w.setString(e, attrSize, b.Size)
	case docfmt.Pending:
		e.SetAttribute(w.txn, attrIntent, b.Intent)
	}
	return e
}

// setString sets an attribute unless it is empty: a missing attribute and an
// empty one are the same thing in the model, and the editor leaves out what is
// null.
func (w writer) setString(e *crdt.YXmlElement, key, value string) {
	if value != "" {
		e.SetAttribute(w.txn, key, value)
	}
}

// items writes the items of a list.
func (w writer) items(list *crdt.YXmlElement, typ string, items []docfmt.Item) {
	for _, it := range items {
		list.InsertElement(w.txn, list.Len(), w.item(typ, it))
	}
}

// item is one item of a list: an element holding a paragraph without an id (the
// item's text) and, when it has them, a list of the same type without an id
// (the nested items).
func (w writer) item(typ string, it docfmt.Item) *crdt.YXmlElement {
	li := crdt.NewYXmlElement(nodeListItem)
	if typ == docfmt.TaskList {
		checked := it.Checked != nil && *it.Checked
		li.SetAttributeValue(w.txn, attrChecked, checked)
	}
	li.InsertElement(w.txn, 0, w.paragraph(it.Content))
	if len(it.Items) > 0 {
		nested := crdt.NewYXmlElement(typ)
		w.items(nested, typ, it.Items)
		li.InsertElement(w.txn, 1, nested)
	}
	return li
}

func (w writer) rows(table *crdt.YXmlElement, rows []docfmt.Row) {
	for _, r := range rows {
		table.InsertElement(w.txn, table.Len(), w.row(r))
	}
}

func (w writer) row(r docfmt.Row) *crdt.YXmlElement {
	row := crdt.NewYXmlElement(nodeRow)
	for _, c := range r.Cells {
		cell := crdt.NewYXmlElement(nodeCell)
		w.setString(cell, attrAlign, c.Align)
		cell.InsertElement(w.txn, 0, w.paragraph(c.Content))
		row.InsertElement(w.txn, row.Len(), cell)
	}
	return row
}

// paragraph is the text container inside a list item or a table cell.
func (w writer) paragraph(content []docfmt.Inline) *crdt.YXmlElement {
	e := crdt.NewYXmlElement(docfmt.Paragraph)
	w.inlines(e, content)
	return e
}

// inlines writes runs of text into parent. Text is one XmlText with its marks
// as attributes, as y-prosemirror does; a newline in a run is a hard_break
// element between two texts, which is how the editor writes a line break, and
// a mention is a mention element with the account and the name.
func (w writer) inlines(parent *crdt.YXmlElement, content []docfmt.Inline) {
	var text *crdt.YXmlText
	var delta []crdt.Delta
	flush := func() {
		if text != nil {
			text.ApplyDelta(w.txn, delta)
		}
		text, delta = nil, nil
	}
	for _, run := range content {
		if run.Dropdown != nil {
			flush()
			e := crdt.NewYXmlElement(nodeDropdown)
			e.SetAttribute(w.txn, attrConfig, run.Dropdown.JSON())
			parent.InsertElement(w.txn, parent.Len(), e)
			continue
		}
		if run.Date != "" {
			flush()
			e := crdt.NewYXmlElement(nodeDate)
			e.SetAttribute(w.txn, attrDate, run.Date)
			parent.InsertElement(w.txn, parent.Len(), e)
			continue
		}
		if run.Mention() {
			flush()
			m := crdt.NewYXmlElement(nodeMention)
			w.setString(m, attrUser, run.User)
			w.setString(m, attrName, run.Name)
			parent.InsertElement(w.txn, parent.Len(), m)
			continue
		}
		attrs := markAttributes(run)
		for i, part := range strings.Split(run.Text, "\n") {
			if i > 0 {
				flush()
				parent.InsertElement(w.txn, parent.Len(), crdt.NewYXmlElement(nodeBreak))
			}
			if part == "" {
				continue
			}
			if text == nil {
				text = crdt.NewYXmlText()
				parent.InsertText(w.txn, parent.Len(), text)
			}
			delta = append(delta, crdt.Delta{Op: crdt.DeltaOpInsert, Insert: part, Attributes: attrs})
		}
	}
	flush()
}

// markAttributes is how y-prosemirror stores a run's marks: an attribute per
// mark whose value is the mark's attributes, an empty object for a mark that
// has none.
func markAttributes(run docfmt.Inline) crdt.Attributes {
	if len(run.Marks) == 0 {
		return nil
	}
	attrs := make(crdt.Attributes, len(run.Marks))
	for _, m := range run.Marks {
		value := map[string]any{}
		if m == docfmt.MarkLink {
			value[attrHref] = run.Href
		}
		attrs[m] = value
	}
	return attrs
}
