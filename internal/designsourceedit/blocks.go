package designsourceedit

import (
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

type sourceBlock struct {
	start, end int
	text       string
}

func findNode(nodes []*designsource.Node, predicate func(*designsource.Node) bool) *designsource.Node {
	for _, n := range nodes {
		if predicate(n) {
			return n
		}
		if found := findNode(n.Children, predicate); found != nil {
			return found
		}
	}
	return nil
}

func block(plan designsource.FramePlan, kind string) (sourceBlock, error) {
	for _, declaration := range plan.Declarations {
		if declaration.Kind != kind {
			continue
		}
		span := declaration.ContentSpan
		if span.File != plan.File || span.StartByte < 0 || span.EndByte < span.StartByte || span.EndByte > len(plan.Source) {
			return sourceBlock{}, fmt.Errorf("invalid declaration content span")
		}
		return sourceBlock{span.StartByte, span.EndByte, plan.Source[span.StartByte:span.EndByte]}, nil
	}
	return sourceBlock{}, fmt.Errorf("missing %s source block", kind)
}

func splice(source string, b sourceBlock, value string) string {
	return source[:b.start] + value + source[b.end:]
}
