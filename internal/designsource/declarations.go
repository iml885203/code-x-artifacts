package designsource

// Parent spans are filled only after the whole source tree is closed. At the
// script's closing token its parent's final boundary is not yet available.
func (p *sourceParser) declarationParents() {
	byStart := map[int]int{}
	for i, d := range p.plan.Declarations {
		byStart[d.Span.StartByte] = i
	}
	var walk func([]*Node, *Node)
	walk = func(nodes []*Node, parent *Node) {
		for _, n := range nodes {
			if i, ok := byStart[n.Span.StartByte]; ok && n.Tag == "script" && parent != nil {
				p.plan.Declarations[i].ParentPID = parent.PID
				p.plan.Declarations[i].ParentSpan = parent.Span
			}
			walk(n.Children, n)
		}
	}
	walk(p.roots, nil)
	for i, parent := range p.opaqueParents {
		if parent != nil {
			p.plan.Opaque[i].ParentPID = parent.PID
			p.plan.Opaque[i].ParentSpan = parent.Span
		}
	}
}

func (p *sourceParser) opaque(span SourceSpan) {
	p.plan.Opaque = append(p.plan.Opaque, SourceBlock{Kind: "comment", Span: span, ContentSpan: span})
	var parent *Node
	if len(p.stack) > 0 {
		parent = p.stack[len(p.stack)-1]
	}
	p.opaqueParents = append(p.opaqueParents, parent)
}
