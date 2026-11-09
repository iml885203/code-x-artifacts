package kind

import (
	_ "embed" // slides.css, slides.js
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

//go:embed slides.css
var slidesCSS string

//go:embed slides.js
var slidesPlayer string

// slidesStyle is the stylesheet with its canvas units expanded.
var slidesStyle = expandUnits(slidesCSS)

var canvasUnit = regexp.MustCompile(`(^|[^\w#.-])(\d+(?:\.\d+)?)([uk])\b`)

// expandUnits turns the stylesheet's "40u" (canvas pixels) and "40k" (canvas
// pixels times the slide's type scale) into calc() on the variables the
// canvas defines.
func expandUnits(css string) string {
	return canvasUnit.ReplaceAllStringFunc(css, func(m string) string {
		parts := canvasUnit.FindStringSubmatch(m)
		variable := "--u"
		if parts[3] == "k" {
			variable = "--k"
		}
		return parts[1] + "calc(" + parts[2] + " * var(" + variable + "))"
	})
}

// Render implements Kind.
func (s Slides) Render(content, format string) (string, error) {
	p, err := s.RenderPage(content, format)
	return p.HTML, err
}

// RenderPage implements PageRenderer. The page's only script is the player,
// which is the same text for every deck, so the server pins script-src to its
// hash and a deck cannot add code of its own.
func (Slides) RenderPage(content, format string) (Page, error) {
	return deckPage(content, format, false)
}

// RenderPreview implements PreviewRenderer: every slide and no script, so a
// card or a filmstrip thumbnail runs nothing. The first slide shows, or the one
// the address names by its fragment (#slide-3), by CSS alone. InlineScripts is
// empty rather than nil, so the server allows no script but its preview stub.
func (Slides) RenderPreview(content, format string) (Page, error) {
	return deckPage(content, format, true)
}

// slidesPreviewStyle shows the slide a preview's fragment names in place of
// the first one.
const slidesPreviewStyle = `.slide:target { display: flex; }
.canvas:has(.slide:target) .slide.on:not(:target) { display: none; }
`

// deckPage draws the deck: with the player and its controls, or as a preview.
func deckPage(content, format string, preview bool) (Page, error) {
	if format != slidesFormatV1 {
		return Page{}, fmt.Errorf("slides has no content format %q", format)
	}
	raw, err := splitDeck(content)
	if err != nil {
		return Page{}, err
	}
	title := "Slides"
	var body strings.Builder
	for i, r := range raw {
		s, problems := parseSlide(i+1, r)
		if i == 0 && len(problems) == 0 {
			title = plain(s.title)
		}
		body.WriteString(renderSlide(i, len(raw), s, problems))
	}
	style := slidesStyle
	if preview {
		style += slidesPreviewStyle
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"" + docLanguage(content) + "\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<meta name=\"color-scheme\" content=\"dark light\">\n")
	b.WriteString("<title>" + esc(title) + "</title>\n<style>@layer kit;\n@import url(\"/kit/v1/pictor.css\") layer(kit);</style>\n<style>\n")
	b.WriteString(style)
	b.WriteString("</style>\n</head>\n<body>\n<div class=\"deck\" id=\"deck\">\n<div class=\"stage\"><div class=\"frame\"><div class=\"canvas\">\n")
	b.WriteString(body.String())
	b.WriteString("</div></div></div>\n")
	if preview {
		b.WriteString("</div>\n</body>\n</html>\n")
		return Page{HTML: b.String(), InlineScripts: []string{}}, nil
	}
	b.WriteString("<div class=\"notes-panel\" id=\"notes-panel\" aria-live=\"polite\"></div>\n")
	b.WriteString(`<div class="controls" id="bar" hidden><button id="prev" type="button" aria-label="Previous slide">&lsaquo;</button>` +
		`<span class="count" id="count"></span><button id="next" type="button" aria-label="Next slide">&rsaquo;</button>` +
		`<button id="notes-toggle" type="button" aria-pressed="false">Notes</button>` +
		`<button id="full-toggle" type="button" aria-pressed="false">Full screen</button></div>` + "\n</div>\n")
	b.WriteString("<script>" + slidesPlayer + "</script>\n</body>\n</html>\n")
	return Page{HTML: b.String(), InlineScripts: []string{slidesPlayer}}, nil
}

// BrowserFormats implements BrowserExporter: the browser prints the deck,
// whose print stylesheet gives every slide a 16:9 page of its own.
func (Slides) BrowserFormats() []string { return []string{"pdf"} }

// renderSlide draws slide i of n. A slide with problems, or one the renderer
// itself fails on, becomes an error slide that says what is wrong, so one bad
// slide never takes the deck down.
func renderSlide(i, n int, s slide, problems []string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = errorSlide(i, n, []string{fmt.Sprint("the slide could not be drawn: ", r)})
		}
	}()
	if len(problems) > 0 {
		return errorSlide(i, n, problems)
	}
	scale, _, _, _ := s.fit()
	class := "slide layout-" + s.layout
	if len(s.elements) == 0 {
		class += " no-body"
	}
	if i == 0 {
		class += " on"
	}
	var b strings.Builder
	heading := "h2"
	if s.layout == layoutTitle {
		heading = "h1"
	}
	fmt.Fprintf(&b, `<section class="%s" id="slide-%d" style="--s:%.2f" aria-label="Slide %d of %d">`, class, i+1, scale, i+1, n)
	b.WriteString("<" + heading + ` class="title"` + editable("title") + `>` + inline(s.title) + "</" + heading + ">")
	if len(s.elements) > 0 {
		b.WriteString(`<div class="body">`)
		for i, e := range s.elements {
			e.render(&b, child("elements", i))
		}
		b.WriteString("</div>")
	}
	b.WriteString(`<aside class="notes" hidden>` + esc(s.notes) + "</aside></section>\n")
	return b.String()
}

func errorSlide(i, n int, problems []string) string {
	class := "slide layout-error"
	if i == 0 {
		class += " on"
	}
	return fmt.Sprintf(`<section class="%s" id="slide-%d" aria-label="Slide %d of %d"><h2 class="title">Slide %d cannot be shown</h2><div class="body"><pre class="problems">%s</pre></div><aside class="notes" hidden></aside></section>`+"\n",
		class, i+1, i+1, n, i+1, esc(strings.Join(problems, "\n")))
}

// ---- fitting ----

// The room a slide's content has, in canvas pixels: the canvas less the
// padding the stylesheet gives it.
const (
	innerWidth  = 1920.0 - 2*120
	innerHeight = 1080.0 - 2*72
	minScale    = 0.75
	maxScale    = 1.2
)

// titleHeight is what the slide's title takes, with the gap under it when
// something follows.
func (s slide) titleHeight() float64 {
	size, line := 76.0, 85.0
	if s.layout == layoutTitle {
		size, line = 120, 128
	}
	h := lines(s.title, innerWidth, size*1.04) * line
	if len(s.elements) > 0 {
		h += 44
	}
	return h
}

// fit chooses the type scale for the slide: the largest in [minScale,
// maxScale] at which the elements are estimated to fit under the title, so a
// sparse slide gets bigger type and a full one smaller. fits is false when
// even the smallest scale is estimated to overflow; need is the height the
// elements take at the scale returned and room the height they have.
func (s slide) fit() (scale float64, fits bool, need, room float64) {
	room = innerHeight - s.titleHeight()
	if len(s.elements) == 0 {
		return 1, s.titleHeight() <= innerHeight, 0, room
	}
	for k := maxScale; k >= minScale-1e-9; k -= 0.05 {
		k = math.Round(k*100) / 100
		need = stack(s.elements, innerWidth, k, gap(k))
		if need <= room {
			return k, true, need, room
		}
	}
	return minScale, false, need, room
}

// Review implements Reviewer: what the agent cannot see for itself after a
// publish. previous is the content of the version this one replaces, or empty
// for a new artifact.
func (Slides) Review(content, format, previous string) []string {
	if format != slidesFormatV1 {
		return nil
	}
	raw, err := splitDeck(content)
	if err != nil {
		return nil
	}
	var notes []string
	if before, err := splitDeck(previous); previous != "" && err == nil && len(before) != len(raw) {
		notes = append(notes, fmt.Sprintf("The deck went from %s to %s.", plural(len(before), "slide"), plural(len(raw), "slide")))
	}
	var overflowing, silent []string
	for i, r := range raw {
		s, problems := parseSlide(i+1, r)
		if len(problems) > 0 {
			continue
		}
		if _, fits, need, room := s.fit(); !fits {
			overflowing = append(overflowing, fmt.Sprintf("Slide %d (about %.0f px of content for %.0f px of room, even at the smallest type)", i+1, need, room))
		}
		if strings.TrimSpace(s.notes) == "" {
			silent = append(silent, fmt.Sprint(i+1))
		}
	}
	for _, o := range overflowing {
		notes = append(notes, o+" is likely to overflow the canvas; shorten it or split it over two slides.")
	}
	if len(silent) > 0 {
		notes = append(notes, "Slides without speaker notes: "+strings.Join(silent, ", ")+". Add \"notes\" to every slide; the presenter reads them in present mode.")
	}
	return notes
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// docLanguage lets the browser pick the right CJK glyph variants: any Han
// character makes the page Traditional Chinese, the house default.
func docLanguage(content string) string {
	for _, r := range content {
		if unicode.Is(unicode.Han, r) {
			return "zh-Hant"
		}
	}
	return "en"
}
