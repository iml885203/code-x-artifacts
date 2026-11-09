package docedit

import "github.com/iml885203/code-x-artifacts/internal/docfmt"

// Removed lists, in the order of the doc before, the ids of the blocks that are
// in before and not in after, blocks inside quotes included: what an edit took
// away, however it did it (a delete, a replace by blocks with ids of their own,
// a deleted tab).
func Removed(before, after docfmt.Doc) []string {
	kept := map[string]bool{}
	for _, id := range blockIDs(after) {
		kept[id] = true
	}
	var gone []string
	for _, id := range blockIDs(before) {
		if !kept[id] {
			gone = append(gone, id)
		}
	}
	return gone
}

func blockIDs(d docfmt.Doc) []string {
	var ids []string
	var walk func(blocks []docfmt.Block)
	walk = func(blocks []docfmt.Block) {
		for _, b := range blocks {
			ids = append(ids, b.ID)
			walk(b.Blocks)
		}
	}
	for _, t := range d.Tabs {
		walk(t.Blocks)
	}
	return ids
}
