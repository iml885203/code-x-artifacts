package kind

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var editMark = regexp.MustCompile(`data-edit="([^"]*)"`)

// editPaths lists the places the page marks as editable on slide n.
func editPaths(t *testing.T, page string, n int) []string {
	t.Helper()
	start := strings.Index(page, `id="slide-`+strconv.Itoa(n)+`"`)
	if start < 0 {
		t.Fatalf("the page has no slide %d", n)
	}
	section := page[start:]
	if end := strings.Index(section, "</section>"); end >= 0 {
		section = section[:end]
	}
	var out []string
	for _, m := range editMark.FindAllStringSubmatch(section, -1) {
		out = append(out, m[1])
	}
	return out
}

// textAt reads the string a path names in a slide, the way the editor does:
// a step given as a string is its own title, a cell may be {"text", "tone"}
// and a stat's value may be a number.
func textAt(slide any, path string) (string, bool) {
	at := slide
	for _, step := range strings.Split(path, "/") {
		switch v := at.(type) {
		case map[string]any:
			at = v[step]
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(v) {
				return "", false
			}
			at = v[i]
		case string:
			if step != "title" {
				return "", false
			}
		default:
			return "", false
		}
	}
	switch v := at.(type) {
	case string:
		return v, true
	case json.Number:
		return v.String(), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case map[string]any:
		s, ok := v["text"].(string)
		return s, ok
	}
	return "", false
}

func TestEveryStringAPersonCanEditIsMarkedWithItsPlaceInTheSlide(t *testing.T) {
	all := everyElement()
	order := []string{"text", "bullets", "table", "stats", "quote", "steps", "columns", "cards"}
	var slides []map[string]any
	for _, typ := range order {
		slides = append(slides, sl("MARK-title "+typ, all[typ]))
	}
	slides = append(slides, sl("Code and bars", all["code"], all["progress"]))
	page, err := (Slides{}).Render(deck(t, slides...), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"text":    {"title", "elements/0/text"},
		"bullets": {"title", "elements/0/items/0", "elements/0/items/1"},
		"table":   {"title", "elements/0/columns/0", "elements/0/columns/1", "elements/0/rows/0/0", "elements/0/rows/0/1", "elements/0/rows/1/0", "elements/0/rows/1/1"},
		"stats":   {"title", "elements/0/items/0/value", "elements/0/items/0/label", "elements/0/items/1/value", "elements/0/items/1/label"},
		"quote":   {"title", "elements/0/text", "elements/0/by"},
		"steps":   {"title", "elements/0/items/0/title", "elements/0/items/0/text", "elements/0/items/1/title"},
		"columns": {"title", "elements/0/columns/0/title", "elements/0/columns/0/elements/0/text", "elements/0/columns/1/elements/0/items/0"},
		"cards":   {"title", "elements/0/items/0/title", "elements/0/items/0/text", "elements/0/items/0/items/0", "elements/0/items/1/title"},
	}
	for i, typ := range order {
		got := editPaths(t, page, i+1)
		sort.Strings(got)
		w := append([]string(nil), want[typ]...)
		sort.Strings(w)
		if strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("%s: the editable places are %v, want %v", typ, got, w)
		}
	}
	// Code keeps its line breaks and indentation, which a one-line field would lose; a bar's text is optional.
	if got := editPaths(t, page, len(order)+1); strings.Join(got, " ") != "title elements/1/items/0/label elements/1/items/1/label" {
		t.Errorf("code and progress mark %v, want only the title and the bar labels", got)
	}
}

// typedDeck is the hostile deck as the editor saves it: a plain deck with each
// slide replaced, one at a time, by the hostile one; it returns the content and
// the hostile slides.
func typedDeck(t *testing.T) (string, []json.RawMessage) {
	t.Helper()
	var top struct {
		Slides []json.RawMessage `json:"slides"`
	}
	if err := json.Unmarshal([]byte(hostileDeck(t)), &top); err != nil {
		t.Fatal(err)
	}
	plainSlides := make([]map[string]any, len(top.Slides))
	for i := range plainSlides {
		plainSlides[i] = sl("plain")
	}
	d, err := ParseDeck(deck(t, plainSlides...), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	for i, raw := range top.Slides {
		if d, err = d.Replace(i+1, raw); err != nil {
			t.Fatal(err)
		}
	}
	return d.Content(), top.Slides
}

// typedPlaces counts the editable places of the page that hold markup, and
// fails on a place that is not a string of its slide or that the hostile deck
// left without markup.
func typedPlaces(t *testing.T, page string, slides []json.RawMessage) int {
	t.Helper()
	marked := 0
	for i, raw := range slides {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var s any
		if err := dec.Decode(&s); err != nil {
			t.Fatal(err)
		}
		for _, path := range editPaths(t, page, i+1) {
			text, ok := textAt(s, path)
			switch {
			case !ok:
				t.Errorf("slide %d marks %q, which is not a string of the slide", i+1, path)
			case strings.Contains(text, markup) || strings.Contains(text, hostileShort):
				marked++
			case text != "x" && text != "plain":
				t.Errorf("slide %d place %q holds %q: every place a person can type into must be one the hostile deck fills", i+1, path, text)
			}
		}
	}
	return marked
}

func TestEveryStringAPersonTypesIntoASlideIsEscapedWhenTheDeckRenders(t *testing.T) {
	content, slides := typedDeck(t)
	if err := (Slides{}).Validate(content, slidesFormatV1); err != nil {
		t.Fatalf("the typed deck is not valid, so it proves nothing: %v", err)
	}
	p, err := (Slides{}).RenderPage(content, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	page := p.HTML
	if n := strings.Count(page, "<script"); n != 1 {
		t.Errorf("the page has %d script elements: the player must be the only one", n)
	}
	for _, bad := range []string{"<img", "window.pwned=1</script>", "onerror=alert(1)>"} {
		if strings.Contains(page, bad) {
			t.Errorf("the page contains %q as markup", bad)
		}
	}
	if marked := typedPlaces(t, page, slides); marked < 20 {
		t.Errorf("only %d editable places hold markup; the test must cover every field a person can type into", marked)
	}
	if got := strings.Count(page, esc(markup)); got < 26 {
		t.Errorf("only %d of the strings reached the page as escaped text, want all 26", got)
	}
}

func TestTheSlideListSaysWhichSlidesAreLikelyToOverflow(t *testing.T) {
	long := make([]string, 8)
	for i := range long {
		long[i] = strings.Repeat("a long line of words that wraps ", 6)
	}
	content := deck(t,
		sl("Short", el("text", map[string]any{"text": "fits"})),
		sl("Full", el("bullets", map[string]any{"items": long}), el("bullets", map[string]any{"items": long})),
	)
	d, err := ParseDeck(content, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	list := d.List()
	if list[0].Overflows || !list[1].Overflows {
		t.Errorf("the list is %+v, want only slide 2 likely to overflow", list)
	}
	b, err := json.Marshal(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"number":1,"title":"Short"}` {
		t.Errorf("a slide that fits is listed as %s, the same as before", b)
	}
}
