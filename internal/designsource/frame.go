package designsource

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"golang.org/x/net/html"
)

type insertion struct {
	at   int
	text string
}
type sourceParser struct {
	plan          FramePlan
	stack         []*Node
	roots         []*Node
	edits         []insertion
	used          map[string]bool
	next          int
	seen          map[string]bool
	blockStarts   map[int]int
	opaqueParents []*Node
}

var voidTags = words("area base br col embed hr img input link meta param source track wbr")

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

// ParseFrame reads authored HTML tokens into a strict source tree. Ambiguous
// unclosed/misnested source is refused instead of silently adopting a browser's
// repaired tree with a different source identity. No source is executed.
func ParseFrame(file, source string) (FramePlan, error) {
	p := sourceParser{plan: FramePlan{File: file, Source: source, Rewritten: source}, used: map[string]bool{}, seen: map[string]bool{}, blockStarts: map[int]int{}}
	if e := designfmt.CheckFramePath(file); e != nil {
		p.plan.problem("path", e.Error(), SourceSpan{file, 0, len(source)})
		return p.plan, p.plan.err()
	}
	if len(source) > designfmt.MaxFrameBytes {
		p.plan.problem("limit", "frame exceeds 512 KiB", SourceSpan{file, 0, len(source)})
		return p.plan, p.plan.err()
	}
	p.reserveIDs(source)
	z := html.NewTokenizer(strings.NewReader(source))
	offset := 0
	for {
		tt := z.Next()
		raw := string(z.Raw())
		span := SourceSpan{file, offset, offset + len(raw)}
		offset += len(raw)
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) && len(raw) > 0 {
				p.plan.problem("html", "incomplete HTML token", span)
			}
			if !errors.Is(z.Err(), io.EOF) {
				p.plan.problem("html", z.Err().Error(), span)
			}
			break
		}
		t := z.Token()
		p.token(tt, t, raw, span)
	}
	for _, n := range p.stack {
		p.plan.problem("html", "unclosed tag "+n.Tag, n.Span)
	}
	p.renderTemplate()
	p.validateDrawing()
	p.validateEvents()
	if p.plan.err() == nil {
		p.plan.Rewritten = p.rewrittenSource()
	}
	sortDiagnostics(p.plan.Diagnostics)
	return p.plan, p.plan.err()
}

func (p *sourceParser) assignID(n *Node, a map[string]string, raw string) {
	if id, ok := a["data-pid"]; ok {
		if !pidPattern.MatchString(id) {
			p.plan.problem("pid", "invalid source PID", n.Span)
		}
		if p.seen[id] {
			p.plan.problem("pid", "duplicate source PID "+id, n.Span)
		}
		p.seen[id] = true
		n.PID = id
		return
	}
	if n.Tag == "script" || n.Tag == "style" || n.Tag == "head" || n.Tag == "html" || n.Tag == "meta" || n.Tag == "title" || n.Tag == "link" {
		return
	}

	for {
		p.next++
		id := fmt.Sprintf("p%d", p.next)
		if !p.used[id] {
			n.PID = id
			p.used[id] = true
			break
		}
	}
	at := n.Span.EndByte - 1
	if strings.HasSuffix(raw, "/>") {
		at--
	}
	p.edits = append(p.edits, insertion{at, ` data-pid="` + n.PID + `"`})
}

func (p *sourceParser) block(n *Node, contentEnd int) {
	p.plan.Declarations = append(p.plan.Declarations, SourceBlock{Kind: n.Kind, Span: n.Span, ContentSpan: SourceSpan{p.plan.File, p.blockStarts[n.Span.StartByte], contentEnd}})
	var source strings.Builder
	for _, c := range n.Children {
		for _, s := range c.Segments {
			source.WriteString(s.Literal)
		}
	}
	text := source.String()
	if n.Kind == "props" {
		if p.plan.Props != nil {
			p.plan.problem("props", "only one props block is allowed", n.Span)
			return
		}
		v, e := parseProps(text)
		if e != nil {
			p.plan.problem("props", e.Error(), n.Span)
		} else {
			p.plan.Props = &v
		}
		return
	}
	if p.plan.Logic != nil {
		p.plan.problem("logic", "only one logic block is allowed", n.Span)
		return
	}
	if len(text) > 128<<10 {
		p.plan.problem("limit", "logic source exceeds 128 KiB", n.Span)
		return
	}
	contract, e := ValidateClassicClass(text, "Frame", "PictorLogic")
	if e != nil {
		p.plan.problem("logic-contract", e.Error(), n.Span)
	}
	digest := sha256.Sum256([]byte(text))
	p.plan.Logic = &LogicSource{Source: text, SHA256: hex.EncodeToString(digest[:]), Span: n.Span, SyntaxChecked: e == nil, Methods: contract.Methods}
}

func (p *sourceParser) token(tt html.TokenType, t html.Token, raw string, span SourceSpan) {
	switch tt {
	case html.StartTagToken, html.SelfClosingTagToken:
		p.start(tt, t, raw, span)
	case html.EndTagToken:
		if len(p.stack) == 0 || p.stack[len(p.stack)-1].Tag != t.Data {
			p.plan.problem("html", "unmatched or misnested closing tag "+t.Data, span)
			return
		}
		n := p.stack[len(p.stack)-1]
		n.Span.EndByte = span.EndByte
		p.stack = p.stack[:len(p.stack)-1]
		if n.Kind == "logic" || n.Kind == "props" {
			p.block(n, span.StartByte)
		}
	case html.TextToken:
		p.text(t, span)
	case html.CommentToken:
		p.opaque(span)
	case html.ErrorToken, html.DoctypeToken:

	}
}

func (p *sourceParser) start(tt html.TokenType, t html.Token, raw string, span SourceSpan) {
	n := &Node{Kind: "element", Tag: t.Data, Span: span, Events: map[string]string{}, Hints: map[string]string{}}
	for _, key := range duplicateAttributes(raw) {
		p.plan.problem("attribute", "duplicate attribute "+key, span)
	}
	if n.Tag == "script" {
		p.blockStarts[span.StartByte] = span.EndByte
	}
	attrs := map[string]string{}
	for _, a := range t.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + key
		}
		if _, dup := attrs[key]; dup {
			p.plan.problem("attribute", "duplicate attribute "+key, span)
		}
		attrs[key] = a.Val
	}
	if tt == html.SelfClosingTagToken && !voidTags[n.Tag] && !p.inSVG(n) {
		p.plan.problem("html", "non-void HTML elements need an explicit closing tag", span)
	}
	p.assignID(n, attrs, raw)
	p.classify(n, attrs)
	if len(p.stack) == 0 {
		p.roots = append(p.roots, n)
	} else {
		parent := p.stack[len(p.stack)-1]
		parent.Children = append(parent.Children, n)
	}
	if tt != html.SelfClosingTagToken && !voidTags[n.Tag] {
		if len(p.stack) >= 128 {
			p.plan.problem("limit", "HTML nesting exceeds 128", span)
			return
		}
		p.stack = append(p.stack, n)
	}
}

func (p *sourceParser) text(t html.Token, span SourceSpan) {
	if len(p.stack) > 0 {
		parent := p.stack[len(p.stack)-1]
		n := &Node{Kind: "text", Span: span}
		if parent.Tag == "script" || parent.Tag == "style" {
			n.Segments = []Segment{{Literal: p.plan.Source[span.StartByte:span.EndByte], Span: span}}
		} else {
			s, e := sourceTextSegments(p.plan.Source[span.StartByte:span.EndByte], span)
			n.Segments = s
			if e != nil {
				code := "lookup"
				if strings.Contains(e.Error(), "binding") {
					code = "binding"
				}
				p.plan.problem(code, e.Error(), span)
			}
		}
		parent.Children = append(parent.Children, n)
	} else if strings.TrimSpace(t.Data) != "" || len(p.roots) > 0 {
		parts, e := sourceTextSegments(p.plan.Source[span.StartByte:span.EndByte], span)
		if e != nil {
			p.plan.problem("lookup", e.Error(), span)
		}
		p.roots = append(p.roots, &Node{Kind: "text", Span: span, Segments: parts})
	}
}

func (p *sourceParser) validateEvents() {
	methods := map[string]bool{}
	if p.plan.Logic != nil {
		for _, name := range p.plan.Logic.Methods {
			methods[name] = true
		}
	}
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			for _, event := range sortedKeys(n.Events) {
				if !methods[n.Events[event]] {
					p.plan.problem("event-method", "event must name a declared instance method: "+n.Events[event], n.Span)
				}
			}
			walk(n.Children)
		}
	}
	walk(p.roots)
}

func (p *sourceParser) inSVG(n *Node) bool {
	if n.Tag == "svg" {
		return true
	}
	for _, parent := range p.stack {
		if parent.Tag == "svg" {
			return true
		}
	}
	return false
}

func sourceTextSegments(raw string, span SourceSpan) ([]Segment, error) {
	parts, e := segmentsAt(raw, span)
	if e != nil {
		return nil, e
	}
	for i := range parts {
		if parts[i].Path == "" {
			literal := strings.ReplaceAll(parts[i].Literal, "\r\n", "\n")
			literal = strings.ReplaceAll(literal, "\r", "\n")
			literal = strings.ReplaceAll(literal, "\x00", "\ufffd")
			parts[i].Literal = html.UnescapeString(literal)
		}
	}
	return parts, nil
}

func (p *sourceParser) reserveIDs(source string) {
	// Reserve all supplied IDs before allocating any missing one.
	z := html.NewTokenizer(strings.NewReader(source))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			t := z.Token()
			for _, a := range t.Attr {
				if a.Key == "data-pid" {
					p.used[a.Val] = true
				}
			}
		}
	}
}
