package designsourceedit

import (
	"fmt"
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

type sourceChild struct {
	node *designsource.Node
	span designsource.SourceSpan
}

func sourceChildren(plan designsource.FramePlan, parent *designsource.Node) []sourceChild {
	out := make([]sourceChild, 0, len(parent.Children))
	for _, child := range parent.Children {
		out = append(out, sourceChild{child, child.Span})
	}
	for _, records := range [][]designsource.SourceBlock{plan.Declarations, plan.Opaque} {
		for _, record := range records {
			if record.ParentSpan == parent.Span {
				out = append(out, sourceChild{span: record.Span})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].span.StartByte < out[j].span.StartByte })
	return out
}

func segmentRange(plan designsource.FramePlan, node *designsource.Node, index int) (int, int, error) {
	span := node.Segments[index].Span
	if span.File != plan.File || span.StartByte < node.Span.StartByte || span.EndByte > node.Span.EndByte || span.EndByte < span.StartByte {
		return 0, 0, fmt.Errorf("literal has no authored source span")
	}
	return span.StartByte, span.EndByte, nil
}
