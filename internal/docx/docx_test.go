package docx_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docx"
)

func everything(t *testing.T) docfmt.Doc {
	t.Helper()
	raw, err := os.ReadFile("../docroom/testdata/cases/everything.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := docfmt.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pngOf(w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// pkg is a .docx opened: its parts by name.
type pkg map[string][]byte

func open(t *testing.T, data []byte) pkg {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	p := pkg{}
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
		p[f.Name] = b
	}
	return p
}

// wellFormed parses every XML part to its end.
func (p pkg) wellFormed(t *testing.T) {
	t.Helper()
	for name, b := range p {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			if _, err := dec.Token(); err != nil {
				if !errors.Is(err, io.EOF) {
					t.Errorf("%s is not well-formed XML: %v", name, err)
				}
				break
			}
		}
	}
}

type relationship struct{ ID, Type, Target, TargetMode string }

func (p pkg) rels(t *testing.T, name string) map[string]relationship {
	t.Helper()
	var doc struct {
		R []struct {
			ID         string `xml:"Id,attr"`
			Type       string `xml:"Type,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(p[name], &doc); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := map[string]relationship{}
	for _, r := range doc.R {
		out[r.ID] = relationship{r.ID, r.Type, r.Target, r.TargetMode}
	}
	return out
}

// read is what the body of document.xml says, read back with the standard decoder.
type read struct {
	headings   map[string][]string // style -> texts
	paragraphs int
	text       strings.Builder
	tables     [][]int // rows of cells per table
	gridCols   []int
	links      []string // r:id of each hyperlink, in order
	blips      []string // r:embed of each picture
	numbered   map[string]bool
	breaks     int

	// where the reading is
	style       string
	para        strings.Builder
	rows        []int
	cells, grid int
}

func (p pkg) read(t *testing.T) *read {
	t.Helper()
	r := &read{headings: map[string][]string{}, numbered: map[string]bool{}}
	dec := xml.NewDecoder(bytes.NewReader(p["word/document.xml"]))
	for {
		tok, err := dec.Token()
		if err != nil {
			return r
		}
		switch e := tok.(type) {
		case xml.StartElement:
			r.start(dec, e)
		case xml.EndElement:
			r.end(e)
		}
	}
}

func (r *read) start(dec *xml.Decoder, e xml.StartElement) {
	switch e.Name.Local {
	case "tbl":
		r.rows, r.grid = nil, 0
	case "gridCol":
		r.grid++
	case "tr":
		r.cells = 0
	case "tc":
		r.cells++
	case "p":
		r.style = ""
		r.para.Reset()
		r.paragraphs++
	case "pStyle":
		r.style = attr(e, "val")
	case "numId":
		r.numbered[attr(e, "val")] = true
	case "t":
		var s string
		_ = dec.DecodeElement(&s, &e)
		r.para.WriteString(s)
		r.text.WriteString(s)
	case "br":
		r.breaks++
	case "hyperlink":
		r.links = append(r.links, attr(e, "id"))
	case "blip":
		r.blips = append(r.blips, attr(e, "embed"))
	}
}

func (r *read) end(e xml.EndElement) {
	switch e.Name.Local {
	case "p":
		if strings.HasPrefix(r.style, "Heading") || r.style == "Title" {
			r.headings[r.style] = append(r.headings[r.style], r.para.String())
		}
		r.text.WriteString("\n")
	case "tr":
		r.rows = append(r.rows, r.cells)
	case "tbl":
		r.tables = append(r.tables, r.rows)
		r.gridCols = append(r.gridCols, r.grid)
	}
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func TestTheOutputIsAValidZipWithTheRequiredPartsAndEveryRelationshipPointsAtAPart(t *testing.T) {
	t.Parallel()
	out, err := docx.Write(everything(t), "Everything", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("PK\x03\x04")) {
		t.Fatal("no zip magic")
	}
	p := open(t, out)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/numbering.xml", "word/_rels/document.xml.rels", "docProps/core.xml"} {
		if p[name] == nil {
			t.Errorf("part %s is missing", name)
		}
	}
	p.wellFormed(t)
	for _, rels := range []string{"_rels/.rels", "word/_rels/document.xml.rels"} {
		base := path.Dir(path.Dir(rels))
		for _, r := range p.rels(t, rels) {
			if r.TargetMode == "External" {
				continue
			}
			if p[path.Join(base, r.Target)] == nil {
				t.Errorf("%s: %s points at %s, which is not in the file", rels, r.ID, r.Target)
			}
		}
	}
	// The same doc is the same bytes.
	again, _ := docx.Write(everything(t), "Everything", nil)
	if !bytes.Equal(out, again) {
		t.Error("two exports of the same doc differ")
	}
}

// packageOf writes d and opens the file, and reads its body.
func packageOf(t *testing.T, d docfmt.Doc) (pkg, *read) {
	t.Helper()
	out, err := docx.Write(d, "Everything", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := open(t, out)
	return p, p.read(t)
}

func readOf(t *testing.T, d docfmt.Doc) *read {
	t.Helper()
	_, r := packageOf(t, d)
	return r
}

func TestHeadingsReadBackFromTheXMLWithTheirLevels(t *testing.T) {
	t.Parallel()
	r := readOf(t, everything(t))

	if got := r.headings["Heading1"]; len(got) != 1 || got[0] != "Everything 世界 😀" {
		t.Errorf("Heading1 = %q", got)
	}
	for level, text := range map[string]string{"Heading2": "Two", "Heading3": "Three", "Heading4": "Four", "Heading5": "Five", "Heading6": "Six"} {
		if got := r.headings[level]; len(got) != 1 || got[0] != text {
			t.Errorf("%s = %q, want %q", level, got, text)
		}
	}
	if got := r.headings["Title"]; len(got) != 2 || got[1] != "Second tab" {
		t.Errorf("tab titles = %q: two tabs are two titles", got)
	}
}

func TestTablesKeepTheirRowsAndColumns(t *testing.T) {
	t.Parallel()
	r := readOf(t, everything(t))

	// Two tables: 3 rows by 3 columns, and 1 row by 2.
	if len(r.tables) != 2 || len(r.tables[0]) != 3 || r.tables[0][0] != 3 || r.tables[0][2] != 3 || len(r.tables[1]) != 1 || r.tables[1][0] != 2 {
		t.Errorf("tables = %v", r.tables)
	}
	if r.gridCols[0] != 3 || r.gridCols[1] != 2 {
		t.Errorf("table grids = %v", r.gridCols)
	}
}

func TestLinksAreExternalWithTheRightAddresses(t *testing.T) {
	t.Parallel()
	p, r := packageOf(t, everything(t))
	rels := p.rels(t, "word/_rels/document.xml.rels")
	var targets []string
	for _, id := range r.links {
		rel := rels[id]
		if rel.TargetMode != "External" {
			t.Errorf("link %s is not external", id)
		}
		targets = append(targets, rel.Target)
	}
	for _, want := range []string{"https://example.com/?a=1&b=2", "mailto:logan@example.com", "https://example.com"} {
		if !contains(targets, want) {
			t.Errorf("links = %v, want %s", targets, want)
		}
	}
	if contains(targets, "#everything") {
		t.Error("an in-document anchor became an external link")
	}
}

func TestListsHaveTheirOwnNumberingAndTheTextOfEveryBlockIsThere(t *testing.T) {
	t.Parallel()
	p, r := packageOf(t, everything(t))
	if len(r.numbered) != 4 {
		t.Errorf("numbering instances in use = %v, want bullets, tasks and two numbered lists", r.numbered)
	}
	if !strings.Contains(string(p["word/numbering.xml"]), `w:startOverride w:val="3"`) {
		t.Error("a numbered list that starts at 3 does not")
	}
	for _, want := range []string{"☑ done", "☐ todo", "deeper", "func main() {", "Tea 🍵", "To be written: Goals"} {
		if !strings.Contains(r.text.String(), want) {
			t.Errorf("the text has no %q", want)
		}
	}
	if r.breaks < 3 {
		t.Errorf("%d line breaks, want those of the paragraph and the cell", r.breaks)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestEveryCharacterOfACJKAndEmojiDocIsInTheFile(t *testing.T) {
	t.Parallel()
	text := "你好，世界 😀 𠮷野家 🎉 and <&> \"quotes\""
	d := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t", Title: "T", Blocks: []docfmt.Block{
		{ID: "a", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: text}}},
		{ID: "b", Type: docfmt.Heading, Level: 2, Content: []docfmt.Inline{{Text: text, Marks: []string{docfmt.MarkBold}}}},
		{ID: "c", Type: docfmt.Code, Text: text},
	}}}}
	out, err := docx.Write(d, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := open(t, out)
	p.wellFormed(t)
	got := p.read(t).text.String()
	if strings.Count(got, text) != 3 {
		t.Errorf("the text was found %d times, want 3:\n%s", strings.Count(got, text), got)
	}
	if !strings.Contains(string(p["docProps/core.xml"]), "&lt;&amp;&gt;") {
		t.Error("the title is not escaped")
	}
}

func TestPicturesAreEmbeddedFromDataAddressesAndFilesAndOthersAreLinks(t *testing.T) {
	t.Parallel()
	small := pngOf(8, 4, color.RGBA{R: 200, A: 255})
	big := pngOf(2000, 1000, color.RGBA{B: 200, A: 255})
	d := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t", Title: "T", Blocks: []docfmt.Block{
		{ID: "a", Type: docfmt.Image, Src: "data:image/png;base64," + base64.StdEncoding.EncodeToString(small), Alt: "red"},
		{ID: "b", Type: docfmt.Image, Src: "/files/abc/big.png", Alt: "wide blue"},
		{ID: "c", Type: docfmt.Image, Src: "/files/abc/big.png"},
		{ID: "d", Type: docfmt.Image, Src: "https://example.com/elsewhere.png", Alt: "far away"},
		{ID: "e", Type: docfmt.Image, Src: "/files/abc/missing.png", Alt: "gone"},
	}}}}
	out, err := docx.Write(d, "T", func(src string) (docx.Image, bool) {
		if src == "/files/abc/big.png" {
			return docx.Image{Data: big, Type: "image/png"}, true
		}
		return docx.Image{}, false
	})
	if err != nil {
		t.Fatal(err)
	}
	p := open(t, out)
	p.wellFormed(t)
	r := p.read(t)
	rels := p.rels(t, "word/_rels/document.xml.rels")
	if len(r.blips) != 3 {
		t.Fatalf("%d pictures embedded, want 3 (the data address and the file, twice)", len(r.blips))
	}
	var media [][]byte
	for _, id := range r.blips {
		media = append(media, p["word/"+rels[id].Target])
	}
	if !bytes.Equal(media[0], small) || !bytes.Equal(media[1], big) || !bytes.Equal(media[2], big) {
		t.Error("an embedded picture is not the bytes that were given")
	}
	if n := mediaParts(p); n != 2 {
		t.Errorf("%d media parts, want 2: the same file twice is stored once", n)
	}
	if !strings.Contains(r.text.String(), "[image: far away]") || !strings.Contains(r.text.String(), "[image: gone]") {
		t.Errorf("pictures that are not in the file are not described: %q", r.text.String())
	}
	if !contains(linkTargets(r, rels), "https://example.com/elsewhere.png") {
		t.Error("a picture on another server is not a link to it")
	}
	// A wide picture is scaled to the page and keeps its shape.
	doc := string(p["word/document.xml"])
	if !strings.Contains(doc, `cx="5731510" cy="2865755"`) {
		t.Errorf("the wide picture is not scaled to the text width at half the height")
	}
	if !strings.Contains(string(p["[Content_Types].xml"]), `Extension="png"`) {
		t.Error("png has no content type")
	}
}

func linkTargets(r *read, rels map[string]relationship) []string {
	var out []string
	for _, id := range r.links {
		out = append(out, rels[id].Target)
	}
	return out
}

func TestAnEmptyDocAndABlocklessTabStillMakeAFileWordCanOpen(t *testing.T) {
	t.Parallel()
	out, err := docx.Write(docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t", Title: "T", Blocks: []docfmt.Block{}}}}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := open(t, out)
	p.wellFormed(t)
	if !strings.Contains(string(p["word/document.xml"]), "<w:p/>") {
		t.Error("a body needs at least one paragraph")
	}
}

// With LibreOffice installed the file is converted to PDF and the pages are read;
// without it the check is the manual step in docs/doc-docx.md.
func TestLibreOfficeConvertsTheFileToAPDFWithTheText(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("LibreOffice is not installed; open the file by hand (docs/doc-docx.md)")
	}
	out, err := docx.Write(everything(t), "Everything", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "e.docx")
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

func mediaParts(p pkg) int {
	n := 0
	for name := range p {
		if strings.HasPrefix(name, "word/media/") {
			n++
		}
	}
	return n
}

func TestAMentionIsItsNameWithAnAtAsPlainTextInEveryPlaceItCanBe(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../docroom/testdata/cases/mentions.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := docfmt.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	p, r := packageOf(t, d)
	p.wellFormed(t)
	for _, want := range []string{"@Anna Lee asks @Bob 王小明 😀", "@Carol <script>", "@Dave", "@Erin", "For @Anna Lee", "☐ @Bob to review", "cc @Carol", "said @Dave"} {
		if !strings.Contains(r.text.String(), want) {
			t.Errorf("the text has no %q:\n%s", want, r.text.String())
		}
	}
}
