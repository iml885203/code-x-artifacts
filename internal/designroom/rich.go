package designroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/reearth/ygo/crdt"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// Formatted text (docs/design-room.md). An element whose content is text with
// inline markup in it (bold, a break, a link, a comment) is held as one
// Y.Text under its `rich` entry instead of as child nodes: the characters are
// the text, a <br> or a piece of markup the editor does not edit is an embed
// (one unit of the text), and each inline element the editor edits is a format
// attribute over the stretch of text it wraps. Two people typing in different
// words of the paragraph edit one text and keep both; a word made bold is a
// format on a range, not a node moved in a tree.

// formatPrefix starts the name of the attribute that stands for one inline
// element; the rest of the name is the element's data-pid, so two elements of
// one kind next to each other are two formats and not one run.
const formatPrefix = "f:"

// Embeds are objects with one field: br (the attribute list of the break, as
// JSON) or raw (markup that is kept untouched and not edited).
const (
	embedBreak = "br"
	embedRaw   = "raw"
)

// richOp is one run of a formatted text: characters, or one embed, with the
// formats it is inside, by attribute name.
type richOp struct {
	text  string
	embed map[string]any
	attrs map[string]any
}

// format is the value of a format attribute, written as JSON in a string, as the
// attribute list of an element is: the tag, how deep the element is among the
// inline elements around it (so <b><i>x</i></b> and <i><b>x</b></i> come back
// as they were written), and its attributes with the data-pid among them.
type format struct {
	Tag   string      `json:"t"`
	Depth int         `json:"d"`
	Attrs [][3]string `json:"a"`
}

// excludedFromText are the elements whose content the HTML writer does not
// escape or treat as text.
var excludedFromText = map[string]bool{
	"body": true, "style": true, "script": true, "textarea": true, "title": true, "plaintext": true, "xmp": true,
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "template": true,
}

// richOf is the formatted text of an element, when it has some: its content is
// text with something in it that is not plain text, and nothing that is a box of
// the design (a block, a picture, a positioned span). An element of plain text
// is a text node, as it always was.
func richOf(el *html.Node) ([]richOp, bool) {
	if el.Namespace != "" || excludedFromText[el.Data] {
		return nil, false
	}
	var ops []richOp
	if !addInline(&ops, el, nil, 0) {
		return nil, false
	}
	words, formatted := false, false
	for _, op := range ops {
		words = words || strings.TrimSpace(op.text) != ""
		formatted = formatted || op.embed != nil || len(op.attrs) > 0
	}
	return ops, words && formatted
}

func withFormat(active map[string]any, key string, value string) map[string]any {
	out := make(map[string]any, len(active)+1)
	for k, v := range active {
		out[k] = v
	}
	out[key] = value
	return out
}

// addInline adds the children of parent as runs, under the formats in active; it
// is false when something is there that text cannot hold.
func addInline(ops *[]richOp, parent *html.Node, active map[string]any, depth int) bool {
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			*ops = append(*ops, richOp{text: c.Data, attrs: active})
		case c.Type == html.CommentNode:
			*ops = append(*ops, richOp{embed: map[string]any{embedRaw: renderNode(c)}, attrs: active})
		case c.Type != html.ElementNode || c.Namespace != "":
			return false
		case c.Data == "br":
			*ops = append(*ops, richOp{embed: map[string]any{embedBreak: attrsJSON(toAttrs(c.Attr))}, attrs: active})
		case designfmt.EditedInline(c) && c.FirstChild != nil && purePhrasing(c):
			pid := pidOf(c)
			if pid == "" {
				return false
			}
			value, _ := json.Marshal(format{Tag: c.Data, Depth: depth, Attrs: attrRows(toAttrs(c.Attr))}) // plain values cannot fail
			if !addInline(ops, c, withFormat(active, formatPrefix+pid, string(value)), depth+1) {
				return false
			}
		case (designfmt.OpaqueInline(c) || designfmt.EditedInline(c)) && purePhrasing(c):
			// Phrasing the editor does not edit, or an inline element with nothing
			// in it: kept as written.
			*ops = append(*ops, richOp{embed: map[string]any{embedRaw: renderNode(c)}, attrs: active})
		default:
			return false
		}
	}
	return true
}

// purePhrasing reports that everything under n is text, breaks, comments and
// inline elements, so that it can be held as markup inside a text.
func purePhrasing(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode, c.Type == html.CommentNode:
		case c.Type == html.ElementNode && c.Namespace == "" && (c.Data == "br" || designfmt.EditedInline(c) || designfmt.OpaqueInline(c)):
			if !purePhrasing(c) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func renderNode(n *html.Node) string {
	var b bytes.Buffer
	_ = html.Render(&b, n) // a bytes.Buffer does not fail
	return b.String()
}

func pidOf(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "data-pid" && a.Namespace == "" {
			return a.Val
		}
	}
	return ""
}

func attrRows(list []attr) [][3]string {
	rows := make([][3]string, len(list))
	for i, a := range list {
		rows[i] = [3]string{a.Ns, a.Key, a.Val}
	}
	return rows
}

// parsedFormat is a format attribute read: its name, its pid and its value.
type parsedFormat struct {
	key, pid string
	format
}

// formatsOf reads the formats a run is inside, outermost first.
func formatsOf(attrs map[string]any) ([]parsedFormat, error) {
	var out []parsedFormat
	for key, v := range attrs {
		pid, ok := strings.CutPrefix(key, formatPrefix)
		if !ok {
			continue // an attribute this reader does not know says nothing about the markup
		}
		s, isString := v.(string)
		if !isString {
			return nil, fmt.Errorf("the format %q is not a string", key)
		}
		p := parsedFormat{key: key, pid: pid}
		if err := json.Unmarshal([]byte(s), &p.format); err != nil {
			return nil, fmt.Errorf("the format %q: %w", key, err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].pid < out[j].pid
	})
	return out, nil
}

// richNodes is the markup of a formatted text: its runs written as nodes, with
// each format opened where it starts and closed where its run ends. Formats that
// cross close and open again, which is the markup that says the same.
func richNodes(ops []richOp) ([]*html.Node, error) {
	type open struct {
		key string
		el  *html.Node
	}
	var roots []*html.Node
	var stack []open
	add := func(n *html.Node) {
		if len(stack) == 0 {
			roots = append(roots, n)
			return
		}
		stack[len(stack)-1].el.AppendChild(n)
	}
	for _, op := range ops {
		want, err := formatsOf(op.attrs)
		if err != nil {
			return nil, err
		}
		keep := 0
		for keep < len(stack) && keep < len(want) && stack[keep].key == want[keep].key {
			keep++
		}
		stack = stack[:keep]
		for _, f := range want[keep:] {
			el := &html.Node{Type: html.ElementNode, Data: f.Tag, DataAtom: atom.Lookup([]byte(f.Tag)), Attr: toHTMLAttrs(attrsOfRows(f.Attrs))}
			designfmt.OpenLinkInANewTab(el)
			add(el)
			stack = append(stack, open{f.key, el})
		}
		n, err := embedNode(op)
		if err != nil {
			return nil, err
		}
		add(n)
	}
	return roots, nil
}

func attrsOfRows(rows [][3]string) []attr {
	out := make([]attr, len(rows))
	for i, r := range rows {
		out[i] = attr{r[0], r[1], r[2]}
	}
	return out
}

// embedNode is the node of a run: a text, a break, or markup kept as written.
func embedNode(op richOp) (*html.Node, error) {
	if op.embed == nil {
		return &html.Node{Type: html.TextNode, Data: op.text}, nil
	}
	if raw, ok := op.embed[embedRaw].(string); ok {
		return &html.Node{Type: html.RawNode, Data: raw}, nil
	}
	if list, ok := op.embed[embedBreak].(string); ok {
		attrs, err := parseAttrs(list)
		if err != nil {
			return nil, err
		}
		return &html.Node{Type: html.ElementNode, Data: "br", DataAtom: atom.Br, Attr: toHTMLAttrs(attrs)}, nil
	}
	return nil, fmt.Errorf("an embed that is neither a break nor markup: %v", op.embed)
}

// readRich is the runs of a Y.Text, from its delta; embeds that are not ours are
// left out and attributes that are not formats are ignored.
func readRich(t *crdt.YText) []richOp {
	var ops []richOp
	for _, d := range t.ToDelta() {
		if d.Op != crdt.DeltaOpInsert {
			continue
		}
		attrs := map[string]any{}
		for k, v := range d.Attributes {
			if strings.HasPrefix(k, formatPrefix) {
				attrs[k] = v
			}
		}
		switch v := d.Insert.(type) {
		case string:
			ops = append(ops, richOp{text: v, attrs: attrs})
		case map[string]any:
			if _, isBreak := v[embedBreak]; isBreak {
				ops = append(ops, richOp{embed: v, attrs: attrs})
			} else if _, isRaw := v[embedRaw]; isRaw {
				ops = append(ops, richOp{embed: v, attrs: attrs})
			}
		}
	}
	return ops
}

// units is how many positions of a Y.Text a run takes: its UTF-16 code units,
// or one for an embed.
func (op richOp) units() int {
	if op.embed != nil {
		return 1
	}
	return len(utf16.Encode([]rune(op.text)))
}

// putRich writes the runs into a new Y.Text: first every character and embed
// with no format at all (an insert with no attributes would otherwise take the
// formatting at the cursor), then each format over the stretch of the text it
// wraps.
func putRich(txn *crdt.Transaction, t *crdt.YText, ops []richOp) {
	type extent struct {
		value      string
		start, end int
	}
	spans := map[string]*extent{}
	var order []string
	pos := 0
	for _, op := range ops {
		if op.embed != nil {
			t.InsertEmbed(txn, pos, op.embed, nil)
		} else if op.text != "" {
			t.Insert(txn, pos, op.text, nil)
		}
		n := op.units()
		for key, v := range op.attrs {
			s, ok := spans[key]
			if !ok {
				value, _ := v.(string)
				s = &extent{value: value, start: pos}
				spans[key] = s
				order = append(order, key)
			}
			s.end = pos + n
		}
		pos += n
	}
	sort.Strings(order)
	for _, key := range order {
		s := spans[key]
		t.Format(txn, s.start, s.end-s.start, crdt.Attributes{key: s.value})
	}
}

// checkRich refuses formatted text that a frame could not say: an element
// outside the kept inline ones, an attribute or a style a frame may not have, or
// markup that is not text-level. It is the rule a published frame is held to,
// applied to what someone wrote into the room.
func checkRich(ops []richOp) error {
	for _, op := range ops {
		formats, err := formatsOf(op.attrs)
		if err != nil {
			return err
		}
		for _, f := range formats {
			if err := checkFormat(f); err != nil {
				return err
			}
		}
		if err := checkEmbed(op.embed); err != nil {
			return err
		}
	}
	return nil
}

func checkFormat(f parsedFormat) error {
	if !designfmt.InlineKept(f.Tag) {
		return fmt.Errorf("the format %s is an <%s>, which text does not hold as a format", f.pid, f.Tag)
	}
	for _, row := range f.Attrs {
		if row[0] != "" {
			return fmt.Errorf("the format %s has the attribute %s:%s, and an inline element has no namespaced attributes", f.pid, row[0], row[1])
		}
		if problems := designfmt.CheckAttribute(f.Tag, row[1], row[2]); len(problems) > 0 {
			return fmt.Errorf("the format %s: %s", f.pid, problems[0])
		}
		if row[1] == "style" && !designfmt.TextLevelStyle(row[2]) {
			return fmt.Errorf("the format %s has a style that places or sizes it, which is not text", f.pid)
		}
	}
	return nil
}

func checkEmbed(embed map[string]any) error {
	if embed == nil {
		return nil
	}
	if list, ok := embed[embedBreak].(string); ok {
		rows, err := parseAttrs(list)
		if err != nil {
			return err
		}
		for _, a := range rows {
			if problems := designfmt.CheckAttribute("br", a.Key, a.Val); len(problems) > 0 {
				return fmt.Errorf("a break: %s", problems[0])
			}
		}
		return nil
	}
	raw, ok := embed[embedRaw].(string)
	if !ok {
		return fmt.Errorf("an embed that is neither a break nor markup: %v", embed)
	}
	if problems := designfmt.CheckFragment(raw); len(problems) > 0 {
		return fmt.Errorf("markup in a text: %s", problems[0])
	}
	return nil
}
