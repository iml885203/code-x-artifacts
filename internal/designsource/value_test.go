package designsource

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestBindingsAreTextAndScalarConditionsDoNotCoerceContainers(t *testing.T) {
	for _, c := range []struct {
		v     any
		text  string
		truth bool
	}{{nil, "", false}, {false, "false", false}, {true, "true", true}, {float64(0), "0", false}, {float64(-2), "-2", true}, {"", "", false}, {"<b>literal</b>", "<b>literal</b>", true}, {json.Number("1"), "1", true}} {
		parts, _ := segments("before{{value}}after")
		s, e := ResolveText(parts, map[string]any{"value": c.v})
		if e != nil || s != "before"+c.text+"after" {
			t.Fatalf("%v: %q %v", c.v, s, e)
		}
		truth, e := Truth(c.v)
		if e != nil || truth != c.truth {
			t.Fatalf("truth %v: %v %v", c.v, truth, e)
		}
	}
	for _, v := range []any{[]any{}, map[string]any{}, math.Inf(1), math.NaN()} {
		if _, e := Truth(v); e == nil {
			t.Fatalf("accepted %v", v)
		}
		if _, e := ResolveText([]Segment{{Path: "v"}}, map[string]any{"v": v}); e == nil {
			t.Fatalf("stringified %v", v)
		}
	}
	for _, path := range []string{"a.__proto__.x", "a.prototype", "constructor", "a[0]", "a()", "a + b", "", "a..b"} {
		if _, e := Lookup(nil, path); e == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	if v, e := Lookup(map[string]any{"a": []any{1}}, "a.length"); e != nil || v != nil {
		t.Fatal("array pseudo-property was evaluated")
	}
}

func TestRepeatIdentityIsTypedBoundedAndPositionIsExplicit(t *testing.T) {
	r := Repeat{As: "item", Key: "item.id"}
	items := []any{map[string]any{"id": float64(1)}, map[string]any{"id": "1"}}
	keys, e := RepeatKeys(r, items, nil)
	if e != nil || keys[0] == keys[1] {
		t.Fatalf("typed identity: %v %v", keys, e)
	}
	for _, bad := range [][]any{{map[string]any{"id": "x"}, map[string]any{"id": "x"}}, {map[string]any{}}, {map[string]any{"id": true}}, {map[string]any{"id": float64(1.2)}}, {map[string]any{"id": strings.Repeat("界", 43)}}} {
		if _, e := RepeatKeys(r, bad, nil); e == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	if _, e := RepeatKeys(r, make([]any, 1001), nil); e == nil {
		t.Fatal("accepted >1000 repeat items")
	}
	keys, e = RepeatKeys(Repeat{Positional: true}, []any{nil, nil}, nil)
	if e != nil || keys[0].Kind != "position" || keys[1].Value != "1" {
		t.Fatalf("position: %v %v", keys, e)
	}
}

func TestPropsAreDeclaredBoundedAndResolvedWithoutMutatingSource(t *testing.T) {
	source := `{"schema":{"name":{"type":"string","default":"default","maxLength":20},"settings":{"type":"object","properties":{"show":{"type":"boolean"}},"default":{"show":true}}},"values":{"name":"saved"}}`
	p, e := parseProps(source)
	if e != nil {
		t.Fatal(e)
	}
	resolved, e := p.Resolve(map[string]any{"name": "override", "settings": map[string]any{}})
	if e != nil || resolved["name"] != "override" || len(resolved["settings"].(map[string]any)) != 0 {
		t.Fatalf("precedence/replace: %v %v", resolved, e)
	}
	resolved["settings"].(map[string]any)["x"] = true
	again, e := p.Resolve(nil)
	if e != nil || again["settings"].(map[string]any)["x"] != nil {
		t.Fatalf("mutated defaults: %v %v", again, e)
	}
	for _, bad := range []string{
		`{"schema":{"constructor":{"type":"boolean"}}}`,
		`{"schema":{"x":{"type":"string"}}}`,
		`{"schema":{"x":{"type":"string","maxLength":2,"default":"long"}}}`,
		`{"schema":{"x":{"type":"number","min":2,"max":1}}}`,
		`{"schema":{"x":{"type":"function"}}}`,
		`{"schema":{"x":{"type":"array","maxItems":2}}}`,
		`{"schema":{},"values":{"unknown":true}}`,
		`{"schema":{"x":{"type":"boolean"}},"values":{"x":"true"}}`,
		`{"schema":{},"unknown":true}`,
		`{"schema":{}} {}`,
	} {
		if _, e := parseProps(bad); e == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestDynamicAddressesAreValidatedAfterResolution(t *testing.T) {
	p, e := ParseFrame("frames/a.html", `<a href="{{props.link}}" title="Go {{props.name}}">link</a>`)
	if e != nil {
		t.Fatal(e)
	}
	a := p.Nodes[0].Attributes[0]
	if _, e := ResolveAttribute("a", a, map[string]any{"props": map[string]any{"link": "javascript:alert(1)"}}); e == nil {
		t.Fatal("unsafe URL accepted")
	}
	if value, e := ResolveAttribute("a", a, map[string]any{"props": map[string]any{"link": "https://example.com"}}); e != nil || value != "https://example.com" {
		t.Fatalf("safe URL: %q %v", value, e)
	}
}

func TestDataLimitsRejectNonJSONAndPrototypeKeysBeforeSerialization(t *testing.T) {
	nested := any(nil)
	for range 17 {
		nested = []any{nested}
	}
	for _, value := range []any{nested, strings.Repeat("x", (256<<10)+1), make([]any, 10001), map[string]any{"constructor": true}, func() { panic("must never execute") }, map[string]any{"nested": map[string]any{"__proto__": nil}}, json.Number("NaN")} {
		if e := CheckData(value); e == nil {
			t.Fatalf("accepted invalid JSON %T", value)
		}
	}
	if e := CheckData(map[string]any{"items": []any{nil, true, json.Number("1"), "text"}}); e != nil {
		t.Fatal(e)
	}
	if _, e := RepeatKeys(Repeat{As: "item", Key: "item.id"}, []any{map[string]any{"id": "x", "callback": func() {}}}, nil); e == nil {
		t.Fatal("repeat accepted callable data")
	}
}

func TestPropsSchemaAndValueBoundariesNameTheFailingPath(t *testing.T) {
	valid := `{"schema":{"count":{"type":"number","min":0,"max":2,"default":1},"kind":{"type":"string","enum":["a","b"],"maxLength":1,"default":"a"},"image":{"type":"asset","default":"/files/example/image.png"},"items":{"type":"array","maxItems":2,"items":{"type":"boolean"},"default":[true,false]}},"values":{"count":2}}`
	p, e := ParseFrame("frames/a.html", `<script type="application/x-pictor-props+json">`+valid+`</script>`)
	if e != nil {
		t.Fatal(e)
	}
	resolved, e := p.Props.Resolve(nil)
	if e != nil || resolved["count"] != json.Number("2") {
		t.Fatalf("%v %v", resolved, e)
	}
	for _, override := range []map[string]any{{"count": json.Number("3")}, {"kind": "c"}, {"items": []any{true, true, true}}, {"items": []any{"true"}}, {"image": "/files/id/../secret"}, {"image": "https://example.com/image.png"}} {
		if _, e := p.Props.Resolve(override); e == nil || !strings.Contains(e.Error(), "props.values.") {
			t.Fatalf("bad override: %v %v", override, e)
		}
	}
	for _, bad := range []string{
		`{"schema":{},"schema":{}}`,
		`{"schema":{"x":{"type":"boolean","default":null}}}`,
		`{"schema":{"x":{"type":"boolean","maxLength":2}}}`,
		`{"schema":{"x":{"type":"object","properties":{"__proto__":{"type":"boolean"}}}}}`,
		`{"schema":{"x":{"type":"array","maxItems":0,"items":{"type":"boolean"}}}}`,
	} {
		if _, e := ParseFrame("frames/a.html", `<script type="application/x-pictor-props+json">`+bad+`</script>`); e == nil {
			t.Fatalf("bad props accepted: %s", bad)
		}
	}
}

func TestNumericRepeatIdentityMatchesJSONNumberCanonicalization(t *testing.T) {
	r := Repeat{As: "item", Key: "item.id"}
	for _, c := range []struct {
		number   float64
		expected string
	}{{1000000, "1000000"}, {1e21, "1e+21"}, {math.Copysign(0, -1), "0"}} {
		keys, e := RepeatKeys(r, []any{map[string]any{"id": c.number}}, nil)
		if e != nil || keys[0].Value != c.expected {
			t.Fatalf("numeric identity %v: %v %v", c.number, keys, e)
		}
	}
	if _, e := RepeatKeys(r, []any{map[string]any{"id": float64(0)}, map[string]any{"id": math.Copysign(0, -1)}}, nil); e == nil {
		t.Fatal("negative zero did not collide with zero")
	}
}
