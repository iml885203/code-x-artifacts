// Package designsource parses version 2 design source without executing author
// code. Plans are source templates, never expanded browser DOM or persisted state.
package designsource

import (
	"fmt"
	"strings"
)

// RuntimeID and Format identify the source grammar, never an execution grant.
const (
	RuntimeID = "pictor-design/2.0.0"
	Format    = "2"
)

// ParseInput is one complete immutable source snapshot, not a partial patch.
type ParseInput struct {
	Format, Manifest string
	Files            map[string]string
}

// SourceRef identifies an authored element across repeated instances.
type SourceRef struct{ File, PID string }

// SourceSpan uses half-open UTF-8 byte offsets in the original source.
type SourceSpan struct {
	File               string
	StartByte, EndByte int
}

// Diagnostic reports a deterministic source refusal.
type Diagnostic struct {
	Code, Message string
	Span          SourceSpan
	Severity      string
}
type (
	// Segment is literal text or one own lookup path.
	Segment struct {
		Literal, Path, ValueType string
		Span                     SourceSpan
	}
	// Attribute is a scalar or whole-address template.
	Attribute struct {
		Name      string
		Namespace string
		Segments  []Segment
		Kind      string
		Style     []StyleDeclaration
	}
)

// Repeat declares keyed item identity or explicit positional slots.
type Repeat struct {
	List, As, Key string
	Positional    bool
}

// Node is a source node. Children retain inline mark elements, so literal and
// binding segments remain distinct without flattening rich text into HTML.
type Node struct {
	Kind, Tag, PID          string
	Namespace               string
	Span                    SourceSpan
	Attributes              []Attribute
	Segments                []Segment
	Children                []*Node
	Repeat                  *Repeat
	Condition, Import       string
	ImportProps, Navigation string
	Events                  map[string]string
	Hints                   map[string]string
}

// LogicSource is inert source plus AST-derived methods. SyntaxChecked includes
// class-contract validation; it is not executable readiness. Compiled fields
// remain empty until a pinned native program builder and runtime proof exist.
type LogicSource struct {
	Source, SHA256                    string
	Span                              SourceSpan
	SyntaxChecked                     bool
	Methods                           []string
	CompiledSHA256, CapabilityProfile string
	WorkerProgram                     []byte
}

// SourceBlock describes an inert source declaration, never a render element.
// Spans address exact original bytes; an empty ParentSpan means no source parent.
type SourceBlock struct {
	Kind                          string
	Span, ContentSpan, ParentSpan SourceSpan
	ParentPID                     string
}

// FramePlan retains one authored source and its declaration-free typed visual
// template. Renderers consume Nodes only, never Source/Rewritten/Logic.Source.
type FramePlan struct {
	File, Source, Rewritten string
	Nodes                   []*Node
	Logic                   *LogicSource
	Declarations            []SourceBlock
	Opaque                  []SourceBlock
	Props                   *Props
	Diagnostics             []Diagnostic
}

// BundlePlan contains one complete version-scoped source snapshot.
type BundlePlan struct {
	Format, RuntimeID, Manifest string
	Frames                      map[string]FramePlan
	Sources                     map[SourceRef]*Node
	Dependencies                map[string][]string
	Rewritten                   map[string]string
	Diagnostics                 []Diagnostic
}

// Problems is the complete ordered list of source diagnostics.
type Problems []Diagnostic

func (p Problems) Error() string {
	var lines []string
	for _, d := range p {
		lines = append(lines, fmt.Sprintf("%s:%d:%d [%s] %s", d.Span.File, d.Span.StartByte, d.Span.EndByte, d.Code, d.Message))
	}
	return strings.Join(lines, "\n")
}

func (p *FramePlan) problem(code, msg string, span SourceSpan) {
	p.Diagnostics = append(p.Diagnostics, Diagnostic{code, msg, span, "error"})
}

func (p FramePlan) err() error {
	if len(p.Diagnostics) > 0 {
		return Problems(p.Diagnostics)
	}
	return nil
}
