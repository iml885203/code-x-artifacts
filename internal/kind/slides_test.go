package kind

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// deck builds deck JSON from slides given as Go values.
func deck(t testing.TB, slides ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"version": 1, "slides": slides})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sl(title string, els ...map[string]any) map[string]any {
	if len(els) == 0 {
		els = []map[string]any{el("text", map[string]any{"text": "body"})}
	}
	return map[string]any{"title": title, "notes": "notes for " + title, "elements": els}
}

func el(typ string, fields map[string]any) map[string]any {
	out := map[string]any{"type": typ}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

// everyElement is one valid element of every type, each with a recognisable
// marker string so a test can find it in the page.
func everyElement() map[string]map[string]any {
	return map[string]map[string]any{
		"text":     el("text", map[string]any{"text": "MARK-text with `inline` and **bold**"}),
		"bullets":  el("bullets", map[string]any{"items": []string{"MARK-bullet one", "two"}}),
		"numbered": el("numbered", map[string]any{"items": []string{"MARK-number one", "two"}}),
		"code":     el("code", map[string]any{"code": "MARK-code()\nreturn 1", "language": "go"}),
		"table": el("table", map[string]any{
			"columns": []string{"MARK-head", "b"},
			"rows":    [][]any{{"MARK-cell", map[string]any{"text": "ok", "tone": "success"}}, {"x", map[string]any{"text": "bad", "tone": "danger"}}},
		}),
		"stats":    el("stats", map[string]any{"items": []map[string]any{{"value": "MARK-42", "label": "answers"}, {"value": 7, "label": "wonders"}}}),
		"quote":    el("quote", map[string]any{"text": "MARK-quote", "by": "someone"}),
		"steps":    el("steps", map[string]any{"items": []any{map[string]any{"title": "MARK-step", "text": "does it"}, "second"}}),
		"columns":  el("columns", map[string]any{"columns": []map[string]any{{"title": "MARK-col", "elements": []map[string]any{{"type": "text", "text": "left"}}}, {"elements": []map[string]any{{"type": "bullets", "items": []string{"right"}}}}}}),
		"cards":    el("cards", map[string]any{"items": []map[string]any{{"title": "MARK-card", "text": "small", "items": []string{"a"}}, {"title": "pick me", "highlight": true}}}),
		"progress": el("progress", map[string]any{"items": []map[string]any{{"label": "MARK-bar", "value": 62.5}, {"label": "done", "value": 100, "text": "all"}}}),
	}
}

func TestEveryElementTypeRendersInEveryLayout(t *testing.T) {
	for _, layout := range slidesLayouts {
		for _, typ := range elementTypes {
			s := sl("Title with `code` in it", everyElement()[typ])
			s["layout"] = layout
			page, err := (Slides{}).Render(deck(t, s), slidesFormatV1)
			if err != nil {
				t.Errorf("%s in %s: %v", typ, layout, err)
				continue
			}
			if !strings.Contains(page, `class="slide layout-`+layout+` on"`) || !strings.Contains(page, "<code>code</code> in it") {
				t.Errorf("%s in %s: the slide or its inline-code title is missing", typ, layout)
			}
			if strings.Contains(page, "cannot be shown") {
				t.Errorf("%s in %s: drawn as an error slide", typ, layout)
			}
			if !strings.Contains(page, "MARK-") && typ != "text" {
				t.Errorf("%s in %s: its content is not on the page", typ, layout)
			}
			if !strings.Contains(page, `class="el `+map[string]string{"bullets": "list", "numbered": "list"}[typ]) && !strings.Contains(page, `class="el `+typ) {
				t.Errorf("%s in %s: no element of that type on the page", typ, layout)
			}
		}
	}
}

func TestEveryLayoutAndElementTypeHasAStyle(t *testing.T) {
	for _, class := range append([]string{"layout-title", "layout-content", "layout-error", "highlight", "tone-success", "tone-warning", "tone-danger", "title"},
		"text", "list", "numbered", "code", "table", "stats", "quote", "steps", "columns", "cards", "progress") {
		if !strings.Contains(slidesCSS, "."+class) {
			t.Errorf("slides.css has no rule for .%s", class)
		}
	}
	if strings.Contains(slidesCSS, "<") {
		t.Error("slides.css contains a < and could end the style element")
	}
}

func TestCanvasUnitsAreAllExpanded(t *testing.T) {
	if stray := regexp.MustCompile(`(^|[^\w#.-])\d+(\.\d+)?[uk]\b`).FindString(slidesStyle); stray != "" {
		t.Errorf("an unexpanded canvas unit is left in the page's stylesheet: %q", stray)
	}
	if !strings.Contains(slidesStyle, "calc(76 * var(--u))") || !strings.Contains(slidesStyle, "calc(40 * var(--k))") {
		t.Error("units were not turned into calc() on --u and --k")
	}
}

func TestSlidesRefusesAnInvalidDeckNamingTheSlideAndElement(t *testing.T) {
	good := sl("Fine", el("text", map[string]any{"text": "ok"}))
	for name, tc := range map[string]struct {
		content string
		want    []string
	}{
		"a table without rows": {
			deck(t, good, sl("b"), sl("Numbers", el("text", map[string]any{"text": "x"}), el("table", map[string]any{"columns": []string{"a"}}))),
			[]string{"Slide 3 element 2 (table)", `table needs "columns"`, `"rows"`},
		},
		"an unknown element type": {
			deck(t, sl("a", el("text", map[string]any{"text": "x"})), sl("b", el("bulets", map[string]any{"items": []string{"x"}}))),
			[]string{"Slide 2 element 1", `unknown type "bulets"`, "text, bullets, numbered, code, table, stats, quote, steps, columns, cards, progress"},
		},
		"an unknown field":             {deck(t, sl("a", el("text", map[string]any{"text": "x", "size": 3}))), []string{"Slide 1 element 1 (text)", `unknown field "size"`, "type, text"}},
		"a slide without a title":      {`{"version":1,"slides":[{"elements":[]}]}`, []string{"Slide 1", `needs "title"`}},
		"an unknown layout":            {deck(t, map[string]any{"title": "a", "layout": "hero", "elements": []any{map[string]any{"type": "text", "text": "x"}}}), []string{"Slide 1", `"layout" must be one of: title, content`}},
		"a content slide with nothing": {deck(t, map[string]any{"title": "a"}), []string{"Slide 1", `needs "elements"`}},
		"too many elements": {
			deck(t, sl("a", repeat(el("text", map[string]any{"text": "x"}), maxSlideElements+1)...)),
			[]string{"Slide 1", "has 9 elements, the limit is 8", "split"},
		},
		"a ragged table row": {
			deck(t, sl("a", el("table", map[string]any{"columns": []string{"a", "b"}, "rows": [][]string{{"1", "2"}, {"1"}}}))),
			[]string{"Slide 1 element 1 (table)", "row 2 must be a list with 2 cells"},
		},
		"a wrong tone": {
			deck(t, sl("a", el("table", map[string]any{"columns": []string{"a"}, "rows": [][]any{{map[string]any{"text": "1", "tone": "purple"}}}}))),
			[]string{"row 1 cell 1", `"tone" must be one of: success, warning, danger`},
		},
		"columns inside columns": {
			deck(t, sl("a", el("columns", map[string]any{"columns": []map[string]any{
				{"elements": []map[string]any{{"type": "columns", "columns": []any{}}}},
				{"elements": []map[string]any{{"type": "text", "text": "x"}}},
			}}))),
			[]string{"Slide 1 element 1 (columns) column 1 element 1", "columns cannot go inside columns or cards"},
		},
		"two highlighted cards": {
			deck(t, sl("a", el("cards", map[string]any{"items": []map[string]any{{"title": "a", "highlight": true}, {"title": "b", "highlight": true}}}))),
			[]string{"Slide 1 element 1 (cards)", "highlight at most one"},
		},
		"progress out of range": {
			deck(t, sl("a", el("progress", map[string]any{"items": []map[string]any{{"label": "a", "value": 120}}}))),
			[]string{"item 1", `"value": a number from 0 to 100`},
		},
		"too many stats":    {deck(t, sl("a", el("stats", map[string]any{"items": repeat(map[string]any{"value": "1", "label": "x"}, 5)}))), []string{"stats", `"items" has 5 entries, it needs 1 to 4`}},
		"code over a limit": {deck(t, sl("a", el("code", map[string]any{"code": strings.Repeat("x\n", 20)}))), []string{"Slide 1 element 1 (code)", "20 lines, the limit is 16"}},
		"a long string":     {deck(t, sl("a", el("text", map[string]any{"text": strings.Repeat("x", 601)}))), []string{`"text" is 601 characters, the limit is 600`}},
		"the wrong version": {`{"version":2,"slides":[{"title":"a","elements":[{"type":"text","text":"x"}]}]}`, []string{"version", "must be 1"}},
		"no slides":         {`{"version":1,"slides":[]}`, []string{"at least one slide"}},
		"a code fence":      {"```json\n{}\n```", []string{"code fence", "JSON itself"}},
		"broken JSON":       {"{\"version\":1,\n\"slides\": [", []string{"not valid JSON", "line"}},
		"text after":        {deck(t, good) + " more", []string{"text after the JSON"}},
		"not an object":     {"[1]", []string{"must be a JSON object"}},
		"unknown top field": {`{"version":1,"slides":[{"title":"a","elements":[{"type":"text","text":"x"}]}],"theme":"red"}`, []string{"deck", `unknown field "theme"`}},
		"an empty body":     {"", []string{"needs content"}},
		"too big":           {strings.Repeat(" ", MaxSlidesBytes+1), []string{"at most 1 MB"}},
	} {
		err := (Slides{}).Validate(tc.content, slidesFormatV1)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error does not mention %q:\n%s", name, want, err)
			}
		}
	}
	if err := (Slides{}).Validate(deck(t, good), "9"); err == nil || !strings.Contains(err.Error(), "format") {
		t.Errorf("a format this build lacks: %v", err)
	}
}

func repeat[T any](v T, n int) []T {
	out := make([]T, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestSlidesReportsEveryProblemInOneRound(t *testing.T) {
	err := (Slides{}).Validate(deck(t,
		sl("a", el("bulets", nil)),
		sl("b", el("table", map[string]any{"columns": []string{"a"}})),
		map[string]any{"elements": []any{}},
	), slidesFormatV1)
	if err == nil {
		t.Fatal("accepted")
	}
	for _, want := range []string{"4 problem(s)", "Slide 1 element 1", "Slide 2 element 1 (table)", "Slide 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%s", want, err)
		}
	}
}

func TestAcceptedBoundaries(t *testing.T) {
	title := map[string]any{"title": "Title only", "layout": "title"}
	atLimit := make([]map[string]any, maxSlideElements)
	for i := range atLimit {
		atLimit[i] = el("text", map[string]any{"text": strings.Repeat("x", maxTextChars)})
	}
	for name, content := range map[string]string{
		"a title slide without elements":       deck(t, title),
		"a title slide with an empty list":     `{"version":1,"slides":[{"layout":"title","title":"a","elements":[]}]}`,
		"a layout left out is content":         deck(t, sl("a", el("text", map[string]any{"text": "x"}))),
		"elements and strings at their limits": deck(t, sl(strings.Repeat("t", maxTitleChars), atLimit...)),
		"a number as a table cell":             deck(t, sl("a", el("table", map[string]any{"columns": []string{"n"}, "rows": [][]any{{3}}}))),
	} {
		if err := (Slides{}).Validate(content, slidesFormatV1); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

func TestOneFailingSlideRendersAsAnErrorSlideAndTheRestOfTheDeckStillWorks(t *testing.T) {
	// Render does not require the deck to pass Validate, so a deck stored before
	// a rule tightened still opens, with the slide that no longer fits shown as such.
	content := deck(t, sl("First", el("text", map[string]any{"text": "fine"})), sl("Second", el("nonsense", nil)), sl("Third", el("text", map[string]any{"text": "also fine"})))
	page, err := (Slides{}).Render(content, slidesFormatV1)
	if err != nil {
		t.Fatalf("a deck with one bad slide did not render: %v", err)
	}
	if strings.Count(page, `<section class="slide`) != 3 {
		t.Errorf("want three slides, the page has %d", strings.Count(page, `<section class="slide`))
	}
	for _, want := range []string{"Slide 2 cannot be shown", `unknown type &#34;nonsense&#34;`, "First", "also fine"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Count(page, "layout-error") < 2 { // the class in the stylesheet and on the slide
		t.Error("the bad slide is not an error slide")
	}
}

func TestADeckThatIsNotJSONCannotBeRendered(t *testing.T) {
	if _, err := (Slides{}).Render("<html>", slidesFormatV1); err == nil {
		t.Error("rendered something that is not a deck")
	}
	if _, err := (Slides{}).Render(deck(t, sl("a", el("text", map[string]any{"text": "x"}))), "9"); err == nil {
		t.Error("rendered a format this build lacks")
	}
}

// markup is text that would be dangerous if it ever reached the page as HTML.
const markup = `</script><script>window.pwned=1</script><img src=x onerror=alert(1)>`

// hostileShort is markup that fits the fields with a tight limit.
const hostileShort = "<img onerror=1>"

// hostileDeck is a valid deck where every string field holds markup.
func hostileDeck(t testing.TB) string {
	m, short := markup, hostileShort
	mk := func(extra string) string { return m + extra }
	return deck(t,
		map[string]any{"layout": "title", "title": mk(" title"), "notes": mk(" notes"), "elements": []map[string]any{
			{"type": "text", "text": mk(" text")},
			{"type": "quote", "text": mk(" quote"), "by": mk(" by")},
		}},
		sl(mk(" t2"),
			el("bullets", map[string]any{"items": []string{mk(" b")}}),
			el("numbered", map[string]any{"items": []string{mk(" n")}}),
			el("code", map[string]any{"code": mk(" code"), "language": short}),
			el("table", map[string]any{"columns": []string{mk(" th")}, "rows": [][]any{{map[string]any{"text": mk(" td"), "tone": "success"}}}}),
			el("stats", map[string]any{"items": []map[string]any{{"value": mk(" v"), "label": mk(" l")}}}),
			el("steps", map[string]any{"items": []any{map[string]any{"title": mk(" st"), "text": mk(" sx")}, mk(" s2")}}),
		),
		sl(mk(" t3"),
			el("columns", map[string]any{"columns": []map[string]any{
				{"title": mk(" ct"), "elements": []map[string]any{{"type": "text", "text": mk(" in col")}}},
				{"elements": []map[string]any{{"type": "bullets", "items": []string{mk(" col b")}}}},
			}}),
			el("cards", map[string]any{"items": []map[string]any{{"title": mk(" card"), "text": mk(" ctext"), "items": []string{mk(" citem")}}, {"title": "x"}}}),
			el("progress", map[string]any{"items": []map[string]any{{"label": mk(" bar"), "value": 5, "text": short}}}),
		),
		sl("`"+m+"` and **"+m+"**", el("text", map[string]any{"text": "plain"})),
	)
}

func TestEveryStringOfADeckIsEscapedAndNothingBecomesMarkup(t *testing.T) {
	m, short := markup, hostileShort
	content := hostileDeck(t)
	if err := (Slides{}).Validate(content, slidesFormatV1); err != nil {
		t.Fatalf("the hostile deck is not valid, so it proves nothing: %v", err)
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
	// Every field's text arrives escaped: the marker survives as text, ten times over.
	escaped := esc(m)
	if got := strings.Count(page, escaped); got < 26 {
		t.Errorf("only %d of the strings reached the page as escaped text, want all 26", got)
	}
	if got := strings.Count(page, esc(short)); got != 2 {
		t.Errorf("%d of the 2 short strings reached the page as escaped text", got)
	}
	if strings.Contains(page, `onerror=alert(1)>`) {
		t.Error("an event-handler attribute reached the page")
	}
}

func TestInlineMarkupIsOnlyBoldAndCode(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                    "plain",
		"a **bold** b":             "a <strong>bold</strong> b",
		"use `x < y` here":         "use <code>x &lt; y</code> here",
		"**bold with `code`**":     "<strong>bold with <code>code</code></strong>",
		"unmatched ** and ` stay":  "unmatched ** and ` stay",
		"<b>no</b> & [link](http)": "&lt;b&gt;no&lt;/b&gt; &amp; [link](http)",
		"**a** **b**":              "<strong>a</strong> <strong>b</strong>",
		"emoji 🎉 and 中文":           "emoji 🎉 and 中文",
	} {
		if got := inline(in); got != want {
			t.Errorf("inline(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThePageDeclaresExactlyItsOneInlineScript(t *testing.T) {
	p, err := (Slides{}).RenderPage(deck(t, sl("a", el("text", map[string]any{"text": "x"}))), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.InlineScripts) != 1 || p.InlineScripts[0] != slidesPlayer {
		t.Fatalf("InlineScripts = %d entries, want the player alone", len(p.InlineScripts))
	}
	if !strings.Contains(p.HTML, "<script>"+slidesPlayer+"</script>") || strings.Count(p.HTML, "<script") != 1 {
		t.Error("the page does not carry the player exactly once, as declared")
	}
	if strings.Contains(slidesPlayer, "</script") || strings.Contains(slidesPlayer, "innerHTML") || strings.Contains(slidesPlayer, "eval(") {
		t.Error("the player ends its own script element, writes markup or evaluates strings")
	}
	if _, ok := Kind(Slides{}).(PageRenderer); !ok {
		t.Error("slides is generated by the platform and must declare its scripts")
	}
	if strings.Contains(p.HTML, "/vendor/") || regexp.MustCompile(`https?://`).MatchString(strings.ReplaceAll(p.HTML, "http://www.w3.org", "")) {
		t.Error("the page loads something that is not the kit")
	}
}

func TestThePageIsACanvasThatScalesWithoutScript(t *testing.T) {
	page, err := (Slides{}).Render(deck(t, sl("Hello", el("text", map[string]any{"text": "x"})), sl("Two")), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`@import url("/kit/v1/pictor.css") layer(kit)`, "<title>Hello</title>", "container-type: inline-size", "aspect-ratio: 16 / 9", `aria-label="Slide 1 of 2"`,
		`class="slide layout-content on"`, `<aside class="notes" hidden>notes for Hello</aside>`, `id="bar" hidden`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(page, `layout-content on"`+"") && strings.Count(page, ` on"`) != 1 {
		t.Error("more than one slide is visible before the player runs")
	}
}

func TestADeckInChineseIsMarkedTraditionalChinese(t *testing.T) {
	page, err := (Slides{}).Render(deck(t, sl("季度回顧")), slidesFormatV1)
	if err != nil || !strings.Contains(page, `<html lang="zh-Hant">`) {
		t.Errorf("lang: %v", err)
	}
}

func TestAPublishNotesWhenTheSlideCountChanges(t *testing.T) {
	three := deck(t, sl("a"), sl("b"), sl("c"))
	two := deck(t, sl("a"), sl("b"))
	one := deck(t, sl("a"))
	review := func(content, previous string) string {
		return strings.Join((Slides{}).Review(content, slidesFormatV1, previous), "\n")
	}
	if got := review(two, three); !strings.Contains(got, "went from 3 slides to 2 slides") {
		t.Errorf("3 to 2: %q", got)
	}
	if got := review(one, two); !strings.Contains(got, "from 2 slides to 1 slide.") {
		t.Errorf("2 to 1: %q", got)
	}
	if got := review(three, three); strings.Contains(got, "went from") {
		t.Errorf("an unchanged count is reported: %q", got)
	}
	if got := review(three, ""); strings.Contains(got, "went from") {
		t.Errorf("a new deck has nothing to compare: %q", got)
	}
	if got := review(three, "not json"); strings.Contains(got, "went from") {
		t.Errorf("an unreadable previous version was compared: %q", got)
	}
}

func TestAPublishListsTheSlidesWithoutNotes(t *testing.T) {
	quiet := sl("quiet")
	quiet["notes"] = "  "
	missing := sl("missing")
	delete(missing, "notes")
	got := strings.Join((Slides{}).Review(deck(t, sl("fine"), quiet, missing), slidesFormatV1, ""), "\n")
	if !strings.Contains(got, "without speaker notes: 2, 3") {
		t.Errorf("review = %q", got)
	}
	if advice := (Slides{}).Review(deck(t, sl("fine")), slidesFormatV1, ""); len(advice) != 0 {
		t.Errorf("a deck with notes everywhere got advice: %v", advice)
	}
}

func longBullets(n int) map[string]any {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf("Bullet %d is a fairly long sentence that is certain to wrap onto a second line when drawn at body size", i+1)
	}
	return el("bullets", map[string]any{"items": items})
}

func TestALongSlideGetsAnOverflowWarningAndAShortOneDoesNot(t *testing.T) {
	short := sl("Short", el("bullets", map[string]any{"items": []string{"one", "two", "three"}}))
	tableRows := make([][]string, 12)
	for i := range tableRows {
		tableRows[i] = []string{"a row", "with some words in the second cell that wrap", "third"}
	}
	long := sl("Long", longBullets(8), el("table", map[string]any{"columns": []string{"a", "b", "c"}, "rows": tableRows}))
	got := strings.Join((Slides{}).Review(deck(t, short, long), slidesFormatV1, ""), "\n")
	if !strings.Contains(got, "Slide 2 (about") || !strings.Contains(got, "likely to overflow the canvas") || !strings.Contains(got, "split it over two slides") {
		t.Errorf("no overflow warning for the long slide: %q", got)
	}
	if strings.Contains(got, "Slide 1 (about") {
		t.Errorf("the short slide was flagged: %q", got)
	}
	// What the guide recommends has to fit: about 6 lines, tables up to 7 rows.
	guideSlide := sl("Recommended", el("text", map[string]any{"text": "Intro line"}), el("table", map[string]any{"columns": []string{"a", "b", "c"}, "rows": tableRows[:7]}))
	if got := strings.Join((Slides{}).Review(deck(t, guideSlide), slidesFormatV1, ""), "\n"); strings.Contains(got, "overflow") {
		t.Errorf("a slide inside the guide's own advice overflows: %q", got)
	}
}

func TestTypeScalesWithHowMuchIsOnTheSlide(t *testing.T) {
	scaleOf := func(s map[string]any) float64 {
		raw, err := splitDeck(deck(t, s))
		if err != nil {
			t.Fatal(err)
		}
		parsed, problems := parseSlide(1, raw[0])
		if len(problems) > 0 {
			t.Fatal(problems)
		}
		k, _, _, _ := parsed.fit()
		return k
	}
	sparse := scaleOf(sl("One line", el("text", map[string]any{"text": "Short."})))
	full := scaleOf(sl("Full", longBullets(8)))
	if sparse != maxScale || full >= 1 || full < minScale {
		t.Errorf("a sparse slide scales to %v and a full one to %v; want %v and something in [%v, 1)", sparse, full, maxScale, minScale)
	}
}

func TestSlidesIsRegisteredVersionedAndTeachesTheRules(t *testing.T) {
	k, err := Lookup("slides")
	if err != nil {
		t.Fatal(err)
	}
	if got := k.FormatVersions(); len(got) != 1 || got[0] != "1" || CurrentFormat(k) != "1" || HasFormat(k, "") {
		t.Errorf("FormatVersions = %v", got)
	}
	if _, ok := k.(Reviewer); !ok {
		t.Error("slides must review what it accepted")
	}
	g := k.Guide()
	for _, want := range []string{
		`"version": 1`, `"layout"`, "notes", "6 lines", "7 rows", "`bullets`", "`table`", "`stats`", "`quote`", "`steps`", "`columns`", "`cards`", "`progress`",
		"`code`", "`numbered`", "`text`", "highlight", "tone", "**bold**", "review", "baseVersion", "full screen", "?slide=3",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("the slides guide never mentions %q", want)
		}
	}
	for _, typ := range elementTypes {
		if !strings.Contains(g, "| `"+typ+"` |") {
			t.Errorf("the guide's element table has no row for %s", typ)
		}
	}
}

// The guide's examples are what an agent copies first, so they must be valid.
func TestEveryJSONExampleInTheGuideIsAValidDeck(t *testing.T) {
	blocks := regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(slidesGuide, -1)
	if len(blocks) < 2 {
		t.Fatalf("the guide has %d JSON examples, want the shape and one complete deck", len(blocks))
	}
	for i, b := range blocks {
		if err := (Slides{}).Validate(b[1], slidesFormatV1); err != nil {
			t.Errorf("guide example %d: %v", i+1, err)
		}
		if notes := (Slides{}).Review(b[1], slidesFormatV1, ""); len(notes) > 0 {
			t.Errorf("guide example %d teaches a deck the review would complain about: %v", i+1, notes)
		}
	}
}
