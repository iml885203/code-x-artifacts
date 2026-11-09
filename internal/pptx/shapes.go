package pptx

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The slide is the browser's 1920 x 1080 canvas: one canvas pixel is 6350 EMU
// (half a point), so 16:9 at PowerPoint's widescreen size.
const (
	emuPerPixel = 6350
	slideWidth  = 1920 * emuPerPixel
	slideHeight = 1080 * emuPerPixel
	padX        = 120.0
	padY        = 72.0
	innerWidth  = 1920 - 2*padX
	innerHeight = 1080 - 2*padY
)

// The kit's light palette.
const (
	colorBackground = "F3F5F9"
	colorPlate      = "FFFFFF"
	colorRaised     = "E5E8ED"
	colorLine       = "CED1D8"
	colorText       = "0F1624"
	colorMuted      = "484D58"
	colorStar       = "BB7400"
	colorStarText   = "8A4C00"
	colorOnStar     = "201301"
	colorDanger     = "B02A2D"
	colorOK         = "00673F"
	colorInfo       = "005F99"
	monoFont        = "Consolas"
	eastAsianFont   = "Microsoft JhengHei"
)

var toneColors = map[Tone][2]string{ // text, fill
	ToneSuccess: {colorOK, "DDEFE6"},
	ToneWarning: {colorStarText, "F6E9D2"},
	ToneDanger:  {colorDanger, "F6E0DF"},
}

// esc makes any string safe as XML text or an attribute value; characters XML
// cannot hold become U+FFFD.
func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s)) // a strings.Builder does not fail
	return b.String()
}

func emu(px float64) string { return strconv.FormatInt(int64(math.Round(px*emuPerPixel)), 10) }

func itoa(n int) string { return strconv.Itoa(n) }

func splitLines(s string) []string { return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }

// The measures below only have to be right enough to share a slide out, not
// to match the browser's pixels.

func advance(r rune) float64 {
	switch {
	case r >= 0x2E80:
		return 1
	case r == ' ':
		return 0.25
	case r >= 'A' && r <= 'Z':
		return 0.6
	default:
		return 0.5
	}
}

func (t Text) String() string {
	var b strings.Builder
	for _, r := range t {
		b.WriteString(r.Text)
	}
	return b.String()
}

// lines is how many lines text takes in a box w pixels wide at px.
func lines(s string, w, px float64) float64 {
	n := 0.0
	for _, line := range splitLines(s) {
		width := 0.0
		for _, r := range line {
			width += advance(r)
		}
		n += math.Max(1, math.Ceil(width*px*1.08/math.Max(w, px)))
	}
	return n
}

// textHeight is the height of text at px with lines 1.25 apart.
func textHeight(t Text, w, px float64) float64 { return lines(t.String(), w, px) * px * 1.25 }

// canvas collects the shapes of one slide.
type canvas struct {
	b    strings.Builder
	next int
	// centred is set on a cover slide, whose text is centred.
	centred bool
}

func (c *canvas) id() int {
	c.next++
	return c.next
}

// look is how a shape is drawn.
type look struct {
	geom   string // prstGeom; rect when empty
	fill   string
	line   string
	lineW  float64
	inset  float64
	anchor string // t, ctr or b; t when empty
}

// shape adds a shape at x, y, w wide and h high holding paragraphs (which may
// be empty: a shape with no text).
func (c *canvas) shape(name string, x, y, w, h float64, l look, paragraphs string) {
	n := c.id()
	geom, fill, line, anchor := l.geom, `<a:noFill/>`, `<a:ln><a:noFill/></a:ln>`, l.anchor
	if geom == "" {
		geom = "rect"
	}
	if l.fill != "" {
		fill = `<a:solidFill><a:srgbClr val="` + l.fill + `"/></a:solidFill>`
	}
	if l.line != "" {
		line = `<a:ln w="` + emu(l.lineW) + `"><a:solidFill><a:srgbClr val="` + l.line + `"/></a:solidFill></a:ln>`
	}
	if anchor == "" {
		anchor = "t"
	}
	box := ""
	if paragraphs != "" && l.fill == "" && l.line == "" {
		box = ` txBox="1"`
	}
	fmt.Fprintf(&c.b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s %d"/><p:cNvSpPr%s/><p:nvPr/></p:nvSpPr>`, n, name, n, box)
	c.b.WriteString(`<p:spPr>` + xfrm(x, y, w, h) + `<a:prstGeom prst="` + geom + `"><a:avLst/></a:prstGeom>` + fill + line + `</p:spPr>`)
	if paragraphs != "" {
		in := emu(l.inset)
		c.b.WriteString(`<p:txBody><a:bodyPr wrap="square" lIns="` + in + `" tIns="` + in + `" rIns="` + in + `" bIns="` + in + `" anchor="` + anchor + `"><a:normAutofit/></a:bodyPr><a:lstStyle/>` + paragraphs + `</p:txBody>`)
	}
	c.b.WriteString(`</p:sp>`)
}

func xfrm(x, y, w, h float64) string {
	return `<a:xfrm><a:off x="` + emu(x) + `" y="` + emu(y) + `"/><a:ext cx="` + emu(math.Max(w, 1)) + `" cy="` + emu(math.Max(h, 1)) + `"/></a:xfrm>`
}

// style is how a paragraph's text looks.
type style struct {
	px      float64
	color   string
	bold    bool
	italic  bool
	mono    bool
	align   string // l, ctr or r; l when empty
	bullet  string // "", "char" or "num"
	indent  float64
	spaceUp float64
}

// para writes t as one paragraph; a newline in it is a line break.
func para(t Text, s style) string {
	var b strings.Builder
	b.WriteString(`<a:p><a:pPr algn="` + orDefault(s.align, "l") + `"`)
	if s.bullet != "" {
		b.WriteString(` marL="` + emu(s.indent) + `" indent="-` + emu(s.indent) + `"`)
	}
	b.WriteString(`>`)
	if s.spaceUp > 0 {
		fmt.Fprintf(&b, `<a:spcBef><a:spcPts val="%d"/></a:spcBef>`, int(math.Round(s.spaceUp*50)))
	}
	switch s.bullet {
	case "char":
		b.WriteString(`<a:buClr><a:srgbClr val="` + colorStar + `"/></a:buClr><a:buFont typeface="Arial"/><a:buChar char="&#8226;"/>`)
	case "num":
		b.WriteString(`<a:buClr><a:srgbClr val="` + colorStarText + `"/></a:buClr><a:buFont typeface="+mj-lt"/><a:buAutoNum type="arabicPeriod"/>`)
	default:
		b.WriteString(`<a:buNone/>`)
	}
	b.WriteString(`</a:pPr>`)
	for _, r := range t {
		rs := s
		rs.bold = s.bold || r.Bold
		rs.mono = s.mono || r.Code
		for i, line := range splitLines(r.Text) {
			if i > 0 {
				b.WriteString(`<a:br>` + runProps(rs) + `</a:br>`)
			}
			if line != "" {
				b.WriteString(`<a:r>` + runProps(rs) + `<a:t>` + esc(line) + `</a:t></a:r>`)
			}
		}
	}
	b.WriteString(`<a:endParaRPr lang="en-US" sz="` + size(s.px) + `" dirty="0"/></a:p>`)
	return b.String()
}

func runProps(s style) string {
	attrs := ` lang="en-US" sz="` + size(s.px) + `"`
	if s.bold {
		attrs += ` b="1"`
	}
	if s.italic {
		attrs += ` i="1"`
	}
	fonts := ""
	if s.mono {
		fonts = `<a:latin typeface="` + monoFont + `"/><a:cs typeface="` + monoFont + `"/>`
	}
	return `<a:rPr` + attrs + ` dirty="0"><a:solidFill><a:srgbClr val="` + orDefault(s.color, colorText) + `"/></a:solidFill>` + fonts + `</a:rPr>`
}

// size is a font size in hundredths of a point: a canvas pixel is half a point.
func size(px float64) string { return itoa(int(math.Round(px * 50))) }

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// align is the paragraph alignment of body text on this slide.
func (c *canvas) align() string {
	if c.centred {
		return "ctr"
	}
	return "l"
}
