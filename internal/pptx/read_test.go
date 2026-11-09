package pptx_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/pptx"
)

// pkg is a .pptx opened: its parts by name.
type pkg map[string][]byte

func write(t *testing.T, d pptx.Deck) pkg {
	t.Helper()
	out, err := pptx.Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("PK\x03\x04")) {
		t.Fatal("the file is not a zip")
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
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

// elements parses an XML part to its end and returns the local name of every
// element in it; a part that is not well-formed fails the test.
func (p pkg) elements(t *testing.T, name string) []string {
	t.Helper()
	var names []string
	dec := xml.NewDecoder(bytes.NewReader(p[name]))
	for {
		tok, err := dec.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Errorf("%s is not well-formed XML: %v", name, err)
			}
			return names
		}
		if e, ok := tok.(xml.StartElement); ok {
			names = append(names, e.Name.Local)
		}
	}
}

// contentTypes reads [Content_Types].xml: the default extensions and the
// type of each part named.
func (p pkg) contentTypes(t *testing.T) (defaults map[string]bool, overridden map[string]string) {
	t.Helper()
	var types struct {
		Defaults []struct {
			Ext string `xml:"Extension,attr"`
		} `xml:"Default"`
		Overrides []struct {
			Part string `xml:"PartName,attr"`
			Type string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err := xml.Unmarshal(p["[Content_Types].xml"], &types); err != nil {
		t.Fatal(err)
	}
	defaults, overridden = map[string]bool{}, map[string]string{}
	for _, d := range types.Defaults {
		defaults[d.Ext] = true
	}
	for _, o := range types.Overrides {
		overridden[strings.TrimPrefix(o.Part, "/")] = o.Type
	}
	return defaults, overridden
}

func (p pkg) xmlParts() []string {
	var out []string
	for name := range p {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

type relationship struct{ ID, Type, Target, TargetMode string }

// rels reads the relationships of a part, with each target resolved to the
// name of a part in the zip.
func (p pkg) rels(t *testing.T, part string) map[string]relationship {
	t.Helper()
	name := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	if part == "" {
		name = "_rels/.rels"
	}
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
		target := r.Target
		if r.TargetMode != "External" {
			target = path.Join(path.Dir(part), r.Target)
		}
		out[r.ID] = relationship{r.ID, path.Base(r.Type), target, r.TargetMode}
	}
	return out
}

// dangling lists the targets of relationships that are not parts of the file.
func (p pkg) dangling(t *testing.T) []string {
	t.Helper()
	var out []string
	for name := range p {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		part := strings.Replace(strings.TrimSuffix(name, ".rels"), "_rels/", "", 1)
		for _, r := range p.rels(t, part) {
			if r.TargetMode != "External" && p[r.Target] == nil {
				out = append(out, name+" "+r.ID+" -> "+r.Target)
			}
		}
	}
	return out
}

func (p pkg) relOfType(t *testing.T, part, typ string) string {
	t.Helper()
	for _, r := range p.rels(t, part) {
		if r.Type == typ {
			return r.Target
		}
	}
	return ""
}

// slides lists the slide parts in the order presentation.xml shows them.
func (p pkg) slides(t *testing.T) []string {
	t.Helper()
	var doc struct {
		IDs []struct {
			ID  string `xml:"id,attr"`
			Rel string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(p["ppt/presentation.xml"], &doc); err != nil {
		t.Fatal(err)
	}
	rels := p.rels(t, "ppt/presentation.xml")
	var out []string
	for _, id := range doc.IDs {
		out = append(out, rels[id.Rel].Target)
	}
	return out
}

// shape is what a test reads of one shape: its text, paragraph by paragraph,
// and where it is.
type shape struct {
	kind       string // sp or graphicFrame
	paragraphs []string
	bold, code []string // the text of the bold and the monospaced runs
	bullets    int
	numbered   int
	x, y, w, h int64
	table      [][]string // rows of cell texts
}

func (s shape) text() string { return strings.Join(s.paragraphs, "\n") }

// shapes reads the shapes of a slide part (or of a notes slide).
func (p pkg) shapes(t *testing.T, part string) []shape {
	t.Helper()
	r := &shapeReader{dec: xml.NewDecoder(bytes.NewReader(p[part]))}
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return r.out
		}
		switch e := tok.(type) {
		case xml.StartElement:
			r.start(e)
		case xml.EndElement:
			r.end(e)
		}
	}
}

// shapeReader is where the reading of a part is.
type shapeReader struct {
	dec        *xml.Decoder
	out        []shape
	para       strings.Builder
	row        []string
	bold, mono bool
	inCell     bool
}

func (r *shapeReader) cur() *shape { return &r.out[len(r.out)-1] }

func (r *shapeReader) start(e xml.StartElement) {
	switch e.Name.Local {
	case "sp", "graphicFrame":
		r.out = append(r.out, shape{kind: e.Name.Local})
	case "off", "ext":
		r.place(e)
	case "p":
		r.para.Reset()
	case "rPr":
		r.bold, r.mono = attr(e, "b") == "1", false
	case "latin":
		r.mono = r.mono || strings.Contains(attr(e, "typeface"), "Consolas")
	case "buChar":
		r.cur().bullets++
	case "buAutoNum":
		r.cur().numbered++
	case "tr":
		r.row = nil
	case "tc":
		r.inCell = true
	case "br":
		r.para.WriteString("\n")
	case "t":
		r.text(e)
	}
}

// place reads the first offset and size of the current shape: its own.
func (r *shapeReader) place(e xml.StartElement) {
	if len(r.out) == 0 || r.cur().w != 0 {
		return
	}
	if e.Name.Local == "off" {
		r.cur().x, r.cur().y = num(attr(e, "x")), num(attr(e, "y"))
	} else {
		r.cur().w, r.cur().h = num(attr(e, "cx")), num(attr(e, "cy"))
	}
}

func (r *shapeReader) text(e xml.StartElement) {
	var s string
	_ = r.dec.DecodeElement(&s, &e)
	r.para.WriteString(s)
	if r.bold {
		r.cur().bold = append(r.cur().bold, s)
	}
	if r.mono {
		r.cur().code = append(r.cur().code, s)
	}
}

func (r *shapeReader) end(e xml.EndElement) {
	switch e.Name.Local {
	case "p":
		if r.inCell {
			r.row = append(r.row, r.para.String())
		} else if len(r.out) > 0 {
			r.cur().paragraphs = append(r.cur().paragraphs, r.para.String())
		}
	case "tc":
		r.inCell = false
	case "tr":
		r.cur().table = append(r.cur().table, r.row)
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

func num(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// allText is every a:t of a part, in order, one per line.
func allText(shapes []shape) string {
	var b strings.Builder
	for _, s := range shapes {
		b.WriteString(s.text() + "\n")
		for _, r := range s.table {
			b.WriteString(strings.Join(r, "\t") + "\n")
		}
	}
	return b.String()
}
