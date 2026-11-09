package docfmt

import (
	"encoding/json"
)

// maxMatchCells bounds the work of matching two long documents: above it the
// blocks between the unchanged ends are paired by position only.
const maxMatchCells = 4 << 20

// KeepIDs gives the tabs and blocks of doc the ids of the ones they replace in
// previous, so that republishing a document does not detach what refers to its
// blocks. A block keeps its id when its content is the same, found by matching
// the two lists in order (longest common subsequence of the content, so an
// inserted or deleted block shifts nothing else). A block between two matched
// ones that has the same type as the one it stands in for is that block edited
// and keeps its id too. Tabs match by title, then by position.
func KeepIDs(doc *Doc, previous Doc) {
	used := make([]bool, len(previous.Tabs))
	match := make([]int, len(doc.Tabs))
	for i := range match {
		match[i] = -1
	}
	for i, t := range doc.Tabs {
		for j, p := range previous.Tabs {
			if !used[j] && p.Title == t.Title {
				used[j], match[i] = true, j
				break
			}
		}
	}
	for i := range doc.Tabs {
		if match[i] < 0 && i < len(previous.Tabs) && !used[i] {
			used[i], match[i] = true, i
		}
	}
	for i := range doc.Tabs {
		if match[i] >= 0 {
			doc.Tabs[i].ID = previous.Tabs[match[i]].ID
			keepBlockIDs(doc.Tabs[i].Blocks, previous.Tabs[match[i]].Blocks)
		}
	}
}

func keepBlockIDs(fresh, old []Block) {
	fk, ok := keys(fresh), keys(old)
	matchedF, matchedO := make([]bool, len(fresh)), make([]bool, len(old))
	pairs := lcs(fk, ok)
	for _, p := range pairs {
		copyIDs(&fresh[p[0]], old[p[1]])
		matchedF[p[0]], matchedO[p[1]] = true, true
	}
	// A block that moved is not in the common subsequence but is still the same block.
	moved := map[string][]int{}
	for j := range old {
		if !matchedO[j] {
			moved[ok[j]] = append(moved[ok[j]], j)
		}
	}
	for i := range fresh {
		if js := moved[fk[i]]; !matchedF[i] && len(js) > 0 {
			copyIDs(&fresh[i], old[js[0]])
			matchedF[i], matchedO[js[0]] = true, true
			moved[fk[i]] = js[1:]
		}
	}
	prevF, prevO := 0, 0
	for _, p := range append(pairs, [2]int{len(fresh), len(old)}) {
		pairGap(fresh, old, matchedF, matchedO, [2]int{prevF, p[0]}, [2]int{prevO, p[1]})
		prevF, prevO = p[0]+1, p[1]+1
	}
}

// pairGap pairs the blocks that are still unmatched between two matches, one to
// one and in order, while their types agree.
func pairGap(fresh, old []Block, matchedF, matchedO []bool, gapF, gapO [2]int) {
	var fi, oi []int
	for i := gapF[0]; i < gapF[1]; i++ {
		if !matchedF[i] {
			fi = append(fi, i)
		}
	}
	for j := gapO[0]; j < gapO[1]; j++ {
		if !matchedO[j] {
			oi = append(oi, j)
		}
	}
	for k := 0; k < len(fi) && k < len(oi); k++ {
		f, o := &fresh[fi[k]], old[oi[k]]
		if f.Type != o.Type {
			continue
		}
		f.ID = o.ID
		if f.Type == Quote {
			keepBlockIDs(f.Blocks, o.Blocks)
		}
	}
}

// copyIDs copies the ids of a block with the same content, nested blocks too.
func copyIDs(dst *Block, src Block) {
	dst.ID = src.ID
	for i := range dst.Blocks {
		copyIDs(&dst.Blocks[i], src.Blocks[i])
	}
}

// keys is the content of each block with ids left out, so equal content is an
// equal string.
func keys(blocks []Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = contentKey(b)
	}
	return out
}

func contentKey(b Block) string {
	b = withoutIDs(b)
	raw, _ := json.Marshal(b)
	return string(raw)
}

func withoutIDs(b Block) Block {
	b.ID = ""
	if len(b.Blocks) > 0 {
		kids := make([]Block, len(b.Blocks))
		for i, k := range b.Blocks {
			kids[i] = withoutIDs(k)
		}
		b.Blocks = kids
	}
	return b
}

// lcs returns the index pairs of a longest common subsequence of a and b, in
// order. The ends the two share are taken first, which is nearly always most of
// the work.
func lcs(a, b []string) [][2]int {
	lo := 0
	for lo < len(a) && lo < len(b) && a[lo] == b[lo] {
		lo++
	}
	ha, hb := len(a), len(b)
	for ha > lo && hb > lo && a[ha-1] == b[hb-1] {
		ha--
		hb--
	}
	var pairs [][2]int
	for i := 0; i < lo; i++ {
		pairs = append(pairs, [2]int{i, i})
	}
	if (ha-lo)*(hb-lo) <= maxMatchCells {
		for _, p := range middle(a[lo:ha], b[lo:hb]) {
			pairs = append(pairs, [2]int{p[0] + lo, p[1] + lo})
		}
	}
	for i := 0; ha+i < len(a); i++ {
		pairs = append(pairs, [2]int{ha + i, hb + i})
	}
	return pairs
}

// middle is the dynamic-programming LCS of two short lists.
func middle(a, b []string) [][2]int {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	w := len(b) + 1
	table := make([]uint16, (len(a)+1)*w)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i*w+j] = table[(i+1)*w+j+1] + 1
			} else {
				table[i*w+j] = max(table[(i+1)*w+j], table[i*w+j+1])
			}
		}
	}
	var pairs [][2]int
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case table[(i+1)*w+j] >= table[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}
