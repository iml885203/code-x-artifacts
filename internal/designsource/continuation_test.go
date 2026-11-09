package designsource

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNativePromiseAndAsyncMethodsRemainExactInertSource(t *testing.T) {
	for _, body := range []string{
		`pick() { return Promise.resolve(1).then(value => this.setState({value})); }`,
		`async pick() { const value = await Promise.resolve(1); this.setState({value}); }`,
		`async pick() { await new Promise(resolve => resolve()); }`,
	} {
		t.Run(body, func(t *testing.T) {
			logic := "class Frame extends PictorLogic {\r\n" + body + "\r\n}"
			source := `<script type="text/x-pictor">` + logic + `</script><button data-pid="pick" p-on:click="{{pick}}">選擇</button>`
			p, err := ParseFrame("frames/a.html", source)
			if err != nil {
				t.Fatal(err)
			}
			if p.Source != source || p.Rewritten != source || p.Logic.Source != logic || p.Logic.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(logic))) {
				t.Fatal("native source changed")
			}
			if !p.Logic.SyntaxChecked || strings.Join(p.Logic.Methods, ",") != "pick" {
				t.Fatal("missing instance method")
			}
			if p.Logic.CompiledSHA256 != "" || p.Logic.CapabilityProfile != "" || len(p.Logic.WorkerProgram) != 0 || p.RequireExecutable() == nil {
				t.Fatal("syntax admitted execution")
			}
		})
	}
}

func TestTableAndSelectDirectivesRetainAuthoredHierarchyAndSourceBoundaries(t *testing.T) {
	for _, c := range []struct{ parent, child, content string }{
		{"table", "tr", `<td data-pid="cell">你好 &amp; {{item.label}}</td>`},
		{"select", "option", `你好 &amp; {{item.label}}`},
	} {
		t.Run(c.parent, func(t *testing.T) {
			source := `<` + c.parent + ` data-pid="parent"><p-for data-pid="repeat" list="{{items}}" as="item" key="{{item.id}}"><script type="application/x-pictor-props+json">{"schema":{},"values":{}}</script><!--界--><p-if data-pid="visible" value="{{item.visible}}"><` + c.child + ` data-pid="child">` + c.content + `</` + c.child + `></p-if></p-for></` + c.parent + `>`
			input := bundle(map[string]string{"frames/a.html": source})
			b, err := ParseBundle(input)
			if err != nil {
				t.Fatal(err)
			}
			p := b.Frames["frames/a.html"]
			if p.Source != source || p.Rewritten != source {
				t.Fatal("authored bytes changed")
			}
			parent := p.Nodes[0]
			repeat := parent.Children[0]
			condition := repeat.Children[0]
			child := condition.Children[0]
			hierarchy := []string{parent.Tag, repeat.Kind, repeat.PID, condition.Kind, child.Tag, child.PID}
			if !reflect.DeepEqual(hierarchy, []string{c.parent, "repeat", "repeat", "condition", c.child, "child"}) {
				t.Fatalf("authored hierarchy: %v", hierarchy)
			}
			checkDirectiveSourceBoundaries(t, b, p, []*Node{parent, repeat, condition, child})
			text := child.Children[0]
			if c.parent == "table" {
				text = text.Children[0]
			}
			checkDirectiveTextBytes(t, p, text)
			again, err := ParseFrame(p.File, p.Rewritten)
			if err != nil || again.Rewritten != source {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
}

func checkDirectiveSourceBoundaries(t *testing.T, b BundlePlan, p FramePlan, nodes []*Node) {
	t.Helper()
	for _, n := range nodes {
		raw := p.Source[n.Span.StartByte:n.Span.EndByte]
		if n.Span.File != p.File || !strings.HasPrefix(raw, "<"+n.Tag+" ") || !strings.HasSuffix(raw, "</"+n.Tag+">") {
			t.Fatalf("invalid half-open source span: %#v", n.Span)
		}
		if b.Sources[SourceRef{p.File, n.PID}] != n {
			t.Fatal("source index lost directive identity")
		}
	}
	repeat := nodes[1]
	if len(p.Declarations) != 1 || len(p.Opaque) != 1 {
		t.Fatal("source block count")
	}
	for _, block := range []SourceBlock{p.Declarations[0], p.Opaque[0]} {
		if block.ParentPID != "repeat" || block.ParentSpan != repeat.Span {
			t.Fatal("direct authored parent lost")
		}
	}
	declaration := p.Declarations[0].ContentSpan
	if p.Source[declaration.StartByte:declaration.EndByte] != `{"schema":{},"values":{}}` {
		t.Fatal("declaration bytes lost")
	}
	opaque := p.Opaque[0].Span
	if p.Source[opaque.StartByte:opaque.EndByte] != `<!--界-->` {
		t.Fatal("opaque bytes lost")
	}
}

func checkDirectiveTextBytes(t *testing.T, p FramePlan, text *Node) {
	t.Helper()
	parts := text.Segments
	if len(parts) != 2 {
		t.Fatal("literal/hole segment count")
	}
	facts := []string{parts[0].Literal, parts[1].Path, p.Source[parts[0].Span.StartByte:parts[0].Span.EndByte], p.Source[parts[1].Span.StartByte:parts[1].Span.EndByte]}
	if !reflect.DeepEqual(facts, []string{"你好 & ", "item.label", "你好 &amp; ", "{{item.label}}"}) {
		t.Fatalf("literal/binding bytes: %v", facts)
	}
}
