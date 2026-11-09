package designsource

import (
	"strings"
	"testing"
)

func TestStyleBindingsKeepDeclarationStructureAndTokens(t *testing.T) {
	p, e := ParseFrame("frames/a.html", `<div style="width:{{props.width}}px; color:{{props.color}}; border-color:var(--ink)"></div>`)
	if e != nil {
		t.Fatal(e)
	}
	attr := p.Nodes[0].Attributes[0]
	value, e := ResolveAttribute("div", attr, map[string]any{"props": map[string]any{"width": float64(12), "color": "#123456"}})
	if e != nil || !strings.Contains(value, "width:12px;") || !strings.Contains(value, "var(--ink)") {
		t.Fatalf("%q %v", value, e)
	}
	for _, data := range []map[string]any{
		{"width": "12", "color": "red"},
		{"width": float64(12), "color": "red;position:fixed"},
		{"width": float64(12), "color": "red!important"},
		{"width": float64(12), "color": "url(https://evil.example/x)"},
		{"width": float64(12), "color": "red/*"},
	} {
		if _, e := ResolveAttribute("div", attr, map[string]any{"props": data}); e == nil {
			t.Fatalf("CSS data accepted %v", data)
		}
	}
	for _, style := range []string{`{{property}}:red`, `color:'{{value}}'`, `background:url({{image}})`, `color:#{{hex}}`, `color:{{name}}-suffix`, `@import 'x'`, `color:red;broken`} {
		if !strings.Contains(style, "{{") {
			continue
		}
		if _, e := ParseFrame("frames/a.html", `<div style="`+style+`"></div>`); e == nil {
			t.Fatalf("accepted CSS structure %s", style)
		}
	}
}
