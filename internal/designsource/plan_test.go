package designsource

import (
	"strings"
	"testing"
)

func TestSourceIsPreservedAndMissingIDsAreAssignedOnce(t *testing.T) {
	source := "<!DOCTYPE html>\n<div data-pid='keep' style='color: red'>Hi <b>{{props.name}}</b></div><!--keep-->"
	p, err := ParseFrame("frames/a.html", source)
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != source || !strings.Contains(p.Rewritten, "data-pid='keep' style='color: red'") {
		t.Fatalf("source changed: %#v", p)
	}
	again, err := ParseFrame(p.File, p.Rewritten)
	if err != nil || again.Rewritten != p.Rewritten {
		t.Fatalf("not stable: %v", err)
	}
	if len(p.Nodes) != 1 || p.Nodes[0].Children[1].Children[0].Segments[0].Path != "props.name" {
		t.Fatalf("missing rich text binding: %#v", p.Nodes)
	}
}

func TestInvalidSourceIsRejectedWithByteSpans(t *testing.T) {
	cases := []struct{ source, code string }{
		{`<p>{{a.constructor}}</p>`, "lookup"},
		{`<p>{{a[0]}}</p>`, "lookup"},
		{`<p>{{a()}}</p>`, "lookup"},
		{`<p>{{a + b}}</p>`, "lookup"},
		{`<p>{{a</p>`, "binding"},
		{`<div data-pid="x"><b data-pid="x"></b></div>`, "pid"},
		{`<div data-pid="bad id"></div>`, "pid"},
		{`<p ONCLICK="evil()"></p>`, "drawing"},
		{`<p data-pictor-x="x"></p>`, "reserved"},
		{`<p-unknown></p-unknown>`, "directive"},
		{`<p-for list="{{items}}" as="props" key-mode="position"></p-for>`, "local"},
		{`<p-for list="{{items}}" as="item"></p-for>`, "repeat"},
		{`<p-for list="{{items}}" as="item" key="{{item.id}}" key-mode="position"></p-for>`, "repeat"},
		{`<p-if value="{{items.length > 0}}"></p-if>`, "lookup"},
		{`<button p-on:click="{{go()}}"></button>`, "event"},
		{`<button p-on:load="{{go}}"></button>`, "event"},
		{`<script>alert(1)</script>`, "script"},
		{`<p-import src="../secret.html"></p-import>`, "import"},
		{`<img src="prefix{{image}}">`, "binding"},
		{`<div id="{{id}}"></div>`, "binding"},
		{`<div><p></div>`, "html"},
	}
	for _, c := range cases {
		t.Run(c.source, func(t *testing.T) {
			p, err := ParseFrame("frames/a.html", c.source)
			if err == nil {
				t.Fatal("accepted invalid source")
			}
			for _, d := range p.Diagnostics {
				if d.Code == c.code && d.Span.File == p.File && d.Span.EndByte > d.Span.StartByte {
					return
				}
			}
			t.Fatalf("missing %s span: %#v", c.code, p.Diagnostics)
		})
	}
}

func TestDirectivesHaveTypedStructureAndPreserveHints(t *testing.T) {
	p, err := ParseFrame("frames/a.html", `<script type="text/x-pictor">class Frame extends PictorLogic { choose() {} }</script><p-for list="{{items}}" as="item" key="{{item.id}}" hint-count="3"><p-if value="{{item.visible}}"><button p-on:click="{{choose}}">{{item.name}}</button></p-if></p-for>`)
	if err != nil {
		t.Fatal(err)
	}
	n := p.Nodes[0]
	if n.Repeat.List != "items" || n.Repeat.Key != "item.id" || n.Hints["hint-count"] != "3" || n.Children[0].Condition != "item.visible" || n.Children[0].Children[0].Events["click"] != "choose" {
		t.Fatalf("bad directive: %#v", n)
	}
}

func TestSourceBoundariesRejectAmbiguousIdentityAndKeepUnicodeByteSpans(t *testing.T) {
	source := `<p data-pid="p1">你好</p><b data-pid="new">{{constructor}}</b>`
	p, e := ParseFrame("frames/a.html", source)
	if e == nil {
		t.Fatal("accepted prototype lookup")
	}
	for _, d := range p.Diagnostics {
		if d.Code == "lookup" {
			if source[d.Span.StartByte:d.Span.EndByte] != "{{constructor}}" {
				t.Fatalf("not UTF-8 byte offsets: %#v", d)
			}
			break
		}
	}
	for _, source := range []string{`<script type="text/x-pictor"/>`, `<div/>`, `<div`, `<p title="unfinished`, `<p data-pid="x" data-pid="y"></p>`, `<p-for list="{{items}}" as="item" key-mode="position"><p-for list="{{item.children}}" as="item" key-mode="position"></p-for></p-for>`, strings.Repeat("<div>", 129) + strings.Repeat("</div>", 129), strings.Repeat("x", (512<<10)+1)} {
		if _, e := ParseFrame("frames/a.html", source); e == nil {
			t.Fatalf("accepted invalid source %.80s", source)
		}
	}
	good, e := ParseFrame("frames/a.html", `<div>你好</div><p data-pid="p1">Hello</p>`)
	if e != nil || good.Nodes[0].PID == "p1" || good.Nodes[1].PID != "p1" {
		t.Fatalf("reservation: %#v %v", good, e)
	}
}

func TestTextSegmentsAddressOriginalEntityAndBindingBytes(t *testing.T) {
	source := `<p data-pid="p">你好 &amp; {{props.name}} &#123;&#123;literal&#125;&#125;</p>`
	p, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	parts := p.Nodes[0].Children[0].Segments
	if len(parts) != 3 || parts[0].Literal != "你好 & " || parts[1].Path != "props.name" || parts[2].Literal != " {{literal}}" {
		t.Fatalf("segments %#v", parts)
	}
	expected := []string{"你好 &amp; ", "{{props.name}}", " &#123;&#123;literal&#125;&#125;"}
	for i, segment := range parts {
		if segment.Span.File != "frames/a.html" || source[segment.Span.StartByte:segment.Span.EndByte] != expected[i] {
			t.Fatalf("original segment span %d: %#v", i, segment)
		}
	}
}

func TestWhitespaceBetweenRootInlineElementsIsSourceText(t *testing.T) {
	source := `<b data-pid="b">A</b> <i data-pid="i">B</i>`
	plan, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Nodes) != 3 || plan.Nodes[1].Kind != "text" || plan.Nodes[1].Segments[0].Literal != " " || plan.Rewritten != source {
		t.Fatalf("lost inline root spacing: %#v", plan.Nodes)
	}
}

func TestInertDeclarationsNeverBecomeRendererElementNodes(t *testing.T) {
	source := `<html><head><script type="text/x-pictor">class Frame extends PictorLogic { pick() {} }</script></head><body><script type="application/x-pictor-props+json">{"schema":{},"values":{}}</script><button p-on:click="{{pick}}">Choose</button></body></html>`
	plan, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	if plan.Logic == nil || plan.Props == nil || plan.Source != source {
		t.Fatal("lost declarations or source")
	}
	var check func([]*Node)
	check = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Tag == "script" || n.Kind == "logic" || n.Kind == "props" {
				t.Fatalf("declaration leaked into render template: %#v", n)
			}
			check(n.Children)
		}
	}
	check(plan.Nodes)
}

func TestSVGNamespaceAndCaseAreResolvedByTheServerAST(t *testing.T) {
	source := `<svg data-pid="svg" viewBox="0 0 10 10"><defs data-pid="defs"><linearGradient data-pid="gradient" id="ink" gradientUnits="userSpaceOnUse"><stop data-pid="stop" offset="0" stop-color="red"></stop></linearGradient></defs><use data-pid="use" xlink:href="#ink"></use><path data-pid="pen" d="M0 0 L10 10" stroke="red"></path></svg>`
	plan, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	root := plan.Nodes[0]
	gradient := root.Children[0].Children[0]
	use := root.Children[1]
	if root.Namespace != "svg" || root.Attributes[0].Name != "viewBox" || gradient.Tag != "linearGradient" || gradient.Namespace != "svg" || gradient.Attributes[0].Name != "gradientUnits" || use.Attributes[0].Namespace != "xlink" || use.Attributes[0].Name != "xlink:href" {
		t.Fatalf("SVG case/namespace lost: %#v %#v %#v", root, gradient, use)
	}
	if plan.Rewritten != source {
		t.Fatal("SVG/pen bytes changed")
	}
}
