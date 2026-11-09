package kind

import (
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/pptx"
)

// Formats implements Exporter: a deck downloads as an editable PowerPoint file.
func (Slides) Formats() []string { return []string{"pptx"} }

// Export implements Exporter: the deck as a PowerPoint file (internal/pptx),
// every slide native text, shapes and tables, with its notes as the speaker
// notes. A slide that does not parse is a slide saying so, as on the page.
func (Slides) Export(content, format, exportFormat, _ string) (Download, error) {
	if exportFormat != "pptx" {
		return Download{}, fmt.Errorf("slides artifacts export as pptx, not %q", exportFormat)
	}
	if format != slidesFormatV1 {
		return Download{}, fmt.Errorf("slides has no content format %q", format)
	}
	raw, err := splitDeck(content)
	if err != nil {
		return Download{}, err
	}
	d := pptx.Deck{Title: "Slides"}
	for i, r := range raw {
		s, problems := parseSlide(i+1, r)
		if len(problems) > 0 {
			d.Slides = append(d.Slides, pptx.Slide{
				Title:    pptx.Plain(fmt.Sprintf("Slide %d cannot be shown", i+1)),
				Elements: []pptx.Element{pptx.Code{Code: strings.Join(problems, "\n")}},
			})
			continue
		}
		if i == 0 {
			d.Title = plain(s.title)
		}
		els, err := pptxElements(s.elements)
		if err != nil {
			return Download{}, fmt.Errorf("slide %d: %w", i+1, err)
		}
		d.Slides = append(d.Slides, pptx.Slide{Cover: s.layout == layoutTitle, Title: runsOf(s.title), Notes: s.notes, Elements: els})
	}
	body, err := pptx.Write(d)
	if err != nil {
		return Download{}, fmt.Errorf("write pptx: %w", err)
	}
	return Download{ContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", Extension: "pptx", Body: body}, nil
}

// runsOf is inline text as runs: the markers become bold and code.
func runsOf(s string) pptx.Text {
	var out pptx.Text
	for _, sp := range spans(s) {
		switch {
		case sp.bold:
			for _, r := range runsOf(sp.text) {
				r.Bold = true
				out = append(out, r)
			}
		default:
			out = append(out, pptx.Run{Text: sp.text, Code: sp.code})
		}
	}
	return out
}

func runsOfAll(list []string) []pptx.Text {
	out := make([]pptx.Text, len(list))
	for i, s := range list {
		out[i] = runsOf(s)
	}
	return out
}

func pptxElements(els []element) ([]pptx.Element, error) {
	out := make([]pptx.Element, 0, len(els))
	for _, e := range els {
		x, err := pptxElement(e)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// pptxElement maps one element type to its PowerPoint counterpart.
func pptxElement(e element) (pptx.Element, error) {
	switch e := e.(type) {
	case textEl:
		return pptx.Paragraph{Text: runsOf(e.text)}, nil
	case listEl:
		return pptx.List{Ordered: e.ordered, Items: runsOfAll(e.items)}, nil
	case codeEl:
		return pptx.Code{Code: e.code, Language: e.lang}, nil
	case tableEl:
		return pptxTable(e), nil
	case statsEl:
		return pptxStats(e), nil
	case quoteEl:
		return pptx.Quote{Text: runsOf(e.text), By: runsOf(e.by)}, nil
	case stepsEl:
		return pptxSteps(e), nil
	case columnsEl:
		return pptxColumns(e)
	case cardsEl:
		return pptxCards(e), nil
	case progressEl:
		return pptxProgress(e), nil
	default:
		return nil, fmt.Errorf("no PowerPoint counterpart for element %T", e)
	}
}

func pptxTable(e tableEl) pptx.Table {
	t := pptx.Table{Columns: runsOfAll(e.columns)}
	for _, row := range e.rows {
		cells := make([]pptx.Cell, len(row))
		for i, c := range row {
			cells[i] = pptx.Cell{Text: runsOf(c.text), Tone: pptx.Tone(c.tone)}
		}
		t.Rows = append(t.Rows, cells)
	}
	return t
}

func pptxStats(e statsEl) pptx.Stats {
	s := pptx.Stats{}
	for _, it := range e.items {
		s.Items = append(s.Items, pptx.Stat{Value: runsOf(it.value), Label: runsOf(it.label)})
	}
	return s
}

func pptxSteps(e stepsEl) pptx.Steps {
	s := pptx.Steps{}
	for _, it := range e.items {
		s.Items = append(s.Items, pptx.Step{Title: runsOf(it.title), Text: runsOf(it.text)})
	}
	return s
}

func pptxColumns(e columnsEl) (pptx.Element, error) {
	c := pptx.Columns{}
	for _, col := range e.cols {
		els, err := pptxElements(col.els)
		if err != nil {
			return nil, err
		}
		c.Columns = append(c.Columns, pptx.Column{Title: runsOf(col.title), Elements: els})
	}
	return c, nil
}

func pptxCards(e cardsEl) pptx.Cards {
	c := pptx.Cards{}
	for _, it := range e.items {
		c.Items = append(c.Items, pptx.Card{Title: runsOf(it.title), Text: runsOf(it.text), Items: runsOfAll(it.items), Highlight: it.highlight})
	}
	return c
}

func pptxProgress(e progressEl) pptx.Progress {
	p := pptx.Progress{}
	for _, it := range e.items {
		p.Items = append(p.Items, pptx.Bar{Label: runsOf(it.label), Text: runsOf(it.text), Value: it.value})
	}
	return p
}
