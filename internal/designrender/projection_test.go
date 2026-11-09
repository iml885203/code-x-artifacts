package designrender

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

func projectionContext(t *testing.T, source string) RenderContext {
	t.Helper()
	s := snapshot()
	s.Input.Files["frames/a.html"] = source
	c, err := BuildContext(s, Request{FrameID: "a", Purpose: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProjectionLowersTableAndSelectDirectivesWithoutHTMLRepair(t *testing.T) {
	for _, fixture := range []struct{ parent, child, body string }{
		{"table", "tr", `<td data-pid="cell">你好 &amp; {{item.label}}</td>`},
		{"select", "option", `你好 &amp; {{item.label}}`},
	} {
		t.Run(fixture.parent, func(t *testing.T) {
			source := `<` + fixture.parent + ` data-pid="parent"><p-for data-pid="loop" list="{{items}}" as="item" key="{{item.id}}"><!--keep--><p-if data-pid="show" value="{{item.visible}}"><` + fixture.child + ` data-pid="child">` + fixture.body + `</` + fixture.child + `></p-if></p-for></` + fixture.parent + `>`
			c := projectionContext(t, source)
			before, _ := json.Marshal(c.Plan.Frames[c.FrameFile])
			values := map[string]any{"items": []any{
				map[string]any{"id": float64(1), "visible": true, "label": "<img src=x onerror=evil()>"},
				map[string]any{"id": "1", "visible": true, "label": "second"},
				map[string]any{"id": "hidden", "visible": false, "label": "hidden"},
			}}
			nodes, err := Project(c, []InstanceData{{File: c.FrameFile, Values: values}})
			if err != nil {
				t.Fatal(err)
			}
			checkOrdinaryContainer(t, nodes, fixture.parent)
			first, second := nodes[0].Children[0], nodes[0].Children[1]
			facts := []string{first.Tag, second.Tag, first.Source.File, first.Source.PID}
			if !reflect.DeepEqual(facts, []string{fixture.child, fixture.child, c.FrameFile, "child"}) {
				t.Fatal("source ownership lost")
			}
			want := []InstanceSegment{{Site: designsource.SourceRef{File: c.FrameFile, PID: "loop"}, Key: &designsource.InstanceKey{Kind: "number", Value: "1"}}}
			if !reflect.DeepEqual(first.Path, want) || reflect.DeepEqual(first.Path, second.Path) {
				t.Fatalf("typed repeat path: %#v", first.Path)
			}
			if fixture.parent == "table" {
				first = first.Children[0]
			}
			if first.Children[0].Text != "你好 & <img src=x onerror=evil()>" || first.Children[0].Kind != "text" {
				t.Fatal("bound text became markup or lost entity bytes")
			}
			proof, err := json.Marshal(nodes)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("ordinary %s projection: %s", fixture.parent, proof)
			after, _ := json.Marshal(c.Plan.Frames[c.FrameFile])
			if string(before) != string(after) {
				t.Fatal("projection mutated source IR")
			}
		})
	}
}

func TestProjectionUsesExplicitImportSnapshotsAndDeclaredInstanceProps(t *testing.T) {
	s := snapshot()
	s.Input.Files["frames/a.html"] = `<p-for data-pid="loop" list="{{items}}" as="item" key="{{item.id}}"><p-import data-pid="site" src="frames/b.html" props="{{item.props}}"></p-import></p-for>`
	s.Input.Files["frames/b.html"] = `<script type="application/x-pictor-props+json">{"schema":{"title":{"type":"string","maxLength":80,"default":"default"}},"values":{"title":"saved"}}</script><script type="text/x-pictor">class Frame extends PictorLogic { renderVals(){ throw new Error("must not run"); } }</script><p data-pid="label">{{props.title}}/{{state.count}}/{{suffix}}</p>`
	c, err := BuildContext(s, Request{FrameID: "a", Purpose: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	key := designsource.InstanceKey{Kind: "string", Value: "x"}
	path := []InstanceSegment{{Site: designsource.SourceRef{File: c.FrameFile, PID: "loop"}, Key: &key}, {Site: designsource.SourceRef{File: c.FrameFile, PID: "site"}}}
	data := []InstanceData{
		{File: c.FrameFile, Values: map[string]any{"items": []any{map[string]any{"id": "x", "props": map[string]any{"title": "instance"}}}}},
		{File: "frames/b.html", Path: path, Values: map[string]any{"suffix": "provided"}, State: map[string]any{"count": json.Number("2")}},
	}
	nodes, err := Project(c, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Source.File != "frames/b.html" || nodes[0].Children[0].Text != "instance/2/provided" || !reflect.DeepEqual(nodes[0].Path, path) {
		t.Fatalf("import projection: %#v", nodes)
	}
	if _, err := Project(c, data[:1]); err == nil {
		t.Fatal("missing instance data silently executed or invented logic output")
	}
	if c.Plan.Frames["frames/b.html"].Props.Values["title"] != "saved" {
		t.Fatal("import override persisted")
	}
}

func checkOrdinaryContainer(t *testing.T, nodes []*ProjectedNode, tag string) {
	t.Helper()
	if len(nodes) != 1 {
		t.Fatalf("ordinary root count: %d", len(nodes))
	}
	if nodes[0].Tag != tag || len(nodes[0].Children) != 2 {
		t.Fatalf("ordinary hierarchy: %#v", nodes[0])
	}
}
