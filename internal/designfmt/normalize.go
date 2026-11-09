package designfmt

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// pidAttribute is the stable id of an element, the one comments and edits refer to.
const pidAttribute = "data-pid"

var validPID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// Normalize returns the canonical form of a frame that passed the rules: one
// parse and one serialisation, standards mode, and a data-pid on every element
// of the body. It is idempotent, so a frame that is read back and sent again is
// not a change, and it keeps every attribute, every style declaration and every
// piece of CSS as written: only the markup around them is regularised.
func Normalize(text string) (string, error) { return NormalizeAvoiding(text, nil) }

// NormalizeAvoiding is Normalize for a frame that is joining others that
// already hold elements with the ids in taken (a frame an agent changes while
// its room holds the people's version of it): an element without an id of its
// own is never given one of those, so it cannot be taken for one of theirs.
func NormalizeAvoiding(text string, taken []string) (string, error) {
	out, err := normalizeOnce(text, taken)
	if err != nil {
		return "", err
	}
	// A serialisation is a fixed point after the first pass for every markup
	// the parser reads as a tree; the loop makes it so by construction and
	// says so for the rest.
	for range 3 {
		next, err := normalizeOnce(out, taken)
		if err != nil {
			return "", err
		}
		if next == out {
			return out, nil
		}
		out = next
	}
	return "", errors.New("this frame's markup does not settle into one form when it is normalised; simplify it (unclosed or misnested tags are the usual cause)")
}

func normalizeOnce(text string, taken []string) (string, error) {
	doc, err := html.ParseWithOptions(strings.NewReader(text), html.ParseOptionEnableScripting(false))
	if err != nil {
		return "", fmt.Errorf("the HTML cannot be read: %w", err)
	}
	if first := doc.FirstChild; first == nil || first.Type != html.DoctypeNode {
		doc.InsertBefore(&html.Node{Type: html.DoctypeNode, Data: "html"}, first)
	}
	assignPIDs(doc, taken)
	bodyElements(doc, OpenLinkInANewTab)
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		return "", fmt.Errorf("the HTML cannot be written: %w", err)
	}
	return out.String(), nil
}

// findBody is the body element of a parsed document.
func findBody(doc *html.Node) *html.Node {
	if doc.Type == html.ElementNode && doc.Data == "body" {
		return doc
	}
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if b := findBody(c); b != nil {
			return b
		}
	}
	return nil
}

// bodyElements calls visit for each element of the body, the body included, in
// document order.
func bodyElements(doc *html.Node, visit func(*html.Node)) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			visit(n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if body := findBody(doc); body != nil {
		walk(body)
	}
}

func pidOf(n *html.Node) (string, int) {
	for i, a := range n.Attr {
		if a.Key == pidAttribute {
			return a.Val, i
		}
	}
	return "", -1
}

// assignPIDs gives every element of the body a data-pid. Ids already there are
// kept; an element that repeats one, or whose id is not a valid one, gets a new
// id, numbered after the highest "e<number>" the frame has so that an id is not
// reused while its elements are still in the file.
func assignPIDs(doc *html.Node, taken []string) {
	seen := map[string]bool{}
	highest := 0
	for _, pid := range taken {
		if num, err := strconv.Atoi(strings.TrimPrefix(pid, "e")); err == nil && strings.HasPrefix(pid, "e") {
			highest = max(highest, num)
		}
	}
	var missing []*html.Node
	bodyElements(doc, func(n *html.Node) {
		pid, at := pidOf(n)
		switch {
		case at >= 0 && validPID.MatchString(pid) && !seen[pid]:
			seen[pid] = true
			if num, err := strconv.Atoi(strings.TrimPrefix(pid, "e")); err == nil && strings.HasPrefix(pid, "e") {
				highest = max(highest, num)
			}
		default:
			missing = append(missing, n)
		}
	})
	for _, n := range missing {
		highest++
		pid := "e" + strconv.Itoa(highest)
		if _, at := pidOf(n); at >= 0 {
			for i := range n.Attr {
				if n.Attr[i].Key == pidAttribute {
					n.Attr[i].Val = pid
				}
			}
			continue
		}
		n.Attr = append(n.Attr, html.Attribute{Key: pidAttribute, Val: pid})
	}
}

// OpenLinkInANewTab makes a link to the outside (https, mailto) open in a new
// tab that knows nothing of the page: target and rel are set in place when the
// element has them and added after its other attributes when not. The room
// applies it to the links in formatted text as it reads them, so a link someone
// wrote there is the same as one that was published.
func OpenLinkInANewTab(n *html.Node) {
	if n.Namespace != "" || n.Data != "a" || !isLink(attr(n, "href")) {
		return
	}
	set := func(key, val string) {
		for i := range n.Attr {
			if n.Attr[i].Key == key && n.Attr[i].Namespace == "" {
				n.Attr[i].Val = val
				return
			}
		}
		n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
	}
	set("target", "_blank")
	set("rel", "noopener noreferrer")
}
