package kind

import (
	"fmt"
	"strings"
	"testing"
)

func TestASlidesPreviewRunsNoScriptAndHoldsEverySlideForItsAddressToPick(t *testing.T) {
	t.Parallel()
	content := deck(t, sl("ONE"), sl("TWO"), sl("THREE"))
	p, err := (Slides{}).RenderPreview(content, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	if p.InlineScripts == nil || len(p.InlineScripts) != 0 {
		t.Errorf("InlineScripts = %#v: a preview declares that it has no script, so the server allows none", p.InlineScripts)
	}
	if strings.Contains(p.HTML, "<script") || strings.Contains(p.HTML, `id="bar"`) {
		t.Error("a preview carries the player or its controls")
	}
	for n, title := range []string{"ONE", "TWO", "THREE"} {
		section := fmt.Sprintf(`" id="slide-%d"`, n+1)
		if !strings.Contains(p.HTML, section) || !strings.Contains(p.HTML, title) {
			t.Errorf("slide %d is not in the preview under an id its address can name", n+1)
		}
	}
	if !strings.Contains(p.HTML, ".slide:target") {
		t.Error("the preview has no rule that shows the slide its address names")
	}
	page, err := (Slides{}).RenderPage(content, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.HTML, `" id="slide-2"`) {
		t.Error("the page and its preview number their slides differently")
	}
	if _, err := (Slides{}).RenderPreview(content, "2"); err == nil {
		t.Error("a preview of a format this build lacks was drawn")
	}
}
