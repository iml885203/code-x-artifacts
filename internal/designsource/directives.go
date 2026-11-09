package designsource

import (
	"fmt"
	"path"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"golang.org/x/net/html"
)

var (
	events    = words("click input change submit keydown keyup pointerdown pointerup focus blur")
	addresses = words("href src srcset poster action formaction background data cite longdesc xlink:href usemap ping archive codebase manifest")
)

func (p *sourceParser) classify(n *Node, a map[string]string) {
	switch n.Tag {
	case "script":
		p.script(n, a)
		return
	case "p-for":
		p.repeat(n, a)
	case "p-if":
		n.Kind = "condition"
		n.Condition = p.path(n, a["value"])
	case "p-import":
		n.Kind = "import"
		n.Import = a["src"]
		if props, ok := a["props"]; ok {
			n.ImportProps = p.path(n, props)
		}
		if e := designfmt.CheckFramePath(n.Import); e != nil {
			p.plan.problem("import", e.Error(), n.Span)
		}
	default:
		if strings.HasPrefix(n.Tag, "p-") {
			p.plan.problem("directive", "unknown CodeXArtifacts directive "+n.Tag, n.Span)
		}
	}
	for _, k := range sortedKeys(a) {
		p.attribute(n, k, a[k])
	}
}

func (p *sourceParser) path(n *Node, value string) string {
	path, e := wholePath(value)
	if e != nil {
		p.plan.problem("lookup", e.Error(), n.Span)
	}
	return path
}

func (p *sourceParser) script(n *Node, a map[string]string) {
	switch a["type"] {
	case "text/x-pictor":
		n.Kind = "logic"
	case "application/x-pictor-props+json":
		n.Kind = "props"
	default:
		p.plan.problem("script", "only inert CodeXArtifacts logic and props scripts are allowed", n.Span)
	}
	for _, k := range sortedKeys(a) {
		if k != "type" {
			p.plan.problem("script", "script attribute is not allowed: "+k, n.Span)
		}
	}
}

func (p *sourceParser) repeat(n *Node, a map[string]string) {
	n.Kind = "repeat"
	r := &Repeat{As: a["as"], Positional: a["key-mode"] == "position"}
	n.Repeat = r
	r.List = p.path(n, a["list"])
	if !safeName(r.As) || r.As == "props" || r.As == "state" {
		p.plan.problem("local", "repeat local must be an unreserved identifier", n.Span)
	}
	depth := 1
	for _, parent := range p.stack {
		if parent.Repeat != nil {
			depth++
			if parent.Repeat.As == r.As {
				p.plan.problem("local", "repeat local shadows an enclosing local", n.Span)
			}
		}
	}
	if depth > 8 {
		p.plan.problem("limit", "repeat nesting exceeds 8", n.Span)
	}
	_, hasKey := a["key"]
	_, hasMode := a["key-mode"]
	if hasKey == hasMode || (hasMode && !r.Positional) {
		p.plan.problem("repeat", "repeat requires exactly one key binding or key-mode=position", n.Span)
	}
	if hasKey {
		r.Key = p.path(n, a["key"])
	}
}

var directiveAttrs = map[string]map[string]bool{"repeat": words("list as key key-mode"), "condition": words("value"), "import": words("src props")}

func (p *sourceParser) attribute(n *Node, k, v string) {
	switch {
	case k == "data-pid":
		return
	case strings.HasPrefix(k, "data-pictor-"):
		p.plan.problem("reserved", "reserved implementation attribute "+k, n.Span)
	case strings.HasPrefix(k, "hint-"):
		n.Hints[k] = v
	case strings.HasPrefix(k, "p-on:"):
		p.event(n, k, v)
	case strings.HasPrefix(k, "p-"):
		p.plan.problem("directive", "unknown directive attribute "+k, n.Span)
	case directiveAttrs[n.Kind] != nil:
		if !directiveAttrs[n.Kind][k] {
			p.plan.problem("directive", "unknown directive attribute "+k, n.Span)
		}
	default:
		p.ordinaryAttribute(n, k, v)
	}
}

func (p *sourceParser) event(n *Node, k, v string) {
	event := strings.TrimPrefix(k, "p-on:")
	method, e := wholePath(v)
	if !events[event] || e != nil || !safeName(method) {
		p.plan.problem("event", "event requires a supported literal event and one method-name binding", n.Span)
		return
	}
	n.Events[event] = method
}

func (p *sourceParser) ordinaryAttribute(n *Node, k, v string) {
	if n.Tag == "a" && k == "href" && strings.HasSuffix(v, ".html") && !strings.Contains(v, "{{") {
		target := path.Join(path.Dir(p.plan.File), v)
		if strings.Contains(v, "..") || strings.ContainsAny(v, ":\\?#") || designfmt.CheckFramePath(target) != nil {
			p.plan.problem("navigation", "prototype link must name a frame beside this frame", n.Span)
		} else {
			n.Navigation = target
		}
		n.Attributes = append(n.Attributes, Attribute{Name: k, Segments: []Segment{{Literal: v}}, Kind: "navigation"})
		return
	}
	parts, e := segments(v)
	if e != nil {
		p.plan.problem("lookup", e.Error(), n.Span)
		return
	}
	kind := "scalar"
	if hasBinding(parts) {
		kind = p.bindingKind(n, k, v)
	}
	attribute := Attribute{Name: k, Segments: parts, Kind: kind}
	if kind == "style" {
		var e error
		attribute.Style, e = parseStyle(v)
		if e != nil {
			p.plan.problem("binding", e.Error(), n.Span)
		}
	}
	n.Attributes = append(n.Attributes, attribute)
}

func hasBinding(parts []Segment) bool {
	for _, s := range parts {
		if s.Path != "" {
			return true
		}
	}
	return false
}

func (p *sourceParser) bindingKind(n *Node, k, v string) string {
	switch {
	case addresses[k]:
		if _, e := wholePath(v); e != nil {
			p.plan.problem("binding", "URL binding must occupy the entire attribute", n.Span)
		}
		return "url"
	case k == "style":
		return "style"
	case k == "class" || k == "title" || k == "alt" || strings.HasPrefix(k, "aria-") || strings.HasPrefix(k, "data-"):
		return "scalar"
	default:
		p.plan.problem("binding", "binding is not allowed in structural attribute "+k, n.Span)
		return "scalar"
	}
}

// ResolveAttribute validates after substitution through the existing static
// drawing rules. The result is an attribute value, never raw HTML or CSS text.
func ResolveAttribute(tag string, a Attribute, data map[string]any) (string, error) {
	if a.Kind == "style" {
		return ResolveStyle(a.Style, data)
	}
	value, e := ResolveText(a.Segments, data)
	if e != nil {
		return "", e
	}
	if issues := designfmt.CheckAttribute(tag, a.Name, value); len(issues) > 0 {
		return "", fmt.Errorf("%s", strings.Join(issues, "; "))
	}
	return value, nil
}

func (p *sourceParser) validateDrawing() {
	for _, n := range p.roots {
		if x := drawingNode(n); x != nil {
			var b strings.Builder
			if e := html.Render(&b, x); e != nil {
				p.plan.problem("html", e.Error(), n.Span)
				continue
			}
			for _, msg := range designfmt.CheckFragment(b.String()) {
				p.plan.problem("drawing", msg, n.Span)
			}
		}
	}
}

func drawingNode(n *Node) *html.Node {
	if n.Kind == "logic" || n.Kind == "props" {
		return nil
	}
	if n.Kind == "text" {
		var b strings.Builder
		for _, s := range n.Segments {
			b.WriteString(s.Literal)
		}
		return &html.Node{Type: html.TextNode, Data: b.String()}
	}
	tag := n.Tag
	if n.Kind != "element" {
		tag = "div"
	}
	x := &html.Node{Type: html.ElementNode, Data: tag, Namespace: n.Namespace}
	for _, a := range n.Attributes {
		value := ""
		dynamic := false
		for _, s := range a.Segments {
			value += s.Literal
			dynamic = dynamic || s.Path != ""
		}
		if a.Kind == "navigation" {
			value = "#"
		}
		if dynamic && a.Kind == "url" {
			value = ""
		}
		x.Attr = append(x.Attr, html.Attribute{Key: a.Name, Val: value})
	}
	for _, c := range n.Children {
		if child := drawingNode(c); child != nil {
			x.AppendChild(child)
		}
	}
	return x
}
