package designroom

import (
	"reflect"
	"sort"
	"unicode/utf16"

	"github.com/reearth/ygo/crdt"
)

// The formatted text of an element is merged the way the editor writes it
// (web/src/design/text/diff.ts): the least that differs between what the agent
// read and what it sent, which is one stretch replaced and the formats that
// changed on what stays, written into the room's text where that stretch is
// now. The same words changed by someone else meanwhile are a conflict.

// unit is one position of a formatted text: a UTF-16 code unit or an embed,
// with the formats it is inside.
type unit struct {
	c     uint16
	embed map[string]any
	attrs map[string]any
}

func unitsOf(ops []richOp) []unit {
	var out []unit
	for _, op := range ops {
		if op.embed != nil {
			out = append(out, unit{embed: op.embed, attrs: op.attrs})
			continue
		}
		for _, c := range utf16.Encode([]rune(op.text)) {
			out = append(out, unit{c: c, attrs: op.attrs})
		}
	}
	return out
}

func sameUnit(a, b unit) bool { return a.c == b.c && reflect.DeepEqual(a.embed, b.embed) }

func sameFormats(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func sameUnits(a, b []unit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameUnit(a[i], b[i]) || !sameFormats(a[i].attrs, b[i].attrs) {
			return false
		}
	}
	return true
}

func isHigh(c uint16) bool { return c >= 0xd800 && c <= 0xdbff }
func isLow(c uint16) bool  { return c >= 0xdc00 && c <= 0xdfff }

// unitRange is where one text became another: [start, end) of the first is what
// was replaced and the second's [start, endNow) is what replaced it. A character
// of two units is kept or replaced whole.
func unitRange(was, now []unit) (start, end, endNow int) {
	most := min(len(was), len(now))
	for start < most && sameUnit(was[start], now[start]) {
		start++
	}
	if start > 0 && start < most && isHigh(was[start-1].c) {
		start--
	}
	tail := 0
	for tail < most-start && sameUnit(was[len(was)-1-tail], now[len(now)-1-tail]) {
		tail++
	}
	if tail > 0 && isLow(was[len(was)-tail].c) {
		tail--
	}
	return start, len(was) - tail, len(now) - tail
}

// formatChange sets (a value) or takes away (nil) formats on units of the text
// as it was.
type formatChange struct {
	index int
	attrs crdt.Attributes
}

// changedFormats are the formats that differ on the units of was that stay in now.
func changedFormats(was, now []unit, start, end int) []formatChange {
	var out []formatChange
	keep := func(from, to, shift int) {
		for i := from; i < to; i++ {
			change := crdt.Attributes{}
			for k, v := range now[i+shift].attrs {
				if was[i].attrs[k] != v {
					change[k] = v
				}
			}
			for k := range was[i].attrs {
				if _, ok := now[i+shift].attrs[k]; !ok {
					change[k] = nil
				}
			}
			if len(change) > 0 {
				out = append(out, formatChange{i, change})
			}
		}
	}
	keep(0, start, 0)
	keep(end, len(was), len(now)-len(was))
	return out
}

// richEdit is the change to make to the room's text: formats on stretches of
// it, then [from, to) replaced by insert.
type richEdit struct {
	formats  []formatChange
	from, to int
	insert   []unit
}

// placeRichChange works out where the agent's change (b became n) goes in the
// room's text r, which people may have changed since b. ok is false when their
// change and the agent's overlap.
func placeRichChange(b, n, r []unit) (e richEdit, ok bool) {
	start, end, endNow := unitRange(b, n)
	r0, r1, _ := unitRange(b, r)
	delta := len(r) - len(b)
	if sameUnits(unitsPlain(r), unitsPlain(b)) {
		r0, r1, delta = len(b)+1, len(b)+1, 0 // the room's words are the version's: every position is where it was
	} else if start < r1 && r0 < end {
		return e, false
	}
	at := func(i int, before bool) (int, bool) {
		switch {
		case i < r0 || (before && i == r0):
			return i, true
		case i >= r1:
			return i + delta, true
		}
		return 0, false
	}
	for _, c := range changedFormats(b, n, start, end) {
		p, ok1 := at(c.index, true)
		q, ok2 := at(c.index+1, false)
		if !ok1 || !ok2 {
			return e, false
		}
		if q > p {
			e.formats = append(e.formats, formatChange{p, c.attrs})
		}
	}
	from, ok1 := at(start, true)
	to, ok2 := at(end, false)
	if !ok1 || !ok2 || to < from {
		return e, false
	}
	e.from, e.to, e.insert = from, to, n[start:endNow]
	return e, true
}

// mergeRichText writes the agent's change to an element's formatted text into the
// room's, or says it cannot.
func (f *frameMerge) mergeRichText(id string, was, now, room []richOp) error {
	b, n, r := unitsOf(was), unitsOf(now), unitsOf(room)
	if sameUnits(r, n) {
		return nil
	}
	e, ok := placeRichChange(b, n, r)
	if !ok {
		return conflict("%s: someone changed the words you changed in %s meanwhile; read the design again", f.file, id)
	}
	around := map[string]bool{}
	for _, i := range []int{e.from - 1, e.to} {
		if i >= 0 && i < len(r) {
			for k := range r[i].attrs {
				around[k] = true
			}
		}
	}
	v, _ := f.r.m.Get(nodeKey(f.file, id, "rich"))
	t, isText := v.(*crdt.YText)
	if !isText {
		return nil
	}
	f.step(func(txn *crdt.Transaction) {
		for _, c := range e.formats {
			t.Format(txn, c.index, 1, c.attrs)
		}
		if e.to > e.from {
			t.Delete(txn, e.from, e.to-e.from)
		}
		writeUnits(txn, t, e.from, e.insert, around)
	})
	f.touch(id)
	return nil
}

func unitsPlain(u []unit) []unit {
	out := make([]unit, len(u))
	for i, x := range u {
		out[i] = unit{c: x.c, embed: x.embed}
	}
	return out
}

// writeUnits inserts units at a position as runs of one set of formats, each
// carrying exactly its own: the formats around the position are taken off.
func writeUnits(txn *crdt.Transaction, t *crdt.YText, at int, units []unit, around map[string]bool) {
	for i := 0; i < len(units); {
		j := i + 1
		for j < len(units) && units[i].embed == nil && units[j].embed == nil && sameFormats(units[i].attrs, units[j].attrs) {
			j++
		}
		attrs := crdt.Attributes{}
		for k := range around {
			attrs[k] = nil
		}
		keys := make([]string, 0, len(units[i].attrs))
		for k := range units[i].attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			attrs[k] = units[i].attrs[k]
		}
		if units[i].embed != nil {
			t.InsertEmbed(txn, at, units[i].embed, attrs)
			at++
		} else {
			run := make([]uint16, 0, j-i)
			for _, u := range units[i:j] {
				run = append(run, u.c)
			}
			s := string(utf16.Decode(run))
			t.Insert(txn, at, s, attrs)
			at += len(run)
		}
		i = j
	}
}
