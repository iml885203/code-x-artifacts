package pptx

import (
	"fmt"
	"math"
)

// The type scale a slide's elements are drawn at: the largest that fits, as
// the browser does, within these bounds.
const (
	minScale = 0.75
	maxScale = 1.2
)

func gap(k float64) float64 { return 36 * k }

// slideXML lays one slide out: the title, then the elements stacked under it
// at the largest type scale that fits. A cover slide centres the title and
// its elements on the slide.
func slideXML(s Slide) string {
	c := &canvas{next: 1, centred: s.Cover}
	px, lineHeight := 76.0, 1.12
	if s.Cover {
		px, lineHeight = 120, 1.07
	}
	titleH := lines(s.Title.String(), innerWidth, px*1.04) * px * lineHeight
	room := innerHeight - titleH
	if len(s.Elements) > 0 {
		room -= 44
	}
	room = math.Max(room, 0)
	k, need := fit(s.Elements, room)
	top := padY
	if s.Cover {
		used := titleH + math.Min(need, room)
		if len(s.Elements) > 0 {
			used += 44
		}
		top += math.Max(0, (innerHeight-used)/2)
	}
	titleShape(c, s.Title, top, titleH, px)
	if len(s.Elements) > 0 {
		stackDraw(c, s.Elements, padX, top+titleH+44, innerWidth, math.Min(need, room), k, gap(k))
	}
	return xmlHeader + `<p:sld ` + namespaces + `><p:cSld><p:spTree>` + groupProps + c.b.String() +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
}

// fit picks the type scale and returns the height the elements take at it.
func fit(els []Element, room float64) (k, need float64) {
	for k = maxScale; k >= minScale-1e-9; k -= 0.05 {
		k = math.Round(k*100) / 100
		if need = stackHeight(els, innerWidth, k, gap(k)); need <= room {
			return k, need
		}
	}
	return minScale, stackHeight(els, innerWidth, minScale, gap(minScale))
}

// titleShape is the slide's title placeholder, so the title is the slide's
// title in PowerPoint's outline and accessibility checks.
func titleShape(c *canvas, t Text, y, h, px float64) {
	n := c.id()
	anchor := "t"
	if c.centred {
		anchor = "ctr"
	}
	fmt.Fprintf(&c.b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Title %d"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>`, n, n)
	c.b.WriteString(`<p:spPr>` + xfrm(padX, y, innerWidth, h) + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`)
	c.b.WriteString(`<p:txBody><a:bodyPr wrap="square" lIns="0" tIns="0" rIns="0" bIns="0" anchor="` + anchor + `"><a:normAutofit/></a:bodyPr><a:lstStyle/>` +
		para(t, style{px: px, bold: true, align: c.align()}) + `</p:txBody></p:sp>`)
}

func stackHeight(els []Element, w, k, between float64) float64 {
	t := 0.0
	for i, e := range els {
		if i > 0 {
			t += between
		}
		t += e.height(w, k)
	}
	return t
}

// stackDraw draws elements one under the other in the box. When they need
// more than h, every element and gap shrinks by the same factor, so nothing
// leaves the slide; the text boxes shrink their text on overflow.
func stackDraw(c *canvas, els []Element, x, y, w, h, k, between float64) {
	need := stackHeight(els, w, k, between)
	f := 1.0
	if need > h && need > 0 {
		f = h / need
	}
	for i, e := range els {
		if i > 0 {
			y += between * f
		}
		eh := e.height(w, k) * f
		e.draw(c, x, y, w, eh, k)
		y += eh
	}
}
