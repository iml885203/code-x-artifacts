package designsource

import (
	"strings"
	"testing"
)

func TestSourceDeclarationsRetainExactBlocksContentsAndDirectParents(t *testing.T) {
	logic := "\r\nclass Frame extends PictorLogic { renderVals(){return {};} }\r\n"
	props := "\r\n{\"schema\":{},\"values\":{}}\r\n"
	source := `<html><head><script type='text/x-pictor'>` + logic + `</script></head><body data-pid="body"> before <script type="application/x-pictor-props+json">` + props + `</script> after </body></html>`
	p, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Declarations) != 2 {
		t.Fatalf("declarations %#v", p.Declarations)
	}
	for i, want := range []string{logic, props} {
		assertDeclarationSpans(t, source, p.File, p.Declarations[i], want)
	}
	if p.Declarations[0].Kind != "logic" || p.Declarations[0].ParentPID != "" || source[p.Declarations[0].ParentSpan.StartByte:p.Declarations[0].ParentSpan.EndByte] != "<head><script type='text/x-pictor'>"+logic+"</script></head>" {
		t.Fatalf("head parent %#v", p.Declarations[0])
	}
	if p.Declarations[1].Kind != "props" || p.Declarations[1].ParentPID != "body" {
		t.Fatalf("body parent %#v", p.Declarations[1])
	}
	var check func([]*Node)
	check = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Tag == "script" {
				t.Fatal("declaration became render node")
			}
			check(n.Children)
		}
	}
	check(p.Nodes)
	if p.Source != source || p.Rewritten != source {
		t.Fatal("declaration metadata changed source bytes")
	}
}

func TestDeclarationAbsenceAndEmptyContentAreRepresentedWithoutScanning(t *testing.T) {
	plain, e := ParseFrame("frames/a.html", `<p data-pid="p">none</p>`)
	if e != nil || len(plain.Declarations) != 0 {
		t.Fatalf("absent declaration: %#v %v", plain.Declarations, e)
	}
	// Empty logic is invalid author syntax, but inspection metadata still records
	// the exact empty source interval and top-level parent absence.
	p, e := ParseFrame("frames/a.html", `<script type="text/x-pictor"></script>`)
	if e == nil || len(p.Declarations) != 1 {
		t.Fatalf("empty declaration inspection: %#v %v", p.Declarations, e)
	}
	d := p.Declarations[0]
	if d.ContentSpan.StartByte != d.ContentSpan.EndByte || d.ContentSpan.StartByte != len(`<script type="text/x-pictor">`) || d.ParentSpan.File != "" || d.ParentPID != "" {
		t.Fatalf("empty/parent metadata %#v", d)
	}
}

func assertDeclarationSpans(t *testing.T, source, file string, d SourceBlock, want string) {
	t.Helper()
	if d.ContentSpan.File != file || source[d.ContentSpan.StartByte:d.ContentSpan.EndByte] != want || !strings.HasPrefix(source[d.Span.StartByte:d.Span.EndByte], "<script ") {
		t.Fatalf("source bytes/spans %#v", d)
	}
	if d.ParentSpan.File != file || d.ParentSpan.StartByte >= d.Span.StartByte || d.ParentSpan.EndByte <= d.Span.EndByte {
		t.Fatalf("not complete direct parent %#v", d)
	}
}

func TestOpaqueSourceCommentsPreserveDistinctLiteralGapsAndParentIdentity(t *testing.T) {
	source := `<html><head><!--head--></head><body data-pid="body"><p data-pid="p">a<!--between-->b<!---->c</p></body></html><!--root-->`
	plan, e := ParseFrame("frames/a.html", source)
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Opaque) != 4 {
		t.Fatalf("opaque source units %#v", plan.Opaque)
	}
	want := []string{"<!--head-->", "<!--between-->", "<!---->", "<!--root-->"}
	assertOpaqueTokens(t, source, plan.Opaque, want)
	for _, i := range []int{1, 2} {
		if plan.Opaque[i].ParentPID != "p" || source[plan.Opaque[i].ParentSpan.StartByte:plan.Opaque[i].ParentSpan.EndByte] != `<p data-pid="p">a<!--between-->b<!---->c</p>` {
			t.Fatalf("literal gap boundary %#v", plan.Opaque[i])
		}
	}
	if plan.Opaque[0].ParentPID != "" || source[plan.Opaque[0].ParentSpan.StartByte:plan.Opaque[0].ParentSpan.EndByte] != "<head><!--head--></head>" {
		t.Fatal("head boundary missing")
	}
	if plan.Opaque[3].ParentSpan.File != "" || plan.Opaque[3].ParentPID != "" {
		t.Fatal("root boundary invented parent")
	}
	p := plan.Nodes[0].Children[1].Children[0]
	if len(p.Children) != 3 {
		t.Fatal("source comments reinserted into visual tree")
	}
	if plan.Source != source || plan.Rewritten != source {
		t.Fatal("opaque metadata changed source bytes")
	}
}

func assertOpaqueTokens(t *testing.T, source string, opaque []SourceBlock, want []string) {
	t.Helper()
	for i, block := range opaque {
		if block.Kind != "comment" || block.ContentSpan != block.Span || source[block.Span.StartByte:block.Span.EndByte] != want[i] {
			t.Fatalf("opaque block %#v", block)
		}
	}
}
