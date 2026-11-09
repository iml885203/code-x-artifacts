// Package pptx writes a deck as a PowerPoint file: PresentationML written by
// hand with archive/zip and strings, so there is no library and no external
// service. Every slide is native text boxes, shapes and tables, so the file
// stays editable; it maps the content, not the browser's pixels
// (docs/slides-pptx.md).
package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Write returns the .pptx of d.
func Write(d Deck) ([]byte, error) {
	parts := []part{
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", coreProps(d.Title)},
		{"docProps/app.xml", appProps},
		{"ppt/presentation.xml", presentationXML(len(d.Slides))},
		{"ppt/_rels/presentation.xml.rels", presentationRels(len(d.Slides))},
		{"ppt/presProps.xml", presProps},
		{"ppt/viewProps.xml", viewProps},
		{"ppt/tableStyles.xml", tableStyles},
		{"ppt/theme/theme1.xml", themeXML},
		{"ppt/theme/theme2.xml", themeXML},
		{"ppt/slideMasters/slideMaster1.xml", slideMaster},
		{"ppt/slideMasters/_rels/slideMaster1.xml.rels", rels(relTo("slideLayout", "../slideLayouts/slideLayout1.xml"), relTo("theme", "../theme/theme1.xml"))},
		{"ppt/slideLayouts/slideLayout1.xml", slideLayout},
		{"ppt/slideLayouts/_rels/slideLayout1.xml.rels", rels(relTo("slideMaster", "../slideMasters/slideMaster1.xml"))},
		{"ppt/notesMasters/notesMaster1.xml", notesMaster},
		{"ppt/notesMasters/_rels/notesMaster1.xml.rels", rels(relTo("theme", "../theme/theme2.xml"))},
	}
	var notes []int
	for i, s := range d.Slides {
		n := i + 1
		slideRels := []string{relTo("slideLayout", "../slideLayouts/slideLayout1.xml")}
		if strings.TrimSpace(s.Notes) != "" {
			notes = append(notes, n)
			slideRels = append(slideRels, relTo("notesSlide", fmt.Sprintf("../notesSlides/notesSlide%d.xml", n)))
			parts = append(parts,
				part{fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n), notesSlideXML(s.Notes)},
				part{fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", n), rels(relTo("notesMaster", "../notesMasters/notesMaster1.xml"), relTo("slide", fmt.Sprintf("../slides/slide%d.xml", n)))})
		}
		parts = append(parts,
			part{fmt.Sprintf("ppt/slides/slide%d.xml", n), slideXML(s)},
			part{fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n), rels(slideRels...)})
	}
	parts = append([]part{{"[Content_Types].xml", contentTypes(len(d.Slides), notes)}}, parts...)
	return zipOf(parts)
}

type part struct{ name, data string }

func zipOf(parts []part) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// A fixed time keeps equal decks equal bytes.
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, p := range parts {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", p.name, err)
		}
		if _, err := f.Write([]byte(p.data)); err != nil {
			return nil, fmt.Errorf("write %s: %w", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close the zip: %w", err)
	}
	return buf.Bytes(), nil
}

const (
	xmlHeader  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	namespaces = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	officeRels = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	pmlType    = "application/vnd.openxmlformats-officedocument.presentationml."
)

// relTo is one relationship of the given type; rels numbers them in order.
func relTo(typ, target string) string {
	return `Type="` + officeRels + typ + `" Target="` + target + `"`
}

func rels(list ...string) string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, r := range list {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" %s/>`, i+1, r)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

const rootRels = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`</Relationships>`

// The fixed relationships of presentation.xml; the slides follow them.
var presentationFixed = []string{
	relTo("slideMaster", "slideMasters/slideMaster1.xml"),
	relTo("notesMaster", "notesMasters/notesMaster1.xml"),
	relTo("presProps", "presProps.xml"),
	relTo("viewProps", "viewProps.xml"),
	relTo("theme", "theme/theme1.xml"),
	relTo("tableStyles", "tableStyles.xml"),
}

func presentationRels(slides int) string {
	list := append([]string{}, presentationFixed...)
	for n := 1; n <= slides; n++ {
		list = append(list, relTo("slide", fmt.Sprintf("slides/slide%d.xml", n)))
	}
	return rels(list...)
}

func presentationXML(slides int) string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<p:presentation ` + namespaces + ` saveSubsetFonts="1">`)
	b.WriteString(`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`)
	b.WriteString(`<p:notesMasterIdLst><p:notesMasterId r:id="rId2"/></p:notesMasterIdLst><p:sldIdLst>`)
	for n := 1; n <= slides; n++ {
		fmt.Fprintf(&b, `<p:sldId id="%d" r:id="rId%d"/>`, 255+n, len(presentationFixed)+n)
	}
	fmt.Fprintf(&b, `</p:sldIdLst><p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`, slideWidth, slideHeight)
	return b.String()
}

func contentTypes(slides int, notes []int) string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`)
	override := func(name, typ string) {
		b.WriteString(`<Override PartName="/` + name + `" ContentType="` + typ + `"/>`)
	}
	override("ppt/presentation.xml", pmlType+"presentation.main+xml")
	override("ppt/presProps.xml", pmlType+"presProps+xml")
	override("ppt/viewProps.xml", pmlType+"viewProps+xml")
	override("ppt/tableStyles.xml", pmlType+"tableStyles+xml")
	override("ppt/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	override("ppt/theme/theme2.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	override("ppt/slideMasters/slideMaster1.xml", pmlType+"slideMaster+xml")
	override("ppt/slideLayouts/slideLayout1.xml", pmlType+"slideLayout+xml")
	override("ppt/notesMasters/notesMaster1.xml", pmlType+"notesMaster+xml")
	for n := 1; n <= slides; n++ {
		override(fmt.Sprintf("ppt/slides/slide%d.xml", n), pmlType+"slide+xml")
	}
	for _, n := range notes {
		override(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n), pmlType+"notesSlide+xml")
	}
	override("docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
	override("docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml")
	b.WriteString(`</Types>`)
	return b.String()
}

const appProps = xmlHeader + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>pictor</Application></Properties>`

func coreProps(title string) string {
	return xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>` +
		esc(title) + `</dc:title><dc:creator>pictor</dc:creator></cp:coreProperties>`
}
