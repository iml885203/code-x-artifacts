package designroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// Node kinds.
const (
	kindElement = "el"
	kindText    = "text"
	kindComment = "comment"
)

// The markers in an element's attribute list for where its style attribute and
// its data-pid were written, so that the attributes come back in the order they
// had.
const (
	markStyle = "\x00style"
	markPID   = "\x00pid"
)

// node is one node of a frame's body, as the room holds it.
type node struct {
	id             string
	kind           string
	tag            string
	ns             string
	parent         string
	order          string
	attrs          []attr
	rest           string // the style attribute with the owned declarations as slots
	style          map[string]string
	styleOverrides map[string]string
	text           string   // text and comment nodes
	rich           []richOp // an element's formatted text: its content, when it has any (rich.go)
	hasRich        bool
	hidden         string // "1": the editor hides it, "0": the editor shows what its own markup hides, "": nothing said (hidden.go)
}

type attr struct{ Ns, Key, Val string }

// attrsJSON is the attribute list as the room keeps it, in one entry: a list is
// replaced as a whole, which is right for what the editor has no controls for.
func attrsJSON(list []attr) string {
	rows := make([][3]string, len(list))
	for i, a := range list {
		rows[i] = [3]string{a.Ns, a.Key, a.Val}
	}
	b, _ := json.Marshal(rows) // strings and arrays of strings cannot fail
	return string(b)
}

func parseAttrs(s string) ([]attr, error) {
	if s == "" {
		return nil, nil
	}
	var rows [][3]string
	if err := json.Unmarshal([]byte(s), &rows); err != nil {
		return nil, fmt.Errorf("attributes %.40q: %w", s, err)
	}
	list := make([]attr, len(rows))
	for i, r := range rows {
		list[i] = attr{r[0], r[1], r[2]}
	}
	return list, nil
}

// frameModel is a frame split into what the room holds: the shell around the
// body, written as the HTML it is, and the nodes of the body.
type frameModel struct {
	docBefore, docAfter string // document-level nodes other than the doctype and html
	htmlAttrs           []attr
	pre, post           string // the children of html before and after body, head included
	nodes               []node
	root                string
}

func renderNodes(nodes []*html.Node) string {
	var b bytes.Buffer
	for _, n := range nodes {
		_ = html.Render(&b, n) // a bytes.Buffer does not fail
	}
	return b.String()
}

func siblings(first *html.Node) []*html.Node {
	var out []*html.Node
	for n := first; n != nil; n = n.NextSibling {
		out = append(out, n)
	}
	return out
}

// modelOf splits the text of a frame into its model. The frame is first put in
// the normal form a stored frame has, so every element has an id.
func modelOf(text string) (frameModel, error) {
	normal, err := designfmt.Normalize(text)
	if err != nil {
		return frameModel{}, err
	}
	doc, err := html.ParseWithOptions(strings.NewReader(normal), html.ParseOptionEnableScripting(false))
	if err != nil {
		return frameModel{}, err
	}
	var m frameModel
	before, htmlEl, after := splitDocument(doc)
	if htmlEl == nil {
		return m, fmt.Errorf("the frame has no html element")
	}
	m.docBefore, m.docAfter = renderNodes(before), renderNodes(after)
	m.htmlAttrs = toAttrs(htmlEl.Attr)
	pre, body, post := splitHTML(htmlEl)
	if body == nil {
		return m, fmt.Errorf("the frame has no body")
	}
	m.pre, m.post = renderNodes(pre), renderNodes(post)
	c := collector{m: &m}
	m.root, err = c.element(body, "", "")
	return m, err
}

// splitDocument is the document's nodes before the html element, the element, and the nodes after it (the doctype is none of them).
func splitDocument(doc *html.Node) (before []*html.Node, htmlEl *html.Node, after []*html.Node) {
	for _, n := range siblings(doc.FirstChild) {
		switch {
		case n.Type == html.DoctypeNode:
		case n.Type == html.ElementNode && n.Data == "html" && htmlEl == nil:
			htmlEl = n
		case htmlEl == nil:
			before = append(before, n)
		default:
			after = append(after, n)
		}
	}
	return before, htmlEl, after
}

// splitHTML is the html element's children before the body (the head among them), the body, and the children after it.
func splitHTML(htmlEl *html.Node) (pre []*html.Node, body *html.Node, post []*html.Node) {
	for _, n := range siblings(htmlEl.FirstChild) {
		switch {
		case n.Type == html.ElementNode && n.Data == "body" && body == nil:
			body = n
		case body == nil:
			pre = append(pre, n)
		default:
			post = append(post, n)
		}
	}
	return pre, body, post
}

func toAttrs(list []html.Attribute) []attr {
	out := make([]attr, len(list))
	for i, a := range list {
		out[i] = attr{a.Namespace, a.Key, a.Val}
	}
	return out
}

type collector struct {
	m      *frameModel
	counts map[string]int
}

func (c *collector) synthetic(kind string) string {
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[kind]++
	return "~" + kind[:1] + strconv.Itoa(c.counts[kind])
}

// element adds an element and everything under it.
func (c *collector) element(n *html.Node, parent, order string) (string, error) {
	nd := node{kind: kindElement, tag: n.Data, ns: n.Namespace, parent: parent, order: order, style: map[string]string{}}
	for _, a := range n.Attr {
		switch {
		case a.Key == "data-pid" && a.Namespace == "":
			nd.id = a.Val
			nd.attrs = append(nd.attrs, attr{Key: markPID})
		case a.Key == "style" && a.Namespace == "":
			nd.rest, nd.style = splitStyle(a.Val)
			nd.attrs = append(nd.attrs, attr{Key: markStyle})
		default:
			nd.attrs = append(nd.attrs, attr{a.Namespace, a.Key, a.Val})
		}
	}
	if nd.id == "" {
		return "", fmt.Errorf("an element <%s> has no data-pid", n.Data)
	}
	idx := len(c.m.nodes)
	if ops, ok := richOf(n); ok && parent != "" {
		nd.rich, nd.hasRich = ops, true
		c.m.nodes = append(c.m.nodes, nd)
		return nd.id, nil
	}
	c.m.nodes = append(c.m.nodes, nd)
	kids := siblings(n.FirstChild)
	keys := startKeys(len(kids))
	for i, k := range kids {
		switch k.Type {
		case html.ElementNode:
			if _, err := c.element(k, nd.id, keys[i]); err != nil {
				return "", err
			}
		case html.TextNode:
			c.m.nodes = append(c.m.nodes, node{id: c.synthetic("text"), kind: kindText, parent: nd.id, order: keys[i], text: k.Data})
		case html.CommentNode:
			c.m.nodes = append(c.m.nodes, node{id: c.synthetic("comment"), kind: kindComment, parent: nd.id, order: keys[i], text: k.Data})
		case html.ErrorNode, html.DocumentNode, html.DoctypeNode, html.RawNode:
			// The parser makes none of these inside an element.
		}
	}
	c.m.nodes[idx] = nd
	return nd.id, nil
}

// build is the text of a frame for a model: the nodes become a tree and the tree
// is written as HTML, with the shell around it as it was.
func build(m frameModel) (string, error) {
	children := map[string][]node{}
	byID := map[string]node{}
	for _, n := range m.nodes {
		byID[n.id] = n
		children[n.parent] = append(children[n.parent], n)
	}
	for _, list := range children {
		sort.Slice(list, func(i, j int) bool {
			if list[i].order != list[j].order {
				return list[i].order < list[j].order
			}
			return list[i].id < list[j].id
		})
	}
	root, ok := byID[m.root]
	if !ok {
		return "", fmt.Errorf("the body %q is not in the room", m.root)
	}
	body, err := htmlNode(root, children)
	if err != nil {
		return "", err
	}
	htmlEl := &html.Node{Type: html.ElementNode, Data: "html", DataAtom: atom.Html, Attr: toHTMLAttrs(m.htmlAttrs)}
	for _, raw := range []string{m.pre} {
		if raw != "" {
			htmlEl.AppendChild(&html.Node{Type: html.RawNode, Data: raw})
		}
	}
	htmlEl.AppendChild(body)
	if m.post != "" {
		htmlEl.AppendChild(&html.Node{Type: html.RawNode, Data: m.post})
	}
	doc := &html.Node{Type: html.DocumentNode}
	doc.AppendChild(&html.Node{Type: html.DoctypeNode, Data: "html"})
	if m.docBefore != "" {
		doc.AppendChild(&html.Node{Type: html.RawNode, Data: m.docBefore})
	}
	doc.AppendChild(htmlEl)
	if m.docAfter != "" {
		doc.AppendChild(&html.Node{Type: html.RawNode, Data: m.docAfter})
	}
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		return "", err
	}
	return out.String(), nil
}

func toHTMLAttrs(list []attr) []html.Attribute {
	out := make([]html.Attribute, len(list))
	for i, a := range list {
		out[i] = html.Attribute{Namespace: a.Ns, Key: a.Key, Val: a.Val}
	}
	return out
}

func htmlNode(n node, children map[string][]node) (*html.Node, error) {
	switch n.kind {
	case kindText:
		return &html.Node{Type: html.TextNode, Data: n.text}, nil
	case kindComment:
		return &html.Node{Type: html.CommentNode, Data: n.text}, nil
	case kindElement:
	default:
		return nil, fmt.Errorf("node %s has the kind %q, which a frame does not have", n.id, n.kind)
	}
	el := &html.Node{Type: html.ElementNode, Data: n.tag, Namespace: n.ns}
	if n.ns == "" {
		el.DataAtom = atom.Lookup([]byte(n.tag))
	}
	el.Attr = elementAttrs(n)
	if n.hasRich {
		nodes, err := richNodes(n.rich)
		if err != nil {
			return nil, fmt.Errorf("the text of %s: %w", n.id, err)
		}
		for _, c := range nodes {
			el.AppendChild(c)
		}
	}
	for _, k := range children[n.id] {
		child, err := htmlNode(k, children)
		if err != nil {
			return nil, err
		}
		el.AppendChild(child)
	}
	return el, nil
}

// elementAttrs is the attribute list of an element: its own, in the order they
// were written, with the style and the data-pid where their markers stand.
func elementAttrs(n node) []html.Attribute {
	var out []html.Attribute
	sawStyle, sawPID := false, false
	for _, a := range n.attrs {
		switch a.Key {
		case markPID:
			sawPID = true
			out = append(out, html.Attribute{Key: "data-pid", Val: n.id})
		case markStyle:
			sawStyle = true
			if style, ok := n.joinStyle(true); ok {
				out = append(out, html.Attribute{Key: "style", Val: style})
			}
		default:
			out = append(out, html.Attribute{Namespace: a.Ns, Key: a.Key, Val: a.Val})
		}
	}
	if !sawStyle {
		if style, ok := n.joinStyle(false); ok {
			out = append(out, html.Attribute{Key: "style", Val: style})
		}
	}
	if !sawPID {
		out = append(out, html.Attribute{Key: "data-pid", Val: n.id})
	}
	return out
}
