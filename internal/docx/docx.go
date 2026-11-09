// Package docx writes a doc (docfmt blocks) as a Word file: Office Open XML,
// written by hand with archive/zip and encoding/xml, so there is no library and
// no external service. It maps the model, not a document layout: headings,
// paragraphs with marks, lists, tables, images, links, code and quotes
// (docs/doc-docx.md).
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Image is a picture to put in the file.
type Image struct {
	Data []byte
	// Type is the media type: image/png, image/jpeg or image/gif.
	Type string
}

// Images finds the picture an image block's src points at, or says it cannot
// (a picture on another server is never fetched: the file would depend on it).
type Images func(src string) (Image, bool)

// Write returns the .docx of d. images may be nil: every picture is then written
// as a link. title and author are the file's properties.
func Write(d docfmt.Doc, title string, images Images) ([]byte, error) {
	w := &writer{images: images, rels: []rel{}, media: map[string]int{}}
	body := w.body(d)
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", w.contentTypes()},
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", coreProps(title)},
		{"docProps/app.xml", appProps},
		{"word/document.xml", documentXML(body)},
		{"word/styles.xml", stylesXML},
		{"word/numbering.xml", w.numberingXML()},
		{"word/_rels/document.xml.rels", w.relsXML()},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// A fixed time keeps equal docs equal bytes.
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(name string, data []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	for _, p := range parts {
		if err := add(p.name, []byte(p.data)); err != nil {
			return nil, fmt.Errorf("write %s: %w", p.name, err)
		}
	}
	for _, m := range w.pictures {
		if err := add(m.path, m.data); err != nil {
			return nil, fmt.Errorf("write %s: %w", m.path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const appProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>pictor</Application></Properties>`

func coreProps(title string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>` + esc(title) + `</dc:title><dc:creator>pictor</dc:creator></cp:coreProperties>`
}

func documentXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` +
		body + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr></w:body></w:document>`
}
