package designrender

import (
	"fmt"
	"maps"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/designsource"
)

func (p *projector) nodes(nodes []*designsource.Node, scope map[string]any, path []InstanceSegment, parent designsource.SourceRef) ([]*ProjectedNode, error) {
	var out []*ProjectedNode
	for _, n := range nodes {
		if err := p.step(); err != nil {
			return nil, p.problem(err, n.Span)
		}
		ref := designsource.SourceRef{File: n.Span.File, PID: n.PID}
		if n.PID == "" {
			ref = parent
		}
		projected, err := p.node(n, scope, path, ref)
		if err != nil {
			return nil, p.problem(err, n.Span)
		}
		out = append(out, projected...)
	}
	return out, nil
}

func (p *projector) node(n *designsource.Node, scope map[string]any, path []InstanceSegment, ref designsource.SourceRef) ([]*ProjectedNode, error) {
	switch n.Kind {
	case "repeat":
		return p.repeat(n, scope, path, ref)
	case "condition":
		value, err := designsource.Lookup(scope, n.Condition)
		if err != nil {
			return nil, err
		}
		show, err := designsource.Truth(value)
		if err != nil {
			return nil, err
		}
		if !show {
			return nil, nil
		}
		return p.nodes(n.Children, scope, path, ref)
	case "import":
		return p.importFrame(n, scope, path, ref)
	case "element", "text":
		node, err := p.ordinary(n, scope, path, ref)
		if err != nil {
			return nil, err
		}
		return []*ProjectedNode{node}, nil
	default:
		return nil, fmt.Errorf("unsupported source node kind %q", n.Kind)
	}
}

func (p *projector) ordinary(n *designsource.Node, scope map[string]any, path []InstanceSegment, ref designsource.SourceRef) (*ProjectedNode, error) {
	out := &ProjectedNode{Kind: n.Kind, Tag: n.Tag, Namespace: n.Namespace, Source: ref, Span: n.Span, Path: clonePath(path), Events: maps.Clone(n.Events), Navigation: n.Navigation}
	if n.Kind == "text" {
		text, err := designsource.ResolveText(n.Segments, scope)
		if err != nil {
			return nil, err
		}
		if err := p.text(text); err != nil {
			return nil, err
		}
		out.Text = text
		return out, nil
	}
	for _, a := range n.Attributes {
		value, err := resolveAttribute(n, a, scope)
		if err != nil {
			return nil, err
		}
		if err := p.text(value); err != nil {
			return nil, err
		}
		out.Attributes = append(out.Attributes, ProjectedAttribute{Name: a.Name, Namespace: a.Namespace, Value: value})
	}
	children, err := p.nodes(n.Children, scope, path, ref)
	if err != nil {
		return nil, err
	}
	out.Children = children
	return out, nil
}

func resolveAttribute(n *designsource.Node, a designsource.Attribute, scope map[string]any) (string, error) {
	if a.Kind == "navigation" {
		// The source parser already resolved this literal against the same manifest.
		// It remains a passive source link, not a navigation intent or granted URL.
		return designsource.ResolveText(a.Segments, scope)
	}
	return designsource.ResolveAttribute(n.Tag, a, scope)
}

func (p *projector) repeat(n *designsource.Node, scope map[string]any, path []InstanceSegment, ref designsource.SourceRef) ([]*ProjectedNode, error) {
	value, err := designsource.Lookup(scope, n.Repeat.List)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("repeat list must be an array")
	}
	keys, err := designsource.RepeatKeys(*n.Repeat, items, scope)
	if err != nil {
		return nil, err
	}
	var out []*ProjectedNode
	for i, item := range items {
		if err := p.step(); err != nil {
			return nil, err
		}
		local := maps.Clone(scope)
		local[n.Repeat.As] = item
		next := append(clonePath(path), InstanceSegment{Site: ref, Key: &keys[i]})
		children, err := p.nodes(n.Children, local, next, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	return out, nil
}

func (p *projector) importFrame(n *designsource.Node, scope map[string]any, path []InstanceSegment, ref designsource.SourceRef) ([]*ProjectedNode, error) {
	var overrides map[string]any
	if n.ImportProps != "" {
		value, err := designsource.Lookup(scope, n.ImportProps)
		if err != nil {
			return nil, err
		}
		var ok bool
		overrides, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("import props must be a declared plain object")
		}
	}
	next := append(clonePath(path), InstanceSegment{Site: ref})
	child, err := p.scope(n.Import, next, overrides)
	if err != nil {
		return nil, err
	}
	return p.nodes(p.plan.Frames[n.Import].Nodes, child, next, designsource.SourceRef{File: n.Import})
}

func clonePath(path []InstanceSegment) []InstanceSegment {
	out := slices.Clone(path)
	for i, s := range out {
		if s.Key != nil {
			key := *s.Key
			out[i].Key = &key
		}
	}
	return out
}
