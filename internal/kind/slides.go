package kind

import (
	_ "embed" // slides_guide.md
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

//go:embed slides_guide.md
var slidesGuide string

// slidesFormatV1 is the first deck format: {"version":1,"slides":[...]} with
// the layouts and element types listed in slidesLayouts and elementTypes.
// Adding an element type or a field is allowed within v1 only if every deck
// that was valid stays valid and draws the same; anything else is a new
// version, so published decks keep rendering as they did (principle 7).
const slidesFormatV1 = "1"

// Limits of a v1 deck. They keep one publish bounded and one slide legible;
// the guide tells agents to stay well below most of them.
const (
	MaxSlidesBytes    = 1 << 20
	maxSlides         = 80
	maxSlideElements  = 8
	maxColumnElements = 6
	maxTitleChars     = 160
	maxNotesChars     = 4000
	maxTextChars      = 600
	maxItemChars      = 300
	maxCodeChars      = 2000
	maxCodeLines      = 16
	maxTableColumns   = 6
	maxTableRows      = 12
	maxProblems       = 20
)

// Slides is a deck: typed slides drawn on a fixed 1920x1080 canvas by the
// platform, so the look improves for every deck when the renderer does, and
// the content stays data that later features (slide edits, export, themes)
// can work on. The page is generated entirely here, and RenderPage names its
// one inline script so the server can pin script-src to its hash
// (docs/script-src.md).
type Slides struct{}

// Name implements Kind.
func (Slides) Name() string { return "slides" }

// FormatVersions implements Kind.
func (Slides) FormatVersions() []string { return []string{slidesFormatV1} }

// Guide implements Kind.
func (Slides) Guide() string { return slidesGuide }

// Validate implements Kind. It reports every problem it finds (up to
// maxProblems), each starting with the slide and element it is about, so an
// agent fixes the deck in one round.
func (Slides) Validate(content, format string) error {
	if format != slidesFormatV1 {
		return fmt.Errorf("a slides artifact has no content format %q; the only one is %q", format, slidesFormatV1)
	}
	if len(content) > MaxSlidesBytes {
		return tooLarge("a slides artifact can be at most %d MB of JSON, this one is %d bytes", MaxSlidesBytes>>20, len(content))
	}
	if strings.TrimSpace(content) == "" {
		return errors.New(`a slides artifact needs content: the deck as JSON, {"version":1,"slides":[...]}`)
	}
	raw, err := splitDeck(content)
	if err != nil {
		return err
	}
	var all problems
	for i, r := range raw {
		_, ps := parseSlide(i+1, r)
		all.list = append(all.list, ps...)
	}
	return all.err()
}

// slide is one validated slide.
type slide struct {
	layout   string
	title    string
	notes    string
	elements []element
}

const (
	layoutTitle   = "title"
	layoutContent = "content"
)

var slidesLayouts = []string{layoutTitle, layoutContent}

// problems collects what is wrong with a deck, in reading order.
type problems struct{ list []string }

func (p *problems) add(where, format string, args ...any) {
	p.list = append(p.list, where+": "+fmt.Sprintf(format, args...))
}

func (p *problems) err() error {
	if len(p.list) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "the deck is not valid (%d problem(s)):", len(p.list))
	for i, s := range p.list {
		if i == maxProblems {
			fmt.Fprintf(&b, "\n- ... and %d more", len(p.list)-maxProblems)
			break
		}
		b.WriteString("\n- " + s)
	}
	return errors.New(b.String())
}

// splitDeck parses the outer shape of the deck and returns each slide as the
// decoded JSON value it is, so one bad slide never hides the others.
func splitDeck(content string) ([]any, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		return nil, errors.New("the content is wrapped in a Markdown code fence; send the JSON itself, starting with {")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var top any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("the content is not valid JSON: %s", jsonProblem(trimmed, err))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("the content has text after the JSON deck; send one JSON object and nothing else")
	}
	var p problems
	m, ok := top.(map[string]any)
	if !ok {
		return nil, errors.New(`the deck must be a JSON object: {"version":1,"slides":[...]}`)
	}
	p.unknown(m, "deck", []string{"version", "slides"})
	if v, ok := m["version"].(json.Number); !ok || v.String() != "1" {
		p.add("deck", `"version" must be 1 (this slides format), got %s`, show(m["version"]))
	}
	list, ok := m["slides"].([]any)
	switch {
	case !ok:
		p.add("deck", `needs "slides": a list of slide objects`)
	case len(list) == 0:
		p.add("deck", `"slides" is empty; a deck needs at least one slide`)
	case len(list) > maxSlides:
		p.add("deck", `"slides" has %d slides, the limit is %d`, len(list), maxSlides)
	}
	if err := p.err(); err != nil {
		return nil, err
	}
	return list, nil
}

// jsonProblem turns a decoder error into one with a line and column.
func jsonProblem(src string, err error) string {
	var syn *json.SyntaxError
	if errors.Is(err, io.ErrUnexpectedEOF) {
		line := strings.Count(src, "\n") + 1
		return fmt.Sprintf("the JSON ends too early (line %d): a bracket, brace or quote is not closed", line)
	}
	if errors.As(err, &syn) {
		upTo := src[:min(int(syn.Offset), len(src))]
		line := strings.Count(upTo, "\n") + 1
		col := utf8.RuneCountInString(upTo[strings.LastIndex(upTo, "\n")+1:])
		return fmt.Sprintf("%s (line %d, column %d)", syn.Error(), line, col)
	}
	return err.Error()
}

// show prints a decoded JSON value for an error message.
func show(v any) string {
	if v == nil {
		return "nothing"
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) > 60 {
		return fmt.Sprintf("a %s", kindOf(v))
	}
	return string(b)
}

func kindOf(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "list"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	default:
		return "value"
	}
}

// parseSlide validates one slide (n counts from 1) and returns it with every
// problem found. A slide with problems is returned incomplete and must not be
// drawn as a slide; Render shows an error slide for it.
func parseSlide(n int, v any) (slide, []string) {
	var p problems
	where := fmt.Sprintf("Slide %d", n)
	m := p.object(v, where)
	if m == nil {
		return slide{}, p.list
	}
	p.unknown(m, where, []string{"layout", "title", "notes", "elements"})
	s := slide{layout: layoutContent}
	if l, ok := m["layout"]; ok {
		name, isString := l.(string)
		if !isString || !contains(slidesLayouts, name) {
			p.add(where, `"layout" must be one of: %s; got %s`, strings.Join(slidesLayouts, ", "), show(l))
		} else {
			s.layout = name
		}
	}
	s.title = p.text(m, "title", where, true, maxTitleChars)
	s.notes = p.text(m, "notes", where, false, maxNotesChars)
	els, present := m["elements"]
	list, isList := els.([]any)
	// A title slide may have nothing under its title; a content slide may not.
	if empty := !present || (isList && len(list) == 0); !empty || s.layout == layoutContent {
		s.elements = parseElements(&p, where, els, present, maxSlideElements, true)
	}
	return s, p.list
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// parseElements validates a list of elements. top is false inside a
// column or a card, where the layout elements (columns, cards) are not allowed.
func parseElements(p *problems, where string, v any, present bool, limit int, top bool) []element {
	list, ok := v.([]any)
	switch {
	case !present:
		p.add(where, `needs "elements": a list of elements (text, bullets, table, ...)`)
		return nil
	case !ok:
		p.add(where, `"elements" must be a list, got %s`, show(v))
		return nil
	case len(list) == 0:
		p.add(where, `"elements" is empty; add at least one element`)
		return nil
	case len(list) > limit:
		p.add(where, `has %d elements, the limit is %d; split it over two slides`, len(list), limit)
		return nil
	}
	out := make([]element, 0, len(list))
	for i, raw := range list {
		if e := parseElement(p, fmt.Sprintf("%s element %d", where, i+1), raw, top); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// element is one validated element of a slide.
type element interface {
	// render writes the element's HTML, marking each string with its place under at (elements/2). Every
	// string passes through esc or inline, so nothing the deck says becomes markup.
	render(b *strings.Builder, at string)
	// height estimates the element's height in canvas pixels when it is laid
	// out w pixels wide with type scaled by k (1 is the base size).
	height(w, k float64) float64
}

// elementTypes lists the allowed element types in the order the guide and
// the error messages use.
var elementTypes = []string{"text", "bullets", "numbered", "code", "table", "stats", "quote", "steps", "columns", "cards", "progress"}

func parseElement(p *problems, where string, v any, top bool) element {
	m := p.object(v, where)
	if m == nil {
		return nil
	}
	t, ok := m["type"].(string)
	if !ok {
		p.add(where, `needs "type", one of: %s`, strings.Join(elementTypes, ", "))
		return nil
	}
	spec, known := elementSpecs()[t]
	if !known {
		p.add(where, "unknown type %q; the types are: %s", t, strings.Join(elementTypes, ", "))
		return nil
	}
	if spec.layout && !top {
		p.add(where, "%s cannot go inside columns or cards; put it directly on the slide", t)
		return nil
	}
	where += " (" + t + ")"
	p.unknown(m, where, append([]string{"type"}, spec.fields...))
	return spec.parse(p, where, m)
}

// elementSpec says what one element type accepts and how to read it.
type elementSpec struct {
	fields []string
	// layout marks the elements that hold other elements.
	layout bool
	parse  func(p *problems, where string, m map[string]any) element
}

func elementSpecs() map[string]elementSpec {
	return map[string]elementSpec{
		"text":     {fields: []string{"text"}, parse: parseText},
		"bullets":  {fields: []string{"items"}, parse: parseList(false)},
		"numbered": {fields: []string{"items"}, parse: parseList(true)},
		"code":     {fields: []string{"code", "language"}, parse: parseCode},
		"table":    {fields: []string{"columns", "rows"}, parse: parseTable},
		"stats":    {fields: []string{"items"}, parse: parseStats},
		"quote":    {fields: []string{"text", "by"}, parse: parseQuote},
		"steps":    {fields: []string{"items"}, parse: parseSteps},
		"columns":  {fields: []string{"columns"}, layout: true, parse: parseColumns},
		"cards":    {fields: []string{"items"}, layout: true, parse: parseCards},
		"progress": {fields: []string{"items"}, parse: parseProgress},
	}
}

// ---- value helpers: every message names the slide, the element and the field ----

func (p *problems) object(v any, where string) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		p.add(where, "must be an object, got %s", show(v))
		return nil
	}
	return m
}

// unknown refuses fields the type does not have, with the ones it does, so a
// misspelt field is not silently dropped from the slide.
func (p *problems) unknown(m map[string]any, where string, allowed []string) {
	for k := range m {
		if !contains(allowed, k) {
			p.add(where, "unknown field %q; the fields here are: %s", k, strings.Join(allowed, ", "))
		}
	}
}

// text reads a string field. Empty is the same as missing.
func (p *problems) text(m map[string]any, key, where string, required bool, limit int) string {
	v, present := m[key]
	if !present || v == nil {
		if required {
			p.add(where, `needs %q: a non-empty string`, key)
		}
		return ""
	}
	s, ok := v.(string)
	switch {
	case !ok:
		p.add(where, "%q must be a string, got %s", key, show(v))
	case required && strings.TrimSpace(s) == "":
		p.add(where, `needs %q: a non-empty string`, key)
	case utf8.RuneCountInString(s) > limit:
		p.add(where, "%q is %d characters, the limit is %d; shorten it or move the detail to notes", key, utf8.RuneCountInString(s), limit)
	default:
		return s
	}
	return ""
}

// scalar reads a field that may be a string or a number (a stat's value).
func (p *problems) scalar(m map[string]any, key, where string) string {
	if n, ok := m[key].(json.Number); ok {
		return n.String()
	}
	return p.text(m, key, where, true, maxItemChars)
}

// strings reads a list of strings.
func (p *problems) strings(m map[string]any, key, where string, lo, hi, limit int) []string {
	list, ok := p.listOf(m, key, where, lo, hi)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for i, v := range list {
		s, isString := v.(string)
		if !isString || strings.TrimSpace(s) == "" {
			p.add(where, "%q item %d must be a non-empty string, got %s", key, i+1, show(v))
			continue
		}
		if n := utf8.RuneCountInString(s); n > limit {
			p.add(where, "%q item %d is %d characters, the limit is %d", key, i+1, n, limit)
			continue
		}
		out = append(out, s)
	}
	return out
}

// listOf reads a list that must have between lo and hi entries.
func (p *problems) listOf(m map[string]any, key, where string, lo, hi int) ([]any, bool) {
	v, present := m[key]
	list, ok := v.([]any)
	switch {
	case !present:
		p.add(where, "needs %q: a list of %d to %d entries", key, lo, hi)
	case !ok:
		p.add(where, "%q must be a list, got %s", key, show(v))
	case len(list) < lo || len(list) > hi:
		p.add(where, "%q has %d entries, it needs %d to %d", key, len(list), lo, hi)
	default:
		return list, true
	}
	return nil, false
}

// ---- the element types ----

func parseText(p *problems, where string, m map[string]any) element {
	return textEl{text: p.text(m, "text", where, true, maxTextChars)}
}

func parseList(ordered bool) func(*problems, string, map[string]any) element {
	return func(p *problems, where string, m map[string]any) element {
		return listEl{ordered: ordered, items: p.strings(m, "items", where, 1, 8, maxItemChars)}
	}
}

func parseCode(p *problems, where string, m map[string]any) element {
	e := codeEl{code: strings.TrimRight(p.text(m, "code", where, true, maxCodeChars), "\n"), lang: p.text(m, "language", where, false, 24)}
	if n := strings.Count(e.code, "\n") + 1; n > maxCodeLines {
		p.add(where, `"code" has %d lines, the limit is %d; show the part that matters`, n, maxCodeLines)
	}
	return e
}

var tones = []string{"success", "warning", "danger"}

func parseTable(p *problems, where string, m map[string]any) element {
	const needs = `table needs "columns" (a list of header strings) and "rows" (a list of rows, each a list with one cell per column)`
	e := tableEl{}
	cols, ok := m["columns"].([]any)
	rows, rowsOK := m["rows"].([]any)
	if !ok || !rowsOK {
		p.add(where, "%s", needs)
		return e
	}
	e.columns = p.strings(m, "columns", where, 1, maxTableColumns, 80)
	if _, ok := p.listOf(m, "rows", where, 1, maxTableRows); !ok {
		return e
	}
	for i, r := range rows {
		row, ok := r.([]any)
		if !ok || len(row) != len(cols) {
			p.add(where, "row %d must be a list with %d cells, one per column, got %s", i+1, len(cols), show(r))
			continue
		}
		cells := make([]cell, len(row))
		for j, c := range row {
			cells[j] = parseCell(p, fmt.Sprintf("%s row %d cell %d", where, i+1, j+1), c)
		}
		e.rows = append(e.rows, cells)
	}
	return e
}

func parseCell(p *problems, where string, v any) cell {
	switch c := v.(type) {
	case string:
		if utf8.RuneCountInString(c) > maxItemChars {
			p.add(where, "is %d characters, the limit is %d", utf8.RuneCountInString(c), maxItemChars)
		}
		return cell{text: c}
	case json.Number:
		return cell{text: c.String()}
	case map[string]any:
		p.unknown(c, where, []string{"text", "tone"})
		out := cell{text: p.text(c, "text", where, false, maxItemChars)}
		if t, present := c["tone"]; present {
			name, _ := t.(string)
			if !contains(tones, name) {
				p.add(where, `"tone" must be one of: %s; got %s`, strings.Join(tones, ", "), show(t))
			}
			out.tone = name
		}
		return out
	default:
		p.add(where, `must be a string or {"text":..., "tone":...}, got %s`, show(v))
		return cell{}
	}
}

func parseStats(p *problems, where string, m map[string]any) element {
	list, ok := p.listOf(m, "items", where, 1, 4)
	e := statsEl{}
	if !ok {
		return e
	}
	for i, v := range list {
		w := fmt.Sprintf("%s item %d", where, i+1)
		item := p.object(v, w)
		if item == nil {
			continue
		}
		p.unknown(item, w, []string{"value", "label"})
		e.items = append(e.items, stat{value: p.scalar(item, "value", w), label: p.text(item, "label", w, true, maxItemChars)})
	}
	return e
}

func parseQuote(p *problems, where string, m map[string]any) element {
	return quoteEl{text: p.text(m, "text", where, true, maxTextChars), by: p.text(m, "by", where, false, 120)}
}

func parseSteps(p *problems, where string, m map[string]any) element {
	list, ok := p.listOf(m, "items", where, 2, 6)
	e := stepsEl{}
	if !ok {
		return e
	}
	for i, v := range list {
		w := fmt.Sprintf("%s item %d", where, i+1)
		if s, isString := v.(string); isString {
			e.items = append(e.items, step{title: s})
			continue
		}
		item := p.object(v, w)
		if item == nil {
			continue
		}
		p.unknown(item, w, []string{"title", "text"})
		e.items = append(e.items, step{title: p.text(item, "title", w, true, 80), text: p.text(item, "text", w, false, maxItemChars)})
	}
	return e
}

func parseColumns(p *problems, where string, m map[string]any) element {
	list, ok := p.listOf(m, "columns", where, 2, 3)
	e := columnsEl{}
	if !ok {
		return e
	}
	for i, v := range list {
		w := fmt.Sprintf("%s column %d", where, i+1)
		col := p.object(v, w)
		if col == nil {
			continue
		}
		p.unknown(col, w, []string{"title", "elements"})
		els, present := col["elements"]
		e.cols = append(e.cols, column{
			title: p.text(col, "title", w, false, 80),
			els:   parseElements(p, w, els, present, maxColumnElements, false),
		})
	}
	return e
}

func parseCards(p *problems, where string, m map[string]any) element {
	list, ok := p.listOf(m, "items", where, 2, 4)
	e := cardsEl{}
	if !ok {
		return e
	}
	highlighted := 0
	for i, v := range list {
		w := fmt.Sprintf("%s item %d", where, i+1)
		item := p.object(v, w)
		if item == nil {
			continue
		}
		p.unknown(item, w, []string{"title", "text", "items", "highlight"})
		c := card{title: p.text(item, "title", w, true, 80), text: p.text(item, "text", w, false, maxItemChars)}
		if _, present := item["items"]; present {
			c.items = p.strings(item, "items", w, 1, 6, maxItemChars)
		}
		if h, present := item["highlight"]; present {
			b, isBool := h.(bool)
			if !isBool {
				p.add(w, `"highlight" must be true or false, got %s`, show(h))
			}
			if c.highlight = b; b {
				highlighted++
			}
		}
		e.items = append(e.items, c)
	}
	if highlighted > 1 {
		p.add(where, "%d cards are highlighted; highlight at most one, the one to look at", highlighted)
	}
	return e
}

func parseProgress(p *problems, where string, m map[string]any) element {
	list, ok := p.listOf(m, "items", where, 1, 6)
	e := progressEl{}
	if !ok {
		return e
	}
	for i, v := range list {
		w := fmt.Sprintf("%s item %d", where, i+1)
		item := p.object(v, w)
		if item == nil {
			continue
		}
		p.unknown(item, w, []string{"label", "value", "text"})
		b := bar{label: p.text(item, "label", w, true, 80), text: p.text(item, "text", w, false, 40)}
		n, isNumber := item["value"].(json.Number)
		f, err := n.Float64()
		if !isNumber || err != nil || f < 0 || f > 100 {
			p.add(w, `needs "value": a number from 0 to 100 (percent), got %s`, show(item["value"]))
		}
		b.value = f
		e.items = append(e.items, b)
	}
	return e
}
