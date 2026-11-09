package designsource

import (
	"strings"

	"golang.org/x/net/html"
)

// templateNodes never exposes declaration scripts as renderable element nodes.
// Original declaration bytes remain in Source and their separate metadata only.
func templateNodes(nodes []*Node) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Tag == "script" {
			continue
		}
		n.Children = templateNodes(n.Children)
		out = append(out, n)
	}
	return out
}

func (p *sourceParser) rewrittenSource() string {
	var b strings.Builder
	last := 0
	for _, e := range p.edits {
		b.WriteString(p.plan.Source[last:e.at])
		b.WriteString(e.text)
		last = e.at
	}
	b.WriteString(p.plan.Source[last:])
	return b.String()
}

func (p *sourceParser) renderTemplate() {
	p.declarationParents()
	p.plan.Nodes = templateNodes(p.roots)
	if p.plan.err() != nil {
		return
	}
	// The real HTML AST owns foreign-content case/namespace rules. Source IDs
	// match authored tokens to AST nodes without serializing CSS or repairing the
	// persisted source. No browser HTML parser or copied SVG name table is needed.
	doc, e := html.ParseWithOptions(strings.NewReader(p.rewrittenSource()), html.ParseOptionEnableScripting(false))
	if e != nil {
		p.plan.problem("html", e.Error(), SourceSpan{p.plan.File, 0, len(p.plan.Source)})
		return
	}
	byPID := map[string]*html.Node{}
	var index func(*html.Node)
	index = func(n *html.Node) {
		for _, a := range n.Attr {
			if a.Key == "data-pid" {
				byPID[a.Val] = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			index(c)
		}
	}
	index(doc)
	var apply func([]*Node, string)
	apply = func(nodes []*Node, parentNS string) {
		for _, n := range nodes {
			ns := parentNS
			if n.Tag == "svg" {
				ns = "svg"
			}
			if ast := byPID[n.PID]; ast != nil {
				n.Namespace = ast.Namespace
				if ast.Namespace == "svg" {
					n.Tag = ast.Data
					canonicalAttributes(n, ast)
				}
			} else {
				n.Namespace = ns
			}
			apply(n.Children, n.Namespace)
		}
	}
	apply(p.plan.Nodes, "")
}

func canonicalAttributes(n *Node, ast *html.Node) {
	for i := range n.Attributes {
		for _, a := range ast.Attr {
			name := a.Key
			if a.Namespace != "" {
				name = a.Namespace + ":" + name
			}
			if strings.EqualFold(n.Attributes[i].Name, name) {
				n.Attributes[i].Name = name
				n.Attributes[i].Namespace = a.Namespace
				break
			}
		}
	}
}
