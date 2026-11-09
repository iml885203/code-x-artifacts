package docfmt

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var marks = []string{MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode, MarkHighlight, MarkLink}

func inlines(raw any, path string) error {
	if raw == nil {
		return nil
	}
	runs, err := list(raw, path)
	if err != nil {
		return err
	}
	for i, r := range runs {
		if err := checkRun(r, idx(path, i)); err != nil {
			return err
		}
	}
	return nil
}

func checkRun(raw any, path string) error {
	obj, err := object(raw, path, "text", "marks", "href", "user", "name", "date", "dropdown")
	if err != nil {
		return err
	}
	if _, dropdown := obj["dropdown"]; dropdown {
		return checkDropdown(obj, path)
	}
	if _, date := obj["date"]; date {
		return checkDate(obj, path)
	}
	_, mention := obj["user"]
	_, named := obj["name"]
	_, textRun := obj["text"]
	if mention || (named && !textRun) {
		return checkMention(obj, path)
	}
	text, ok := obj["text"].(string)
	switch {
	case obj["text"] == nil:
		return at(path+".text", "required")
	case !ok:
		return at(path+".text", "must be text")
	case text == "":
		return at(path+".text", "must not be empty; leave the run out")
	}
	if err := noNUL(text, path+".text"); err != nil {
		return err
	}
	linked, err := runMarks(obj["marks"], path+".marks")
	if err != nil {
		return err
	}
	if _, has := obj["name"]; has {
		return at(path+".name", "only a mention has one")
	}
	return runHref(obj["href"], linked, path+".href")
}

var accountPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// checkMention validates a run that mentions a person: the account and the name
// it had then, and nothing a text run has.
func checkMention(obj map[string]any, path string) error {
	if err := mentionUser(obj["user"], path+".user"); err != nil {
		return err
	}
	if err := mentionName(obj["name"], path+".name"); err != nil {
		return err
	}
	for field, why := range map[string]string{"text": "a mention has no text", "marks": "a mention has no marks", "href": "a mention has no address"} {
		if _, has := obj[field]; has {
			return at(path+"."+field, "%s", why)
		}
	}
	return nil
}

func mentionUser(raw any, path string) error {
	s, ok := raw.(string)
	switch {
	case raw == nil:
		return at(path, "required: the account, such as anna.lee")
	case !ok:
		return at(path, "must be text")
	case s == "":
		return at(path, "must not be empty")
	case utf8.RuneCountInString(s) > MaxMentionUserRunes:
		return at(path, "is %d characters; at most %d", utf8.RuneCountInString(s), MaxMentionUserRunes)
	case !accountPattern.MatchString(s):
		return at(path, "must be letters, digits, . _ or - (the sign-in name, such as anna.lee)")
	}
	return nil
}

func mentionName(raw any, path string) error {
	s, ok := raw.(string)
	switch {
	case raw == nil:
		return at(path, "required: the name to show, such as Anna Lee")
	case !ok:
		return at(path, "must be text")
	case strings.TrimSpace(s) == "":
		return at(path, "must not be empty")
	case strings.ContainsAny(s, "\r\n"):
		return at(path, "must be one line")
	case utf8.RuneCountInString(s) > MaxMentionNameRunes:
		return at(path, "is %d characters; at most %d", utf8.RuneCountInString(s), MaxMentionNameRunes)
	}
	if err := noNUL(s, path); err != nil {
		return err
	}
	if strings.ContainsFunc(s, unicode.IsControl) {
		return at(path, "must not contain control characters")
	}
	return nil
}

// runMarks checks a run's marks and reports whether one is the link mark.
func runMarks(raw any, path string) (bool, error) {
	if raw == nil {
		return false, nil
	}
	l, err := list(raw, path)
	if err != nil {
		return false, err
	}
	seen := map[string]bool{}
	for i, m := range l {
		name, _ := m.(string)
		if !contains(marks, name) {
			return false, at(idx(path, i), "unknown mark %q; the marks are: %s", fmt.Sprint(m), strings.Join(marks, ", "))
		}
		if seen[name] {
			return false, at(idx(path, i), "repeats the mark %s", name)
		}
		seen[name] = true
	}
	return seen[MarkLink], nil
}

func runHref(raw any, linked bool, path string) error {
	switch {
	case raw == nil && linked:
		return at(path, "a link needs an address: https:, http:, mailto: or #section")
	case raw == nil:
		return nil
	case !linked:
		return at(path, "only a run with the link mark has one")
	}
	href, ok := raw.(string)
	if !ok || href == "" {
		return at(path, "a link needs an address: https:, http:, mailto: or #section")
	}
	if err := linkTarget(href); err != nil {
		return at(path, "%s", err)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// plain refuses spaces and control characters, which a browser would strip or
// read differently from this check.
func plain(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f })
}

const linkSchemes = "a link must start with https:, http:, mailto: or #"

// linkTarget applies the one scheme rule every link follows, in JSON and in
// Markdown alike.
func linkTarget(href string) error {
	switch {
	case len(href) > MaxLinkBytes:
		return fmt.Errorf("is %d bytes; at most %d", len(href), MaxLinkBytes)
	case !plain(href):
		return fmt.Errorf("must not contain spaces or control characters")
	case strings.HasPrefix(href, "#"):
		return nil
	}
	scheme := schemeOf(href)
	switch scheme {
	case "":
		return fmt.Errorf("is a relative address; %s", linkSchemes)
	case "https", "http":
		if u, err := url.Parse(href); err != nil || u.Host == "" {
			return fmt.Errorf("has no host: %s", href)
		}
		return nil
	case "mailto":
		if len(href) == len("mailto:") {
			return fmt.Errorf("has nothing after mailto")
		}
		return nil
	}
	return fmt.Errorf("the scheme %s: is not allowed; %s", scheme, linkSchemes)
}

// schemeOf is the lowercase scheme of href, or "" when it has none.
func schemeOf(href string) string {
	i := strings.IndexAny(href, ":/?#")
	if i <= 0 || href[i] != ':' {
		return ""
	}
	return strings.ToLower(href[:i])
}

var dataImage = regexp.MustCompile(`^data:image/(png|jpeg|gif|webp);base64,[A-Za-z0-9+/]*=*$`)

const imageSources = "must be an https: address, a file uploaded to this doc (starting /files/), or a data:image/png, jpeg, gif or webp base64 address"

// imageSource applies the image rules of doc_guide.md.
func imageSource(src string) error {
	switch {
	case !plain(src):
		return fmt.Errorf("must not contain spaces or control characters")
	case strings.HasPrefix(src, "data:"):
		if len(src) > MaxDataURIBytes {
			return fmt.Errorf("is %d bytes; a data: address can be at most %d bytes, upload bigger pictures to the doc instead", len(src), MaxDataURIBytes)
		}
		if !dataImage.MatchString(src) {
			return fmt.Errorf("%s", imageSources)
		}
		return nil
	case strings.HasPrefix(src, "/files/") && len(src) > len("/files/"):
		return nil
	case strings.HasPrefix(strings.ToLower(src), "https://"):
		if u, err := url.Parse(src); err != nil || u.Host == "" {
			return fmt.Errorf("has no host: %s", src)
		}
		return nil
	}
	return fmt.Errorf("%s", imageSources)
}

// listBlock validates a list block and its items.
func (v *validator) listBlock(typ string, obj map[string]any, path string) error {
	if raw, ok := obj["start"]; ok {
		if n, whole := whole(raw); !whole || n < 0 || n > 1e9 {
			return at(path+".start", "must be 0 or more, a whole number")
		}
	}
	items, err := list(obj["items"], path+".items")
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return at(path+".items", "a list needs at least one item")
	}
	return listItems(typ, items, path+".items")
}

func listItems(typ string, items []any, path string) error {
	for i, raw := range items {
		p := idx(path, i)
		obj, err := object(raw, p, "content", "checked", "items")
		if err != nil {
			return err
		}
		if err := itemChecked(typ, obj, p); err != nil {
			return err
		}
		if err := inlines(obj["content"], p+".content"); err != nil {
			return err
		}
		if raw, ok := obj["items"]; ok {
			nested, err := list(raw, p+".items")
			if err != nil {
				return err
			}
			if err := listItems(typ, nested, p+".items"); err != nil {
				return err
			}
		}
	}
	return nil
}

func itemChecked(typ string, obj map[string]any, path string) error {
	raw, has := obj["checked"]
	switch {
	case typ != TaskList && has:
		return at(path+".checked", "only a task list has it")
	case typ == TaskList && !has:
		return at(path+".checked", "required: true or false")
	case typ == TaskList:
		if _, ok := raw.(bool); !ok {
			return at(path+".checked", "must be true or false")
		}
	}
	return nil
}

var aligns = []string{"left", "center", "right"}

func checkTable(obj map[string]any, path string) error {
	if raw, ok := obj["header"]; ok {
		if _, isBool := raw.(bool); !isBool {
			return at(path+".header", "must be true or false")
		}
	}
	rows, err := list(obj["rows"], path+".rows")
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return at(path+".rows", "a table needs at least one row")
	}
	if err := tableSize(rows, path); err != nil {
		return err
	}
	width := -1
	for i, raw := range rows {
		p := idx(path+".rows", i)
		row, err := object(raw, p, "cells")
		if err != nil {
			return err
		}
		cells, err := list(row["cells"], p+".cells")
		if err != nil {
			return err
		}
		if width < 0 {
			width = len(cells)
		}
		if err := checkRow(cells, width, p+".cells"); err != nil {
			return err
		}
	}
	return nil
}

func checkRow(cells []any, width int, path string) error {
	switch {
	case len(cells) == 0:
		return at(path, "a row needs at least one cell")
	case len(cells) != width:
		return at(path, "has %d cells, the first row has %d", len(cells), width)
	}
	for i, raw := range cells {
		p := idx(path, i)
		cell, err := object(raw, p, "align", "content")
		if err != nil {
			return err
		}
		if a, ok := cell["align"]; ok {
			if s, _ := a.(string); s != "" && !contains(aligns, s) {
				return at(p+".align", "must be left, center or right")
			}
		}
		if err := inlines(cell["content"], p+".content"); err != nil {
			return err
		}
	}
	return nil
}

// shape is what the whole tree holds, counted before any of it is read in
// detail so that a limit message can say what the doc really contains.
type shape struct{ blocks, items, depth int }

func (s *shape) blockList(list []any, level int) {
	for _, raw := range list {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		s.blocks++
		s.depth = max(s.depth, level)
		if kids, ok := b["blocks"].([]any); ok {
			s.blockList(kids, level+1)
		}
		if items, ok := b["items"].([]any); ok {
			s.itemList(items, level+1)
		}
	}
}

func (s *shape) itemList(list []any, level int) {
	for _, raw := range list {
		it, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		s.items++
		s.depth = max(s.depth, level)
		if kids, ok := it["items"].([]any); ok {
			s.itemList(kids, level+1)
		}
	}
}

func limits(tabs []any) error {
	if len(tabs) > MaxTabs {
		return at("tabs", "this doc has %d tabs; at most %d are allowed", len(tabs), MaxTabs)
	}
	var s shape
	for _, raw := range tabs {
		if t, ok := raw.(map[string]any); ok {
			if blocks, ok := t["blocks"].([]any); ok {
				s.blockList(blocks, 1)
			}
		}
	}
	switch {
	case s.blocks > MaxBlocks:
		return at("tabs", "this doc has %d blocks; at most %d are allowed", s.blocks, MaxBlocks)
	case s.items > MaxListItems:
		return at("tabs", "this doc has %d list items; at most %d are allowed", s.items, MaxListItems)
	case s.depth > MaxDepth:
		return at("tabs", "this doc is nested %d levels deep; at most %d are allowed", s.depth, MaxDepth)
	}
	return nil
}

// tableSize checks the table against its limits from the first row and the
// total, before the cells are read one by one.
func tableSize(rows []any, path string) error {
	cells := 0
	for _, raw := range rows {
		if row, ok := raw.(map[string]any); ok {
			if c, ok := row["cells"].([]any); ok {
				cells += len(c)
			}
		}
	}
	if first, ok := rows[0].(map[string]any); ok {
		if c, ok := first["cells"].([]any); ok && len(c) > MaxTableColumns {
			return at(path, "this table has %d columns; at most %d are allowed", len(c), MaxTableColumns)
		}
	}
	if cells > MaxTableCells {
		return at(path, "this table has %d cells; at most %d are allowed", cells, MaxTableCells)
	}
	return nil
}
