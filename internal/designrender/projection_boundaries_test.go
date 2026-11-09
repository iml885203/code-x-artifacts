package designrender

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

func TestProjectionRejectsInvalidDataAtomicallyWithOriginalSourceSpans(t *testing.T) {
	cases := []struct {
		source  string
		values  map[string]any
		message string
	}{
		{`<p data-pid="p">你好 {{value}}</p>`, map[string]any{"value": map[string]any{}}, "finite JSON scalar"},
		{`<p-if data-pid="if" value="{{value}}"><p>hidden</p></p-if>`, map[string]any{"value": []any{}}, "finite JSON scalar"},
		{`<p-for data-pid="loop" list="{{items}}" as="item" key="{{item.id}}"><p>{{item.label}}</p></p-for>`, map[string]any{"items": []any{map[string]any{"id": "same"}, map[string]any{"id": "same"}}}, "duplicate repeat key"},
		{`<p-for data-pid="loop" list="{{items}}" as="item" key-mode="position"><p></p></p-for>`, nil, "repeat list must be an array"},
		{`<a data-pid="link" href="{{url}}">link</a>`, map[string]any{"url": "javascript:evil()"}, "href"},
		{`<p data-pid="p" style="width:{{width}}px">text</p>`, map[string]any{"width": "1;display:none"}, "finite number"},
		{`<p-import data-pid="site" src="frames/b.html" props="{{child}}"></p-import>`, map[string]any{"child": "not an object"}, "plain object"},
	}
	for _, c := range cases {
		t.Run(c.message, func(t *testing.T) {
			context := projectionContext(t, c.source)
			nodes, err := Project(context, []InstanceData{{File: context.FrameFile, Values: c.values}})
			if nodes != nil || err == nil || !strings.Contains(err.Error(), c.message) {
				t.Fatalf("partial or incorrect refusal: %#v %v", nodes, err)
			}
			var problems designsource.Problems
			if !errors.As(err, &problems) || len(problems) != 1 {
				t.Fatalf("diagnostic shape: %T %v", err, err)
			}
			span := problems[0].Span
			if span.File != context.FrameFile || span.StartByte < 0 || span.EndByte > len(c.source) || span.EndByte <= span.StartByte {
				t.Fatalf("original source span: %#v", span)
			}
		})
	}
}

func TestProjectionBoundsExpansionIncludingEmptyLoopsAndRepeatedText(t *testing.T) {
	for _, c := range []struct{ source, message string }{
		{`<p-for list="{{items}}" as="outer" key-mode="position"><p-for list="{{items}}" as="inner" key-mode="position"></p-for></p-for>`, "work exceeds"},
		{`<p-for list="{{items}}" as="item" key-mode="position"><p>` + strings.Repeat("x", 1100) + `</p></p-for>`, "text/attributes exceed"},
	} {
		t.Run(c.message, func(t *testing.T) {
			context := projectionContext(t, c.source)
			values := map[string]any{"items": make([]any, 1000)}
			nodes, err := Project(context, []InstanceData{{File: context.FrameFile, Values: values}})
			if nodes != nil || err == nil || !strings.Contains(err.Error(), c.message) {
				t.Fatalf("unbounded projection: %v", err)
			}
		})
	}
}

func TestProjectionRetainsKeyedAndExplicitPositionalIdentityOnReorder(t *testing.T) {
	for _, mode := range []string{`key="{{item.id}}"`, `key-mode="position"`} {
		t.Run(mode, func(t *testing.T) {
			context := projectionContext(t, `<p-for data-pid="loop" list="{{items}}" as="item" `+mode+`><b data-pid="label">{{item.id}}</b></p-for>`)
			items := []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
			first, err := Project(context, []InstanceData{{File: context.FrameFile, Values: map[string]any{"items": items}}})
			if err != nil {
				t.Fatal(err)
			}
			second, err := Project(context, []InstanceData{{File: context.FrameFile, Values: map[string]any{"items": []any{items[1], items[0]}}}})
			if err != nil {
				t.Fatal(err)
			}
			target := second[1]
			if mode == `key-mode="position"` {
				target = second[0]
			}
			if !reflect.DeepEqual(first[0].Path, target.Path) || first[0].Source != target.Source {
				t.Fatal("reorder changed source/instance identity")
			}
			first[0].Path[0].Key.Value = "mutated output"
			if second[0].Path[0].Key.Value == "mutated output" {
				t.Fatal("projected paths alias")
			}
		})
	}
}

func TestProjectionRefusesReservedDuplicateUnusedAndNonJSONSnapshots(t *testing.T) {
	context := projectionContext(t, `<p data-pid="p">literal</p>`)
	for _, data := range [][]InstanceData{
		{{File: context.FrameFile, Values: map[string]any{"props": map[string]any{}}}},
		{{File: context.FrameFile, Values: map[string]any{"state": map[string]any{}}}},
		{{File: context.FrameFile, Values: map[string]any{"x": func() {}}}},
		{{File: context.FrameFile}, {File: context.FrameFile, Path: []InstanceSegment{}}},
		{{File: context.FrameFile}, {File: "frames/b.html"}},
		{{File: context.FrameFile, Values: map[string]any{"constructor": "bad"}}},
	} {
		nodes, err := Project(context, data)
		if nodes != nil || err == nil {
			t.Fatalf("invalid data accepted: %v", err)
		}
	}
	if context.Plan.Frames[context.FrameFile].Source != `<p data-pid="p">literal</p>` {
		t.Fatal("refusal changed source")
	}
}

func TestProjectionRevalidatesSourceAndPinnedRuntimeIdentityWithoutAdmission(t *testing.T) {
	context := projectionContext(t, `<p data-pid="p">safe</p>`)
	data := []InstanceData{{File: context.FrameFile}}
	for _, change := range []func(*RenderContext){
		func(c *RenderContext) { c.RuntimeID = "pictor-design/latest" },
		func(c *RenderContext) { c.RuntimeSHA256 = strings.Repeat("a", 64) },
		func(c *RenderContext) { c.Format = "1" },
		func(c *RenderContext) { c.FrameFile = "frames/b.html" },
		func(c *RenderContext) { c.DependencySHA256 = "stale" },
	} {
		altered := context
		change(&altered)
		if nodes, err := Project(altered, data); err == nil || nodes != nil {
			t.Fatal("context mismatch admitted")
		}
	}
	context.Plan.Frames[context.FrameFile].Nodes[0].Tag = "script"
	nodes, err := Project(context, data)
	if err != nil || nodes[0].Tag != "p" {
		t.Fatalf("mutable IR bypassed original source revalidation: %v", err)
	}
}

func TestNestedRepeatsAndImportsKeepIndependentSuppliedInstanceData(t *testing.T) {
	s := snapshot()
	s.Input.Files["frames/a.html"] = `<p-for data-pid="outer" list="{{groups}}" as="group" key="{{group.id}}"><p-for data-pid="inner" list="{{group.items}}" as="item" key="{{item.id}}"><p-import data-pid="site" src="frames/b.html"></p-import></p-for></p-for>`
	s.Input.Files["frames/b.html"] = `<p data-pid="label">{{state.value}}</p>`
	context, err := BuildContext(s, Request{FrameID: "a", Purpose: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	groups := []any{map[string]any{"id": "one", "items": []any{map[string]any{"id": "child"}}}, map[string]any{"id": "two", "items": []any{map[string]any{"id": "child"}}}}
	data := []InstanceData{{File: context.FrameFile, Values: map[string]any{"groups": groups}}}
	for _, id := range []string{"one", "two"} {
		path := []InstanceSegment{
			{Site: designsource.SourceRef{File: context.FrameFile, PID: "outer"}, Key: &designsource.InstanceKey{Kind: "string", Value: id}},
			{Site: designsource.SourceRef{File: context.FrameFile, PID: "inner"}, Key: &designsource.InstanceKey{Kind: "string", Value: "child"}},
			{Site: designsource.SourceRef{File: context.FrameFile, PID: "site"}},
		}
		data = append(data, InstanceData{File: "frames/b.html", Path: path, State: map[string]any{"value": id}})
	}
	nodes, err := Project(context, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Source != nodes[1].Source || reflect.DeepEqual(nodes[0].Path, nodes[1].Path) {
		t.Fatal("imported source instances collapsed")
	}
	if nodes[0].Children[0].Text != "one" || nodes[1].Children[0].Text != "two" {
		t.Fatal("supplied instance data leaked across imports")
	}
}

func TestProjectionBoundsAggregateInstanceDataBeforeExpansion(t *testing.T) {
	context := projectionContext(t, `<p data-pid="p">literal</p>`)
	data := []InstanceData{
		{File: context.FrameFile, Values: map[string]any{"text": strings.Repeat("x", 150000)}},
		{File: "frames/b.html", Values: map[string]any{"text": strings.Repeat("x", 150000)}},
	}
	nodes, err := Project(context, data)
	if err == nil || nodes != nil || !strings.Contains(err.Error(), "snapshot data exceeds") {
		t.Fatalf("aggregate data not bounded: %v", err)
	}
}

func TestProjectionRetainsTheSharedPlainDataDepthBoundary(t *testing.T) {
	context := projectionContext(t, `<p data-pid="p">literal</p>`)
	var value any = "leaf"
	for range 15 {
		value = map[string]any{"next": value}
	}
	values := map[string]any{"nested": value}
	if err := designsource.CheckData(values); err != nil {
		t.Fatalf("fixture exceeds existing data bound: %v", err)
	}
	if _, err := Project(context, []InstanceData{{File: context.FrameFile, Values: values}}); err != nil {
		t.Fatalf("projection envelope tightened shared data depth: %v", err)
	}
}
