package boardedit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

// flow is a board of two labelled boxes joined by an arrow.
func flow(t *testing.T) boardfmt.Board {
	t.Helper()
	full, err := boardfmt.Expand(`{"version":1,"elements":[
	  {"id":"a","type":"rectangle","x":0,"y":0,"label":"Start"},
	  {"id":"b","type":"rectangle","x":300,"y":0,"label":"End"},
	  {"id":"ab","type":"arrow","from":"a","to":"b"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := boardfmt.Parse(full)
	if err != nil {
		t.Fatal(err)
	}
	return boardfmt.Canonical(b)
}

func byID(b boardfmt.Board) map[string]boardfmt.Element {
	out := map[string]boardfmt.Element{}
	for _, el := range b.Elements {
		out[el.ID()] = el
	}
	return out
}

func raw(v any) json.RawMessage {
	out, _ := json.Marshal(v)
	return out
}

func TestAChangeSetsTheFieldsItNamesAndTheLabelOfAShapeAndLeavesTheRest(t *testing.T) {
	t.Parallel()
	b := flow(t)
	h := Hashes(b)
	next, res, err := Apply(b, []Op{{Op: "change", ID: "a", IfHash: h["a"], Fields: map[string]json.RawMessage{
		"label": raw("Begin here"), "backgroundColor": raw("#ffc9c9"), "link": json.RawMessage("null"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	els := byID(next)
	if els["a"]["backgroundColor"] != "#ffc9c9" || els["a-label"]["text"] != "Begin here" || els["a-label"]["originalText"] != "Begin here" {
		t.Errorf("a = %v, its label %v", els["a"], els["a-label"])
	}
	if els["b"]["x"] != 300.0 || Hash(els["b"]) != h["b"] || Hash(els["ab"]) != h["ab"] {
		t.Error("elements the call did not name changed")
	}
	if res.Written["a"] != Hash(els["a"]) || res.Written["a-label"] != Hash(els["a-label"]) || len(res.Written) != 2 {
		t.Errorf("written = %v", res.Written)
	}
	// The label is centred on its box again for its new width.
	box, label := els["a"], els["a-label"]
	if c := label["x"].(float64) + label["width"].(float64)/2; c != box["x"].(float64)+box["width"].(float64)/2 {
		t.Errorf("the label's centre is at %v", c)
	}
}

func TestAnAddDrawsASkeletonElementBoundToShapesAlreadyThereAndAboveThem(t *testing.T) {
	t.Parallel()
	b := flow(t)
	next, res, err := Apply(b, []Op{
		{Op: "add", Element: raw(map[string]any{"id": "c", "type": "ellipse", "x": 600, "y": 0, "label": "Done"})},
		{Op: "add", Element: raw(map[string]any{"id": "bc", "type": "arrow", "from": "b", "to": "c"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	els := byID(next)
	if els["c-label"]["text"] != "Done" || els["bc"]["endBinding"].(map[string]any)["elementId"] != "c" {
		t.Fatalf("the added elements: %v %v", els["c-label"], els["bc"])
	}
	bound, _ := json.Marshal(els["b"]["boundElements"])
	if !strings.Contains(string(bound), `"bc"`) {
		t.Errorf("b does not list the new arrow: %s", bound)
	}
	for _, id := range []string{"c", "c-label", "bc", "b"} {
		if res.Written[id] == "" {
			t.Errorf("%s is not in written: %v", id, res.Written)
		}
	}
	last := next.Elements[len(next.Elements)-1].ID()
	if last != "bc" {
		t.Errorf("the last drawn is %s; what an agent adds draws above what was there", last)
	}
}

func TestADeleteRemovesTheElementWithItsLabelAndUnbindsWhatWasBoundToIt(t *testing.T) {
	t.Parallel()
	b := flow(t)
	next, res, err := Apply(b, []Op{{Op: "delete", ID: "b", IfHash: Hashes(b)["b"]}})
	if err != nil {
		t.Fatal(err)
	}
	els := byID(next)
	if els["b"] != nil || els["b-label"] != nil {
		t.Error("b or its label is still there")
	}
	if els["ab"]["endBinding"] != nil {
		t.Errorf("the arrow is still bound to b: %v", els["ab"]["endBinding"])
	}
	if len(res.Deleted) != 1 || res.Deleted[0] != "b" {
		t.Errorf("deleted = %v", res.Deleted)
	}
}

func TestAnOpOnAnElementSomeoneChangedSinceItWasReadIsRefusedWithTheElementAsItIsNow(t *testing.T) {
	t.Parallel()
	b := flow(t)
	read := Hashes(b)
	moved, _, err := Apply(b, []Op{{Op: "change", ID: "a", IfHash: read["a"], Fields: map[string]json.RawMessage{"x": raw(50)}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Apply(moved, []Op{
		{Op: "change", ID: "b", IfHash: read["b"], Fields: map[string]json.RawMessage{"x": raw(400)}},
		{Op: "change", ID: "a", IfHash: read["a"], Fields: map[string]json.RawMessage{"label": raw("Go")}},
	})
	var stale *Stale
	if !errors.As(err, &stale) {
		t.Fatalf("err = %v, want a Stale", err)
	}
	if !strings.Contains(stale.Msg, "a changed since you read it") || !strings.Contains(stale.Msg, "send the call again") {
		t.Errorf("message %q", stale.Msg)
	}
	if cur := stale.Current["a"]; cur.Element["x"] != 50.0 || cur.Hash != Hashes(moved)["a"] || len(stale.Current) != 1 {
		t.Errorf("current = %v", stale.Current)
	}
}

func TestAnOpThatCannotBeDoneRefusesTheWholeCallSayingWhich(t *testing.T) {
	t.Parallel()
	b := flow(t)
	h := Hashes(b)
	cases := []struct {
		name string
		ops  []Op
		want string
	}{
		{"an unknown op", []Op{{Op: "move"}}, `ops[0]: op is "move"`},
		{"a change of nothing", []Op{{Op: "change", ID: "zz", IfHash: "x", Fields: map[string]json.RawMessage{"x": raw(1)}}}, `ops[0]: the board has no element "zz"`},
		{"a change without its hash", []Op{{Op: "change", ID: "a", Fields: map[string]json.RawMessage{"x": raw(1)}}}, "ops[0]: a change needs ifHash"},
		{"a change of the type", []Op{{Op: "change", ID: "a", IfHash: h["a"], Fields: map[string]json.RawMessage{"type": raw("ellipse")}}}, "type cannot be changed"},
		{"a field a board does not keep", []Op{{Op: "change", ID: "a", IfHash: h["a"], Fields: map[string]json.RawMessage{"customData": raw(1)}}}, "a.customData"},
		{"a javascript link", []Op{{Op: "change", ID: "a", IfHash: h["a"], Fields: map[string]json.RawMessage{"link": raw("javascript:x")}}}, "a.link"},
		{"an add of an id taken", []Op{{Op: "add", Element: raw(map[string]any{"id": "a", "type": "ellipse"})}}, `ops[0]: the board already has an element "a"`},
		{"an arrow to nothing", []Op{{Op: "add", Element: raw(map[string]any{"id": "x", "type": "arrow", "from": "a", "to": "zz"})}}, `ops[0]: arrow x: to names no element "zz"`},
		{"a delete twice", []Op{{Op: "delete", ID: "a", IfHash: h["a"]}, {Op: "delete", ID: "a", IfHash: h["a"]}}, `ops[1]: the board has no element "a"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Apply(b, c.ops)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to say %q", err, c.want)
			}
		})
	}
	if Hashes(b)["a"] != h["a"] {
		t.Error("a refused call changed the board it was given")
	}
}
