package pptx_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/pptx"
)

// everything is a deck with every element type, a cover slide and notes.
func everything() pptx.Deck {
	p := pptx.Plain
	return pptx.Deck{Title: "Everything", Slides: []pptx.Slide{
		{Cover: true, Title: p("Cover title"), Notes: "Welcome.\nSecond line of notes.", Elements: []pptx.Element{pptx.Paragraph{Text: p("Subtitle under the cover")}}},
		{Title: p("Text and lists"), Notes: "Lists.", Elements: []pptx.Element{
			pptx.Paragraph{Text: pptx.Text{{Text: "A paragraph with "}, {Text: "bold", Bold: true}, {Text: " and "}, {Text: "code", Code: true}, {Text: "\nand a second line"}}},
			pptx.List{Items: []pptx.Text{p("bullet one"), p("bullet two")}},
			pptx.List{Ordered: true, Items: []pptx.Text{p("first"), p("second"), p("third")}},
		}},
		{Title: p("Code and table"), Elements: []pptx.Element{
			pptx.Code{Code: "POST /v1/charges\nIdempotency-Key: 7f3a", Language: "http"},
			pptx.Table{Columns: []pptx.Text{p("Metric"), p("Now"), p("Target")}, Rows: [][]pptx.Cell{
				{{Text: p("Revenue")}, {Text: p("$1.1M"), Tone: pptx.ToneWarning}, {Text: p("$1.4M")}},
				{{Text: p("Churn")}, {Text: p("2.1%"), Tone: pptx.ToneSuccess}, {Text: p("2.5%"), Tone: pptx.ToneDanger}},
			}},
		}},
		{Title: p("Numbers"), Elements: []pptx.Element{
			pptx.Stats{Items: []pptx.Stat{{Value: p("12 min"), Label: p("median deploy")}, {Value: p("99.95%"), Label: p("uptime")}}},
			pptx.Quote{Text: p("Price is what you pay."), By: p("Warren Buffett")},
		}},
		{Title: p("Flow"), Elements: []pptx.Element{
			pptx.Steps{Items: []pptx.Step{{Title: p("Queue"), Text: p("Accepted fast")}, {Title: p("Validate")}, {Title: p("Charge"), Text: p("Idempotent")}}},
			pptx.Progress{Items: []pptx.Bar{{Label: p("Revenue to target"), Value: 78}, {Label: p("Hiring"), Value: 40, Text: p("4 of 10")}}},
		}},
		{Title: p("Side by side"), Elements: []pptx.Element{
			pptx.Columns{Columns: []pptx.Column{
				{Title: p("Left column"), Elements: []pptx.Element{pptx.Paragraph{Text: p("in the left column")}}},
				{Elements: []pptx.Element{pptx.List{Items: []pptx.Text{p("in the right column")}}}},
			}},
			pptx.Cards{Items: []pptx.Card{
				{Title: p("Starter"), Text: p("For trying it out"), Items: []pptx.Text{p("1 project")}},
				{Title: p("Team"), Text: p("Recommended"), Highlight: true, Items: []pptx.Text{p("10 projects"), p("SSO")}},
			}},
		}},
	}}
}

func TestTheFileIsAValidPackageWhereEveryPartIsTypedAndEveryRelationshipResolves(t *testing.T) {
	t.Parallel()
	p := write(t, everything())
	for _, name := range []string{
		"[Content_Types].xml", "_rels/.rels", "ppt/presentation.xml", "ppt/_rels/presentation.xml.rels",
		"ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml", "ppt/theme/theme1.xml", "docProps/core.xml", "docProps/app.xml",
	} {
		if p[name] == nil {
			t.Errorf("part %s is missing", name)
		}
	}
	for _, name := range p.xmlParts() {
		p.elements(t, name)
	}
	defaults, overridden := p.contentTypes(t)
	for name := range p {
		if name != "[Content_Types].xml" && overridden[name] == "" && !defaults[name[strings.LastIndex(name, ".")+1:]] {
			t.Errorf("part %s has no content type", name)
		}
	}
	for name := range overridden {
		if p[name] == nil {
			t.Errorf("[Content_Types].xml names %s, which is not in the file", name)
		}
	}
	if overridden["ppt/presentation.xml"] != "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
		t.Errorf("presentation.xml is typed %q", overridden["ppt/presentation.xml"])
	}
	for _, d := range p.dangling(t) {
		t.Errorf("a relationship points at %s, which is not in the file", d)
	}
}

func TestEverySlideIsInThePresentationInOrderAndNoSlidePartIsLeftOut(t *testing.T) {
	t.Parallel()
	d := everything()
	p := write(t, d)
	listed := p.slides(t)
	if len(listed) != len(d.Slides) {
		t.Fatalf("presentation.xml lists %d slides, the deck has %d", len(listed), len(d.Slides))
	}
	parts := 0
	for name := range p {
		if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
			parts++
		}
	}
	if parts != len(listed) {
		t.Errorf("the file has %d slide parts and lists %d", parts, len(listed))
	}
	for i, part := range listed {
		shapes := p.shapes(t, part)
		if len(shapes) == 0 || !strings.Contains(shapes[0].text(), strings.TrimSpace(d.Slides[i].Title[0].Text)) {
			t.Errorf("slide %d (%s) does not start with its title %q", i+1, part, d.Slides[i].Title[0].Text)
		}
		if p.relOfType(t, part, "slideLayout") == "" {
			t.Errorf("slide %d has no layout", i+1)
		}
	}
}

func TestEveryElementTypeIsNativeTextShapesOrATable(t *testing.T) {
	t.Parallel()
	p := write(t, everything())
	slides := p.slides(t)
	for i, want := range map[int][]string{
		0: {"Cover title", "Subtitle under the cover"},
		1: {"A paragraph with bold and code\nand a second line", "bullet one", "bullet two", "first", "third"},
		2: {"POST /v1/charges\nIdempotency-Key: 7f3a", "http", "Metric\tNow\tTarget", "Churn\t2.1%\t2.5%"},
		3: {"12 min", "median deploy", "99.95%", "Price is what you pay.", "Warren Buffett"},
		4: {"1", "Queue", "Accepted fast", "Validate", "3", "Charge", "Revenue to target", "78%", "4 of 10"},
		5: {"Left column", "in the left column", "in the right column", "Starter", "1 project", "Recommended", "SSO"},
	} {
		p.holdsAsText(t, slides[i], want)
	}
	lists := p.shapes(t, slides[1])
	bullets, numbered := 0, 0
	for _, s := range lists {
		bullets += s.bullets
		numbered += s.numbered
	}
	if bullets != 2 || numbered != 3 {
		t.Errorf("slide 2 has %d bulleted and %d numbered paragraphs, want 2 and 3", bullets, numbered)
	}
	var table *shape
	for _, s := range p.shapes(t, slides[2]) {
		if s.kind == "graphicFrame" {
			table = &s
		}
	}
	if table == nil || len(table.table) != 3 || len(table.table[0]) != 3 {
		t.Fatalf("slide 3 has no 3 x 3 table: %+v", table)
	}
	if n := len(p.shapes(t, slides[4])); n < 3+3+2*2 {
		t.Errorf("slide 5 has %d shapes; each step is a number and a text box and each bar a track and a fill", n)
	}
}

func TestBoldAndCodeAreRunPropertiesAndTheMarkersAreGone(t *testing.T) {
	t.Parallel()
	p := write(t, everything())
	shapes := p.shapes(t, p.slides(t)[1])
	var bold, code []string
	for _, s := range shapes {
		bold, code = append(bold, s.bold...), append(code, s.code...)
	}
	if !slices.Contains(bold, "bold") || !slices.Contains(code, "code") {
		t.Errorf("bold runs %q, code runs %q", bold, code)
	}
	if strings.Contains(allText(shapes), "**") || strings.Contains(allText(shapes), "`") {
		t.Error("a marker is in the text")
	}
}

func TestSpeakerNotesAreTheNotesSlideOfTheirSlideAndASlideWithoutNotesHasNone(t *testing.T) {
	t.Parallel()
	p := write(t, everything())
	slides := p.slides(t)
	notes := p.relOfType(t, slides[0], "notesSlide")
	if notes == "" {
		t.Fatal("the first slide has no notes slide")
	}
	if back := p.relOfType(t, notes, "slide"); back != slides[0] {
		t.Errorf("the notes slide points back at %q, not %q", back, slides[0])
	}
	if p.relOfType(t, notes, "notesMaster") == "" {
		t.Error("the notes slide has no notes master")
	}
	if got := allText(p.shapes(t, notes)); !strings.Contains(got, "Welcome.\nSecond line of notes.") {
		t.Errorf("the notes slide holds %q", got)
	}
	if p.relOfType(t, slides[2], "notesSlide") != "" {
		t.Error("a slide without notes has a notes slide")
	}
}

func TestEveryCharacterOfACJKAndEmojiDeckIsInTheFileAndCharactersXMLCannotHoldAreReplaced(t *testing.T) {
	t.Parallel()
	d := pptx.Deck{Title: "週報", Slides: []pptx.Slide{{Title: pptx.Plain("週報 😀 第三季"), Notes: "講者備註 🎤\x01", Elements: []pptx.Element{
		pptx.Paragraph{Text: pptx.Plain("中文段落 with ASCII 🚀\x00\x0b end")},
	}}}}
	p := write(t, d)
	for _, name := range p.xmlParts() {
		p.elements(t, name)
	}
	slide := p.slides(t)[0]
	got := allText(p.shapes(t, slide))
	for _, want := range []string{"週報 😀 第三季", "中文段落 with ASCII 🚀", "end"} {
		if !strings.Contains(got, want) {
			t.Errorf("the slide does not hold %q: %s", want, got)
		}
	}
	if notes := allText(p.shapes(t, p.relOfType(t, slide, "notesSlide"))); !strings.Contains(notes, "講者備註 🎤") {
		t.Errorf("the notes hold %q", notes)
	}
	if !bytes.Contains(p["docProps/core.xml"], []byte("週報")) {
		t.Error("the title property is not the deck's title")
	}
}

func TestEveryShapeIsOnTheSlide(t *testing.T) {
	t.Parallel()
	p := write(t, everything())
	const cx, cy = 12192000, 6858000
	for i, part := range p.slides(t) {
		for j, s := range p.shapes(t, part) {
			if s.x < 0 || s.y < 0 || s.w <= 0 || s.h <= 0 || s.x+s.w > cx || s.y+s.h > cy {
				t.Errorf("slide %d shape %d (%q) is at %d,%d size %d x %d, outside the %d x %d slide", i+1, j+1, s.text(), s.x, s.y, s.w, s.h, cx, cy)
			}
		}
	}
}

func TestTheSameDeckIsTheSameBytesAndAnEmptyDeckIsStillAFile(t *testing.T) {
	t.Parallel()
	a, err := pptx.Write(everything())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pptx.Write(everything())
	if !bytes.Equal(a, b) {
		t.Error("two exports of the same deck differ")
	}
	p := write(t, pptx.Deck{Slides: []pptx.Slide{{Cover: true, Title: pptx.Plain("Only a title")}}})
	if got := p.slides(t); len(got) != 1 {
		t.Errorf("a one-slide deck lists %v", got)
	}
}

// With LibreOffice installed the file is converted to PDF; without it the check
// is the manual step in docs/slides-pptx.md.
func TestLibreOfficeConvertsTheDeckToAPDF(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("LibreOffice is not installed; open the file by hand (docs/slides-pptx.md)")
	}
	out, err := pptx.Write(everything())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "e.pptx")
	if err := os.WriteFile(file, out, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), soffice, //nolint:gosec // G204: soffice comes from LookPath and the arguments are fixed, with a file this test just wrote
		"--headless", "--convert-to", "pdf", "--outdir", dir, file)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("convert: %v\n%s", err, b)
	}
	pdf, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "e.pdf")))
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("no pdf: %v", err)
	}
}

// holdsAsText checks a slide holds each of want as text, and no picture.
func (p pkg) holdsAsText(t *testing.T, part string, want []string) {
	t.Helper()
	text := allText(p.shapes(t, part))
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("%s does not hold %q as text; it holds:\n%s", part, w, text)
		}
	}
	if slices.Contains(p.elements(t, part), "pic") {
		t.Errorf("%s holds a picture: everything must stay editable", part)
	}
}
