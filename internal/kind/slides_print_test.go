package kind

import (
	"slices"
	"strings"
	"testing"
)

func TestADeckPrintsToPDFFromTheBrowserAndTheServerHasNoPDF(t *testing.T) {
	var _ BrowserExporter = Slides{}
	if got := (Slides{}).BrowserFormats(); !slices.Equal(got, []string{"pdf"}) {
		t.Errorf("browser formats = %v, want [pdf]", got)
	}
	if x, ok := any(Slides{}).(Exporter); ok && slices.Contains(x.Formats(), "pdf") {
		t.Error("the server offers PDF, which would need a headless browser")
	}
}

func TestAPrintedDeckIsOneSixteenByNinePagePerSlideWithNoMarginAndNoControls(t *testing.T) {
	page, err := (Slides{}).Render(deck(t, sl("One"), sl("Two")), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	_, printed, ok := strings.Cut(page, "@media print")
	if !ok {
		t.Fatal("the page has no print stylesheet")
	}
	for _, want := range []string{
		"@page { size: 1920px 1080px; margin: 0; }", // the canvas, so a page is 16:9 and the browser has no margin to write the address in
		"break-after: page",
		"print-color-adjust: exact", // the canvas background prints even with background graphics off
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("the print stylesheet lacks %q", want)
		}
	}
}
