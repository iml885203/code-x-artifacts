package kind

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// pptxParts exports content as PowerPoint and returns the file's parts by name.
func pptxParts(t *testing.T, content string) map[string][]byte {
	t.Helper()
	d, err := (Slides{}).Export(content, slidesFormatV1, "pptx", "https://pictor.example")
	if err != nil {
		t.Fatal(err)
	}
	if d.Extension != "pptx" || d.ContentType != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Errorf("the download is %s typed %s", d.Extension, d.ContentType)
	}
	zr, err := zip.NewReader(bytes.NewReader(d.Body), int64(len(d.Body)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name] = b
	}
	return parts
}

// runs reads an XML part and returns the text of each a:t and the local name
// of every element, failing the test if the part is not well-formed.
func runs(t *testing.T, name string, b []byte) (texts, elements []string) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Errorf("%s is not well-formed XML: %v", name, err)
			}
			return texts, elements
		}
		if e, ok := tok.(xml.StartElement); ok {
			elements = append(elements, e.Name.Local)
			if e.Name.Local == "t" {
				var s string
				_ = dec.DecodeElement(&s, &e)
				texts = append(texts, s)
			}
		}
	}
}

var slidePart = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// slideTexts is the text of each slide, joined, by slide number.
func slideTexts(t *testing.T, parts map[string][]byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, b := range parts {
		if m := slidePart.FindStringSubmatch(name); m != nil {
			texts, _ := runs(t, name, b)
			out[m[1]] = strings.Join(texts, "|")
		}
	}
	return out
}

func TestSlidesExportOnlyAsPowerPointAndRefuseAnythingElse(t *testing.T) {
	if got := (Slides{}).Formats(); !slices.Equal(got, []string{"pptx"}) {
		t.Errorf("Formats() = %v, want [pptx]", got)
	}
	content := deck(t, sl("a"))
	if _, err := (Slides{}).Export(content, slidesFormatV1, "pdf", ""); err == nil || !strings.Contains(err.Error(), "pptx") {
		t.Errorf("exporting as pdf: %v", err)
	}
	if _, err := (Slides{}).Export(content, "9", "pptx", ""); err == nil {
		t.Error("exported a format version this server lacks")
	}
}

func TestADeckExportsAsAPowerPointFileWithEveryElementAsTextAndItsNotes(t *testing.T) {
	slides := []map[string]any{{"layout": "title", "title": "Cover with `code`", "notes": "cover notes"}}
	for _, typ := range elementTypes {
		slides = append(slides, sl("Slide of "+typ, everyElement()[typ]))
	}
	parts := pptxParts(t, deck(t, slides...))
	texts := slideTexts(t, parts)
	if len(texts) != len(slides) {
		t.Fatalf("the file has %d slides, the deck %d", len(texts), len(slides))
	}
	if !strings.Contains(texts["1"], "Cover with |code") {
		t.Errorf("the cover's title is %q", texts["1"])
	}
	for i, typ := range elementTypes {
		got := texts[strconv.Itoa(i+2)]
		if !strings.Contains(got, "Slide of "+typ) || !strings.Contains(got, "MARK-") {
			t.Errorf("the %s slide holds %q", typ, got)
		}
		if strings.Contains(got, "**") || strings.Contains(got, "`") {
			t.Errorf("the %s slide shows an inline marker: %q", typ, got)
		}
	}
	if !strings.Contains(texts["2"], "MARK-text with |inline| and |bold") {
		t.Errorf("the text slide's runs are %q", texts["2"])
	}
	notes := 0
	for name := range parts {
		if strings.HasPrefix(name, "ppt/notesSlides/notesSlide") {
			notes++
		}
	}
	if notes != len(slides) {
		t.Errorf("%d notes slides for %d slides with notes", notes, len(slides))
	}
}

func TestADeckWithABrokenSlideExportsTheRestAndSaysWhichSlideCannotBeShown(t *testing.T) {
	content := deck(t, sl("First", el("text", map[string]any{"text": "fine"})), sl("Second", el("nonsense", nil)), sl("Third", el("text", map[string]any{"text": "also fine"})))
	texts := slideTexts(t, pptxParts(t, content))
	if len(texts) != 3 || !strings.Contains(texts["1"], "fine") || !strings.Contains(texts["3"], "also fine") {
		t.Errorf("the slides hold %q", texts)
	}
	if !strings.Contains(texts["2"], "Slide 2 cannot be shown") || !strings.Contains(texts["2"], `unknown type "nonsense"`) {
		t.Errorf("the broken slide holds %q", texts["2"])
	}
}

func TestEveryStringOfADeckIsTextInThePowerPointFileAndNothingBecomesMarkup(t *testing.T) {
	parts := pptxParts(t, hostileDeck(t))
	withMarkup, short := 0, 0
	for name, b := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		for _, bad := range []string{"<img", "<script", "</script"} {
			if bytes.Contains(b, []byte(bad)) {
				t.Errorf("%s holds %q unescaped", name, bad)
			}
		}
		texts, elements := runs(t, name, b)
		for _, e := range elements {
			if e == "script" || e == "img" {
				t.Errorf("%s has a %s element", name, e)
			}
		}
		for _, s := range texts {
			if strings.Contains(s, markup) {
				withMarkup++
			}
			if strings.Contains(s, hostileShort) {
				short++
			}
		}
	}
	if withMarkup < 26 {
		t.Errorf("only %d of the 26 strings reached the file as text", withMarkup)
	}
	if short != 2 {
		t.Errorf("%d of the 2 short strings reached the file as text", short)
	}
}
