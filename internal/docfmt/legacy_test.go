package docfmt

import (
	"strings"
	"testing"
)

func TestTheFallbackKeepsAnyMarkdownAsCodeBlocksWithinTheLimits(t *testing.T) {
	t.Parallel()
	md := strings.Repeat("# 標題 line\n", 60000) // about 800 KB
	d, err := fallback(md, counting())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tabs[0].Blocks) < 3 {
		t.Errorf("%d blocks, want the text in several", len(d.Tabs[0].Blocks))
	}
	var got strings.Builder
	for _, b := range d.Tabs[0].Blocks {
		got.WriteString(b.Text)
	}
	if got.String() != md {
		t.Error("the fallback lost or changed text")
	}
}

func TestLegacyMarkdownDropsWhatBlockDocsRefuseAndKeepsTheText(t *testing.T) {
	t.Parallel()
	md := "[bad](javascript:x) <!-- pictor:pending hi -->\n\n![gone](rel.png)\n\n<!-- pictor:tab T -->\n"
	d, err := FromLegacyMarkdown(md, nil, counting())
	if err != nil {
		t.Fatal(err)
	}
	s, _ := Canonical(d)
	if Validate(s) != nil || strings.Contains(s, "javascript") || strings.Contains(s, "pending") || len(d.Tabs) != 1 || TextOfDoc(d) != TextOfMarkdown(md) {
		t.Errorf("%s", s)
	}
}
