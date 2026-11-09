package boardfmt

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
)

// Expand turns a skeleton into a board. A skeleton is a board whose shapes may
// have a label (the words inside them) and whose arrows and lines may name the
// shapes they join with from and to, so an agent writes a diagram without the
// fields the editor needs (bindings, label sizes and places, where an arrow
// meets a shape). A box without a size gets one that fits its label. When no
// element has an index, the elements are drawn in the order written, each label
// right after its shape. A board with none of this is returned as it was.
//
// Shapes still need x and y: a skeleton places nothing by itself.
func Expand(content string) (string, error) {
	top, raws, ok := elementsOf(content)
	if !ok || !skeletal(raws) {
		return content, nil // what is not a board is Parse's to refuse, in its own words
	}
	s := sketch{byID: map[string]map[string]any{}}
	for _, el := range raws {
		if id, ok := el["id"].(string); ok {
			s.byID[id] = el
		}
	}
	ordered := !anyIndex(raws)
	for _, el := range raws {
		if err := s.shape(el); err != nil {
			return "", err
		}
	}
	for _, el := range raws {
		if err := s.join(el); err != nil {
			return "", err
		}
	}
	var out []map[string]any
	for _, el := range raws {
		out = append(out, el)
		if label := s.labels[str(el["id"])]; label != nil {
			out = append(out, label)
		}
	}
	if ordered {
		for i, el := range out {
			el["index"] = fractionalIndex(i)
		}
	}
	top["elements"], _ = json.Marshal(out) // only JSON values went in
	written, _ := json.Marshal(top)
	return string(written), nil
}

// elementsOf is the content's top level and its elements, when it has them.
func elementsOf(content string) (map[string]json.RawMessage, []map[string]any, bool) {
	var top map[string]json.RawMessage
	var raws []map[string]any
	if json.Unmarshal([]byte(content), &top) != nil || json.Unmarshal(top["elements"], &raws) != nil {
		return nil, nil, false
	}
	return top, raws, true
}

func skeletal(raws []map[string]any) bool {
	for _, el := range raws {
		for _, f := range []string{"label", "from", "to"} {
			if _, ok := el[f]; ok {
				return true
			}
		}
	}
	return false
}

func anyIndex(raws []map[string]any) bool {
	for _, el := range raws {
		if _, ok := el["index"]; ok {
			return true
		}
	}
	return false
}

type sketch struct {
	byID   map[string]map[string]any
	labels map[string]map[string]any
}

var shapes = set("rectangle", "ellipse", "diamond")

const (
	labelFontSize   = 20
	labelLineHeight = 1.25
	labelPadX       = 40
	labelPadY       = 30
)

// shape gives a shape with a label its label element, and a size that fits it
// when it has none.
func (s *sketch) shape(el map[string]any) error {
	id := str(el["id"])
	if _, ok := el["from"]; ok && !isLine(el) {
		return fmt.Errorf("board: %s: only an arrow or a line has from and to", id)
	}
	if _, ok := el["to"]; ok && !isLine(el) {
		return fmt.Errorf("board: %s: only an arrow or a line has from and to", id)
	}
	raw, ok := el["label"]
	if !ok {
		return nil
	}
	if !shapes[str(el["type"])] {
		return fmt.Errorf("board: %s: only a rectangle, an ellipse or a diamond has a label", id)
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("board: %s: label is not a text", id)
	}
	if _, ok := el["x"].(float64); !ok {
		return fmt.Errorf("board: %s: give x and y; a skeleton places nothing by itself", id)
	}
	if _, ok := el["y"].(float64); !ok {
		return fmt.Errorf("board: %s: give x and y; a skeleton places nothing by itself", id)
	}
	labelID := id + "-label"
	if _, taken := s.byID[labelID]; taken {
		return fmt.Errorf("board: %s: its label would be %q, which another element already is", id, labelID)
	}
	w, h := TextSize(text, labelFontSize, labelLineHeight)
	if _, ok := el["width"]; !ok {
		el["width"] = math.Max(120, w+labelPadX)
	}
	if _, ok := el["height"]; !ok {
		el["height"] = math.Max(60, h+labelPadY)
	}
	delete(el, "label")
	el["boundElements"] = append(boundOf(el), map[string]any{"id": labelID, "type": "text"})
	label := map[string]any{
		"id": labelID, "type": "text", "text": text, "originalText": text,
		"fontSize": float64(labelFontSize), "lineHeight": labelLineHeight, "width": w, "height": h,
		"containerId": id, "textAlign": "center", "verticalAlign": "middle",
	}
	if c, ok := el["strokeColor"]; ok {
		label["strokeColor"] = c
	}
	if s.labels == nil {
		s.labels = map[string]map[string]any{}
	}
	s.labels[id] = label
	s.byID[labelID] = label
	return nil
}

func isLine(el map[string]any) bool { t := str(el["type"]); return t == "arrow" || t == "line" }

// join draws an arrow or a line with from and to between its two shapes and
// binds it to both.
func (s *sketch) join(el map[string]any) error {
	_, hasFrom := el["from"]
	_, hasTo := el["to"]
	if !hasFrom && !hasTo {
		return nil
	}
	id := str(el["id"])
	if !hasFrom || !hasTo {
		return fmt.Errorf("board: arrow %s: give both from and to", id)
	}
	from, err := s.end(el, "from")
	if err != nil {
		return err
	}
	to, err := s.end(el, "to")
	if err != nil {
		return err
	}
	if str(from["id"]) == str(to["id"]) {
		return fmt.Errorf("board: arrow %s: from and to are the same element", id)
	}
	const gap = 5
	fx, fy := centre(from)
	tx, ty := centre(to)
	sx, sy := edgeToward(from, tx, ty, gap)
	ex, ey := edgeToward(to, fx, fy, gap)
	el["x"], el["y"] = sx, sy
	el["points"] = []any{[]any{0.0, 0.0}, []any{ex - sx, ey - sy}}
	el["startBinding"] = map[string]any{"elementId": str(from["id"]), "focus": 0.0, "gap": float64(gap)}
	el["endBinding"] = map[string]any{"elementId": str(to["id"]), "focus": 0.0, "gap": float64(gap)}
	if _, ok := el["endArrowhead"]; !ok && str(el["type"]) == "arrow" {
		el["endArrowhead"] = "arrow"
	}
	delete(el, "from")
	delete(el, "to")
	kind := str(el["type"])
	from["boundElements"] = append(boundOf(from), map[string]any{"id": id, "type": kind})
	to["boundElements"] = append(boundOf(to), map[string]any{"id": id, "type": kind})
	return nil
}

func (s *sketch) end(el map[string]any, which string) (map[string]any, error) {
	name, ok := el[which].(string)
	id := str(el["id"])
	if !ok {
		return nil, fmt.Errorf("board: arrow %s: %s is not an element id", id, which)
	}
	target, ok := s.byID[name]
	if !ok {
		return nil, fmt.Errorf("board: arrow %s: %s names no element %q", id, which, name)
	}
	if !shapes[str(target["type"])] && str(target["type"]) != "text" && str(target["type"]) != "frame" {
		return nil, fmt.Errorf("board: arrow %s: %s is %s, which is not a shape", id, which, name)
	}
	return target, nil
}

func boundOf(el map[string]any) []any {
	list, _ := el["boundElements"].([]any)
	return list
}

func centre(el map[string]any) (float64, float64) {
	return num(el["x"]) + num(el["width"])/2, num(el["y"]) + num(el["height"])/2
}

// edgeToward is where the line from el's centre toward (px, py) leaves el's
// outline, gap further on.
func edgeToward(el map[string]any, px, py, gap float64) (float64, float64) {
	cx, cy := centre(el)
	dx, dy := px-cx, py-cy
	length := math.Hypot(dx, dy)
	if length == 0 {
		return cx, cy
	}
	hw, hh := math.Max(num(el["width"])/2, 1e-9), math.Max(num(el["height"])/2, 1e-9)
	var t float64 // the share of (dx, dy) from the centre to the outline
	switch str(el["type"]) {
	case "ellipse":
		t = 1 / math.Hypot(dx/hw, dy/hh)
	case "diamond":
		t = 1 / (math.Abs(dx)/hw + math.Abs(dy)/hh)
	default:
		t = math.Min(hw/math.Max(math.Abs(dx), 1e-9), hh/math.Max(math.Abs(dy), 1e-9))
	}
	t += gap / length
	return cx + dx*t, cy + dy*t
}

// TextSize estimates how much room a text takes in the editor's hand-drawn
// font: about 0.6 em per Latin letter and 1 em per wide (CJK) character, one
// line height per line. The editor measures it again when it draws it.
func TextSize(text string, fontSize, lineHeight float64) (w, h float64) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		width := 0.0
		for _, r := range line {
			if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) || r >= 0xFF00 && r <= 0xFFEF {
				width += fontSize
			} else {
				width += 0.6 * fontSize
			}
		}
		w = math.Max(w, width)
	}
	return math.Ceil(w), float64(len(lines)) * fontSize * lineHeight
}

// fractionalIndex is the i-th of Excalidraw's own ordering keys: a0 … az, then
// b00 … bzz, then c000 …, which compare in order as plain strings.
func fractionalIndex(i int) string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for head, width := byte('a'), 1; ; head, width = head+1, width+1 {
		span := int(math.Pow(float64(len(digits)), float64(width)))
		if i < span {
			key := make([]byte, width)
			for p := width - 1; p >= 0; p-- {
				key[p] = digits[i%len(digits)]
				i /= len(digits)
			}
			return string(head) + string(key)
		}
		i -= span
	}
}

// IndexAfter is the first of Excalidraw's own keys (fractionalIndex) that draws
// above top; with no top it is the first.
func IndexAfter(top string) string {
	for i := 0; ; i++ {
		if key := fractionalIndex(i); key > top {
			return key
		}
	}
}
