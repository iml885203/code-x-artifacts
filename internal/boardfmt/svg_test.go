package boardfmt

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

func svgOf(t *testing.T, content string) string {
	t.Helper()
	b, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	out := SVG(b)
	d := xml.NewDecoder(strings.NewReader(out))
	for {
		if _, err := d.Token(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, out)
		}
	}
	return out
}

func TestEveryElementABoardHoldsIsDrawnInItsSVG(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, element, want string }{
		{"a rectangle", `{"id":"r","type":"rectangle","x":0,"y":0,"width":100,"height":50,"backgroundColor":"#a5d8ff"}`, `<rect x="0" y="0" width="100" height="50" rx="0" fill="#a5d8ff"`},
		{"a rounded rectangle", `{"id":"r","type":"rectangle","x":0,"y":0,"width":100,"height":40,"roundness":{"type":3}}`, `rx="10"`},
		{"an ellipse", `{"id":"e","type":"ellipse","x":0,"y":0,"width":100,"height":50}`, `<ellipse cx="50" cy="25" rx="50" ry="25"`},
		{"a diamond", `{"id":"d","type":"diamond","x":0,"y":0,"width":100,"height":50}`, `<polygon points="50,0 100,25 50,50 0,25"`},
		{"an arrow with its head", `{"id":"a","type":"arrow","x":10,"y":10,"points":[[0,0],[100,0]],"endArrowhead":"arrow"}`, `<polyline points="10,10 110,10" fill="none"`},
		{"a dashed line", `{"id":"l","type":"line","x":0,"y":0,"points":[[0,0],[50,50]],"strokeStyle":"dashed"}`, `stroke-dasharray="8 8"`},
		{"a pen stroke", `{"id":"p","type":"freedraw","x":0,"y":0,"points":[[0,0],[1,2],[3,1]]}`, `<polyline points="0,0 1,2 3,1" fill="none"`},
		{"a text of two lines", `{"id":"t","type":"text","x":0,"y":0,"width":80,"height":50,"text":"one\ntwo","fontSize":20,"lineHeight":1.25}`, `<tspan x="0" y="20">one</tspan><tspan x="0" y="45">two</tspan>`},
		{"a centred label", `{"id":"t","type":"text","x":10,"y":0,"width":80,"height":25,"text":"Go","textAlign":"center"}`, `text-anchor="middle"`},
		{"a frame with its name", `{"id":"f","type":"frame","x":0,"y":0,"width":300,"height":200,"name":"Flow"}`, `>Flow</text>`},
		{"a half-transparent shape", `{"id":"r","type":"rectangle","x":0,"y":0,"width":10,"height":10,"opacity":50}`, `opacity="0.5"`},
		{"a turned shape", `{"id":"r","type":"rectangle","x":0,"y":0,"width":10,"height":10,"angle":1.5707963267948966}`, `transform="rotate(90 5 5)"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out := svgOf(t, `{"version":1,"elements":[`+c.element+`]}`); !strings.Contains(out, c.want) {
				t.Errorf("no %s in\n%s", c.want, out)
			}
		})
	}
}

func TestABoardsTextIsTextInItsSVGAndNeverMarkup(t *testing.T) {
	t.Parallel()
	out := svgOf(t, `{"version":1,"elements":[
	  {"id":"t","type":"text","x":0,"y":0,"width":80,"height":25,"text":"<script>alert(1)</script> & </svg>"},
	  {"id":"f","type":"frame","x":0,"y":0,"width":80,"height":80,"name":"\"><script>"}]}`)
	if strings.Contains(out, "<script") || strings.Count(out, "</svg>") != 1 {
		t.Errorf("the board's words became markup:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt; &amp; &lt;/svg&gt;") {
		t.Errorf("the words are not there as text:\n%s", out)
	}
}

func TestABoardsSVGFramesEveryElementAndAnEmptyBoardIsABlankOne(t *testing.T) {
	t.Parallel()
	out := svgOf(t, `{"version":1,"elements":[
	  {"id":"r","type":"rectangle","x":-50,"y":20,"width":100,"height":50},
	  {"id":"a","type":"arrow","x":100,"y":100,"points":[[0,0],[200,80]]}]}`)
	if !strings.Contains(out, `viewBox="-66 4 382 192"`) {
		t.Errorf("the view does not hold every element with its margin:\n%s", out[:120])
	}
	if empty := svgOf(t, `{"version":1,"elements":[]}`); !strings.Contains(empty, `viewBox="-16 -16 232 132"`) {
		t.Errorf("an empty board: %s", empty)
	}
}
