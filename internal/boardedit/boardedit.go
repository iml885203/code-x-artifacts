// Package boardedit is how an agent changes part of a board without sending all
// of it: add an element, change some fields of one, delete one. Every change and
// delete names the hash the agent read for the element, so a person's edit made
// since is not overwritten unseen. Pure functions over boardfmt.
package boardedit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

// Op is one operation.
type Op struct {
	// Op is add, change or delete.
	Op string `json:"op"`
	// ID is the element a change or delete is for.
	ID string `json:"id,omitempty"`
	// IfHash is the hash read gave for that element.
	IfHash string `json:"ifHash,omitempty"`
	// Element is what an add adds: an element as a skeleton may write it (a
	// shape's label, an arrow's from and to).
	Element json.RawMessage `json:"element,omitempty"`
	// Fields are what a change sets, by name; null removes a field. A shape's
	// label is its words.
	Fields map[string]json.RawMessage `json:"fields,omitempty"`
}

// Result is what a call did.
type Result struct {
	// Written are the elements the call added or changed, with their new hashes.
	Written map[string]string `json:"written,omitempty"`
	// Deleted are the elements it removed.
	Deleted []string `json:"deleted,omitempty"`
}

// Refusal is an operation that cannot be done.
type Refusal struct{ Msg string }

func (e *Refusal) Error() string { return e.Msg }

// Stale is an element that is not what the agent read: someone changed it.
type Stale struct {
	Msg string
	// Current are the stale elements as they are now, with their hashes.
	Current map[string]Current
}

// Current is an element and its hash.
type Current struct {
	Hash    string           `json:"hash"`
	Element boardfmt.Element `json:"element"`
}

func (e *Stale) Error() string { return e.Msg }

func refuse(i int, format string, args ...any) error {
	return &Refusal{Msg: fmt.Sprintf("ops[%d]: ", i) + fmt.Sprintf(format, args...)}
}

// Hash is an element's fingerprint as read gives it.
func Hash(el boardfmt.Element) string {
	raw, _ := json.Marshal(el) // an element is JSON values; map keys come out sorted
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:6])
}

// Hashes are the hashes of every element of a board, by id.
func Hashes(b boardfmt.Board) map[string]string {
	out := map[string]string{}
	for _, el := range boardfmt.Canonical(b).Elements {
		out[el.ID()] = Hash(el)
	}
	return out
}

// Apply does ops in order on board b, all or none. Hashes are checked against
// the board as it was before the call.
func Apply(b boardfmt.Board, ops []Op) (boardfmt.Board, Result, error) {
	before := boardfmt.Canonical(b)
	if err := checkGuards(before, ops); err != nil {
		return boardfmt.Board{}, Result{}, err
	}
	els := clone(before.Elements)
	touched := map[string]bool{}
	var deleted []string
	for i, op := range ops {
		var err error
		switch op.Op {
		case "add":
			els, err = add(els, i, op, touched)
		case "change":
			els, err = change(els, i, op, touched)
		case "delete":
			els, err = remove(els, i, op)
			deleted = append(deleted, op.ID)
		default:
			err = refuse(i, "op is %q; it is add, change or delete", op.Op)
		}
		if err != nil {
			return boardfmt.Board{}, Result{}, err
		}
	}
	next, err := boardfmt.Parse(boardfmt.Encode(boardfmt.Board{Version: 1, Elements: els}))
	if err != nil {
		return boardfmt.Board{}, Result{}, &Refusal{Msg: "the board after these ops is not one: " + err.Error()}
	}
	next = boardfmt.Canonical(next)
	res := Result{Written: map[string]string{}, Deleted: deleted}
	for _, el := range next.Elements {
		if touched[el.ID()] {
			res.Written[el.ID()] = Hash(el)
		}
	}
	return next, res, nil
}

// checkGuards refuses the call when an element a change or delete names is not
// what the agent read, naming each and giving it as it is now.
func checkGuards(b boardfmt.Board, ops []Op) error {
	byID := index(b.Elements)
	stale := map[string]Current{}
	for i, op := range ops {
		if op.Op != "change" && op.Op != "delete" {
			continue
		}
		el, ok := byID[op.ID]
		if !ok {
			return refuse(i, "the board has no element %q; read it again for the ids", op.ID)
		}
		if op.IfHash == "" {
			return refuse(i, "a %s needs ifHash: the hash read gave for %s", op.Op, op.ID)
		}
		if h := Hash(el); h != op.IfHash {
			stale[op.ID] = Current{Hash: h, Element: el}
		}
	}
	if len(stale) == 0 {
		return nil
	}
	ids := make([]string, 0, len(stale))
	for id := range stale {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return &Stale{
		Msg:     fmt.Sprintf("%s changed since you read it (someone edited the board); nothing was done: merge your change into current and send the call again with its hashes", strings.Join(ids, ", ")),
		Current: stale,
	}
}

func add(els []boardfmt.Element, i int, op Op, touched map[string]bool) ([]boardfmt.Element, error) {
	var el map[string]any
	if err := json.Unmarshal(op.Element, &el); err != nil || el == nil {
		return nil, refuse(i, "an add needs element: one element object")
	}
	id, _ := el["id"].(string)
	if index(els)[id] != nil {
		return nil, refuse(i, "the board already has an element %q", id)
	}
	out, err := expandWith(els, el)
	if err != nil {
		return nil, refuse(i, "%s", err)
	}
	markChanged(els, out, touched)
	return out, nil
}

// expandWith is the board's elements with extra added, as a skeleton (boardfmt.Expand):
// a label, and from and to bound to shapes already there. What has no index
// draws above the rest, in order.
func expandWith(els []boardfmt.Element, extra map[string]any) ([]boardfmt.Element, error) {
	whole := toAny(els)
	if extra != nil {
		whole = append(whole, extra)
	}
	raw, _ := json.Marshal(map[string]any{"version": 1, "elements": whole})
	expanded, err := boardfmt.Expand(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "board: "))
	}
	var out struct{ Elements []boardfmt.Element }
	if err := json.Unmarshal([]byte(expanded), &out); err != nil {
		return nil, fmt.Errorf("the element is not one: %w", err)
	}
	for _, e := range out.Elements {
		if _, has := e["index"]; !has {
			e["index"] = after(out.Elements)
		}
	}
	return out.Elements, nil
}

// markChanged notes every element of after that is new or differs from before.
func markChanged(before, after []boardfmt.Element, touched map[string]bool) {
	was := index(before)
	for _, e := range after {
		if old := was[e.ID()]; old == nil || Hash(old) != Hash(e) {
			touched[e.ID()] = true
		}
	}
}

func change(els []boardfmt.Element, i int, op Op, touched map[string]bool) ([]boardfmt.Element, error) {
	byID := index(els)
	el := byID[op.ID]
	if el == nil {
		return nil, refuse(i, "the board has no element %q", op.ID)
	}
	if len(op.Fields) == 0 {
		return nil, refuse(i, "a change needs fields: the fields to set")
	}
	for name, raw := range op.Fields {
		switch {
		case name == "id" || name == "type":
			return nil, refuse(i, "%s cannot be changed; delete the element and add another", name)
		case name == "label":
			continue // last: it may rebuild the list of elements
		case string(raw) == "null":
			delete(el, name)
		default:
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, refuse(i, "%s is not JSON", name)
			}
			el[name] = v
		}
	}
	if raw, ok := op.Fields["label"]; ok {
		var err error
		if els, err = relabel(els, el, i, raw, touched); err != nil {
			return nil, err
		}
	}
	touched[op.ID] = true
	return els, nil
}

// relabel sets the words of a shape's label, adding the label when it has none.
func relabel(els []boardfmt.Element, shape boardfmt.Element, i int, raw json.RawMessage, touched map[string]bool) ([]boardfmt.Element, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, refuse(i, "label is not a text")
	}
	for _, el := range els {
		if el["containerId"] == shape.ID() && el["type"] == "text" {
			w, h := boardfmt.TextSize(text, num(el["fontSize"], 20), num(el["lineHeight"], 1.25))
			el["text"], el["originalText"], el["width"], el["height"] = text, text, w, h
			touched[el.ID()] = true
			return els, nil
		}
	}
	before := clone(els)
	shape["label"] = text
	out, err := expandWith(els, nil)
	if err != nil {
		return nil, refuse(i, "%s", err)
	}
	markChanged(before, out, touched)
	return out, nil
}

// remove deletes an element with its label, and unbinds whatever was bound to it.
func remove(els []boardfmt.Element, i int, op Op) ([]boardfmt.Element, error) {
	if index(els)[op.ID] == nil {
		return nil, refuse(i, "the board has no element %q (deleted earlier in the call?)", op.ID)
	}
	gone := map[string]bool{op.ID: true}
	for _, el := range els {
		if el["containerId"] == op.ID {
			gone[el.ID()] = true
		}
	}
	out := els[:0:0]
	for _, el := range els {
		if gone[el.ID()] {
			continue
		}
		for _, end := range []string{"startBinding", "endBinding"} {
			if b, ok := el[end].(map[string]any); ok && gone[fmt.Sprint(b["elementId"])] {
				el[end] = nil
			}
		}
		if list, ok := el["boundElements"].([]any); ok {
			el["boundElements"] = slices.DeleteFunc(slices.Clone(list), func(b any) bool {
				m, _ := b.(map[string]any)
				return gone[fmt.Sprint(m["id"])]
			})
		}
		out = append(out, el)
	}
	return out, nil
}

// after is an index that draws above every element (one of Excalidraw's own keys).
func after(els []boardfmt.Element) string {
	top := ""
	for _, el := range els {
		if s, _ := el["index"].(string); s > top {
			top = s
		}
	}
	return boardfmt.IndexAfter(top)
}

func index(els []boardfmt.Element) map[string]boardfmt.Element {
	out := map[string]boardfmt.Element{}
	for _, el := range els {
		out[el.ID()] = el
	}
	return out
}

func clone(els []boardfmt.Element) []boardfmt.Element {
	raw, _ := json.Marshal(els)
	var out []boardfmt.Element
	_ = json.Unmarshal(raw, &out) // what Marshal wrote
	return out
}

func toAny(els []boardfmt.Element) []any {
	out := make([]any, len(els))
	for i, el := range els {
		out[i] = el
	}
	return out
}

func num(v any, otherwise float64) float64 {
	if n, ok := v.(float64); ok && n > 0 {
		return n
	}
	return otherwise
}
