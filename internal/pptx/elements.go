package pptx

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// The sizes follow the browser's stylesheet (internal/kind/slides.css), in
// canvas pixels scaled by the slide's k.

func (e Paragraph) height(w, k float64) float64 { return textHeight(e.Text, w, 40*k) }

func (e Paragraph) draw(c *canvas, x, y, w, h, k float64) {
	c.shape("Text", x, y, w, h, look{}, para(e.Text, style{px: 40 * k, align: c.align()}))
}

func (e List) height(w, k float64) float64 {
	t := 0.0
	for i, it := range e.Items {
		if i > 0 {
			t += 14 * k
		}
		t += textHeight(it, w-56*k, 40*k)
	}
	return t
}

func (e List) draw(c *canvas, x, y, w, h, k float64) {
	bullet := "char"
	if e.Ordered {
		bullet = "num"
	}
	var b strings.Builder
	for i, it := range e.Items {
		s := style{px: 40 * k, bullet: bullet, indent: 56 * k}
		if i > 0 {
			s.spaceUp = 14 * k
		}
		b.WriteString(para(it, s))
	}
	c.shape("List", x, y, w, h, look{}, b.String())
}

func (e Code) height(w, k float64) float64 {
	px := 28 * k
	per := math.Max(1, math.Floor((w-64*k)/(0.55*px)))
	n := 0.0
	for _, line := range splitLines(e.Code) {
		n += math.Max(1, math.Ceil(float64(utf8.RuneCountInString(line))/per))
	}
	return n*px*1.25 + 56*k
}

func (e Code) draw(c *canvas, x, y, w, h, k float64) {
	var b strings.Builder
	for _, line := range splitLines(e.Code) {
		b.WriteString(para(Plain(line), style{px: 28 * k, mono: true}))
	}
	c.shape("Code", x, y, w, h, look{geom: "rect", fill: colorPlate, line: colorLine, lineW: 2, inset: 28 * k}, b.String())
	if e.Language != "" {
		c.shape("Language", x+w-316*k, y+6*k, 300*k, 30*k, look{}, para(Plain(e.Language), style{px: 20 * k, mono: true, color: colorMuted, align: "r"}))
	}
}

func (e Table) rowHeights(w, k float64) []float64 {
	inner := w/float64(max(1, len(e.Columns))) - 48*k
	head := 1.0
	for _, c := range e.Columns {
		head = math.Max(head, lines(c.String(), inner, 30*k))
	}
	out := []float64{head*30*k*1.3 + 30*k}
	for _, row := range e.Rows {
		n := 1.0
		for _, c := range row {
			n = math.Max(n, lines(c.Text.String(), inner, 32*k))
		}
		out = append(out, n*32*k*1.3+28*k)
	}
	return out
}

func (e Table) height(w, k float64) float64 {
	t := 0.0
	for _, h := range e.rowHeights(w, k) {
		t += h
	}
	return t
}

// draw writes a native table: a header row and one row per row of the deck,
// with the tone of a cell as its colour.
func (e Table) draw(c *canvas, x, y, w, h, k float64) {
	n := c.id()
	cols := max(1, len(e.Columns))
	colW := w / float64(cols)
	heights := e.rowHeights(w, k)
	f := h / e.height(w, k)
	fmt.Fprintf(&c.b, `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="%d" name="Table %d"/><p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>`, n, n)
	c.b.WriteString(`<p:xfrm><a:off x="` + emu(x) + `" y="` + emu(y) + `"/><a:ext cx="` + emu(w) + `" cy="` + emu(h) + `"/></p:xfrm>`)
	c.b.WriteString(`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1" bandRow="1"/><a:tblGrid>`)
	for range cols {
		c.b.WriteString(`<a:gridCol w="` + emu(colW) + `"/>`)
	}
	c.b.WriteString(`</a:tblGrid>`)
	header := make([]Cell, len(e.Columns))
	for i, t := range e.Columns {
		header[i] = Cell{Text: t}
	}
	for r, row := range append([][]Cell{header}, e.Rows...) {
		c.b.WriteString(`<a:tr h="` + emu(heights[r]*f) + `">`)
		for _, cell := range row {
			c.b.WriteString(tableCell(cell, r == 0, k))
		}
		c.b.WriteString(`</a:tr>`)
	}
	c.b.WriteString(`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`)
}

func tableCell(cell Cell, header bool, k float64) string {
	s := style{px: 32 * k}
	fill, border := "", 2.0
	if header {
		s = style{px: 30 * k, bold: true, color: colorMuted}
		border = 3
	}
	if tone, ok := toneColors[cell.Tone]; ok {
		s.color, s.bold, fill = tone[0], true, tone[1]
	}
	props := fmt.Sprintf(`<a:tcPr marL="%[1]s" marR="%[1]s" marT="%[2]s" marB="%[2]s">`, emu(24*k), emu(14*k))
	for _, side := range []string{"lnL", "lnR", "lnT"} {
		props += `<a:` + side + ` w="0"><a:noFill/></a:` + side + `>`
	}
	props += `<a:lnB w="` + emu(border) + `"><a:solidFill><a:srgbClr val="` + colorLine + `"/></a:solidFill></a:lnB>`
	if fill != "" {
		props += `<a:solidFill><a:srgbClr val="` + fill + `"/></a:solidFill>`
	} else {
		props += `<a:noFill/>`
	}
	return `<a:tc><a:txBody><a:bodyPr/><a:lstStyle/>` + para(cell.Text, s) + `</a:txBody>` + props + `</a:tcPr></a:tc>`
}

// across splits w into n boxes gap apart: the distance from one to the next
// and the width of each.
func across(w float64, n int, gap float64) (step, width float64) {
	width = (w - float64(n-1)*gap) / float64(max(1, n))
	return width + gap, width
}

func (e Stats) height(w, k float64) float64 {
	_, itemW := across(w, len(e.Items), 32*k)
	h := 0.0
	for _, s := range e.Items {
		h = math.Max(h, lines(s.Value.String(), itemW-80*k, 112*k)*112*k*1.1+textHeight(s.Label, itemW-80*k, 32*k))
	}
	return h + 92*k
}

func (e Stats) draw(c *canvas, x, y, w, h, k float64) {
	step, itemW := across(w, len(e.Items), 32*k)
	for i, s := range e.Items {
		text := para(s.Value, style{px: 112 * k, bold: true}) + para(s.Label, style{px: 32 * k, color: colorMuted, spaceUp: 12 * k})
		c.shape("Stat", x+float64(i)*step, y, itemW, h, look{geom: "roundRect", fill: colorPlate, line: colorLine, lineW: 2, inset: 40 * k}, text)
	}
}

func (e Steps) height(w, k float64) float64 {
	_, itemW := across(w, len(e.Items), 28*k)
	h := 0.0
	for _, s := range e.Items {
		t := textHeight(s.Title, itemW, 36*k)
		if len(s.Text) > 0 {
			t += textHeight(s.Text, itemW, 30*k) + 8*k
		}
		h = math.Max(h, t)
	}
	return 84*k + h
}

// draw writes each step as its number in a square and its text under it.
func (e Steps) draw(c *canvas, x, y, w, h, k float64) {
	step, itemW := across(w, len(e.Items), 28*k)
	f := h / e.height(w, k)
	size, under := 64*k*f, 20*k*f
	for i, s := range e.Items {
		left := x + float64(i)*step
		c.shape("Step number", left, y, size, size, look{geom: "roundRect", fill: colorStar, anchor: "ctr"},
			para(Plain(itoa(i+1)), style{px: 34 * k, bold: true, mono: true, color: colorOnStar, align: "ctr"}))
		text := para(s.Title, style{px: 36 * k, bold: true})
		if len(s.Text) > 0 {
			text += para(s.Text, style{px: 30 * k, color: colorMuted, spaceUp: 8 * k})
		}
		c.shape("Step", left, y+size+under, itemW, h-size-under, look{}, text)
	}
}

func (e Columns) height(w, k float64) float64 {
	_, colW := across(w, len(e.Columns), 56*k)
	h := 0.0
	for _, col := range e.Columns {
		h = math.Max(h, col.titleHeight(colW, k)+stackHeight(col.Elements, colW, k*0.85, 28*k))
	}
	return h
}

func (col Column) titleHeight(w, k float64) float64 {
	if len(col.Title) == 0 {
		return 0
	}
	return textHeight(col.Title, w, 44*k) + 20*k
}

func (e Columns) draw(c *canvas, x, y, w, h, k float64) {
	step, colW := across(w, len(e.Columns), 56*k)
	f := h / math.Max(1, e.height(w, k))
	for i, col := range e.Columns {
		left, top := x+float64(i)*step, y
		if th := col.titleHeight(colW, k) * f; th > 0 {
			c.shape("Column title", left, top, colW, th, look{}, para(col.Title, style{px: 44 * k, bold: true}))
			top += th
		}
		stackDraw(c, col.Elements, left, top, colW, y+h-top, k*0.85, 28*k)
	}
}

func (e Cards) height(w, k float64) float64 {
	_, cardW := across(w, len(e.Items), 32*k)
	inner := cardW - 72*k
	h := 0.0
	for _, cd := range e.Items {
		t := textHeight(cd.Title, inner, 40*k)
		if len(cd.Text) > 0 {
			t += 12*k + textHeight(cd.Text, inner, 32*k)
		}
		for _, it := range cd.Items {
			t += 10*k + textHeight(it, inner-36*k, 30*k)
		}
		h = math.Max(h, t)
	}
	return h + 72*k
}

func (e Cards) draw(c *canvas, x, y, w, h, k float64) {
	step, cardW := across(w, len(e.Items), 32*k)
	for i, cd := range e.Items {
		l := look{geom: "roundRect", fill: colorPlate, line: colorLine, lineW: 3, inset: 36 * k}
		title := style{px: 40 * k, bold: true}
		if cd.Highlight {
			l.fill, l.line, title.color = colorRaised, colorStar, colorStarText
		}
		text := para(cd.Title, title)
		if len(cd.Text) > 0 {
			text += para(cd.Text, style{px: 32 * k, color: colorMuted, spaceUp: 12 * k})
		}
		for _, it := range cd.Items {
			text += para(it, style{px: 30 * k, bullet: "char", indent: 36 * k, spaceUp: 10 * k})
		}
		c.shape("Card", x+float64(i)*step, y, cardW, h, l, text)
	}
}

func (e Progress) height(_, k float64) float64 {
	return float64(len(e.Items))*86*k + float64(max(0, len(e.Items)-1))*28*k
}

// draw writes each bar as its label and value over a track with a fill as
// wide as the value.
func (e Progress) draw(c *canvas, x, y, w, h, k float64) {
	f := h / math.Max(1, e.height(w, k))
	for i, b := range e.Items {
		top := y + float64(i)*114*k*f
		value := b.Text
		if len(value) == 0 {
			value = Plain(fmt.Sprintf("%g%%", b.Value))
		}
		c.shape("Bar label", x, top, w*0.7, 50*k*f, look{}, para(b.Label, style{px: 36 * k}))
		c.shape("Bar value", x+w*0.7, top, w*0.3, 50*k*f, look{}, para(value, style{px: 36 * k, mono: true, color: colorMuted, align: "r"}))
		track := top + 66*k*f
		c.shape("Bar track", x, track, w, 20*k*f, look{geom: "roundRect", fill: colorRaised}, "")
		if b.Value > 0 {
			c.shape("Bar fill", x, track, w*math.Min(b.Value, 100)/100, 20*k*f, look{geom: "roundRect", fill: colorInfo}, "")
		}
	}
}

func (e Quote) height(w, k float64) float64 {
	h := textHeight(e.Text, w-64*k, 56*k)
	if len(e.By) > 0 {
		h += textHeight(e.By, w-64*k, 34*k) + 20*k
	}
	return h
}

func (e Quote) draw(c *canvas, x, y, w, h, k float64) {
	c.shape("Quote bar", x, y, 8, h, look{fill: colorStar}, "")
	text := para(e.Text, style{px: 56 * k, italic: true, align: c.align()})
	if len(e.By) > 0 {
		text += para(append(Plain("— "), e.By...), style{px: 34 * k, color: colorMuted, spaceUp: 20 * k, align: c.align()})
	}
	c.shape("Quote", x+56*k, y, w-56*k, h, look{}, text)
}
