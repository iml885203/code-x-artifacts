package designroom

import (
	"sort"
	"unicode/utf16"

	"github.com/reearth/ygo/crdt"
)

// A slot is where a piece of text sits among the elements around it that the
// version, the agent's frame and the room all have: after which of them, and
// which of the texts there it is. Text nodes have no id that means the same in
// the three, so they are matched by slot.
type slot struct {
	after string
	kind  string
	i     int
}

func slotsOf(kids []node, anchors map[string]bool) (map[slot]node, map[string]slot) {
	out := map[slot]node{}
	where := map[string]slot{}
	after := ""
	counts := map[string]int{}
	for _, k := range kids {
		if k.kind == kindElement {
			if anchors[k.id] {
				after, counts = k.id, map[string]int{}
			}
			continue
		}
		at := slot{after, k.kind, counts[k.kind]}
		out[at], where[k.id] = k, at
		counts[k.kind]++
	}
	return out, where
}

func sortedSlots(m map[slot]node) []slot {
	out := make([]slot, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.after != b.after {
			return a.after < b.after
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.i < b.i
	})
	return out
}

// mergeText writes the agent's change to a text (or comment) into the room's.
// The change is the range between the version's text and the agent's; it
// applies where the room's text still has the same words around it.
func (f *frameMerge) mergeText(was, now, room node) error {
	f.touch(room.parent)
	if room.kind == kindComment {
		f.set(room.id, "data", now.text)
		return nil
	}
	if room.text == now.text {
		return nil
	}
	start, delLen, insert, ok := textChange(was.text, now.text, room.text)
	if !ok {
		return conflict("%s: someone changed the words you changed in %s meanwhile; read the design again", f.file, room.parent)
	}
	key := nodeKey(f.file, room.id, "text")
	v, there := f.r.m.Get(key)
	t, isText := v.(*crdt.YText)
	f.step(func(txn *crdt.Transaction) {
		if !there || !isText {
			nt := crdt.NewTextPrelim()
			f.r.m.Set(txn, key, nt)
			nt.Insert(txn, 0, now.text, nil)
			return
		}
		if delLen > 0 {
			t.Delete(txn, start, delLen)
		}
		if insert != "" {
			t.Insert(txn, start, insert, nil)
		}
	})
	return nil
}

// textChange is the edit that applies the agent's change to a text (the version's
// text, was, became now) to the room's text, which people may have changed since:
// the first position, how many UTF-16 units to remove and what to put there. Each
// of the two changes is the one range between the texts' common beginning and
// end; they apply together when the ranges do not overlap, so words typed before
// or after the agent's are kept. ok is false when they overlap.
func textChange(was, now, room string) (start, remove int, insert string, ok bool) {
	w, n, r := utf16.Encode([]rune(was)), utf16.Encode([]rune(now)), utf16.Encode([]rune(room))
	a0, a1, a2 := changedRange(w, n)
	if equalUnits(w, r) {
		return a0, a1 - a0, string(utf16.Decode(n[a0:a2])), true
	}
	r0, r1, r2 := changedRange(w, r)
	switch {
	case a1 <= r0:
		return a0, a1 - a0, string(utf16.Decode(n[a0:a2])), true
	case r1 <= a0:
		return a0 + (r2 - r0) - (r1 - r0), a1 - a0, string(utf16.Decode(n[a0:a2])), true
	}
	return 0, 0, "", false
}

// changedRange is where a text became another: [start, end) of the first is
// what was replaced, and the second's [start, endNow) is what replaced it.
func changedRange(was, now []uint16) (start, end, endNow int) {
	for start < len(was) && start < len(now) && was[start] == now[start] {
		start++
	}
	end, endNow = len(was), len(now)
	for end > start && endNow > start && was[end-1] == now[endNow-1] {
		end--
		endNow--
	}
	return start, end, endNow
}

func equalUnits(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
