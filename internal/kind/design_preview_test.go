package kind

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

func manifestOf(frames int) string {
	var row []string
	for i := 1; i <= frames; i++ {
		row = append(row, fmt.Sprintf(`{"id":"f%d","name":"Frame %d","file":"frames/f%d.html","w":%d,"h":600}`, i, i, i, 400+i))
	}
	return `{"version":1,"pages":[{"id":"p","name":"Page","rows":[[` + strings.Join(row, ",") + `]]}]}`
}

func TestAPreviewOfEightFramesLoadsTheFirstOneWhileTheOverviewKeepsEvery(t *testing.T) {
	content := manifestOf(8)
	full, err := (Design{}).RenderPage(content, "1")
	if err != nil {
		t.Fatal(err)
	}
	card, err := (Design{}).RenderPreview(content, "1")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(full.HTML, "<iframe"); got != 8 {
		t.Fatalf("the overview has %d frames, want all 8", got)
	}
	if got := strings.Count(card.HTML, "<iframe"); got != 1 {
		t.Fatalf("the preview has %d frames, want 1", got)
	}
	if !strings.Contains(card.HTML, `src="frames/f1.html?preview"`) || !strings.Contains(card.HTML, "aspect-ratio: 401 / 600") {
		t.Errorf("the preview is not the first frame at its own proportions: %s", card.HTML)
	}
	if strings.Contains(card.HTML, "<script") || !strings.Contains(card.Titled("T").HTML, "<title>T</title>") {
		t.Errorf("a preview page runs no script and takes the artifact's title: %s", card.HTML)
	}
}

func TestAPreviewTakesTheFirstFrameOfTheFirstPageThatHasOne(t *testing.T) {
	m := designfmt.Manifest{Pages: []designfmt.Page{
		{ID: "a", Name: "Empty"},
		{ID: "b", Name: "Empty row", Rows: [][]designfmt.Frame{{}}},
		{ID: "c", Name: "Real", Rows: [][]designfmt.Frame{{{ID: "x", Name: "X", File: "frames/x.html", W: 300, H: 200}, {ID: "y", Name: "Y", File: "frames/y.html", W: 300, H: 200}}}},
	}}
	got := previewPage(m).HTML
	if strings.Count(got, "<iframe") != 1 || !strings.Contains(got, `src="frames/x.html?preview"`) {
		t.Fatalf("want only frames/x.html: %s", got)
	}
}

func TestADesignWithNoFrameHasAnEmptyPreviewNotAnError(t *testing.T) {
	got := previewPage(designfmt.Manifest{Pages: []designfmt.Page{{ID: "a", Name: "A"}}}).HTML
	if strings.Contains(got, "<iframe") || !strings.Contains(got, "<body>") {
		t.Fatalf("an empty design draws an empty page: %s", got)
	}
}

func TestAPreviewRefusesWhatTheOverviewRefuses(t *testing.T) {
	if _, err := (Design{}).RenderPreview(manifestOf(1), "9"); err == nil {
		t.Error("an unknown format was drawn")
	}
	if _, err := (Design{}).RenderPreview(`{"version":1,"pages":[]}`, "1"); err == nil {
		t.Error("a design that does not parse was drawn")
	}
}
