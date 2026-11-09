package docfmt

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/text"
)

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTag     = regexp.MustCompile(`(?s)<[^>]*>`)
)

// maxFallbackChunk is the largest code block the fallback of FromLegacyMarkdown
// writes, so a long document stays within the limits as several blocks.
const maxFallbackChunk = 256 << 10

// FromLegacyMarkdown converts a document that was Markdown before blocks existed
// (the one-time migration of format 1). It never refuses a document for its
// links or pictures: a link that breaks the rules keeps its text, a picture with
// such an address is left out, and every comment line is raw HTML, which a
// Markdown page never showed. previous, when not nil, is the doc this version
// follows, so blocks that did not change keep their ids through the history.
//
// The result has the same text as the Markdown (TextOfMarkdown against
// TextOfDoc). If it does not, or is not a valid doc, the Markdown itself is kept
// as code blocks, so that no text is lost.
func FromLegacyMarkdown(md string, previous *Doc, newID func(prefix string) string) (Doc, error) {
	source := []byte(md)
	root := markdown.Parser().Parse(text.NewReader(source))
	c := &converter{src: source, legacy: true}
	tabs, err := c.tabs(root)
	if err == nil {
		doc := Doc{Tabs: tabs}
		used := map[string]bool{}
		if previous != nil {
			collectIDs(*previous, used)
		}
		assignIDs(&doc, newID, used)
		if previous != nil {
			KeepIDs(&doc, *previous)
		}
		if s, cerr := Canonical(doc); cerr == nil && Validate(s) == nil && TextOfDoc(doc) == TextOfMarkdown(md) {
			return doc, nil
		}
	}
	return fallback(md, newID)
}

func fallback(md string, newID func(prefix string) string) (Doc, error) {
	tab := Tab{ID: newID("t_"), Title: DefaultTabTitle, Blocks: []Block{}}
	for rest := md; rest != ""; {
		n := min(len(rest), maxFallbackChunk)
		for n < len(rest) && !utf8.RuneStart(rest[n]) {
			n--
		}
		tab.Blocks = append(tab.Blocks, Block{ID: newID("b_"), Type: Code, Text: rest[:n]})
		rest = rest[n:]
	}
	doc := Doc{Tabs: []Tab{tab}}
	s, err := Canonical(doc)
	if err == nil {
		err = Validate(s)
	}
	if err != nil {
		return Doc{}, fmt.Errorf("keep Markdown as code: %w", err)
	}
	return doc, nil
}

// TextOfMarkdown is the text a Markdown page shows for md, with the tags,
// comments and spacing left out, so it can be compared with TextOfDoc.
func TextOfMarkdown(md string) string {
	var b strings.Builder
	if err := markdown.Renderer().Render(&b, []byte(md), markdown.Parser().Parse(text.NewReader([]byte(md)))); err != nil {
		return ""
	}
	return textOfHTML(b.String())
}

// TextOfDoc is the text the read-only page of d shows.
func TextOfDoc(d Doc) string { return textOfHTML(RenderBody(d)) }

func textOfHTML(markup string) string {
	markup = htmlComment.ReplaceAllString(markup, " ")
	markup = htmlTag.ReplaceAllString(markup, " ")
	return strings.Join(strings.Fields(html.UnescapeString(markup)), " ")
}
