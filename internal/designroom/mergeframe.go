package designroom

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/reearth/ygo/crdt"
	"golang.org/x/net/html"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// frameMerge is the merge of one frame: b is the frame as the version the agent
// started from has it, n what the agent made of it, c what the room holds now.
type frameMerge struct {
	r    *Room
	e    map[string]any
	file string

	b, n, c    frameModel
	bi, ni, ci map[string]node
	bk, nk, ck map[string][]node

	steps   []func(*crdt.Transaction)
	touched map[string]bool
	taken   map[string]bool // ids the room holds or this merge has made
	dropped map[string]bool
	parents map[string]string // the room's tree as the merge leaves it, for the cycle check
	placed  []string          // elements the merge puts somewhere
	fresh   map[string]int
}

func (f *frameMerge) touchedPIDs() []string {
	out := make([]string, 0, len(f.touched))
	for pid := range f.touched {
		out = append(out, pid)
	}
	sort.Strings(out)
	return out
}

func byID(m frameModel) map[string]node {
	out := make(map[string]node, len(m.nodes))
	for _, n := range m.nodes {
		out[n.id] = n
	}
	return out
}

// childrenOf are the children of each node, in the order they are drawn.
func childrenOf(m frameModel) map[string][]node {
	out := map[string][]node{}
	for _, n := range m.nodes {
		out[n.parent] = append(out[n.parent], n)
	}
	for _, list := range out {
		sort.Slice(list, func(i, j int) bool {
			if list[i].order != list[j].order {
				return list[i].order < list[j].order
			}
			return list[i].id < list[j].id
		})
	}
	return out
}

// renameRoot gives the body of a model the id the room's body has: a frame an
// agent wrote without ids has a body with an id of its own making.
func renameRoot(m frameModel, to string) frameModel {
	if m.root == to {
		return m
	}
	from := m.root
	nodes := make([]node, len(m.nodes))
	copy(nodes, m.nodes)
	for i := range nodes {
		if nodes[i].id == from {
			nodes[i].id = to
		}
		if nodes[i].parent == from {
			nodes[i].parent = to
		}
	}
	m.nodes, m.root = nodes, to
	return m
}

func (r *Room) mergeFrame(e map[string]any, file, baseText, nextText string) (*frameMerge, error) {
	f := &frameMerge{r: r, e: e, file: file, touched: map[string]bool{}, dropped: map[string]bool{}, fresh: map[string]int{}}
	var err error
	if f.c, err = r.model(e, file); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if f.b, err = modelOf(baseText); err != nil {
		return nil, fmt.Errorf("%s of the version: %w", file, err)
	}
	f.b = renameRoot(f.b, f.c.root)
	if err := f.refuseWipingFrame(baseText, nextText); err != nil {
		return nil, err
	}
	f.ci = byID(f.c)
	f.bi = byID(f.b)
	f.taken = map[string]bool{}
	var avoid []string
	for id := range f.ci {
		f.taken[id] = true
		avoid = append(avoid, id)
	}
	for id := range f.bi {
		avoid = append(avoid, id)
	}
	normal, err := designfmt.NormalizeAvoiding(nextText, avoid)
	if err != nil {
		return nil, notHeld("%s: %v", file, err)
	}
	if f.n, err = modelOf(normal); err != nil {
		return nil, notHeld("%s: %v", file, err)
	}
	f.n = renameRoot(f.n, f.c.root)
	f.ni = byID(f.n)
	f.bk, f.nk, f.ck = childrenOf(f.b), childrenOf(f.n), childrenOf(f.c)
	f.parents = make(map[string]string, len(f.ci))
	for id, n := range f.ci {
		f.parents[id] = n.parent
	}

	f.shell()
	f.removed()
	if err := f.visit(f.c.root); err != nil {
		return nil, err
	}
	for _, id := range f.placed {
		if inCycle(f.parents, id, f.c.root) {
			return nil, conflict("%s: your change moves %s into something that someone moved into it meanwhile; read the design again", file, id)
		}
	}
	return f, nil
}

// refuseWipingFrame refuses a frame sent without any data-pid when the room holds
// changes to that frame since the agent's base: with no ids nothing in it can be
// matched, so it would replace the frame and wipe those changes.
func (f *frameMerge) refuseWipingFrame(baseText, nextText string) error {
	doc, parseErr := html.Parse(strings.NewReader(nextText))
	now, builtErr := build(f.c)
	was, normErr := designfmt.Normalize(baseText)
	if parseErr != nil || builtErr != nil || normErr != nil || hasPID(doc) || now == was {
		return nil //nolint:nilerr // a frame that cannot be read is refused when it is normalised
	}
	return conflict("%s: the frame has no data-pid on its elements, so it would replace the live frame that people have changed since you read it; read the live frame (read with file) and publish it with its data-pid ids kept", f.file)
}

func hasPID(n *html.Node) bool {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "data-pid" {
				return true
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasPID(c) {
			return true
		}
	}
	return false
}

func (f *frameMerge) step(do func(*crdt.Transaction)) { f.steps = append(f.steps, do) }

func (f *frameMerge) set(id, field, value string) {
	key := nodeKey(f.file, id, field)
	f.step(func(txn *crdt.Transaction) { f.r.m.Set(txn, key, value) })
}

func (f *frameMerge) drop(keys []string) {
	f.step(func(txn *crdt.Transaction) {
		for _, k := range keys {
			f.r.m.Delete(txn, k)
		}
	})
}

// touch marks an element as changed, for the agent's outline; text and comment
// nodes are marked by the element they are in.
func (f *frameMerge) touch(id string) {
	for id != "" {
		n, ok := f.ci[id]
		if !ok {
			n, ok = f.ni[id]
		}
		if !ok {
			return
		}
		if n.kind == kindElement {
			f.touched[id] = true
			return
		}
		id = n.parent
	}
}

// shell takes the agent's changes to what surrounds the body.
func (f *frameMerge) shell() {
	set := func(field, was, now string) {
		if was != now {
			key := shellKey(f.file, field)
			f.step(func(txn *crdt.Transaction) { f.r.m.Set(txn, key, now) })
		}
	}
	set("docBefore", f.b.docBefore, f.n.docBefore)
	set("docAfter", f.b.docAfter, f.n.docAfter)
	set("pre", f.b.pre, f.n.pre)
	set("post", f.b.post, f.n.post)
	set("htmlAttrs", attrsJSON(f.b.htmlAttrs), attrsJSON(f.n.htmlAttrs))
}

// removed drops the elements the agent took out, with what is inside them
// except the elements the agent put somewhere else.
func (f *frameMerge) removed() {
	var gone []string
	for id, n := range f.bi {
		if n.kind == kindElement && id != f.b.root && f.ni[id].id == "" {
			gone = append(gone, id)
		}
	}
	sort.Strings(gone)
	for _, id := range gone {
		if _, there := f.ci[id]; !there || f.dropped[id] {
			continue
		}
		f.touch(f.ci[id].parent)
		f.dropSubtree(id)
	}
}

func (f *frameMerge) dropSubtree(id string) {
	var keys []string
	pending := []string{id}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if f.dropped[next] {
			continue
		}
		f.dropped[next] = true
		delete(f.parents, next)
		keys = append(keys, f.keysOfNode(next)...)
		for _, k := range f.ck[next] {
			if _, rescued := f.ni[k.id]; rescued && k.kind == kindElement {
				continue // the agent put it somewhere else
			}
			pending = append(pending, k.id)
		}
	}
	f.drop(keys)
}

func (f *frameMerge) keysOfNode(id string) []string {
	prefix := nodeKey(f.file, id, "")
	var keys []string
	for k := range f.e {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// visit merges the element id of the agent's frame and everything under it.
func (f *frameMerge) visit(id string) error {
	_, inBase := f.bi[id]
	_, inRoom := f.ci[id]
	switch {
	case inBase && !inRoom, f.dropped[id]:
		return nil // a person deleted it; it stays deleted
	case inBase:
		if err := f.mergeProperties(id); err != nil {
			return err
		}
		if err := f.mergeChildren(id, false); err != nil {
			return err
		}
	default:
		if err := f.mergeChildren(id, true); err != nil {
			return err
		}
	}
	for _, k := range f.nk[id] {
		if k.kind == kindElement {
			if err := f.visit(k.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeProperties writes what the agent changed of an element that is in the
// version and in the room: each attribute list, style skeleton, owned style
// property and the formatted text as a whole, if the agent changed it.
func (f *frameMerge) mergeProperties(id string) error {
	b, n := retainUntouchedCoveredSlots(f.bi[id], promoteCoveredStyleEdits(f.bi[id], f.ni[id]), f.ci[id])
	f.ni[id] = n
	changed := false
	if n.tag != b.tag || n.ns != b.ns {
		f.set(id, "tag", n.tag)
		f.set(id, "ns", n.ns)
		changed = true
	}
	if attrsJSON(n.attrs) != attrsJSON(b.attrs) {
		f.set(id, "attrs", attrsJSON(n.attrs))
		changed = true
	}
	if n.rest != b.rest {
		f.set(id, "rest", n.rest)
		changed = true
	}
	styled, err := f.mergeStyle(id)
	if err != nil {
		return err
	}
	richChanged, err := f.mergeRich(id)
	if err != nil {
		return err
	}
	if richChanged || styled {
		changed = true
	}
	if changed {
		f.touch(id)
	}
	return nil
}

// mergeStyle writes the owned style properties the agent set or removed.
func (f *frameMerge) mergeStyle(id string) (changed bool, err error) {
	b, n := f.bi[id], f.ni[id]
	props := map[string]bool{}
	for p := range n.style {
		props[p] = true
	}
	for p := range b.style {
		props[p] = true
	}
	names := make([]string, 0, len(props))
	for p := range props {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, prop := range names {
		now, has := n.style[prop]
		was, hadBefore := b.style[prop]
		switch {
		case has && (!hadBefore || now != was):
			if err := ValidateStyleValue(prop, now); err != nil {
				return false, notHeld("%s: the style of %s: %v; while people are editing the design it takes the values the editor does", f.file, id, err)
			}

			f.setStyleValue(id, prop, now)
			changed = true

		case !has && hadBefore:
			f.dropStyleValue(id, prop)
			changed = true
		}
	}
	return changed, nil
}

// mergeRich writes the agent's change to the formatted text of an element. An
// element that is formatted text in the version, in the agent's frame and in the
// room is merged like text (mergeRichText); one that becomes or stops being
// formatted text has its text replaced, or taken away, as a whole.
func (f *frameMerge) mergeRich(id string) (changed bool, err error) {
	b, n, c := f.bi[id], f.ni[id], f.ci[id]
	if n.hasRich == b.hasRich && (!n.hasRich || reflect.DeepEqual(n.rich, b.rich)) {
		return false, nil
	}
	if n.hasRich && b.hasRich && c.hasRich {
		return true, f.mergeRichText(id, b.rich, n.rich, c.rich)
	}
	key := nodeKey(f.file, id, "rich")
	if !n.hasRich {
		f.drop([]string{key})
		return true, nil
	}
	ops := n.rich
	f.step(func(txn *crdt.Transaction) {
		t := crdt.NewTextPrelim()
		f.r.m.Set(txn, key, t)
		putRich(txn, t, ops)
	})
	return true, nil
}

// item is one child of an element as the agent has it, and what that means for
// the room.
type item struct {
	element bool
	roomID  string // the node in the room, for what is already there
	node    node   // the agent's node, for what is new
	isNew   bool
	moved   bool
	stable  bool   // it is where the agent left it and the room's order stands
	key     string // its order key: the room's, or the one it is given
}

// mergeChildren puts the children of an element where the agent has them: new
// ones are added, ones the agent moved are moved, the texts the agent changed
// are changed, and what the agent took away is taken away. Children the agent
// left alone stay where the room has them, wherever the people put them.
func (f *frameMerge) mergeChildren(id string, parentIsNew bool) error {
	nk := f.nk[id]
	var bk, ck []node
	if !parentIsNew {
		bk, ck = f.bk[id], f.ck[id]
	}
	anchors := anchors(parentIsNew, bk, nk, ck)
	bs, _ := slotsOf(bk, anchors)
	ns, nWhere := slotsOf(nk, anchors)
	cs, _ := slotsOf(ck, anchors)
	reordered := f.reordered(bk, nk)

	var items []item
	for _, k := range nk {
		var it *item
		var err error
		if k.kind == kindElement {
			it, err = f.elementItem(id, k, reordered)
		} else {
			s := nWhere[k.id]
			it, err = f.textItem(k, bs, cs, s)
		}
		if err != nil {
			return err
		}
		if it != nil {
			items = append(items, *it)
		}
	}
	f.dropTakenOutTexts(id, bs, ns, cs)
	f.place(id, items)
	return nil
}

// anchors are the elements that are children of the element in the version, in the agent's frame and in the room.
func anchors(parentIsNew bool, bk, nk, ck []node) map[string]bool {
	out := map[string]bool{}
	if parentIsNew {
		return out
	}
	inB, inC := map[string]bool{}, map[string]bool{}
	for _, k := range bk {
		inB[k.id] = k.kind == kindElement
	}
	for _, k := range ck {
		inC[k.id] = k.kind == kindElement
	}
	for _, k := range nk {
		if k.kind == kindElement && inB[k.id] && inC[k.id] {
			out[k.id] = true
		}
	}
	return out
}

// elementItem is what the agent's having an element under parent means for the
// room, or nil when it means nothing to do.
func (f *frameMerge) elementItem(parent string, k node, reordered map[string]bool) (*item, error) {
	b, inBase := f.bi[k.id]
	c, inRoom := f.ci[k.id]
	switch {
	case !inBase:
		if inRoom || f.taken[k.id] {
			return nil, conflict("%s: %s was added to the design by someone else while you were adding it; read the design again", f.file, k.id)
		}
		f.taken[k.id] = true
		return &item{element: true, node: k, isNew: true}, nil
	case !inRoom || f.dropped[k.id]:
		return nil, nil // deleted by a person
	case b.parent != parent || reordered[k.id]:
		return &item{element: true, roomID: k.id, moved: true}, nil
	case c.parent == parent:
		return &item{element: true, roomID: k.id, stable: true, key: c.order}, nil
	}
	return nil, nil // a person moved it elsewhere, which the agent did not
}

// textItem is what the agent's having a text in a slot means for the room, with
// the text merged into the room's when the agent changed it.
func (f *frameMerge) textItem(k node, bs, cs map[slot]node, s slot) (*item, error) {
	nb, inBase := bs[s]
	if !inBase {
		return &item{node: k, isNew: true}, nil
	}
	nc, inRoom := cs[s]
	if !inRoom {
		return nil, nil // a person deleted that text
	}
	if k.text != nb.text {
		if err := f.mergeText(nb, k, nc); err != nil {
			return nil, err
		}
	}
	return &item{roomID: nc.id, stable: true, key: nc.order}, nil
}

// dropTakenOutTexts removes from the room the texts the version had and the agent's frame does not.
func (f *frameMerge) dropTakenOutTexts(parent string, bs, ns, cs map[slot]node) {
	for _, s := range sortedSlots(bs) {
		if _, still := ns[s]; still {
			continue
		}
		if nc, ok := cs[s]; ok {
			f.drop(f.keysOfNode(nc.id))
			f.dropped[nc.id] = true
			f.touch(parent)
		}
	}
}

// freshID is an id for a text or comment node that nothing in the room has.
func (f *frameMerge) freshID(kind string) string {
	prefix := "~" + kind[:1]
	if _, started := f.fresh[prefix]; !started {
		highest := 0
		for id := range f.ci {
			if num, err := strconv.Atoi(strings.TrimPrefix(id, prefix)); err == nil && strings.HasPrefix(id, prefix) {
				highest = max(highest, num)
			}
		}
		f.fresh[prefix] = highest
	}
	for {
		f.fresh[prefix]++
		id := prefix + strconv.Itoa(f.fresh[prefix])
		if !f.taken[id] {
			f.taken[id] = true
			return id
		}
	}
}
