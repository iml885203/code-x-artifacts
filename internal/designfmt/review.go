package designfmt

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

const maxNotesPerFrame = 5

// Review says what a publisher cannot see for itself about frames that passed
// Validate: a frame that is empty of text, a picture without alt text, an
// element placed outside its frame, and a file no frame uses. fs holds the
// text of every file; only the frames in changed are looked at, so a note
// appears once, in the publish that made the problem, and a publish that
// touched one frame is not answered with notes about the others.
func Review(content string, fs map[string]string, changed []string) []string {
	m, problems := Parse(content)
	if len(problems) > 0 {
		return nil
	}
	var notes []string
	used := map[string]bool{}
	for _, placed := range m.Frames() {
		used[placed.Frame.File] = true
		if !slices.Contains(changed, placed.Frame.File) {
			continue
		}
		for _, note := range reviewFrame(placed.Frame, fs[placed.Frame.File]) {
			notes = append(notes, placed.Label()+": "+note)
		}
	}
	var orphans []string
	for _, p := range changed {
		if _, present := fs[p]; present && !used[p] {
			orphans = append(orphans, p)
		}
	}
	sort.Strings(orphans)
	for _, p := range orphans {
		notes = append(notes, fmt.Sprintf("%s is not in design.json, so no frame shows it; add a frame for it, or send it as null to remove it", p))
	}
	return notes
}

func reviewFrame(f Frame, text string) []string {
	doc, err := html.ParseWithOptions(strings.NewReader(text), html.ParseOptionEnableScripting(false))
	if err != nil {
		return nil
	}
	var notes []string
	hasText := false
	bodyElements(doc, func(n *html.Node) {
		if n.Data == "style" {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
				hasText = true
			}
		}
		if n.Data == "img" && !hasAttr(n, "alt") {
			notes = append(notes, fmt.Sprintf("img %s has no alt text; add alt=\"\" if it is only decoration", describe(n)))
		}
		if note := outsideNote(n, f); note != "" {
			notes = append(notes, note)
		}
	})
	if body := findBody(doc); body != nil {
		notes = append(notes, reviewStyles(body, f)...)
	}
	if !hasText {
		notes = append(notes, "the frame has no text; if that is on purpose ignore this, otherwise the file may be empty or its text is in pictures")
	}
	if len(notes) > maxNotesPerFrame {
		extra := len(notes) - maxNotesPerFrame
		notes = append(notes[:maxNotesPerFrame], fmt.Sprintf("and %d more notes of the same kind", extra))
	}
	return notes
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// describe names an element the way the publisher can find it.
func describe(n *html.Node) string {
	if pid := attr(n, pidAttribute); pid != "" {
		return `data-pid "` + pid + `"`
	}
	return "<" + n.Data + ">"
}

// outsideNote notes an element that its own inline style places, in pixels,
// partly or wholly outside the frame it is in.
func outsideNote(n *html.Node, f Frame) string {
	props := inlineStyle(attr(n, "style"))
	if props["position"] != "absolute" && props["position"] != "fixed" {
		return ""
	}
	box := pixelBox{}
	box.left, box.hasLeft = pixels(props["left"])
	box.top, box.hasTop = pixels(props["top"])
	box.width, box.hasWidth = pixels(props["width"])
	box.height, box.hasHeight = pixels(props["height"])
	switch {
	case box.entirelyOutside(f):
		return fmt.Sprintf("%s is placed outside the %dx%d frame and cannot be seen", describe(n), f.W, f.H)
	case box.partlyOutside(f):
		return fmt.Sprintf("%s reaches past the edge of the %dx%d frame", describe(n), f.W, f.H)
	}
	return ""
}

// inlineStyle reads the declarations of a style attribute, lower case.
func inlineStyle(style string) map[string]string {
	props := map[string]string{}
	for _, decl := range strings.Split(style, ";") {
		if k, v, ok := strings.Cut(decl, ":"); ok {
			props[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(strings.ToLower(v))
		}
	}
	return props
}

// pixelBox is the position and size an element gives itself in pixels; a side
// it does not give is not known.
type pixelBox struct {
	left, top, width, height             int
	hasLeft, hasTop, hasWidth, hasHeight bool
}

func (b pixelBox) entirelyOutside(f Frame) bool {
	return b.hasLeft && (b.left >= f.W || b.hasWidth && b.left+b.width <= 0) ||
		b.hasTop && (b.top >= f.H || b.hasHeight && b.top+b.height <= 0)
}

func (b pixelBox) partlyOutside(f Frame) bool {
	return b.hasLeft && b.left < 0 || b.hasTop && b.top < 0 ||
		b.hasLeft && b.hasWidth && b.left+b.width > f.W || b.hasTop && b.hasHeight && b.top+b.height > f.H
}

// pixels reads a whole number of pixels from a value like "48px" or "0".
func pixels(v string) (int, bool) {
	n, ok := pixelsOf(v)
	return int(n), ok
}
