package designsourceedit

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

type literalEdit struct {
	block sourceBlock
	next  string
}

func literalChange(plan designsource.FramePlan, op Op) (literalEdit, Op, string, error) {
	parent := findNode(plan.Nodes, func(n *designsource.Node) bool { return n.PID != "" && n.PID == op.PID })
	if parent == nil || op.TextChild < 0 || op.Segment < 0 {
		return literalEdit{}, Op{}, "", fmt.Errorf("literal source target does not exist")
	}
	span, segments, err := textGap(plan, parent, op.TextChild)
	if err != nil {
		return literalEdit{}, Op{}, "", err
	}
	raw := plan.Source[span.StartByte:span.EndByte]
	start, end, err := literalSlot(plan, span, segments, op.Segment)
	if err != nil {
		return literalEdit{}, Op{}, "", err
	}
	old := raw[start:end]
	current := Digest(old)
	if err := conflict(op.Expected, current); err != nil {
		return literalEdit{}, Op{}, current, err
	}
	replacement := strings.NewReplacer("{", "&#123;", "}", "&#125;").Replace(html.EscapeString(op.Text))
	if op.RawText != nil {
		replacement = *op.RawText
	}
	if html.UnescapeString(replacement) != op.Text {
		return literalEdit{}, Op{}, current, fmt.Errorf("raw literal does not encode the supplied text")
	}
	if err := CheckLiteralSource(replacement); err != nil {
		return literalEdit{}, Op{}, current, err
	}

	undo := op
	undo.Expected = Digest(replacement)
	undo.Text = html.UnescapeString(old)
	undo.RawText = &old
	b := sourceBlock{span.StartByte + start, span.StartByte + end, old}
	return literalEdit{b, replacement}, undo, current, nil
}

// CheckLiteralSource accepts only text-token source bytes without template holes.
// Entity spelling may vary; it never permits raw markup or raw template holes.
func CheckLiteralSource(raw string) error {
	if strings.Contains(raw, "{{") || strings.Contains(raw, "}}") {
		return fmt.Errorf("literal cannot become a binding")
	}
	z := xhtml.NewTokenizer(strings.NewReader(raw))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			if errors.Is(z.Err(), io.EOF) {
				return nil
			}
			return z.Err()
		case xhtml.TextToken:
		case xhtml.StartTagToken, xhtml.EndTagToken, xhtml.SelfClosingTagToken, xhtml.CommentToken, xhtml.DoctypeToken:
			return fmt.Errorf("literal cannot become markup")
		}
	}
}

// Text gaps count non-text children, so deleting all text does not move its
// source target onto a sibling or binding. Empty gaps remain addressable by Undo.
func textGap(plan designsource.FramePlan, parent *designsource.Node, gap int) (designsource.SourceSpan, []designsource.Segment, error) {
	z := xhtml.NewTokenizer(strings.NewReader(plan.Source[parent.Span.StartByte:parent.Span.EndByte]))
	if z.Next() != xhtml.StartTagToken {
		return designsource.SourceSpan{}, nil, fmt.Errorf("literal owner needs an opening tag")
	}
	at := parent.Span.StartByte + len(z.Raw())
	if at >= parent.Span.EndByte {
		return designsource.SourceSpan{}, nil, fmt.Errorf("element has no source content region")
	}
	current := 0
	for _, child := range sourceChildren(plan, parent) {
		if child.node != nil && child.node.Kind == "text" {
			if current == gap {
				return child.span, child.node.Segments, nil
			}
		} else {
			if current == gap {
				return designsource.SourceSpan{File: plan.File, StartByte: at, EndByte: at}, nil, nil
			}
			current++
		}
		at = child.span.EndByte
	}
	if current != gap {
		return designsource.SourceSpan{}, nil, fmt.Errorf("literal gap does not exist")
	}
	return designsource.SourceSpan{File: plan.File, StartByte: at, EndByte: at}, nil, nil
}

// Literal slots count preceding holes, including empty runs at hole boundaries.
func literalSlot(plan designsource.FramePlan, span designsource.SourceSpan, segments []designsource.Segment, target int) (int, int, error) {
	start, holes := span.StartByte, 0
	for _, segment := range segments {
		if segment.Path == "" {
			continue
		}
		s := segment.Span
		if s.File != plan.File || s.StartByte < start || s.EndByte > span.EndByte {
			return 0, 0, fmt.Errorf("binding has no authored source span")
		}
		if holes == target {
			return start - span.StartByte, s.StartByte - span.StartByte, nil
		}
		start = s.EndByte
		holes++
	}
	if holes == target {
		return start - span.StartByte, span.EndByte - span.StartByte, nil
	}
	return 0, 0, fmt.Errorf("literal slot does not exist")
}
