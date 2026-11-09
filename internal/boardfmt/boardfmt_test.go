package boardfmt

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// flow is a board an agent writes: two boxes, a label in the first, and an
// arrow bound to both. The label's x and y are wrong on purpose.
const flow = `{"version": 1, "elements": [
  {"id": "b", "type": "rectangle", "x": 300, "y": 0, "width": 120, "height": 60, "index": "a1"},
  {"id": "a", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 100, "index": "a0",
   "boundElements": [{"id": "la", "type": "text"}, {"id": "ar", "type": "arrow"}]},
  {"id": "la", "type": "text", "x": 999, "y": 999, "width": 60, "height": 25, "text": "Login 登入",
   "originalText": "Login 登入", "containerId": "a", "textAlign": "center", "verticalAlign": "middle", "index": "a2"},
  {"id": "ar", "type": "arrow", "x": 200, "y": 50, "points": [[0, 0], [100, -20]], "index": "a3",
   "startBinding": {"elementId": "a", "focus": 0, "gap": 5}, "endBinding": {"elementId": "b", "focus": 0, "gap": 5},
   "endArrowhead": "triangle", "version": 7, "versionNonce": 12, "updated": 1730000000000}
]}`

func TestABoardAnAgentWritesIsReadBackWithEveryElementAndItsBindings(t *testing.T) {
	t.Parallel()
	b, err := Parse(flow)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(flow, Format); err != nil {
		t.Fatal(err)
	}
	got := map[string]Element{}
	for _, el := range b.Elements {
		got[el.ID()] = el
	}
	if len(got) != 4 || got["la"]["text"] != "Login 登入" || got["la"]["containerId"] != "a" {
		t.Fatalf("elements = %v", b.Elements)
	}
	if bind, _ := got["ar"]["endBinding"].(map[string]any); bind["elementId"] != "b" {
		t.Errorf("the arrow's end binding is %v", got["ar"]["endBinding"])
	}
	for f := range Bookkeeping {
		if _, kept := got["ar"][f]; kept {
			t.Errorf("%s was kept; it is each editor's own bookkeeping", f)
		}
	}
}

func TestTheStoredFormIsInDrawingOrderWithoutDeletedElementsAndWithLabelsOnTheirShapes(t *testing.T) {
	t.Parallel()
	b, err := Parse(strings.Replace(flow, `"index": "a1"}`, `"index": "a1"}, {"id": "gone", "type": "ellipse", "isDeleted": true, "index": "a4"}`, 1))
	if err != nil {
		t.Fatal(err)
	}
	c := Canonical(b)
	var order []string
	for _, el := range c.Elements {
		order = append(order, el.ID())
		if _, ok := el["isDeleted"]; ok {
			t.Errorf("%s keeps isDeleted", el.ID())
		}
	}
	if strings.Join(order, ",") != "a,b,la,ar" {
		t.Errorf("order = %v", order)
	}
	label := c.Elements[2]
	if label["x"] != 70.0 || label["y"] != 37.5 {
		t.Errorf("the label is at %v,%v; its box centres it at 70,37.5", label["x"], label["y"])
	}
	again, err := Parse(Encode(c))
	if err != nil {
		t.Fatal(err)
	}
	if Encode(Canonical(again)) != Encode(c) {
		t.Error("the stored form is not stable through a second round")
	}
}

func TestALabelIsLaidOutInsideItsShapeTheWayTheEditorDoes(t *testing.T) {
	t.Parallel()
	inset := 100 * (1 - math.Sqrt2/2)
	cases := []struct {
		shape, align, valign string
		x, y                 float64
	}{
		{"rectangle", "center", "middle", 70, 37.5},
		{"rectangle", "left", "top", 5, 5},
		{"rectangle", "right", "bottom", 135, 70},
		{"diamond", "left", "top", 55, 30},
		{"ellipse", "left", "top", 5 + inset, 5 + 50*(1-math.Sqrt2/2)},
		{"ellipse", "center", "middle", 70, 37.5},
	}
	for _, c := range cases {
		t.Run(c.shape+" "+c.align+" "+c.valign, func(t *testing.T) {
			t.Parallel()
			els := []Element{
				{"id": "box", "type": c.shape, "x": 0.0, "y": 0.0, "width": 200.0, "height": 100.0},
				{"id": "t", "type": "text", "containerId": "box", "width": 60.0, "height": 25.0, "textAlign": c.align, "verticalAlign": c.valign},
			}
			LayoutLabels(els)
			if math.Abs(num(els[1]["x"])-c.x) > 1e-9 || math.Abs(num(els[1]["y"])-c.y) > 1e-9 {
				t.Errorf("at %v,%v, want %v,%v", els[1]["x"], els[1]["y"], c.x, c.y)
			}
		})
	}
}

func TestWhatABoardMayNotHoldIsRefusedAtPublishAndTheMessageSaysWhereAndWhat(t *testing.T) {
	t.Parallel()
	one := func(el string) string { return `{"version": 1, "elements": [` + el + `]}` }
	cases := []struct{ name, content, want string }{
		{"nothing", "  ", "empty"},
		{"not JSON", "{", "not a JSON object"},
		{"an Excalidraw file", `{"type": "excalidraw", "version": 1, "elements": []}`, `"type" is not part of a board`},
		{"an app state beside the elements", `{"version": 1, "elements": [], "appState": {}}`, `"appState"`},
		{"another version", `{"version": 2, "elements": []}`, "the only version is 1"},
		{"elements that are not a list", `{"version": 1, "elements": {}}`, "not a list"},
		{"no elements", `{"version": 1}`, "not a list"},
		{"an element without an id", one(`{"type": "rectangle"}`), "elements[0] has no id"},
		{"an id with a space", one(`{"id": "r 1", "type": "rectangle"}`), `"r 1"`},
		{"an element without a type", one(`{"id": "r1", "x": 1}`), "r1 has no type"},
		{"two elements with one id", one(`{"id": "r1", "type": "rectangle"}, {"id": "r1", "type": "ellipse"}`), "two elements have the id r1"},
		{"an embedded web page", one(`{"id": "e1", "type": "embeddable", "link": "https://example.com"}`), "e1.type"},
		{"an iframe", one(`{"id": "e1", "type": "iframe"}`), "e1.type"},
		{"an image of a file in a folder", one(`{"id": "i1", "type": "image", "fileId": "../other/x.png"}`), "i1.fileId"},
		{"an image of another site", one(`{"id": "i1", "type": "image", "fileId": "https://example.com/x.png"}`), "i1.fileId"},
		{"an image written into the board", one(`{"id": "i1", "type": "image", "fileId": "data:image/png;base64,AAAA"}`), "i1.fileId"},
		{"a javascript link", one(`{"id": "r1", "type": "rectangle", "link": "javascript:alert(1)"}`), "r1.link"},
		{"a link in another case", one(`{"id": "r1", "type": "rectangle", "link": " JavaScript:alert(1)"}`), "r1.link"},
		{"a data URL anywhere", one(`{"id": "r1", "type": "rectangle", "groupIds": ["data:text/html,x"]}`), "r1.groupIds holds a data URL"},
		{"custom data", one(`{"id": "r1", "type": "rectangle", "customData": {"x": 1}}`), "r1.customData"},
		{"a coordinate that is a text", one(`{"id": "r1", "type": "rectangle", "x": "10"}`), "r1.x"},
		{"a coordinate too far away", one(`{"id": "r1", "type": "rectangle", "x": 1e300}`), "r1.x"},
		{"an opacity over 100", one(`{"id": "r1", "type": "rectangle", "opacity": 101}`), "r1.opacity"},
		{"a colour that is CSS", one(`{"id": "r1", "type": "rectangle", "strokeColor": "red;background:url(x)"}`), "r1.strokeColor"},
		{"isDeleted that is not a flag", one(`{"id": "r1", "type": "rectangle", "isDeleted": "yes"}`), "r1.isDeleted"},
		{"a bookkeeping field that is not a number", one(`{"id": "r1", "type": "rectangle", "version": "7"}`), "r1.version"},
		{"a point that is not a pair", one(`{"id": "a1", "type": "arrow", "points": [[0, 0], [1]]}`), "a1.points"},
		{"a text too long", one(`{"id": "t1", "type": "text", "text": "` + strings.Repeat("x", maxText+1) + `"}`), "t1.text"},
		{"a board over the size limit", one(`{"id": "t1", "type": "text", "text": "` + strings.Repeat("x", MaxContent) + `"}`), "over"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(c.content, Format)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the message %q does not say %q", err, c.want)
			}
		})
	}
}

func TestAnImageOfAnUploadedFileIsAcceptedAndItsFileIsListed(t *testing.T) {
	t.Parallel()
	content := `{"version":1,"elements":[
	  {"id":"i1","type":"image","x":0,"y":0,"width":200,"height":100,"fileId":"screen-1a2b3c4d.png","status":"saved","scale":[1,1],"crop":null,"index":"a0"},
	  {"id":"i2","type":"image","x":0,"y":200,"width":200,"height":100,"fileId":"screen-1a2b3c4d.png","index":"a1"},
	  {"id":"i3","type":"image","x":0,"y":400,"width":200,"height":100,"fileId":"plan.v2.jpg","index":"a2"}]}`
	b, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(FileIDs(b), ","); got != "screen-1a2b3c4d.png,plan.v2.jpg" {
		t.Errorf("files = %s", got)
	}
}

func TestABoardJustUnderTheSizeLimitIsAcceptedAndAnotherFormatIsRefused(t *testing.T) {
	t.Parallel()
	head, tail := `{"version": 1, "elements": [{"id": "t1", "type": "text", "text": "`, `"}]}`
	words := strings.Repeat("字", maxText) // 3 bytes each, far under the size limit
	if err := Validate(head+words+tail, Format); err != nil {
		t.Errorf("a text of exactly the most characters was refused: %v", err)
	}
	var many []string
	for i := range maxElements {
		many = append(many, `{"id":"e`+itoa(i)+`","type":"line"}`)
	}
	full := `{"version":1,"elements":[` + strings.Join(many, ",") + `]}`
	if len(full) > MaxContent {
		t.Fatalf("the fixture is %d bytes", len(full))
	}
	if err := Validate(full, Format); err != nil {
		t.Errorf("the most elements were refused: %v", err)
	}
	if err := Validate(`{"version":1,"elements":[]}`, "2"); err == nil || !strings.Contains(err.Error(), "the only one is 1") {
		t.Errorf("format 2: %v", err)
	}
}

func TestAnEmptyBoardEncodesAsAnEmptyList(t *testing.T) {
	t.Parallel()
	var got struct{ Elements []any }
	if err := json.Unmarshal([]byte(Encode(Board{Version: 1})), &got); err != nil || got.Elements == nil {
		t.Errorf("encoded %s", Encode(Board{Version: 1}))
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
