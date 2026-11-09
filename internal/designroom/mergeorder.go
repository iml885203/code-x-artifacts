package designroom

import (
	"github.com/reearth/ygo/crdt"
)

// reordered are the elements that stay under the same parent but were put in a
// different order by the agent: the ones outside the longest run of siblings
// that is in the same order in both.
func (f *frameMerge) reordered(bk, nk []node) map[string]bool {
	var was, now []string
	inNow := map[string]bool{}
	for _, k := range nk {
		if k.kind == kindElement {
			if b, ok := f.bi[k.id]; ok && b.parent == k.parent {
				inNow[k.id] = true
				now = append(now, k.id)
			}
		}
	}
	for _, k := range bk {
		if k.kind == kindElement && inNow[k.id] {
			was = append(was, k.id)
		}
	}
	keep := lcs(was, now)
	out := map[string]bool{}
	for _, id := range now {
		if !keep[id] {
			out[id] = true
		}
	}
	return out
}

// lcs is the elements of a longest common subsequence of two lists of distinct ids.
func lcs(a, b []string) map[string]bool {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	out := map[string]bool{}
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			out[a[i]] = true
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// place gives the new and moved children of parent their keys, between the
// children the agent left where they were, and writes them.
func (f *frameMerge) place(parent string, items []item) {
	lo := ""
	for i := range items {
		it := &items[i]
		if it.stable {
			if it.key > lo {
				lo = it.key
			}
			continue
		}
		hi := ""
		for _, later := range items[i+1:] {
			if later.stable && later.key > lo {
				hi = later.key
				break
			}
		}
		it.key = between(lo, hi)
		lo = it.key
	}
	for _, it := range items {
		switch {
		case it.stable:
		case it.moved:
			f.set(it.roomID, "parent", parent)
			f.set(it.roomID, "order", it.key)
			f.parents[it.roomID] = parent
			f.placed = append(f.placed, it.roomID)
			f.touch(it.roomID)
		case it.element:
			n := it.node
			n.parent, n.order = parent, it.key
			f.addNode(n)
			f.parents[n.id] = parent
			f.placed = append(f.placed, n.id)
			f.touch(n.id)
		default:
			n := it.node
			n.id, n.parent, n.order = f.freshID(n.kind), parent, it.key
			f.addNode(n)
			f.touch(parent)
		}
	}
}

func (f *frameMerge) addNode(n node) {
	f.step(func(txn *crdt.Transaction) { f.r.putNode(txn, f.file, n) })
}
