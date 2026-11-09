package kind

import (
	"slices"
	"testing"
)

func TestAWebPagePrintsToPDFFromTheBrowserAndTheServerHasNoPDF(t *testing.T) {
	for _, k := range []BrowserExporter{HTML{}, React{}} {
		if got := k.BrowserFormats(); !slices.Equal(got, []string{"pdf"}) {
			t.Errorf("%s: browser formats = %v, want [pdf]", k.Name(), got)
		}
		if x, ok := k.(Exporter); ok && slices.Contains(x.Formats(), "pdf") {
			t.Errorf("%s: the server offers PDF, which would need a headless browser", k.Name())
		}
	}
}
