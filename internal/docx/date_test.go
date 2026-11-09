package docx_test

import (
	"os"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

func TestWordExportsFieldValuesAsTextWhereverRunsAreAllowed(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, value string }{{"z_dates", "2026-10-24"}, {"z_dropdowns", "Ready"}} {
		raw, err := os.ReadFile("../docroom/testdata/cases/" + fixture.name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		d, err := docfmt.Parse(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		p, r := packageOf(t, d)
		p.wellFormed(t)
		if count := strings.Count(r.text.String(), fixture.value); count != 9 {
			t.Fatalf("Word contains %d values for %s: %s", count, fixture.name, r.text.String())
		}
		for _, control := range []string{"w:sdt", "w:date", "w:dropDownList"} {
			if strings.Contains(string(p["word/document.xml"]), control) {
				t.Fatalf("Word contains an interactive control: %s", control)
			}
		}
	}
}
