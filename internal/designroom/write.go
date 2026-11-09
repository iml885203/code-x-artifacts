package designroom

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/reearth/ygo/crdt"
)

// putFrame writes the entries of one frame.
func (r *Room) putFrame(txn *crdt.Transaction, file string, m frameModel) {
	set := func(key string, v string) { r.m.Set(txn, key, v) }
	set(shellKey(file, "docBefore"), m.docBefore)
	set(shellKey(file, "docAfter"), m.docAfter)
	set(shellKey(file, "pre"), m.pre)
	set(shellKey(file, "post"), m.post)
	set(shellKey(file, "root"), m.root)
	set(shellKey(file, "htmlAttrs"), attrsJSON(m.htmlAttrs))
	for _, n := range m.nodes {
		r.putNode(txn, file, n)
	}
}

// putNode writes the entries of one node of a frame.
func (r *Room) putNode(txn *crdt.Transaction, file string, n node) {
	set := func(key string, v string) { r.m.Set(txn, key, v) }
	set(nodeKey(file, n.id, "kind"), n.kind)
	set(nodeKey(file, n.id, "parent"), n.parent)
	set(nodeKey(file, n.id, "order"), n.order)
	switch n.kind {
	case kindElement:
		set(nodeKey(file, n.id, "tag"), n.tag)
		set(nodeKey(file, n.id, "ns"), n.ns)
		set(nodeKey(file, n.id, "attrs"), attrsJSON(n.attrs))
		set(nodeKey(file, n.id, "rest"), n.rest)
		props := make([]string, 0, len(n.style))
		for prop := range n.style {
			props = append(props, prop)
		}
		sort.Strings(props) // the same room is the same bytes every time
		for _, prop := range props {
			set(styleKey(file, n.id, prop), n.style[prop])
		}
		if n.hasRich {
			t := crdt.NewTextPrelim()
			r.m.Set(txn, nodeKey(file, n.id, "rich"), t)
			putRich(txn, t, n.rich)
		}
	case kindText:
		t := crdt.NewTextPrelim()
		r.m.Set(txn, nodeKey(file, n.id, "text"), t)
		t.Insert(txn, 0, n.text, nil)
	case kindComment:
		set(nodeKey(file, n.id, "data"), n.text)
	}
}

// keysOf are the keys of a frame (every entry that starts with its file and a bar).
func keysOf(e map[string]any, file string) []string {
	var keys []string
	for k := range e {
		if strings.HasPrefix(k, file+"|") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func frameFiles(e map[string]any) []string {
	seen := map[string]bool{}
	for k := range e {
		if file, rest, ok := strings.Cut(k, "|"); ok && strings.HasPrefix(rest, "shell|") {
			seen[file] = true
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// Write is the change that makes the room hold d, for Hub.Edit, which runs
// the plan (this) before the write. A frame the room already holds as d has it
// is left alone, so what people are doing in it goes on; a frame that differs is
// replaced as a whole, and a frame d lacks is removed.
func (r *Room) Write(d Design) (func(*crdt.Transaction), error) {
	if r.writing.Load() {
		return nil, ErrReadWhileWriting
	}
	e := r.entries()
	current, err := r.Read()
	if err != nil {
		current = Design{Frames: map[string]string{}} // a room that holds what no design can is replaced, not repaired
	}
	models := map[string]frameModel{}
	for file, text := range d.Frames {
		if cur, ok := current.Frames[file]; ok && cur == text {
			continue
		}
		m, err := modelOf(text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		models[file] = m
	}
	var drop []string
	for _, file := range frameFiles(e) {
		if _, kept := d.Frames[file]; !kept {
			drop = append(drop, keysOf(e, file)...)
		} else if _, replaced := models[file]; replaced {
			drop = append(drop, keysOf(e, file)...)
		}
	}
	structure, err := r.setStructure(e, d.Manifest)
	if err != nil {
		return nil, err
	}
	return func(txn *crdt.Transaction) {
		r.writing.Store(true)
		defer r.writing.Store(false)
		for _, k := range drop {
			r.m.Delete(txn, k)
		}
		structure(txn)
		files := make([]string, 0, len(models))
		for f := range models {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			r.putFrame(txn, f, models[f])
		}
	}, nil
}

// State is the encoded state of a new document that holds d, for a room that
// starts from it.
func State(d Design) ([]byte, error) {
	doc := crdt.New()
	defer doc.Destroy()
	r := Open(doc)
	write, err := r.Write(d)
	if err != nil {
		return nil, err
	}
	doc.Transact(write)
	return crdt.EncodeStateAsUpdateV1(doc, nil), nil
}

// SetStyle is the change that sets an inline style property of an element that
// the editor owns; an empty value removes the declaration. Each property is an
// entry of its own: two people setting different ones keep both, and for the same
// one the later write in the room's order wins for everyone.
func (r *Room) SetStyle(file, id, prop, value string) (func(*crdt.Transaction), error) {
	if !Owned(prop) {
		return nil, fmt.Errorf("the style property %q is not one the editor owns; it stays in the element's own markup", prop)
	}
	if _, ok := r.m.Get(nodeKey(file, id, "kind")); !ok {
		return nil, fmt.Errorf("%s has no element %s", file, id)
	}
	if err := ValidateStyleValue(prop, value); err != nil {
		return nil, err
	}

	key := styleKey(file, id, prop)
	overrideKey := nodeKey(file, id, "override|"+prop)
	rest, _ := r.m.Get(nodeKey(file, id, "rest"))
	if _, exists := r.m.Get(overrideKey); exists || coveredSlot(str(rest), prop) {
		key = overrideKey
	}
	return func(txn *crdt.Transaction) { r.m.Set(txn, key, value) }, nil
}

// EditText is the change that makes a text node hold text, changing only what
// differs: from the first character that does not match to the last, so what
// someone else is typing in the same text, outside that range, stays.
func (r *Room) EditText(file, id, text string) (func(*crdt.Transaction), error) {
	v, ok := r.m.Get(nodeKey(file, id, "text"))
	t, isText := v.(*crdt.YText)
	if !ok || !isText {
		return nil, fmt.Errorf("%s has no text %s", file, id)
	}
	old := utf16.Encode([]rune(t.ToString()))
	next := utf16.Encode([]rune(text))
	start := 0
	for start < len(old) && start < len(next) && old[start] == next[start] {
		start++
	}
	endOld, endNew := len(old), len(next)
	for endOld > start && endNew > start && old[endOld-1] == next[endNew-1] {
		endOld--
		endNew--
	}
	insert := string(utf16.Decode(next[start:endNew]))
	return func(txn *crdt.Transaction) {
		if endOld > start {
			t.Delete(txn, start, endOld-start)
		}
		if insert != "" {
			t.Insert(txn, start, insert, nil)
		}
	}, nil
}

// Move is the change that puts an element under parent at the given position
// among its children (0 is first). It writes the element's parent and order and
// nothing else, so an edit to the element's text or style made meanwhile stays.
// A move that would make the element a descendant of itself is ErrCycle, and
// nothing changes.
func (r *Room) Move(file, id, parent string, index int) (func(*crdt.Transaction), error) {
	if r.writing.Load() {
		return nil, ErrReadWhileWriting
	}
	m, err := r.model(r.entries(), file)
	if err != nil {
		return nil, err
	}
	if id == m.root {
		return nil, fmt.Errorf("%s: the body cannot be moved", file)
	}
	if err := checkMove(m, file, id, parent); err != nil {
		return nil, err
	}
	order := orderAt(m, id, parent, index)
	return func(txn *crdt.Transaction) {
		r.m.Set(txn, nodeKey(file, id, "parent"), parent)
		r.m.Set(txn, nodeKey(file, id, "order"), order)
	}, nil
}

// checkMove refuses a move of something that is not there, into something that
// cannot hold it, or into itself.
func checkMove(m frameModel, file, id, parent string) error {
	parents := map[string]string{}
	byID := map[string]node{}
	for _, n := range m.nodes {
		parents[n.id] = n.parent
		byID[n.id] = n
	}
	if _, ok := byID[id]; !ok {
		return fmt.Errorf("%s has no node %s", file, id)
	}
	if p, ok := byID[parent]; !ok || p.kind != kindElement {
		return fmt.Errorf("%s has no element %s to move into", file, parent)
	}
	parents[id] = parent
	if inCycle(parents, parent, m.root) || inCycle(parents, id, m.root) {
		return fmt.Errorf("%w: %s into %s in %s", ErrCycle, id, parent, file)
	}
	return nil
}

// orderAt is an order key that puts id at index among the children of parent.
func orderAt(m frameModel, id, parent string, index int) string {
	var sibs []node
	for _, n := range m.nodes {
		if n.parent == parent && n.id != id {
			sibs = append(sibs, n)
		}
	}
	sort.Slice(sibs, func(i, j int) bool {
		if sibs[i].order != sibs[j].order {
			return sibs[i].order < sibs[j].order
		}
		return sibs[i].id < sibs[j].id
	})
	index = max(0, min(index, len(sibs)))
	lo, hi := "", ""
	if index > 0 {
		lo = sibs[index-1].order
	}
	if index < len(sibs) {
		hi = sibs[index].order
	}
	return between(lo, hi)
}

// Delete is the change that removes an element and everything inside it: every
// entry of each of those nodes, so nothing of them is left to read back or to
// save. The body cannot be deleted. Whatever someone does to a node meanwhile
// (a style entry written after this was planned) is an entry without a kind,
// which reading ignores; an element someone moved into it meanwhile is kept at
// the end of the body (see settle).
func (r *Room) Delete(file, id string) (func(*crdt.Transaction), error) {
	if r.writing.Load() {
		return nil, ErrReadWhileWriting
	}
	e := r.entries()
	m, err := r.model(e, file)
	if err != nil {
		return nil, err
	}
	if id == m.root {
		return nil, fmt.Errorf("%s: the body cannot be deleted", file)
	}
	children := map[string][]string{}
	present := false
	for _, n := range m.nodes {
		children[n.parent] = append(children[n.parent], n.id)
		present = present || n.id == id
	}
	if !present {
		return nil, fmt.Errorf("%s has no node %s", file, id)
	}
	gone := map[string]bool{}
	for pending := []string{id}; len(pending) > 0; {
		next := pending[len(pending)-1]
		pending = append(pending[:len(pending)-1], children[next]...)
		gone[next] = true
	}
	var keys []string
	for id := range gone {
		prefix := nodeKey(file, id, "")
		for k := range e {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
	}
	return func(txn *crdt.Transaction) {
		for _, k := range keys {
			r.m.Delete(txn, k)
		}
	}, nil
}
