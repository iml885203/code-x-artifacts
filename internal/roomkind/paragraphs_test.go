package roomkind_test

import (
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// paragraphs is the text of each paragraph of a doc, in order.
func paragraphs(d docfmt.Doc) []string {
	var out []string
	for _, tab := range d.Tabs {
		for _, b := range tab.Blocks {
			if b.Type != docfmt.Paragraph {
				continue
			}
			var sb strings.Builder
			for _, r := range b.Content {
				sb.WriteString(r.Text)
			}
			out = append(out, sb.String())
		}
	}
	return out
}
