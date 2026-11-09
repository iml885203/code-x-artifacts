package boardroom

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"
)

// write sets entries in the board map of a fresh document, as a browser's
// binding does: one entry per element field, each a JSON text.
func write(t *testing.T, entries map[string]string) *crdt.Doc {
	t.Helper()
	doc := crdt.New()
	m := doc.GetMap(Map)
	doc.Transact(func(txn *crdt.Transaction) {
		for k, v := range entries {
			m.Set(txn, k, v)
		}
	})
	return doc
}

// rectangle is the entries of a plain rectangle r1 the editor writes.
func rectangle() map[string]string {
	const id = "r1"
	return map[string]string{
		id + "|type":            `"rectangle"`,
		id + "|x":               `10`,
		id + "|y":               `-20.5`,
		id + "|width":           `120`,
		id + "|height":          `80`,
		id + "|angle":           `0`,
		id + "|strokeColor":     `"#1e1e1e"`,
		id + "|backgroundColor": `"transparent"`,
		id + "|fillStyle":       `"solid"`,
		id + "|strokeWidth":     `2`,
		id + "|strokeStyle":     `"solid"`,
		id + "|roughness":       `1`,
		id + "|opacity":         `100`,
		id + "|roundness":       `{"type":3}`,
		id + "|seed":            `1968410350`,
		id + "|groupIds":        `[]`,
		id + "|frameId":         `null`,
		id + "|index":           `"a0"`,
		id + "|boundElements":   `[{"id":"label","type":"text"}]`,
		id + "|isDeleted":       `false`,
		id + "|link":            `null`,
		id + "|locked":          `false`,
	}
}

func with(base map[string]string, more map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

func TestTheElementsTheEditorDrawsAreAccepted(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]string{
		"an empty board": {},
		"a rectangle":    rectangle(),
		"a label inside a shape": with(rectangle(), map[string]string{
			"label|type": `"text"`, "label|x": `20`, "label|y": `10`, "label|text": `"Login 登入"`,
			"label|originalText": `"Login 登入"`, "label|fontSize": `20`, "label|fontFamily": `5`,
			"label|textAlign": `"center"`, "label|verticalAlign": `"middle"`, "label|containerId": `"r1"`,
			"label|autoResize": `true`, "label|lineHeight": `1.25`,
		}),
		"an arrow bound at both ends": {
			"a1|type": `"arrow"`, "a1|x": `0`, "a1|y": `0`, "a1|points": `[[0,0],[100,40]]`,
			"a1|startBinding": `{"elementId":"r1","focus":0,"gap":5}`, "a1|endBinding": `null`,
			"a1|startArrowhead": `null`, "a1|endArrowhead": `"triangle"`, "a1|elbowed": `false`,
			"a1|lastCommittedPoint": `null`,
		},
		"a freehand stroke": {
			"f1|type": `"freedraw"`, "f1|points": `[[0,0],[1.5,2],[3,1]]`, "f1|pressures": `[0.5,0.6,0.4]`,
			"f1|simulatePressure": `false`,
		},
		"a frame": {"fr|type": `"frame"`, "fr|name": `"Flow"`},
		"an image of an uploaded file": {
			"i1|type": `"image"`, "i1|fileId": `"screen-1a2b3c4d.png"`, "i1|status": `"saved"`, "i1|scale": `[1,1]`, "i1|crop": `null`,
		},
		"a link to https and mailto": with(rectangle(), map[string]string{"r1|link": `"https://example.com/a?b=c"`, "r2|type": `"ellipse"`, "r2|link": `"mailto:team@example.com"`}),
		"a deleted element":          with(rectangle(), map[string]string{"r1|isDeleted": `true`}),
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := Check(write(t, entries)); err != nil {
				t.Errorf("refused: %v", err)
			}
		})
	}
}

func TestWhatABoardMayNotHoldIsRefusedAndTheMessageNamesTheElementAndField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{"an embedded web page", map[string]string{"e1|type": `"embeddable"`, "e1|link": `"https://example.com"`}, `e1.type`},
		{"an iframe", map[string]string{"e1|type": `"iframe"`}, `e1.type`},
		{"an element type nobody knows", map[string]string{"e1|type": `"widget"`}, `e1.type`},
		{"a javascript link", with(rectangle(), map[string]string{"r1|link": `"javascript:alert(1)"`}), `r1.link`},
		{"a data link", with(rectangle(), map[string]string{"r1|link": `"data:text/html,<script>alert(1)</script>"`}), `r1.link`},
		{"a link with a scheme in another case", with(rectangle(), map[string]string{"r1|link": `" JavaScript:alert(1)"`}), `r1.link`},
		{"a picture written into the board", map[string]string{"i1|type": `"image"`, "i1|fileId": `"data:image/png;base64,iVBORw0KGgo="`}, `i1.fileId`},
		{"a picture under a field nobody knows", with(rectangle(), map[string]string{"r1|dataURL": `"data:image/png;base64,iVBORw0KGgo="`}), `r1.dataURL`},
		{"a file entry beside the elements", map[string]string{"files|f1": `{"dataURL":"data:image/png;base64,AAAA"}`}, `files.f1`},
		{"an image of a file in a folder", map[string]string{"i1|type": `"image"`, "i1|fileId": `"../x.png"`}, `i1.fileId`},
		{"an editor's own bookkeeping", with(rectangle(), map[string]string{"r1|versionNonce": `12`}), `r1.versionNonce`},
		{"custom data", with(rectangle(), map[string]string{"r1|customData": `{"x":1}`}), `r1.customData`},
		{"a field without its element's type", map[string]string{"r9|x": `1`}, `r9`},
		{"an id with a separator", map[string]string{"r 1|type": `"rectangle"`}, `r 1`},
		{"a value that is not JSON", with(rectangle(), map[string]string{"r1|x": `ten`}), `r1.x`},
		{"a coordinate that is not a number", with(rectangle(), map[string]string{"r1|x": `"10"`}), `r1.x`},
		{"a coordinate too far away", with(rectangle(), map[string]string{"r1|x": `1e300`}), `r1.x`},
		{"an opacity over 100", with(rectangle(), map[string]string{"r1|opacity": `101`}), `r1.opacity`},
		{"a colour that is a CSS expression", with(rectangle(), map[string]string{"r1|strokeColor": `"red;background:url(x)"`}), `r1.strokeColor`},
		{"a fill style nobody draws", with(rectangle(), map[string]string{"r1|fillStyle": `"pattern"`}), `r1.fillStyle`},
		{"a point that is not a pair", map[string]string{"a1|type": `"arrow"`, "a1|points": `[[0,0],[1]]`}, `a1.points`},
		{"a text too long", map[string]string{"t1|type": `"text"`, "t1|text": `"` + strings.Repeat("x", 20001) + `"`}, `t1.text`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := Check(write(t, c.entries))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the message %q does not name %q", err, c.want)
			}
		})
	}
}

func TestARoomWithoutABoardIsNotTheBoardsToRefuse(t *testing.T) {
	t.Parallel()
	doc := crdt.New()
	design, text := doc.GetMap("design"), doc.GetText("t")
	doc.Transact(func(txn *crdt.Transaction) {
		design.Set(txn, "anything", "<script>")
		text.Insert(txn, 0, "javascript:", nil)
	})
	if err := Check(doc); err != nil {
		t.Errorf("a room of another kind was refused: %v", err)
	}
}
