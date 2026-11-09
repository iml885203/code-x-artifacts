// Package boardfmt is the content of a board: the Excalidraw elements it
// draws, as JSON {version, elements}, and the one set of rules for what a
// board may hold. A published version and every update a live room keeps
// (internal/boardroom) are held to the same rules, so nothing a publisher
// could not write reaches a board through its room, and the other way round.
package boardfmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Format is the only content format version: {version: 1, elements: [...]}.
const Format = "1"

const (
	// MaxContent is the size of a board's JSON. A live room keeps every change
	// within the generic 2 MiB log budget, so a version stays well under it.
	MaxContent  = 1 << 20
	maxElements = 10000
	maxText     = 20000 // characters of one text element
	maxPoints   = 10000 // points of one line or stroke
	maxCoord    = 1e7   // farther than anyone pans
	maxRawValue = 65536 // bytes of any one field's JSON
	maxIDLength = 64
)

// Element is one Excalidraw element: its fields by name, as JSON decodes them.
type Element map[string]any

// Board is a board's content.
type Board struct {
	Version  int       `json:"version"`
	Elements []Element `json:"elements"`
}

// Bookkeeping are the fields each Excalidraw replica counts for itself. A
// publisher may send them (an exported scene has them) and they are dropped:
// a board neither stores nor shares them.
var Bookkeeping = set("version", "versionNonce", "updated")

var (
	idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	// fileNamePattern is the name of an uploaded file as the server cleans it.
	fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,127}$`)
	colourPattern   = regexp.MustCompile(`^(transparent|#[0-9a-fA-F]{3,4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8})$`)
)

// types are the elements a board draws. An embeddable or iframe would load
// another site into the board, and a board's content is data, never a page. An
// image shows a file uploaded to the board's artifact, named by fileId.
var types = set("rectangle", "diamond", "ellipse", "arrow", "line", "freedraw", "text", "frame", "image")

type rule func(v any) error

// fields are the element fields a board keeps and what each may hold. A field
// that is not here (customData, a picture's data) is refused.
var fields = map[string]rule{
	"type":               oneOf(types),
	"x":                  coordinate,
	"y":                  coordinate,
	"width":              coordinate,
	"height":             coordinate,
	"angle":              coordinate,
	"strokeWidth":        coordinate,
	"roughness":          coordinate,
	"seed":               between(0, math.MaxInt32),
	"fontSize":           coordinate,
	"fontFamily":         coordinate,
	"lineHeight":         coordinate,
	"opacity":            between(0, 100),
	"strokeColor":        colour,
	"backgroundColor":    colour,
	"fillStyle":          oneOf(set("hachure", "cross-hatch", "solid", "zigzag")),
	"strokeStyle":        oneOf(set("solid", "dashed", "dotted")),
	"textAlign":          oneOf(set("left", "center", "right")),
	"verticalAlign":      oneOf(set("top", "middle", "bottom")),
	"startArrowhead":     arrowhead,
	"endArrowhead":       arrowhead,
	"link":               link,
	"text":               text,
	"originalText":       text,
	"name":               nullable(text),
	"points":             points,
	"lastCommittedPoint": nullable(pair),
	"pressures":          numbers,
	"containerId":        nullable(identifier),
	"fileId":             nullable(fileName),
	"status":             oneOf(set("pending", "saved", "error")),
	"scale":              pair,
	"crop":               anything,
	"frameId":            nullable(identifier),
	"index":              nullable(identifier),
	"isDeleted":          boolean,
	// The rest are structure the editor reads (bindings, groups, flags); they
	// are held to the JSON and size rules and to no data URL anywhere inside.
	"roundness":        anything,
	"groupIds":         anything,
	"boundElements":    anything,
	"startBinding":     anything,
	"endBinding":       anything,
	"elbowed":          anything,
	"fixedSegments":    anything,
	"startIsSpecial":   anything,
	"endIsSpecial":     anything,
	"polygon":          anything,
	"simulatePressure": anything,
	"autoResize":       anything,
	"locked":           anything,
}

// CheckID refuses an element id a board cannot key a field by.
func CheckID(id string) error {
	if len(id) == 0 || len(id) > maxIDLength || !idPattern.MatchString(id) {
		return fmt.Errorf("board: %q is not an element id (letters, digits, - and _, at most %d)", id, maxIDLength)
	}
	return nil
}

// CheckField refuses a value an element's field may not hold. raw is the
// field's JSON; the message names the element and the field.
func CheckField(id, field string, raw []byte) error {
	where := id + "." + field
	check, known := fields[field]
	if !known {
		return fmt.Errorf("board: %s is not a field a board keeps", where)
	}
	if len(raw) > maxRawValue {
		return fmt.Errorf("board: %s is over %d bytes", where, maxRawValue)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("board: %s is not JSON: %w", where, err)
	}
	if holdsDataURL(v) {
		return fmt.Errorf("board: %s holds a data URL; a board holds no pictures yet", where)
	}
	if err := check(v); err != nil {
		return fmt.Errorf("board: %s %w", where, err)
	}
	return nil
}

// Parse reads a board's content and refuses one a board may not hold, naming
// the element and field.
func Parse(content string) (Board, error) {
	if strings.TrimSpace(content) == "" {
		return Board{}, errors.New(`board: the content is empty; send {"version": 1, "elements": [...]}`)
	}
	if len(content) > MaxContent {
		return Board{}, fmt.Errorf("board: the content is %d bytes, over %d", len(content), MaxContent)
	}
	if !utf8.ValidString(content) {
		return Board{}, errors.New("board: the content is not UTF-8")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &top); err != nil {
		return Board{}, fmt.Errorf(`board: the content is not a JSON object {"version": 1, "elements": [...]}: %w`, err)
	}
	for k := range top {
		if k != "version" && k != "elements" {
			return Board{}, fmt.Errorf(`board: %q is not part of a board; a board is {"version": 1, "elements": [...]} (an Excalidraw file's appState and files are not kept)`, k)
		}
	}
	if string(bytes.TrimSpace(top["version"])) != Format {
		return Board{}, fmt.Errorf("board: version is %s; the only version is %s", top["version"], Format)
	}
	var raws []map[string]json.RawMessage
	if err := json.Unmarshal(top["elements"], &raws); err != nil || top["elements"] == nil {
		return Board{}, errors.New("board: elements is not a list of element objects")
	}
	if len(raws) > maxElements {
		return Board{}, fmt.Errorf("board: %d elements, over %d", len(raws), maxElements)
	}
	b := Board{Version: 1, Elements: make([]Element, 0, len(raws))}
	seen := map[string]bool{}
	for i, raw := range raws {
		el, err := element(i, raw)
		if err != nil {
			return Board{}, err
		}
		id := el.ID()
		if seen[id] {
			return Board{}, fmt.Errorf("board: two elements have the id %s", id)
		}
		seen[id] = true
		b.Elements = append(b.Elements, el)
	}
	return b, nil
}

func element(i int, raw map[string]json.RawMessage) (Element, error) {
	var id string
	if err := json.Unmarshal(raw["id"], &id); err != nil {
		return nil, fmt.Errorf("board: elements[%d] has no id", i)
	}
	if err := CheckID(id); err != nil {
		return nil, fmt.Errorf("board: elements[%d]: %w", i, err)
	}
	if _, ok := raw["type"]; !ok {
		return nil, fmt.Errorf("board: element %s has no type", id)
	}
	names := make([]string, 0, len(raw))
	for f := range raw {
		names = append(names, f)
	}
	sort.Strings(names)
	el := Element{}
	for _, f := range names {
		if f == "id" {
			continue
		}
		if Bookkeeping[f] {
			if _, err := number(decode(raw[f])); err != nil {
				return nil, fmt.Errorf("board: %s.%s %w", id, f, err)
			}
			continue
		}
		if err := CheckField(id, f, raw[f]); err != nil {
			return nil, err
		}
		el[f] = decode(raw[f])
	}
	el["id"] = id
	return el, nil
}

func decode(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v) // the field was decoded once already by CheckField or number
	return v
}

// Validate refuses content a board may not hold in the given format version.
func Validate(content, format string) error {
	if format != Format {
		return fmt.Errorf("board: format %q is not one this server draws; the only one is %s", format, Format)
	}
	_, err := Parse(content)
	return err
}

// Canonical is the form a board is stored and read in: without deleted
// elements, in drawing order (index, then id), with every label laid out from
// its shape.
func Canonical(b Board) Board {
	out := Board{Version: 1, Elements: make([]Element, 0, len(b.Elements))}
	for _, el := range b.Elements {
		if el["isDeleted"] == true {
			continue
		}
		c := Element{}
		for k, v := range el {
			if k != "isDeleted" {
				c[k] = v
			}
		}
		out.Elements = append(out.Elements, c)
	}
	sort.SliceStable(out.Elements, func(i, j int) bool {
		a, b := out.Elements[i], out.Elements[j]
		if ia, ib := str(a["index"]), str(b["index"]); ia != ib {
			return ia < ib
		}
		return a.ID() < b.ID()
	})
	LayoutLabels(out.Elements)
	return out
}

// Encode is a board's JSON.
func Encode(b Board) string {
	if b.Elements == nil {
		b.Elements = []Element{}
	}
	out, err := json.Marshal(b)
	if err != nil {
		panic(fmt.Sprintf("boardfmt: a parsed board does not encode: %v", err)) // every value came from JSON
	}
	return string(out)
}

// ID is the element's id.
func (e Element) ID() string { return str(e["id"]) }

// boundTextPadding is Excalidraw's BOUND_TEXT_PADDING: a label sits this far
// inside its shape.
const boundTextPadding = 5

// LayoutLabels places every label (text with a containerId) where its shape
// puts it, as Excalidraw does. A label's x and y follow from the shape, so two
// people's writes of them (one moved the shape, one retyped the label and so
// changed its width) would otherwise leave it where neither put it.
func LayoutLabels(elements []Element) {
	byID := map[string]Element{}
	for _, el := range elements {
		byID[el.ID()] = el
	}
	for _, el := range elements {
		container := str(el["containerId"])
		if str(el["type"]) != "text" || container == "" {
			continue
		}
		box, ok := byID[container]
		if !ok || box["isDeleted"] == true {
			continue
		}
		bx, by, bw, bh := num(box["x"]), num(box["y"]), num(box["width"]), num(box["height"])
		w, h := num(el["width"]), num(el["height"])
		ix, iy := insetOf(str(box["type"]), bw, bh)
		x, y := bx+(bw-w)/2, by+(bh-h)/2
		switch str(el["textAlign"]) {
		case "left":
			x = bx + ix
		case "right":
			x = bx + bw - ix - w
		}
		switch str(el["verticalAlign"]) {
		case "top":
			y = by + iy
		case "bottom":
			y = by + bh - iy - h
		}
		el["x"], el["y"] = x, y
	}
}

// insetOf is how far inside a shape its label's box starts (Excalidraw's
// getContainerCoords).
func insetOf(shape string, w, h float64) (x, y float64) {
	switch shape {
	case "ellipse":
		return boundTextPadding + w/2*(1-math.Sqrt2/2), boundTextPadding + h/2*(1-math.Sqrt2/2)
	case "diamond":
		return boundTextPadding + w/4, boundTextPadding + h/4
	}
	return boundTextPadding, boundTextPadding
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	n, _ := v.(float64)
	return n
}

func holdsDataURL(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "data:")
	case []any:
		for _, x := range t {
			if holdsDataURL(x) {
				return true
			}
		}
	case map[string]any:
		for _, x := range t {
			if holdsDataURL(x) {
				return true
			}
		}
	}
	return false
}

func set(names ...string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

func anything(any) error { return nil }

func boolean(v any) error {
	if _, ok := v.(bool); !ok {
		return fmt.Errorf("is %v, not true or false", v)
	}
	return nil
}

func nullable(r rule) rule {
	return func(v any) error {
		if v == nil {
			return nil
		}
		return r(v)
	}
}

func oneOf(allowed map[string]bool) rule {
	return func(v any) error {
		s, ok := v.(string)
		if !ok || !allowed[s] {
			return fmt.Errorf("is %v, which a board does not draw", v)
		}
		return nil
	}
}

func number(v any) (float64, error) {
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("is %v, not a number", v)
	}
	return n, nil
}

func coordinate(v any) error {
	n, err := number(v)
	if err != nil {
		return err
	}
	if math.Abs(n) > maxCoord {
		return fmt.Errorf("is %g, beyond ±%g", n, maxCoord)
	}
	return nil
}

func between(lo, hi float64) rule {
	return func(v any) error {
		n, err := number(v)
		if err != nil {
			return err
		}
		if n < lo || n > hi {
			return fmt.Errorf("is %g, not between %g and %g", n, lo, hi)
		}
		return nil
	}
}

func colour(v any) error {
	s, ok := v.(string)
	if !ok || !colourPattern.MatchString(s) {
		return fmt.Errorf("is %v, not transparent or a #hex colour", v)
	}
	return nil
}

var arrowhead = nullable(oneOf(set("arrow", "bar", "dot", "circle", "circle_outline", "triangle",
	"triangle_outline", "diamond", "diamond_outline", "crowfoot_one", "crowfoot_many", "crowfoot_one_or_many")))

// link is where clicking an element goes. Only addresses that open another
// page or a mail are kept: any other scheme could run in whoever clicks it.
func link(v any) error {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("is %v, not an address", v)
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "mailto") {
		return fmt.Errorf("is %q; a link is an https: or mailto: address", s)
	}
	return nil
}

func text(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("is %v, not a text", v)
	}
	if n := utf8.RuneCountInString(s); n > maxText {
		return fmt.Errorf("is %d characters, over %d", n, maxText)
	}
	return nil
}

// fileName is an image's file: a name of a file uploaded to the board's own
// artifact. The editor loads it from that artifact only, so it cannot name
// another artifact's file or another site.
func fileName(v any) error {
	s, ok := v.(string)
	if !ok || !fileNamePattern.MatchString(s) {
		return fmt.Errorf("is %v, not the name of a file uploaded to this board", v)
	}
	return nil
}

func identifier(v any) error {
	s, ok := v.(string)
	if !ok || len(s) > maxIDLength || !idPattern.MatchString(s) {
		return fmt.Errorf("is %v, not an id", v)
	}
	return nil
}

func numbers(v any) error {
	list, ok := v.([]any)
	if !ok || len(list) > maxPoints {
		return fmt.Errorf("is not a list of at most %d numbers", maxPoints)
	}
	for _, x := range list {
		if err := coordinate(x); err != nil {
			return err
		}
	}
	return nil
}

func pair(v any) error {
	p, ok := v.([]any)
	if !ok || len(p) != 2 {
		return fmt.Errorf("has %v, not an [x, y] point", v)
	}
	for _, x := range p {
		if err := coordinate(x); err != nil {
			return err
		}
	}
	return nil
}

func points(v any) error {
	list, ok := v.([]any)
	if !ok || len(list) > maxPoints {
		return fmt.Errorf("is not a list of at most %d points", maxPoints)
	}
	for _, p := range list {
		if err := pair(p); err != nil {
			return err
		}
	}
	return nil
}

// FileIDs are the files the board's images show, each once, in drawing order.
func FileIDs(b Board) []string {
	var out []string
	seen := map[string]bool{}
	for _, el := range Canonical(b).Elements {
		if id := str(el["fileId"]); str(el["type"]) == "image" && id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
