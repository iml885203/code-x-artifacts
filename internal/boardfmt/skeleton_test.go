package boardfmt

import (
	"math"
	"strings"
	"testing"
)

// login is a skeleton an agent writes: three labelled boxes and two arrows.
const login = `{"version": 1, "elements": [
  {"id": "open", "type": "rectangle", "x": 0, "y": 0, "label": "Open app"},
  {"id": "check", "type": "diamond", "x": 300, "y": -20, "width": 160, "height": 110, "label": "密碼對嗎?"},
  {"id": "home", "type": "ellipse", "x": 600, "y": 0, "label": "Home"},
  {"id": "go", "type": "arrow", "from": "open", "to": "check"},
  {"id": "in", "type": "arrow", "from": "check", "to": "home", "strokeStyle": "dashed"}
]}`

func expanded(t *testing.T, content string) map[string]Element {
	t.Helper()
	out, err := Expand(content)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(out)
	if err != nil {
		t.Fatalf("the expanded board is not one: %v\n%s", err, out)
	}
	byID := map[string]Element{}
	for _, el := range Canonical(b).Elements {
		byID[el.ID()] = el
	}
	return byID
}

func TestASkeletonOfLabelledBoxesAndArrowsBecomesBoundElementsWithCentredLabels(t *testing.T) {
	t.Parallel()
	els := expanded(t, login)
	if len(els) != 8 {
		t.Fatalf("%d elements, want 3 shapes, 3 labels and 2 arrows: %v", len(els), els)
	}
	for _, shape := range []string{"open", "check", "home"} {
		labelCentredIn(t, els, shape)
	}
	arrowBound(t, els, "go", "open", "check")
	arrowBound(t, els, "in", "check", "home")
	// The arrow from open runs from open's right edge to the diamond's left corner.
	go1 := els["go"]
	pts := pointsOf(go1)
	sx, sy := num(go1["x"]), num(go1["y"])
	ex := sx + pts[len(pts)-1][0]
	if math.Abs(sx-(num(els["open"]["x"])+num(els["open"]["width"])+5)) > 1 || ex > 300-2 || ex < 300-6 || math.Abs(sy-num(els["open"]["y"])-num(els["open"]["height"])/2) > 2 {
		t.Errorf("go runs from %v,%v to x %v", sx, sy, ex)
	}
	if els["in"]["strokeStyle"] != "dashed" {
		t.Error("an arrow's own fields are kept")
	}
}

func labelCentredIn(t *testing.T, els map[string]Element, shape string) {
	t.Helper()
	box, label := els[shape], els[shape+"-label"]
	if label == nil || label["containerId"] != shape || !holds(box["boundElements"], shape+"-label", "text") {
		t.Fatalf("%s has no bound label: %v / %v", shape, box, label)
	}
	bx, by := num(box["x"])+num(box["width"])/2, num(box["y"])+num(box["height"])/2
	lx, ly := num(label["x"])+num(label["width"])/2, num(label["y"])+num(label["height"])/2
	if math.Abs(bx-lx) > 1e-6 || math.Abs(by-ly) > 1e-6 {
		t.Errorf("%s's label is centred at %v,%v, its box at %v,%v", shape, lx, ly, bx, by)
	}
	if num(label["width"]) >= num(box["width"]) {
		t.Errorf("%s's label (%v wide) does not fit its box (%v)", shape, label["width"], box["width"])
	}
}

func arrowBound(t *testing.T, els map[string]Element, arrow, from, to string) {
	t.Helper()
	a := els[arrow]
	start, _ := a["startBinding"].(map[string]any)
	end, _ := a["endBinding"].(map[string]any)
	if start["elementId"] != from || end["elementId"] != to || a["endArrowhead"] != "arrow" {
		t.Errorf("%s is bound %v → %v, head %v", arrow, start, end, a["endArrowhead"])
	}
	if !holds(els[from]["boundElements"], arrow, "arrow") || !holds(els[to]["boundElements"], arrow, "arrow") {
		t.Errorf("the shapes of %s do not list it", arrow)
	}
	if _, kept := a["from"]; kept {
		t.Errorf("%s keeps from", arrow)
	}
}

func holds(bound any, id, kind string) bool {
	list, _ := bound.([]any)
	for _, b := range list {
		if m, ok := b.(map[string]any); ok && m["id"] == id && m["type"] == kind {
			return true
		}
	}
	return false
}

func TestASkeletonIsDrawnInTheOrderItIsWrittenWithEachLabelAfterItsShape(t *testing.T) {
	t.Parallel()
	out, err := Expand(login)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Parse(out)
	var order []string
	for _, el := range Canonical(b).Elements {
		order = append(order, el.ID())
	}
	if got := strings.Join(order, " "); got != "open open-label check check-label home home-label go in" {
		t.Errorf("drawing order %s", got)
	}
}

func TestABoardWithoutSkeletonFieldsIsLeftAsItWas(t *testing.T) {
	t.Parallel()
	plain := `{"version":1,"elements":[{"id":"r","type":"rectangle","x":0,"y":0,"width":10,"height":10}]}`
	if out, err := Expand(plain); err != nil || out != plain {
		t.Errorf("Expand changed a plain board: %v\n%s", err, out)
	}
}

func TestASkeletonThatCannotBeDrawnIsRefusedNamingTheElement(t *testing.T) {
	t.Parallel()
	one := func(els string) string { return `{"version":1,"elements":[` + els + `]}` }
	box := `{"id":"a","type":"rectangle","x":0,"y":0,"label":"A"}`
	cases := []struct{ name, content, want string }{
		{"an arrow to nothing", one(box + `,{"id":"x","type":"arrow","from":"a","to":"nowhere"}`), `arrow x: to names no element "nowhere"`},
		{"an arrow from nothing", one(box + `,{"id":"x","type":"arrow","from":"ghost","to":"a"}`), `arrow x: from names no element "ghost"`},
		{"an arrow with one end", one(box + `,{"id":"x","type":"arrow","from":"a"}`), "arrow x: give both from and to"},
		{"an arrow to itself", one(box + `,{"id":"x","type":"arrow","from":"a","to":"a"}`), "arrow x: from and to are the same element"},
		{"an arrow to another arrow", one(box + `,{"id":"b","type":"rectangle","x":200,"y":0,"label":"B"},{"id":"x","type":"arrow","from":"a","to":"b"},{"id":"y","type":"arrow","from":"a","to":"x"}`), "arrow y: to is x, which is not a shape"},
		{"from on a shape", one(`{"id":"a","type":"rectangle","x":0,"y":0,"from":"b"}`), "a: only an arrow or a line has from and to"},
		{"a label on an arrow", one(box + `,{"id":"b","type":"rectangle","x":200,"y":0},{"id":"x","type":"arrow","from":"a","to":"b","label":"go"}`), "x: only a rectangle, an ellipse or a diamond has a label"},
		{"a label that is not text", one(`{"id":"a","type":"rectangle","x":0,"y":0,"label":3}`), "a: label is not a text"},
		{"a shape with a label and no place", one(`{"id":"a","type":"rectangle","label":"A"}`), "a: give x and y"},
		{"a label whose id is taken", one(box + `,{"id":"a-label","type":"ellipse","x":0,"y":0}`), `a: its label would be "a-label", which another element already is`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Expand(c.content)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want it to say %q", err, c.want)
			}
		})
	}
}

func TestTheIndexesASkeletonGetsKeepTheirOrderAsPlainStringsAcrossEveryLength(t *testing.T) {
	t.Parallel()
	for i := 1; i < maxElements; i++ {
		a, b := fractionalIndex(i-1), fractionalIndex(i)
		if a >= b || CheckField("x", "index", []byte(`"`+b+`"`)) != nil {
			t.Fatalf("index %d is %q after %q", i, b, a)
		}
	}
}
