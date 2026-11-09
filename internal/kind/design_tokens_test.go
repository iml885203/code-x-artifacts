package kind

import (
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

func TestEveryExportedFrameCarriesSharedVariablesAndKeepsItsBindings(t *testing.T) {
	content := strings.Replace(exportManifest, `"version":1`, `"version":1,"tokens":{"--accent":"#336699","--font":"'Noto Sans', sans-serif","--space":"16px"}`, 1)
	source := `<!doctype html><html><head><style>:root{--accent:red}</style></head><body style="color:var(--accent);font-family:var(--font);padding:var(--space)">Bound</body></html>`
	frames := files.Set{"frames/hero.html": source, "frames/mobile.html": source}
	out, err := (Design{}).ExportBundle("Theme", content, designFormat, "zip", "", frames, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := unzipAll(t, out.Body)
	for name := range frames {
		got := entries[name]
		if !strings.Contains(got, `--accent:#336699;`) || !strings.Contains(got, `color:var(--accent)`) || !strings.Contains(got, `font-family:var(--font)`) || strings.Index(got, `data-pictor-tokens`) < strings.Index(got, `:root{--accent:red}`) {
			t.Fatalf("frame %s: %s", name, got)
		}
		if frames[name] != source {
			t.Fatal("export changed source")
		}
	}
	if entries["design.json"] != content {
		t.Fatal("manifest changed")
	}
}

func TestTheKitInAFrameIsInlinedAndItsFontsAreLocalInTheZip(t *testing.T) {
	source := `<!doctype html><html><head><link rel="stylesheet" href="/kit/v5/pictor.css"></head><body style="color:var(--accent)">Kit</body></html>`
	open := func(src string) (File, bool) {
		if src == "/kit/v5/pictor.css" {
			return File{Data: []byte(`@font-face{font-family:F;src:url(/static/fonts/F.woff2)}:root{--accent:red}`)}, true
		}
		return fakeOpen(src)
	}
	out, err := (Design{}).ExportBundle("Theme", exportManifest, designFormat, "zip", "", files.Set{"frames/hero.html": source}, open)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Design{}).ExportBundle("Theme", exportManifest, designFormat, "zip", "", files.Set{"frames/hero.html": source}, nil); err == nil {
		t.Fatal("a missing kit must be reported")
	}
	entries := unzipAll(t, out.Body)
	frame := entries["frames/hero.html"]
	if strings.Contains(frame, `<link`) || !strings.Contains(frame, `url(../fonts/F.woff2)`) || entries["fonts/F.woff2"] != "font" {
		t.Fatalf("kit or fonts missing: %v", entries)
	}
}
