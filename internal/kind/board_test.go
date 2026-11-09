package kind

import (
	"regexp"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

func TestTheBoardGuidesExamplesAreBoardsAndStoreWithTheirLabelsOnTheirShapes(t *testing.T) {
	t.Parallel()
	k, err := Lookup("board")
	if err != nil {
		t.Fatal(err)
	}
	examples := regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(k.Guide(), -1)
	if len(examples) != 2 {
		t.Fatalf("the guide has %d json examples, want the skeleton and the full board", len(examples))
	}
	var stored string
	for _, example := range examples {
		var err error
		if stored, err = k.(Composer).Compose(CurrentFormat(k), example[1], nil, ""); err != nil {
			t.Fatalf("a guide example is refused: %v\n%s", err, example[1])
		}
		if err := k.Validate(stored, CurrentFormat(k)); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := boardfmt.Parse(stored)
	for _, el := range b.Elements {
		if el.ID() == "start-label" && (el["x"] != 35.0 || el["y"] != 22.5) {
			t.Errorf("the label is at %v,%v, not centred on its shape", el["x"], el["y"])
		}
	}
}

func TestABoardIsStoredInOneFormWhateverOrderItWasWrittenIn(t *testing.T) {
	t.Parallel()
	k := Board{}
	a, err := k.Compose("1", `{"version":1,"elements":[{"id":"b","type":"ellipse","index":"a1"},{"id":"a","type":"rectangle","index":"a0","version":3}]}`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := k.Compose("1", `{"elements":[{"index":"a0","type":"rectangle","id":"a"},{"type":"ellipse","id":"b","index":"a1"},{"id":"c","type":"line","isDeleted":true}],"version":1}`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("one board stored two ways:\n%s\n%s", a, b)
	}
	if written, _ := k.Written(a, "1"); written != a {
		t.Error("read does not give back what is stored")
	}
	if _, err := k.Compose("1", "", []byte(`[{"type":"paragraph"}]`), ""); err == nil || !strings.Contains(err.Error(), "no blocks") {
		t.Errorf("blocks: %v", err)
	}
	if _, err := k.Compose("2", `{"version":1,"elements":[]}`, nil, ""); err == nil {
		t.Error("format 2 composed")
	}
}

func TestABoardPageDrawsTheBoardAndRunsNoScript(t *testing.T) {
	t.Parallel()
	page, err := Board{}.RenderPage(`{"version":1,"elements":[{"id":"r","type":"rectangle","x":0,"y":0,"width":80,"height":40},{"id":"t","type":"text","text":"<script>alert(1)</script>"}]}`, "1")
	if err != nil {
		t.Fatal(err)
	}
	if page.InlineScripts == nil || len(page.InlineScripts) != 0 {
		t.Errorf("inline scripts = %#v; want an empty list so script-src allows the bridge alone", page.InlineScripts)
	}
	if strings.Contains(page.HTML, "<script") || !strings.Contains(page.HTML, "&lt;script&gt;") || !strings.Contains(page.HTML, "<rect x=\"0\" y=\"0\" width=\"80\"") {
		t.Errorf("the page does not draw the board with its words as text:\n%s", page.HTML)
	}
	if _, err := (Board{}).Render(`{"version":1,"elements":[{"id":"e","type":"iframe"}]}`, "1"); err == nil {
		t.Error("an invalid board rendered")
	}
}

func TestABoardExportsAsTheSVGItsPageDraws(t *testing.T) {
	t.Parallel()
	content := `{"version":1,"elements":[{"id":"t","type":"text","x":0,"y":0,"width":60,"height":25,"text":"Hello"}]}`
	d, err := Board{}.Export(content, "1", "svg", "https://pictor.example")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := Board{}.Render(content, "1")
	if d.ContentType != "image/svg+xml" || d.Extension != "svg" || !strings.Contains(page, string(d.Body)) || !strings.Contains(string(d.Body), ">Hello<") {
		t.Errorf("export = %s %s %s", d.ContentType, d.Extension, d.Body)
	}
	if _, err := (Board{}).Export(content, "1", "pdf", ""); err == nil {
		t.Error("exported as pdf")
	}
}
