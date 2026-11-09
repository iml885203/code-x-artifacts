package docroom

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Apply returns the change that makes the room hold next and touches only what
// differs, for Hub.Edit: a block that is the same stays as it is, so the people
// typing in it keep their place and their characters. Blocks and tabs are
// matched by id, the items of a list and the rows of a table by their content,
// and the text of a block that changed is edited in place, from the first
// character that differs to the last, so what is typed outside that stretch
// survives. It reads the room, so it belongs in the plan.
func (c *Content) Apply(next docfmt.Doc) (func(*crdt.Transaction), error) {
	next = Canon(next)
	if c.isLegacy() {
		return c.Write(next), nil
	}
	old, err := c.Read()
	if err != nil {
		return nil, err
	}
	p := &planner{}
	oldAt := map[string]int{}
	for i, t := range old.Tabs {
		oldAt[t.ID] = i
	}
	nextIDs := make([]string, len(next.Tabs))
	for n, t := range next.Tabs {
		nextIDs[n] = t.ID
		frag := c.fragment(t.ID)
		o, kept := oldAt[t.ID]
		if !kept {
			p.add(func(w writer) {
				emptyFragment(w, frag)
				frag.InsertElement(w.txn, 0, w.tab(t))
			})
			continue
		}
		el, _ := c.TabElement(t.ID)
		if err := p.tab(el, old.Tabs[o], t); err != nil {
			return nil, err
		}
	}
	for _, t := range old.Tabs {
		if !slices.Contains(nextIDs, t.ID) {
			frag := c.fragment(t.ID)
			p.add(func(w writer) { emptyFragment(w, frag) })
		}
	}
	p.order(c.order, c.rawOrder(), nextIDs)
	return func(txn *crdt.Transaction) {
		c.writing.Store(true)
		defer c.writing.Store(false)
		for _, op := range p.ops {
			op(writer{txn})
		}
	}, nil
}

func emptyFragment(w writer, f *crdt.YXmlFragment) {
	if n := f.Len(); n > 0 {
		f.Delete(w.txn, 0, n)
	}
}

// order plans the change of the array of tab ids. The ids that keep their
// relative order stay; the others are deleted and inserted where they go. It is
// the ids that move, never a tab's content: a tab that moves is one id taken out
// and put back, and what is typed in the tab is not touched.
func (p *planner) order(array *crdt.YArray, old, next []string) {
	matchedOld := map[int]bool{}
	matchedNew := map[int]bool{}
	for _, pr := range lcs(old, next) {
		matchedOld[pr[0]], matchedNew[pr[1]] = true, true
	}
	for o := len(old) - 1; o >= 0; o-- {
		if !matchedOld[o] {
			p.add(func(w writer) { array.Delete(w.txn, o, 1) })
		}
	}
	for n, id := range next {
		if !matchedNew[n] {
			p.add(func(w writer) { array.Insert(w.txn, n, []any{id}) })
		}
	}
}

// planner collects the steps of a change while the room is read; they run, in
// order, inside the transaction.
type planner struct{ ops []func(w writer) }

func (p *planner) add(op func(w writer)) { p.ops = append(p.ops, op) }

// sequence is a list of children in the room and the list it has to become.
type sequence struct {
	parent  *crdt.YXmlFragment
	oldKeys []string
	newKeys []string
	oldEls  []*crdt.YXmlElement
	// update plans the in-place change of the old child o to the new child n.
	update func(o, n int) error
	// build writes the new child n.
	build func(w writer, n int) *crdt.YXmlElement
}

// sequence plans the change of one list of children. The children whose keys
// match, in the longest order they share, stay and are updated; the others are
// deleted, and the new ones inserted at their place in the result.
func (p *planner) sequence(s sequence) error {
	pairs := lcs(s.oldKeys, s.newKeys)
	matchedOld := map[int]bool{}
	matchedNew := map[int]bool{}
	for _, pr := range pairs {
		matchedOld[pr[0]], matchedNew[pr[1]] = true, true
		if err := s.update(pr[0], pr[1]); err != nil {
			return err
		}
	}
	for o := len(s.oldKeys) - 1; o >= 0; o-- {
		if !matchedOld[o] {
			p.add(func(w writer) { s.parent.Delete(w.txn, o, 1) })
		}
	}
	for n := range s.newKeys {
		if !matchedNew[n] {
			p.add(func(w writer) { s.parent.InsertElement(w.txn, n, s.build(w, n)) })
		}
	}
	return nil
}

func (p *planner) tab(e *crdt.YXmlElement, old, next docfmt.Tab) error {
	p.syncString(e, attrTitle, old.Title, next.Title)
	return p.blocks(e, old.Blocks, next.Blocks)
}

// blocks plans the change of the blocks inside parent (a tab or a quote).
func (p *planner) blocks(parent *crdt.YXmlElement, old, next []docfmt.Block) error {
	els, err := elementsOf(&parent.YXmlFragment, "blocks")
	if err != nil {
		return err
	}
	if len(els) != len(old) {
		return fmt.Errorf("<%s> holds %d nodes and %d blocks", parent.NodeName, len(els), len(old))
	}
	return p.sequence(sequence{
		parent:  &parent.YXmlFragment,
		oldKeys: keysOf(len(old), func(i int) string { return old[i].ID + "|" + old[i].Type }),
		newKeys: keysOf(len(next), func(i int) string { return next[i].ID + "|" + next[i].Type }),
		oldEls:  els,
		update: func(o, n int) error {
			if same(old[o], next[n]) {
				return nil
			}
			return p.block(els[o], old[o], next[n])
		},
		build: func(w writer, n int) *crdt.YXmlElement { return w.block(next[n]) },
	})
}

// block plans the in-place change of a block that keeps its id and type.
func (p *planner) block(e *crdt.YXmlElement, old, next docfmt.Block) error {
	switch next.Type {
	case docfmt.Paragraph:
		return p.inlines(e, old.Content, next.Content)
	case docfmt.Heading:
		p.syncInt(e, attrLevel, old.Level, next.Level)
		return p.inlines(e, old.Content, next.Content)
	case docfmt.BulletList, docfmt.NumberedList, docfmt.TaskList:
		p.syncInt(e, attrStart, old.Start, next.Start)
		return p.items(e, next.Type, old.Items, next.Items)
	case docfmt.Quote:
		return p.blocks(e, old.Blocks, next.Blocks)
	case docfmt.Code:
		p.syncString(e, attrLang, old.Lang, next.Lang)
		return p.inlines(e, textRuns(old.Text), textRuns(next.Text))
	case docfmt.Table:
		return p.table(e, old, next)
	case docfmt.Image:
		p.syncString(e, attrSrc, old.Src, next.Src)
		p.syncString(e, attrAlt, old.Alt, next.Alt)
		p.syncString(e, attrSize, old.Size, next.Size)
	case docfmt.Pending:
		p.syncString(e, attrIntent, old.Intent, next.Intent)
	}
	return nil
}

func textRuns(s string) []docfmt.Inline {
	if s == "" {
		return nil
	}
	return []docfmt.Inline{{Text: s}}
}

// items retains field-only changes in place; other changes use content matching.
func (p *planner) items(list *crdt.YXmlElement, typ string, old, next []docfmt.Item) error {
	fields := &planner{}
	if fields.fieldItems(list, old, next) {
		p.ops = append(p.ops, fields.ops...)
		return nil
	}
	els, err := elementsOf(&list.YXmlFragment, "items")
	if err != nil {
		return err
	}
	return p.sequence(sequence{
		parent:  &list.YXmlFragment,
		oldKeys: keysOf(len(old), func(i int) string { return keyOf(old[i]) }),
		newKeys: keysOf(len(next), func(i int) string { return keyOf(next[i]) }),
		oldEls:  els,
		update:  func(int, int) error { return nil },
		build:   func(w writer, n int) *crdt.YXmlElement { return w.item(typ, next[n]) },
	})
}

// table retains field-only changes in place; other changes use row matching.
func (p *planner) table(e *crdt.YXmlElement, old, next docfmt.Block) error {
	if old.Header != next.Header {
		p.syncBool(e, attrHeader, old.Header, next.Header)
	}
	els, err := elementsOf(&e.YXmlFragment, "rows")
	if err != nil {
		return err
	}
	fields := &planner{}
	if fields.fieldRows(els, old.Rows, next.Rows) {
		p.ops = append(p.ops, fields.ops...)
		return nil
	}
	return p.sequence(sequence{
		parent:  &e.YXmlFragment,
		oldKeys: keysOf(len(old.Rows), func(i int) string { return keyOf(old.Rows[i]) }),
		newKeys: keysOf(len(next.Rows), func(i int) string { return keyOf(next.Rows[i]) }),
		oldEls:  els,
		update:  func(int, int) error { return nil },
		build:   func(w writer, n int) *crdt.YXmlElement { return w.row(next.Rows[n]) },
	})
}

func keyOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func same(a, b docfmt.Block) bool { return keyOf(a) == keyOf(b) }

func keysOf(n int, key func(int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = key(i)
	}
	return out
}

func elementsOf(f *crdt.YXmlFragment, what string) ([]*crdt.YXmlElement, error) {
	var out []*crdt.YXmlElement
	for i, child := range f.Children() {
		e, ok := child.(*crdt.YXmlElement)
		if !ok {
			return nil, unexpected(fmt.Sprintf("%s[%d]", what, i), child)
		}
		out = append(out, e)
	}
	return out, nil
}

func (p *planner) syncString(e *crdt.YXmlElement, key, old, next string) {
	switch {
	case old == next:
	case next == "":
		p.add(func(w writer) { e.DeleteAttribute(w.txn, key) })
	default:
		p.add(func(w writer) { e.SetAttribute(w.txn, key, next) })
	}
}

func (p *planner) syncInt(e *crdt.YXmlElement, key string, old, next int) {
	switch {
	case old == next:
	case next == 0:
		p.add(func(w writer) { e.DeleteAttribute(w.txn, key) })
	default:
		p.add(func(w writer) { e.SetAttributeValue(w.txn, key, next) })
	}
}

func (p *planner) syncBool(e *crdt.YXmlElement, key string, old, next bool) {
	switch {
	case old == next:
	case !next:
		p.add(func(w writer) { e.DeleteAttribute(w.txn, key) })
	default:
		p.add(func(w writer) { e.SetAttributeValue(w.txn, key, true) })
	}
}

// inlines plans the text of a paragraph, heading, code block or cell: when it
// is one piece of text in the room and stays one, it is edited in place.
func (p *planner) inlines(e *crdt.YXmlElement, old, next []docfmt.Inline) error {
	if keyOf(old) == keyOf(next) {
		return nil
	}
	if p.fields(e, old, next) {
		return nil
	}
	text, single := onlyText(e, old)
	if !single || hasObject(old) || hasObject(next) {
		p.add(func(w writer) {
			if n := e.Len(); n > 0 {
				e.Delete(w.txn, 0, n)
			}
			w.inlines(e, next)
		})
		return nil
	}
	if len(old) == 0 {
		p.add(func(w writer) { w.inlines(e, next) })
		return nil
	}
	if len(next) == 0 {
		p.add(func(w writer) { e.Delete(w.txn, 0, e.Len()) })
		return nil
	}
	delta := textDelta(old, next)
	p.add(func(w writer) { text.ApplyDelta(w.txn, delta) })
	return nil
}

// onlyText is the one XmlText a text container holds when its text is one
// piece (old is what it reads as), and whether that is how it looks.
func onlyText(e *crdt.YXmlElement, old []docfmt.Inline) (*crdt.YXmlText, bool) {
	kids := e.Children()
	if len(old) == 0 {
		return nil, len(kids) == 0
	}
	if len(kids) != 1 {
		return nil, false
	}
	text, ok := kids[0].(*crdt.YXmlText)
	return text, ok
}

// hasObject says the runs hold something that is not text in the room: a line
// break or a mention.
func hasObject(runs []docfmt.Inline) bool {
	for _, r := range runs {
		if r.Mention() || r.Date != "" || r.Dropdown != nil || strings.Contains(r.Text, "\n") {
			return true
		}
	}
	return false
}

// cell is one character of a text with the run it belongs to.
type cell struct {
	r   rune
	key string
	run int
}

func cells(runs []docfmt.Inline) []cell {
	var out []cell
	for i, run := range runs {
		key := strings.Join(run.Marks, ",") + "|" + run.Href
		for _, r := range run.Text {
			out = append(out, cell{r: r, key: key, run: i})
		}
	}
	return out
}

func (c cell) same(o cell) bool { return c.r == o.r && c.key == o.key }

// units is the length of cs in UTF-16 code units, which is what Yjs counts in.
func units(cs []cell) int {
	n := 0
	for _, c := range cs {
		n++
		if c.r > 0xFFFF {
			n++
		}
	}
	return n
}

// textDelta is the edit that turns the text of old into the text of next: keep
// the characters, marks included, that begin and end both, and replace what is
// between.
func textDelta(old, next []docfmt.Inline) []crdt.Delta {
	a, b := cells(old), cells(next)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix].same(b[prefix]) {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix].same(b[len(b)-1-suffix]) {
		suffix++
	}
	var delta []crdt.Delta
	if n := units(a[:prefix]); n > 0 {
		delta = append(delta, crdt.Delta{Op: crdt.DeltaOpRetain, Retain: n})
	}
	if n := units(a[prefix : len(a)-suffix]); n > 0 {
		delta = append(delta, crdt.Delta{Op: crdt.DeltaOpDelete, Delete: n})
	}
	inserted := b[prefix : len(b)-suffix]
	for i := 0; i < len(inserted); {
		j := i
		var sb strings.Builder
		for j < len(inserted) && inserted[j].run == inserted[i].run {
			sb.WriteRune(inserted[j].r)
			j++
		}
		delta = append(delta, crdt.Delta{Op: crdt.DeltaOpInsert, Insert: sb.String(), Attributes: markAttributes(next[inserted[i].run])})
		i = j
	}
	return delta
}

// lcs matches a and b: the longest common subsequence, as pairs of indexes.
// What both begin and end with is matched first, so the search is only over what
// is between, and a middle too large to search is left unmatched (its items are
// replaced, which is always correct, only less gentle).
func lcs(a, b []string) [][2]int {
	var out [][2]int
	lo := 0
	for lo < len(a) && lo < len(b) && a[lo] == b[lo] {
		out = append(out, [2]int{lo, lo})
		lo++
	}
	endA, endB := len(a), len(b)
	var tail [][2]int
	for endA > lo && endB > lo && a[endA-1] == b[endB-1] {
		endA--
		endB--
		tail = append(tail, [2]int{endA, endB})
	}
	if (endA-lo)*(endB-lo) <= maxSearch {
		for _, pr := range middle(a[lo:endA], b[lo:endB]) {
			out = append(out, [2]int{pr[0] + lo, pr[1] + lo})
		}
	}
	for i := len(tail) - 1; i >= 0; i-- {
		out = append(out, tail[i])
	}
	return out
}

// maxSearch bounds the table of the search for the longest common subsequence.
const maxSearch = 4 << 20

func middle(a, b []string) [][2]int {
	// length[i][j] is the LCS of a[i:] and b[j:].
	length := make([][]int32, len(a)+1)
	for i := range length {
		length[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				length[i][j] = length[i+1][j+1] + 1
			} else {
				length[i][j] = max(length[i+1][j], length[i][j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			out = append(out, [2]int{i, j})
			i++
			j++
		case length[i+1][j] >= length[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}
