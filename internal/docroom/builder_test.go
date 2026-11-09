package docroom_test

import (
	"github.com/reearth/ygo/crdt"
)

// roomBuilder writes a room by hand, the way an editor client could, to put
// things in it that Write never would.
type roomBuilder struct {
	doc  *crdt.Doc
	frag *crdt.YXmlFragment
	last *crdt.YXmlElement
}

// A room as the editor keeps it: the order of the tab ids and a fragment per tab.

func newRoomBuilder() *roomBuilder {
	doc := crdt.New()
	return &roomBuilder{doc: doc, frag: doc.GetXmlFragment("tab:t1")}
}

func (b *roomBuilder) tab() *roomBuilder {
	e := crdt.NewYXmlElement("tab")
	order := b.doc.GetArray("tabs")
	b.doc.Transact(func(txn *crdt.Transaction) {
		order.Push(txn, []any{"t1"})
		b.frag.InsertElement(txn, b.frag.Len(), e)
		e.SetAttribute(txn, "id", "t1")
	})
	b.last = e
	return b
}

func (b *roomBuilder) child(name, id string) *roomBuilder {
	parent := b.last
	e := crdt.NewYXmlElement(name)
	b.doc.Transact(func(txn *crdt.Transaction) {
		parent.InsertElement(txn, parent.Len(), e)
		if id != "" {
			e.SetAttribute(txn, "id", id)
		}
	})
	b.last = e
	return b
}

// text adds a run carrying a mark of the given name (and no attributes).
func (b *roomBuilder) text(s, mark string) *roomBuilder {
	parent := b.last
	t := crdt.NewYXmlText()
	b.doc.Transact(func(txn *crdt.Transaction) {
		parent.InsertText(txn, 0, t)
		t.ApplyDelta(txn, []crdt.Delta{{Op: crdt.DeltaOpInsert, Insert: s, Attributes: crdt.Attributes{mark: map[string]any{}}}})
	})
	return b
}

// stray puts text straight into a tab, where only blocks belong.
func (b *roomBuilder) stray() *roomBuilder { return b.text("loose", "bold") }

// link adds a link run to the paragraph being built.
func (b *roomBuilder) link(s, href string) *roomBuilder {
	parent := b.last
	t := crdt.NewYXmlText()
	b.doc.Transact(func(txn *crdt.Transaction) {
		parent.InsertText(txn, 0, t)
		t.ApplyDelta(txn, []crdt.Delta{{Op: crdt.DeltaOpInsert, Insert: s, Attributes: crdt.Attributes{"link": map[string]any{"href": href}}}})
	})
	return b
}
