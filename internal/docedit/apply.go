package docedit

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Apply does the operations in order on a copy of the doc and returns the
// result. Every operation must be possible and every guard must hold, and the
// result must be a valid doc, or nothing is returned: the caller never sees a
// half-done edit. Guards are checked against the doc as it was before the call.
func Apply(doc docfmt.Doc, ops []Op, newID func(prefix string) string) (docfmt.Doc, Result, error) {
	if len(ops) == 0 {
		return doc, Result{}, &Refusal{Msg: "ops is empty: send at least one operation"}
	}
	if len(ops) > MaxOps {
		return doc, Result{}, &Refusal{Msg: fmt.Sprintf("a call can carry at most %d operations, this one has %d", MaxOps, len(ops))}
	}
	if err := checkGuards(doc, ops); err != nil {
		return doc, Result{}, err
	}
	work, err := clone(doc)
	if err != nil {
		return doc, Result{}, err
	}
	var res Result
	written := map[string]bool{}
	tabbed := false
	for i, op := range ops {
		if err := do(&work, op, i, newID, &res, written); err != nil {
			return doc, Result{}, err
		}
		tabbed = tabbed || isTabOp(op.Op)
	}
	if tabbed {
		res.Tabs = tabRefs(work)
	}
	for _, t := range work.Tabs {
		collect(t.Blocks, written, &res)
	}
	content, err := docfmt.Canonical(work)
	if err != nil {
		return doc, Result{}, &Refusal{Msg: "the result is not a valid doc: " + err.Error()}
	}
	if err := docfmt.Validate(content); err != nil {
		return doc, Result{}, &Refusal{Msg: "the result is not a valid doc, so nothing was changed: " + err.Error()}
	}
	out, err := docfmt.Parse(content)
	if err != nil {
		return doc, Result{}, err
	}
	return out, res, nil
}

func clone(d docfmt.Doc) (docfmt.Doc, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return d, err
	}
	var out docfmt.Doc
	return out, json.Unmarshal(raw, &out)
}

// checkGuards refuses the call when a block named with an ifHash is gone or is
// not what the hash says, and then says what every such block is now.
func checkGuards(doc docfmt.Doc, ops []Op) error {
	var stale []State
	for i, op := range ops {
		if op.IfHash == "" || isTabOp(op.Op) {
			continue
		}
		id := op.ID
		if op.Op == Insert {
			id = op.Before + op.After
		}
		b, tab, ok := find(doc, id)
		if !ok {
			return refuse(i, "ifHash is for block %q, which is not in the doc as it was before this call; read the doc again", id)
		}
		if Hash(b) != op.IfHash {
			stale = append(stale, State{Hashed: Hashed{Block: b, Hash: Hash(b)}, Tab: tab})
		}
	}
	if len(stale) == 0 {
		return nil
	}
	ids := make([]string, len(stale))
	for i, s := range stale {
		ids[i] = s.ID
	}
	return &Stale{
		Msg:    fmt.Sprintf("nothing was changed: %d block(s) are not what you read: %v. Their current state is attached; merge your change into it and send the call again with the new hashes", len(stale), ids),
		Blocks: stale,
	}
}

// find looks a block up by id in every tab, inside quotes too.
func find(doc docfmt.Doc, id string) (docfmt.Block, string, bool) {
	var walk func(blocks []docfmt.Block) (docfmt.Block, bool)
	walk = func(blocks []docfmt.Block) (docfmt.Block, bool) {
		for _, b := range blocks {
			if b.ID == id {
				return b, true
			}
			if got, ok := walk(b.Blocks); ok {
				return got, true
			}
		}
		return docfmt.Block{}, false
	}
	for _, t := range doc.Tabs {
		if b, ok := walk(t.Blocks); ok {
			return b, t.ID, true
		}
	}
	return docfmt.Block{}, "", false
}

// place is where a block is: the slice that holds it and its index.
type place struct {
	list *[]docfmt.Block
	at   int
}

func locate(doc *docfmt.Doc, id string) (place, bool) {
	var walk func(list *[]docfmt.Block) (place, bool)
	walk = func(list *[]docfmt.Block) (place, bool) {
		for i := range *list {
			if (*list)[i].ID == id {
				return place{list, i}, true
			}
			if p, ok := walk(&(*list)[i].Blocks); ok {
				return p, true
			}
		}
		return place{}, false
	}
	for i := range doc.Tabs {
		if p, ok := walk(&doc.Tabs[i].Blocks); ok {
			return p, true
		}
	}
	return place{}, false
}

func do(doc *docfmt.Doc, op Op, i int, newID func(string) string, res *Result, written map[string]bool) error {
	switch op.Op {
	case TabAdd, TabRename, TabDelete, TabMove:
		return doTab(doc, op, i, newID, res, written)
	case Delete:
		p, ok := locate(doc, op.ID)
		if !ok {
			return refuse(i, "there is no block %q to delete; read the doc to see the ids", op.ID)
		}
		res.Deleted = append(res.Deleted, op.ID)
		*p.list = slices.Delete(*p.list, p.at, p.at+1)
		return nil
	case Replace:
		p, ok := locate(doc, op.ID)
		if !ok {
			return refuse(i, "there is no block %q to replace; read the doc to see the ids", op.ID)
		}
		blocks, err := newBlocks(op, i, op.ID, newID)
		if err != nil {
			return err
		}
		for _, b := range blocks {
			written[b.ID] = true
		}
		if blocks[0].ID != op.ID {
			res.Deleted = append(res.Deleted, op.ID)
		}
		*p.list = slices.Replace(*p.list, p.at, p.at+1, blocks...)
		return nil
	case Insert:
		blocks, err := newBlocks(op, i, "", newID)
		if err != nil {
			return err
		}
		for _, b := range blocks {
			written[b.ID] = true
		}
		return insert(doc, op, i, blocks)
	case "":
		return refuse(i, "op is missing: it is %q, %q, %q, %q, %q, %q or %q", Replace, Insert, Delete, TabAdd, TabRename, TabDelete, TabMove)
	}
	return refuse(i, "unknown op %q: it is %q, %q, %q, %q, %q, %q or %q", op.Op, Replace, Insert, Delete, TabAdd, TabRename, TabDelete, TabMove)
}

func insert(doc *docfmt.Doc, op Op, i int, blocks []docfmt.Block) error {
	places := 0
	for _, set := range []bool{op.Before != "", op.After != "", op.Tab != ""} {
		if set {
			places++
		}
	}
	if places != 1 {
		return refuse(i, "an insert says where with exactly one of before (a block id), after (a block id) or tab with at (start or end)")
	}
	switch {
	case op.Before != "":
		p, ok := locate(doc, op.Before)
		if !ok {
			return refuse(i, "there is no block %q to insert before; read the doc to see the ids", op.Before)
		}
		*p.list = slices.Insert(*p.list, p.at, blocks...)
	case op.After != "":
		p, ok := locate(doc, op.After)
		if !ok {
			return refuse(i, "there is no block %q to insert after; read the doc to see the ids", op.After)
		}
		*p.list = slices.Insert(*p.list, p.at+1, blocks...)
	default:
		return insertInTab(doc, op, i, blocks)
	}
	return nil
}

func insertInTab(doc *docfmt.Doc, op Op, i int, blocks []docfmt.Block) error {
	if op.At != "start" && op.At != "end" {
		return refuse(i, "with tab, at is %q or %q", "start", "end")
	}
	for t := range doc.Tabs {
		if doc.Tabs[t].ID != op.Tab {
			continue
		}
		if op.At == "start" {
			doc.Tabs[t].Blocks = slices.Insert(doc.Tabs[t].Blocks, 0, blocks...)
		} else {
			doc.Tabs[t].Blocks = append(doc.Tabs[t].Blocks, blocks...)
		}
		return nil
	}
	return refuse(i, "there is no tab %q; the tabs are in the read", op.Tab)
}

// collect adds to the result every block the call wrote that is still in the
// doc, in the order of the doc, and a review note for every image of the doc
// that has no alt text: a person may have put one in with the editor, and the
// agent's next edit is where it is mentioned.
func collect(blocks []docfmt.Block, written map[string]bool, res *Result) {
	for _, b := range blocks {
		if written[b.ID] {
			res.Blocks = append(res.Blocks, Written{ID: b.ID, Type: b.Type, Hash: Hash(b)})
		}
		if b.Type == docfmt.Image && b.Alt == "" {
			res.Review = append(res.Review, fmt.Sprintf("block %s is an image with no alt text; add alt so it can be read aloud and found", b.ID))
		}
		collect(b.Blocks, written, res)
	}
}
