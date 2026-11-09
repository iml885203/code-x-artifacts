package designrender

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

func TestProjectionResolvesAttributesAndSVGWithoutEnablingSourceEvents(t *testing.T) {
	source := `<script type="text/x-pictor">class Frame extends PictorLogic { choose(){ throw new Error("must not execute"); } }</script><p data-pid="p" title="Hello {{name}}" style="width:{{width}}px;color:var(--ink)" p-on:click="{{choose}}">{{missing}}</p><a data-pid="next" href="b.html">Next</a><svg data-pid="svg" viewBox="0 0 10 10"><defs data-pid="defs"><linearGradient data-pid="gradient" id="ink" gradientUnits="userSpaceOnUse"></linearGradient></defs><use data-pid="use" xlink:href="#ink"></use></svg>`
	context := projectionContext(t, source)
	values := map[string]any{"name": "<b>plain</b>", "width": float64(12)}
	nodes, err := Project(context, []InstanceData{{File: context.FrameFile, Values: values}})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatal("declaration entered projected tree")
	}
	if !reflect.DeepEqual(nodes[0].Events, map[string]string{"click": "choose"}) || nodes[0].Children[0].Text != "" {
		t.Fatal("passive method/missing text semantics lost")
	}
	attrs := map[string]string{}
	for _, a := range nodes[0].Attributes {
		attrs[a.Name] = a.Value
	}
	if attrs["title"] != "Hello <b>plain</b>" || attrs["style"] != "width:12px;color:var(--ink);" {
		t.Fatalf("attribute assignment values: %v", attrs)
	}
	if nodes[1].Navigation != "frames/b.html" || nodes[1].Attributes[0].Value != "b.html" {
		t.Fatal("literal source navigation lost")
	}
	checkProjectedSVG(t, nodes[2])
	nodes[0].Events["click"] = "changed"
	if context.Plan.Frames[context.FrameFile].Nodes[0].Events["click"] != "choose" || values["width"] != float64(12) {
		t.Fatal("projection output aliases input")
	}
}

func TestContextRequiresExactPinnedRuntimeIdentifierAndFormatPair(t *testing.T) {
	for _, runtime := range []string{"", "pictor-design/latest", "pictor-design/2.0.1", "PICTOR-DESIGN/2.0.0"} {
		s := snapshot()
		s.Input.Manifest = strings.Replace(s.Input.Manifest, designsource.RuntimeID, runtime, 1)
		if _, err := BuildContext(s, Request{FrameID: "a", Purpose: "raw"}); err == nil {
			t.Fatalf("accepted runtime alias %q", runtime)
		}
	}
	for _, change := range []func(*Snapshot){
		func(s *Snapshot) { s.Input.Format = "1" },
		func(s *Snapshot) {
			s.Input.Manifest = strings.Replace(s.Input.Manifest, `"version":2`, `"version":1`, 1)
		},
		func(s *Snapshot) {
			s.Input.Manifest = strings.Replace(s.Input.Manifest, `"runtime":"pictor-design/2.0.0",`, "", 1)
		},
	} {
		s := snapshot()
		change(&s)
		if _, err := BuildContext(s, Request{FrameID: "a", Purpose: "raw"}); err == nil {
			t.Fatal("accepted mismatched runtime/format pair")
		}
	}
}

func checkProjectedSVG(t *testing.T, svg *ProjectedNode) {
	t.Helper()
	gradient := svg.Children[0].Children[0]
	if gradient.Tag != "linearGradient" || gradient.Namespace != "svg" {
		t.Fatal("SVG source case lost")
	}
	attributes := map[string]string{}
	for _, a := range gradient.Attributes {
		attributes[a.Name] = a.Value
	}
	if !reflect.DeepEqual(attributes, map[string]string{"id": "ink", "gradientUnits": "userSpaceOnUse"}) {
		t.Fatalf("SVG attributes: %v", attributes)
	}
	use := svg.Children[1]
	if use.Attributes[0].Namespace != "xlink" || use.Attributes[0].Name != "xlink:href" {
		t.Fatal("SVG attribute namespace lost")
	}
}

func TestProjectionResolvesRootSavedPropsWithoutInventingState(t *testing.T) {
	context := projectionContext(t, `<script type="application/x-pictor-props+json">{"schema":{"title":{"type":"string","maxLength":80,"default":"default"},"detail":{"type":"string","maxLength":80,"default":"fallback"}},"values":{"title":"saved"}}</script><p data-pid="p">{{props.title}}/{{props.detail}}/{{state.absent}}</p>`)
	nodes, err := Project(context, []InstanceData{{File: context.FrameFile}})
	if err != nil || nodes[0].Children[0].Text != "saved/fallback/" {
		t.Fatalf("root props/defaults: %#v %v", nodes, err)
	}
}
