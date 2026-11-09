// Package boardroom is a whiteboard in a room: the elements an Excalidraw
// editor draws, one shared-map entry per element field, and the rules the hub
// holds every update to (docs/spikes/board.md). The rules are the board
// format's (internal/boardfmt), so a room holds only what a publisher could
// write.
//
// A field is its own entry so two people changing different fields of one
// element (one moves a shape, another edits its label) both keep their change;
// the same field has one winner, the same on every replica.
package boardroom

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

// Map is the shared map that holds a board. Its keys are "<element id>|<field>"
// and each value is the field's JSON text. An element's version, versionNonce
// and updated are each replica's own bookkeeping and are not kept.
const Map = "board"

func key(id, field string) string { return id + "|" + field }

// Check refuses a room holding a board an editor may not draw. It reads only
// the board's map, so a room of another kind is never refused by it.
func Check(doc *crdt.Doc) error {
	_, err := read(doc.GetMap(Map).Entries())
	return err
}

// Read is the board a room holds, in no particular order and with deleted
// elements still in it (boardfmt.Canonical makes it a version).
func Read(doc *crdt.Doc) (boardfmt.Board, error) {
	byID, err := read(doc.GetMap(Map).Entries())
	if err != nil {
		return boardfmt.Board{}, err
	}
	b := boardfmt.Board{Version: 1, Elements: make([]boardfmt.Element, 0, len(byID))}
	for _, el := range byID {
		b.Elements = append(b.Elements, el)
	}
	return b, nil
}

// read checks every entry and groups them by element.
func read(entries map[string]any) (map[string]boardfmt.Element, error) {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	byID := map[string]boardfmt.Element{}
	for _, k := range keys {
		id, field, ok := strings.Cut(k, "|")
		if !ok || boardfmt.CheckID(id) != nil {
			return nil, fmt.Errorf("board: %q is not an element field (want <element id>|<field>)", k)
		}
		raw, ok := entries[k].(string)
		if !ok {
			return nil, fmt.Errorf("board: %s.%s is not a JSON text", id, field)
		}
		if err := boardfmt.CheckField(id, field, []byte(raw)); err != nil {
			return nil, err
		}
		var v any
		_ = json.Unmarshal([]byte(raw), &v) // CheckField decoded it
		el := byID[id]
		if el == nil {
			el = boardfmt.Element{"id": id}
			byID[id] = el
		}
		el[field] = v
	}
	for _, k := range keys {
		id, _, _ := strings.Cut(k, "|")
		if _, typed := byID[id]["type"]; !typed {
			return nil, fmt.Errorf("board: element %s has fields but no type", id)
		}
	}
	return byID, nil
}

// Write is the change that makes the room hold board b, for Hub.Edit: it
// writes only the fields that differ, so what people change elsewhere in the
// same instant stays. An element the room has and b does not is marked
// deleted rather than removed: an editor keeps an element until it is told it
// is deleted.
func Write(doc *crdt.Doc, b boardfmt.Board) (func(*crdt.Transaction), error) {
	m := doc.GetMap(Map)
	have, err := read(m.Entries())
	p := plan{m: m, have: have, broken: err != nil, set: map[string]string{}}
	want := map[string]bool{}
	for _, el := range b.Elements {
		want[el.ID()] = true
		if err := p.element(el); err != nil {
			return nil, err
		}
	}
	for id, el := range have {
		if !want[id] && el["isDeleted"] != true {
			p.set[key(id, "isDeleted")] = "true"
		}
	}
	if p.broken {
		for _, k := range m.Keys() {
			if _, kept := p.set[k]; !kept {
				p.drop = append(p.drop, k)
			}
		}
	}
	return func(txn *crdt.Transaction) {
		for _, k := range p.drop {
			m.Delete(txn, k)
		}
		for k, v := range p.set {
			m.Set(txn, k, v)
		}
	}, nil
}

// plan is the entries a Write sets and removes.
type plan struct {
	m    *crdt.YMap
	have map[string]boardfmt.Element
	// broken is a room holding what no board can: it is replaced, not repaired.
	broken bool
	set    map[string]string
	drop   []string
}

// element plans the entries of one element of the board being written: the
// fields that differ, isDeleted false, and the removal of fields it no longer has.
func (p *plan) element(el boardfmt.Element) error {
	id := el.ID()
	fields := map[string]any{"isDeleted": false}
	for f, v := range el {
		if f != "id" {
			fields[f] = v
		}
	}
	for f, v := range fields {
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("board: %s.%s: %w", id, f, err)
		}
		if cur, ok := p.m.Get(key(id, f)); p.broken || !ok || cur != string(raw) {
			p.set[key(id, f)] = string(raw)
		}
	}
	for f := range p.have[id] {
		if _, kept := fields[f]; !kept && f != "id" {
			p.drop = append(p.drop, key(id, f))
		}
	}
	return nil
}

// State is the room state that starts a room holding board b.
func State(b boardfmt.Board) ([]byte, error) {
	doc := crdt.New()
	defer doc.Destroy()
	write, err := Write(doc, b)
	if err != nil {
		return nil, err
	}
	doc.Transact(write)
	return crdt.EncodeStateAsUpdateV1(doc, nil), nil
}

// Merge is the change that board next makes to board base, applied to what the
// room holds now: only the fields that differ between base and next are
// written, so what people changed in the room since base (another element, or
// another field of the same one) stays. An element base has and next does not
// is marked deleted. It also says which elements it touched.
func Merge(doc *crdt.Doc, base, next boardfmt.Board) (func(*crdt.Transaction), []string, error) {
	m := doc.GetMap(Map)
	was := map[string]boardfmt.Element{}
	for _, el := range base.Elements {
		was[el.ID()] = el
	}
	set := map[string]string{}
	var drop []string
	touched := map[string]bool{}
	for _, el := range next.Elements {
		id := el.ID()
		old := was[id]
		delete(was, id)
		gone, err := fieldChanges(el, old, set)
		if err != nil {
			return nil, nil, err
		}
		for _, f := range gone {
			drop = append(drop, key(id, f))
		}
		if len(gone) > 0 || old == nil || changedAny(set, id) {
			touched[id] = true
		}
		if old == nil {
			set[key(id, "isDeleted")] = "false"
		}
	}
	for id := range was {
		set[key(id, "isDeleted")] = "true"
		touched[id] = true
	}
	ids := make([]string, 0, len(touched))
	for id := range touched {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return func(txn *crdt.Transaction) {
		for _, k := range drop {
			m.Delete(txn, k)
		}
		for k, v := range set {
			m.Set(txn, k, v)
		}
	}, ids, nil
}

// fieldChanges puts into set every field of el that old does not hold the same,
// and gives the fields old has and el does not.
func fieldChanges(el, old boardfmt.Element, set map[string]string) ([]string, error) {
	id := el.ID()
	for f, v := range el {
		if f == "id" {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("board: %s.%s: %w", id, f, err)
		}
		if prev, ok := old[f]; ok {
			if same, _ := json.Marshal(prev); string(same) == string(raw) {
				continue
			}
		}
		set[key(id, f)] = string(raw)
	}
	var gone []string
	for f := range old {
		if _, kept := el[f]; !kept && f != "id" {
			gone = append(gone, f)
		}
	}
	return gone, nil
}

func changedAny(set map[string]string, id string) bool {
	for k := range set {
		if strings.HasPrefix(k, id+"|") {
			return true
		}
	}
	return false
}
