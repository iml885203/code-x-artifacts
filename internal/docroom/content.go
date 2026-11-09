package docroom

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// ErrReadWhileWriting is what a read made from inside a write returns. In ygo,
// reading inside a transaction waits for a lock the transaction holds and never
// comes back; here it comes back at once, with this.
var ErrReadWhileWriting = errors.New("docroom: the document was read from inside a write, which would never return")

// Content is a room's document as a doc. It is made before anything is
// changed, because looking up a shared type inside a transaction never
// returns, and its writes are functions that only write: they hold the shared
// types they need, so they cannot look anything up, and a read made while one
// runs is refused instead of hanging.
//
//	hub.Edit(ctx, room, who, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
//		c := docroom.Open(doc)
//		current, err := c.Read() // plan: read freely
//		...
//		return c.Write(next), nil // write: only writes
//	})
//
// A doc in a room is the order of its tabs (a shared array of tab ids) and, for
// each tab, a fragment of its own that holds the tab element and its blocks
// (docs/doc-room.md). Moving a tab changes the array and nothing else, so what
// people are typing in the tab is never touched.
type Content struct {
	doc    *crdt.Doc
	order  *crdt.YArray
	legacy *crdt.YXmlFragment
	frags  map[string]*crdt.YXmlFragment
	// writing is set while a write runs.
	writing atomic.Bool
}

// Open looks up the shared types that hold the document: the order, and the
// fragment of every tab in it. Call it in the plan, never inside a transaction.
func Open(doc *crdt.Doc) *Content {
	c := &Content{doc: doc, order: doc.GetArray(OrderName), legacy: doc.GetXmlFragment(Fragment), frags: map[string]*crdt.YXmlFragment{}}
	for _, id := range c.ids() {
		c.fragment(id)
	}
	return c
}

// fragment is the fragment of a tab, looked up the first time it is asked for.
func (c *Content) fragment(id string) *crdt.YXmlFragment {
	f, ok := c.frags[id]
	if !ok {
		f = c.doc.GetXmlFragment(TabPrefix + id)
		c.frags[id] = f
	}
	return f
}

// rawOrder is the array as it is: two people who move a tab at once can leave
// an id in it twice.
func (c *Content) rawOrder() []string {
	items := c.order.ToSlice()
	out := make([]string, 0, len(items))
	for _, v := range items {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ids is the order of the tabs, each id once (its first place).
func (c *Content) ids() []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range c.rawOrder() {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// isLegacy says the room still holds its tabs in the one fragment the first
// editor wrote (`content`): Migrate turns it into the current form.
func (c *Content) isLegacy() bool { return c.order.Len() == 0 && c.legacy.Len() > 0 }

// TabElement is the element of a tab (the one child of its fragment) as it is
// now, for what needs to edit inside a tab the way a person's editor does.
func (c *Content) TabElement(id string) (*crdt.YXmlElement, bool) {
	kids := c.fragment(id).Children()
	if len(kids) != 1 {
		return nil, false
	}
	e, ok := kids[0].(*crdt.YXmlElement)
	return e, ok && e.NodeName == nodeTab
}

// FirstTabElement is the element of the first tab, for what edits inside it
// the way a person's editor does.
func (c *Content) FirstTabElement() (*crdt.YXmlElement, bool) {
	ids := c.ids()
	if len(ids) == 0 {
		return nil, false
	}
	return c.TabElement(ids[0])
}

// Read returns the document the room holds, in the order of the room. A node
// the editor schema does not have is an error naming where it is.
func (c *Content) Read() (docfmt.Doc, error) {
	if c.writing.Load() {
		return docfmt.Doc{}, ErrReadWhileWriting
	}
	if c.isLegacy() {
		return reader{}.doc(c.legacy)
	}
	d := docfmt.Doc{Tabs: []docfmt.Tab{}}
	for i, id := range c.ids() {
		path := fmt.Sprintf("tabs[%d]", i)
		kids := c.fragment(id).Children()
		if len(kids) != 1 {
			return docfmt.Doc{}, fmt.Errorf("%s: the fragment of tab %q holds %d nodes, not its tab", path, id, len(kids))
		}
		e, ok := kids[0].(*crdt.YXmlElement)
		if !ok || e.NodeName != nodeTab {
			return docfmt.Doc{}, unexpected(path, kids[0])
		}
		t, err := reader{}.tab(e, path)
		if err != nil {
			return docfmt.Doc{}, err
		}
		if t.ID != id {
			return docfmt.Doc{}, fmt.Errorf("%s: the fragment of tab %q holds tab %q", path, id, t.ID)
		}
		d.Tabs = append(d.Tabs, t)
	}
	return d, nil
}

// Writer is what a write has to work with: it can change the document and
// nothing else, because it holds no way to look a shared type up or to read.
type Writer struct {
	c *Content
	w writer
}

// Replace makes the room hold d: what it held before is gone and every block of
// d is new to the people who are in it. The tabs of d were named when the change
// was made (Content.Change).
func (w *Writer) Replace(d docfmt.Doc) { w.c.replace(w.w, d) }

// Change returns the change fn makes, for Hub.Edit. tabs are the ids of the
// tabs fn may write, so their fragments are looked up now, in the plan; fn runs
// inside the transaction, and a Read of the same Content made meanwhile is
// refused with ErrReadWhileWriting.
func (c *Content) Change(fn func(*Writer), tabs ...string) func(*crdt.Transaction) {
	for _, id := range tabs {
		c.fragment(id)
	}
	return func(txn *crdt.Transaction) {
		c.writing.Store(true)
		defer c.writing.Store(false)
		fn(&Writer{c: c, w: writer{txn}})
	}
}

// Write returns the change that makes the room hold d.
func (c *Content) Write(d docfmt.Doc) func(*crdt.Transaction) {
	ids := make([]string, len(d.Tabs))
	for i, t := range d.Tabs {
		ids[i] = t.ID
	}
	return c.Change(func(w *Writer) { w.Replace(d) }, ids...)
}

// Migrate is the change that moves a room that holds its tabs the old way into
// the current form, or nil when it does not need one.
func (c *Content) Migrate() (func(*crdt.Transaction), error) {
	if !c.isLegacy() {
		return nil, nil
	}
	d, err := c.Read()
	if err != nil {
		return nil, err
	}
	return c.Write(d), nil
}

// replace writes d over everything: the order, every tab's fragment (the ones
// that are no longer in the doc are emptied) and the old one-fragment form.
func (c *Content) replace(w writer, d docfmt.Doc) {
	emptyFragment(w, c.legacy)
	if n := c.order.Len(); n > 0 {
		c.order.Delete(w.txn, 0, n)
	}
	keep := map[string]bool{}
	ids := make([]any, len(d.Tabs))
	for i, t := range d.Tabs {
		keep[t.ID] = true
		ids[i] = t.ID
		f := c.frags[t.ID]
		emptyFragment(w, f)
		f.InsertElement(w.txn, 0, w.tab(t))
	}
	for id, f := range c.frags {
		if !keep[id] {
			emptyFragment(w, f)
		}
	}
	if len(ids) > 0 {
		c.order.Push(w.txn, ids)
	}
}

// State is the encoded state of a new document that holds d, for a room that
// starts from it (Hub.Replace, a log that has no rows yet).
func State(d docfmt.Doc) []byte {
	doc := crdt.New()
	defer doc.Destroy()
	c := Open(doc)
	doc.Transact(c.Write(d))
	return crdt.EncodeStateAsUpdateV1(doc, nil)
}
