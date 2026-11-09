package kind

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// everyBlock holds one of each block, the marks Markdown cannot hold, and an
// uploaded picture, the way the editor will leave a real doc.
const everyBlock = `{"tabs":[{"id":"t1","title":"Document","blocks":[
{"id":"b1","type":"heading","level":1,"content":[{"text":"Plan 季度"}]},
{"id":"b2","type":"paragraph","content":[{"text":"Some "},{"text":"bold","marks":["bold"]},{"text":", "},{"text":"italic","marks":["italic"]},{"text":", "},{"text":"old","marks":["strike"]},{"text":", "},{"text":"code","marks":["code"]},{"text":" and a "},{"text":"link","marks":["link"],"href":"https://example.com/a"},{"text":", "},{"text":"underlined","marks":["underline"]},{"text":" and "},{"text":"marked","marks":["highlight"]},{"text":"."}]},
{"id":"b3","type":"bullet_list","items":[{"content":[{"text":"one"}],"items":[{"content":[{"text":"nested"}]}]},{"content":[{"text":"two"}]}]},
{"id":"b4","type":"numbered_list","start":3,"items":[{"content":[{"text":"three"}]},{"content":[{"text":"four"}]}]},
{"id":"b5","type":"task_list","items":[{"content":[{"text":"done"}],"checked":true},{"content":[{"text":"todo"}],"checked":false}]},
{"id":"b6","type":"quote","blocks":[{"id":"b6a","type":"paragraph","content":[{"text":"quoted"}]}]},
{"id":"b7","type":"code","lang":"go","text":"fmt.Println(1)\n"},
{"id":"b8","type":"rule"},
{"id":"b9","type":"table","header":true,"rows":[{"cells":[{"align":"left","content":[{"text":"Name"}]},{"align":"right","content":[{"text":"Qty"}]}]},{"cells":[{"align":"left","content":[{"text":"a"}]},{"align":"right","content":[{"text":"1"}]}]}]},
{"id":"b10","type":"image","src":"/files/abc123/chart.png","alt":"Chart"},
{"id":"b11","type":"pending","intent":"Goals: the three outcomes"}]}]}`

// withoutWhatMarkdownCannotHold is the doc as it should come back from
// Markdown: no underline or highlight, and a file of this server by its full
// address, which is what the export wrote so that it opens anywhere.
func withoutWhatMarkdownCannotHold(d docfmt.Doc, origin string) docfmt.Doc {
	runs := func(in []docfmt.Inline) []docfmt.Inline {
		var out []docfmt.Inline
		for _, r := range in {
			r.Marks = slices.DeleteFunc(slices.Clone(r.Marks), func(m string) bool { return m == docfmt.MarkUnderline || m == docfmt.MarkHighlight })
			if len(r.Marks) == 0 {
				r.Marks = nil
			}
			if n := len(out); n > 0 && reflect.DeepEqual(out[n-1].Marks, r.Marks) && out[n-1].Href == r.Href {
				out[n-1].Text += r.Text
				continue
			}
			out = append(out, r)
		}
		return out
	}
	var items func(in []docfmt.Item) []docfmt.Item
	items = func(in []docfmt.Item) []docfmt.Item {
		for i := range in {
			in[i].Content = runs(in[i].Content)
			in[i].Items = items(in[i].Items)
		}
		return in
	}
	var blocks func(in []docfmt.Block) []docfmt.Block
	blocks = func(in []docfmt.Block) []docfmt.Block {
		for i := range in {
			b := &in[i]
			b.Content = runs(b.Content)
			b.Items = items(b.Items)
			b.Blocks = blocks(b.Blocks)
			for r := range b.Rows {
				for c := range b.Rows[r].Cells {
					b.Rows[r].Cells[c].Content = runs(b.Rows[r].Cells[c].Content)
				}
			}
			if b.Type == docfmt.Image && strings.HasPrefix(b.Src, "/files/") {
				b.Src = origin + b.Src
			}
		}
		return in
	}
	for i := range d.Tabs {
		d.Tabs[i].Blocks = blocks(d.Tabs[i].Blocks)
	}
	return d
}

func TestADocWithEveryBlockExportsMarkdownThatRepublishesToTheSameBlocksExceptTheNotedDegradations(t *testing.T) {
	const origin = "https://pictor.example"
	d := Doc{}
	dl, err := d.Export(everyBlock, "2", "md", origin)
	if err != nil {
		t.Fatal(err)
	}
	md := string(dl.Body)
	for _, want := range []string{
		"![Chart](" + origin + "/files/abc123/chart.png)", // a picture is a working address, not a file name
		"| Name | Qty |", "| :-- | --: |", // the table keeps its header and alignment
		"- [x] done", "- [ ] todo", "3. three", "[link](https://example.com/a)", "<!-- pictor:pending Goals: the three outcomes -->",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the Markdown lacks %q:\n%s", want, md)
		}
	}
	if !strings.HasSuffix(md, "<!-- pictor: Markdown cannot hold underline, highlight; they are plain text or left out here -->\n") {
		t.Errorf("what degraded is not named at the end:\n%s", md)
	}

	again, err := d.Compose("2", md, nil, everyBlock)
	if err != nil {
		t.Fatalf("the exported Markdown does not publish: %v", err)
	}
	got, err := docfmt.Parse(again)
	if err != nil {
		t.Fatal(err)
	}
	original, err := docfmt.Parse(everyBlock)
	if err != nil {
		t.Fatal(err)
	}
	want := withoutWhatMarkdownCannotHold(original, origin)
	if !reflect.DeepEqual(got, want) {
		g, _ := docfmt.Canonical(got)
		w, _ := docfmt.Canonical(want)
		t.Errorf("republishing the export changed the blocks\n got %s\nwant %s", g, w)
	}
}

func TestExportingADocNeverChangesIt(t *testing.T) {
	before := everyBlock
	if _, err := (Doc{}).Export(everyBlock, "2", "md", "https://pictor.example"); err != nil {
		t.Fatal(err)
	}
	if everyBlock != before {
		t.Error("export changed the stored content")
	}
	if page, err := (Doc{}).Render(everyBlock, "2"); err != nil || !strings.Contains(page, `src="/files/abc123/chart.png"`) {
		t.Errorf("the page still shows the file by its own address: %v", err)
	}
}

func TestAnExportOnlyMakesAFileAddressFullWhenTheServerIsReachedOverHTTPS(t *testing.T) {
	// A full http: address is not an image address a doc accepts, so writing one
	// would make the export fail to publish again.
	dl, err := (Doc{}).Export(everyBlock, "2", "md", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dl.Body), "![Chart](/files/abc123/chart.png)") {
		t.Errorf("the picture address changed:\n%s", dl.Body)
	}
}

func TestADocPrintsToPDFFromTheBrowserAndTheServerHasNoPDF(t *testing.T) {
	var _ BrowserExporter = Doc{}
	if got := (Doc{}).BrowserFormats(); !slices.Equal(got, []string{"pdf"}) {
		t.Errorf("browser formats = %v, want [pdf]", got)
	}
	if slices.Contains((Doc{}).Formats(), "pdf") {
		t.Error("the server offers PDF, which would need a headless browser")
	}
	if _, err := (Doc{}).Export(everyBlock, "2", "pdf", ""); err == nil {
		t.Error("the export route made a PDF")
	}
}

func TestADocPageLeavesNoMarginForTheBrowserToWriteTheAddressInAndLoadsTheChineseFont(t *testing.T) {
	page, err := (Doc{}).Render(everyBlock, "2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`@import url("/static/fonts/noto-sans-tc.css");`, // Traditional Chinese the same on every machine
		"@page { size: A4; margin: 0; }",                 // no margin box for the browser to put the address in
		"@media print", "break-inside: avoid", "table-header-group",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}
