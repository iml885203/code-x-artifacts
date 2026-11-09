package boardfmt

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
)

// SVG draws a board as a standalone SVG document: the read-only view, the
// gallery card and the export an agent gets. It draws neat lines rather than
// the editor's hand-drawn strokes (a person's own export comes from the editor
// itself, in the browser). Every value it writes passed the board's rules, and
// every string is escaped, so nothing on a board becomes markup.
func SVG(b Board) string {
	els := Canonical(b).Elements
	minX, minY, maxX, maxY := bounds(els)
	const pad = 16
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%s" height="%s">`,
		f(minX-pad), f(minY-pad), f(maxX-minX+2*pad), f(maxY-minY+2*pad), f(maxX-minX+2*pad), f(maxY-minY+2*pad))
	out.WriteString(`<rect x="` + f(minX-pad) + `" y="` + f(minY-pad) + `" width="100%" height="100%" fill="#ffffff"/>`)
	for _, el := range els {
		drawElement(&out, el)
	}
	out.WriteString("</svg>")
	return out.String()
}

// bounds is the box every element lies in; an empty board is a small blank one.
func bounds(els []Element) (minX, minY, maxX, maxY float64) {
	minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	grow := func(x, y float64) {
		minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
	}
	for _, el := range els {
		x, y := num(el["x"]), num(el["y"])
		if pts := pointsOf(el); len(pts) > 0 {
			for _, p := range pts {
				grow(x+p[0], y+p[1])
			}
			continue
		}
		grow(x, y)
		grow(x+num(el["width"]), y+num(el["height"]))
	}
	if math.IsInf(minX, 1) {
		return 0, 0, 200, 100
	}
	return minX, minY, maxX, maxY
}

func drawElement(out *strings.Builder, el Element) {
	x, y, w, h := num(el["x"]), num(el["y"]), num(el["width"]), num(el["height"])
	rotate := ""
	if angle := num(el["angle"]); angle != 0 {
		rotate = fmt.Sprintf(` transform="rotate(%s %s %s)"`, f(angle*180/math.Pi), f(x+w/2), f(y+h/2))
	}
	attrs := style(el, fillOf(el)) + rotate
	switch str(el["type"]) {
	case "rectangle":
		rx := 0.0
		if el["roundness"] != nil {
			rx = math.Min(32, math.Min(w, h)/4)
		}
		fmt.Fprintf(out, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s"%s/>`, f(x), f(y), f(w), f(h), f(rx), attrs)
	case "ellipse":
		fmt.Fprintf(out, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s"%s/>`, f(x+w/2), f(y+h/2), f(w/2), f(h/2), attrs)
	case "diamond":
		fmt.Fprintf(out, `<polygon points="%s,%s %s,%s %s,%s %s,%s"%s/>`, f(x+w/2), f(y), f(x+w), f(y+h/2), f(x+w/2), f(y+h), f(x), f(y+h/2), attrs)
	case "frame":
		fmt.Fprintf(out, `<rect x="%s" y="%s" width="%s" height="%s" fill="none" stroke="#bbbbbb"/>`, f(x), f(y), f(w), f(h))
		if name := str(el["name"]); name != "" {
			fmt.Fprintf(out, `<text x="%s" y="%s" font-family="sans-serif" font-size="14" fill="#666666">%s</text>`, f(x), f(y-6), html.EscapeString(name))
		}
	case "line", "arrow", "freedraw":
		drawLine(out, el, x, y, style(el, "none")+rotate)
	case "text":
		drawText(out, el, x, y, w)
	}
}

func drawLine(out *strings.Builder, el Element, x, y float64, attrs string) {
	pts := pointsOf(el)
	if len(pts) < 2 {
		return
	}
	var path strings.Builder
	for i, p := range pts {
		if i > 0 {
			path.WriteByte(' ')
		}
		path.WriteString(f(x+p[0]) + "," + f(y+p[1]))
	}
	fmt.Fprintf(out, `<polyline points="%s"%s/>`, path.String(), attrs)
	n := len(pts)
	if head := str(el["endArrowhead"]); head != "" {
		drawArrowhead(out, el, x+pts[n-1][0], y+pts[n-1][1], x+pts[n-2][0], y+pts[n-2][1], head)
	}
	if head := str(el["startArrowhead"]); head != "" {
		drawArrowhead(out, el, x+pts[0][0], y+pts[0][1], x+pts[1][0], y+pts[1][1], head)
	}
}

// drawArrowhead draws a head at (tx, ty) pointing away from (fx, fy): a chevron for
// an arrow, a filled triangle for a triangle, a dot for the round kinds, a bar
// for the rest.
func drawArrowhead(out *strings.Builder, el Element, tx, ty, fx, fy float64, head string) {
	stroke := colourOr(el["strokeColor"], "#1e1e1e")
	size := 10 + 2*num(el["strokeWidth"])
	a := math.Atan2(ty-fy, tx-fx)
	wing := func(turn float64) (float64, float64) {
		return tx - size*math.Cos(a+turn), ty - size*math.Sin(a+turn)
	}
	lx, ly := wing(math.Pi / 7)
	rx, ry := wing(-math.Pi / 7)
	switch {
	case head == "arrow":
		fmt.Fprintf(out, `<polyline points="%s,%s %s,%s %s,%s" fill="none" stroke="%s" stroke-width="%s"/>`, f(lx), f(ly), f(tx), f(ty), f(rx), f(ry), stroke, f(strokeWidth(el)))
	case strings.HasPrefix(head, "triangle"), strings.HasPrefix(head, "diamond"):
		fill := stroke
		if strings.HasSuffix(head, "_outline") {
			fill = "none"
		}
		fmt.Fprintf(out, `<polygon points="%s,%s %s,%s %s,%s" fill="%s" stroke="%s"/>`, f(lx), f(ly), f(tx), f(ty), f(rx), f(ry), fill, stroke)
	case strings.HasPrefix(head, "dot"), strings.HasPrefix(head, "circle"):
		fmt.Fprintf(out, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`, f(tx), f(ty), f(size/3), stroke)
	default:
		bx, by := tx+size/2*math.Sin(a), ty-size/2*math.Cos(a)
		ex, ey := tx-size/2*math.Sin(a), ty+size/2*math.Cos(a)
		fmt.Fprintf(out, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`, f(bx), f(by), f(ex), f(ey), stroke, f(strokeWidth(el)))
	}
}

func drawText(out *strings.Builder, el Element, x, y, w float64) {
	size := num(el["fontSize"])
	if size <= 0 {
		size = 20
	}
	lineHeight := num(el["lineHeight"])
	if lineHeight <= 0 {
		lineHeight = 1.25
	}
	anchor, ax := "start", x
	switch str(el["textAlign"]) {
	case "center":
		anchor, ax = "middle", x+w/2
	case "right":
		anchor, ax = "end", x+w
	}
	fmt.Fprintf(out, `<text font-family="%s" font-size="%s" fill="%s" text-anchor="%s"%s>`,
		fontFamily(num(el["fontFamily"])), f(size), colourOr(el["strokeColor"], "#1e1e1e"), anchor, opacityOf(el))
	for i, line := range strings.Split(str(el["text"]), "\n") {
		fmt.Fprintf(out, `<tspan x="%s" y="%s">%s</tspan>`, f(ax), f(y+float64(i)*size*lineHeight+size), html.EscapeString(line))
	}
	out.WriteString("</text>")
}

// fontFamily names the editor's font for a family number, then what a page
// without it falls back to.
func fontFamily(n float64) string {
	switch n {
	case 2, 6:
		return "Nunito, Helvetica, Arial, sans-serif"
	case 3:
		return "Cascadia Code, ui-monospace, monospace"
	case 7:
		return "Lilita One, sans-serif"
	}
	return "Excalifont, Virgil, Xiaolai, Segoe Print, sans-serif"
}

func style(el Element, fill string) string {
	s := fmt.Sprintf(` fill="%s" stroke="%s" stroke-width="%s"`, fill, colourOr(el["strokeColor"], "#1e1e1e"), f(strokeWidth(el)))
	switch str(el["strokeStyle"]) {
	case "dashed":
		s += ` stroke-dasharray="8 8"`
	case "dotted":
		s += ` stroke-dasharray="1.5 6" stroke-linecap="round"`
	}
	return s + opacityOf(el)
}

func fillOf(el Element) string {
	return colourOr(el["backgroundColor"], "none")
}

func colourOr(v any, otherwise string) string {
	if s := str(v); s != "" && s != "transparent" {
		return s // a #hex colour: the rules let nothing else through
	}
	return otherwise
}

func strokeWidth(el Element) float64 {
	if w := num(el["strokeWidth"]); w > 0 {
		return w
	}
	return 2
}

func opacityOf(el Element) string {
	if o, ok := el["opacity"].(float64); ok && o < 100 {
		return ` opacity="` + f(o/100) + `"`
	}
	return ""
}

func pointsOf(el Element) [][2]float64 {
	list, _ := el["points"].([]any)
	out := make([][2]float64, 0, len(list))
	for _, p := range list {
		if pair, ok := p.([]any); ok && len(pair) == 2 {
			out = append(out, [2]float64{num(pair[0]), num(pair[1])})
		}
	}
	return out
}

func f(n float64) string { return strconv.FormatFloat(math.Round(n*100)/100, 'f', -1, 64) }
