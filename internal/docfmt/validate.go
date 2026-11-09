package docfmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Validate refuses content that is not a valid doc. Every message starts with
// the path of the problem (tabs[0].blocks[2].content[1].href) and says what to
// send instead; a limit message names the limit and the current value.
func Validate(content string) error {
	_, err := Parse(content)
	return err
}

// Parse validates content and returns the doc it holds.
func Parse(content string) (Doc, error) {
	root, err := decode([]byte(content))
	if err != nil {
		return Doc{}, err
	}
	return check(root, content)
}

// decode reads one JSON value, refusing what is not valid text or JSON.
func decode(raw []byte) (any, error) {
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("a doc can be at most %d bytes of JSON, this one is %d bytes", MaxBytes, len(raw))
	}
	if !utf8.Valid(raw) {
		return nil, errors.New("a doc must be UTF-8 text; this content is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("this is not valid JSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("this is not valid JSON: there is more after the document")
	}
	return root, nil
}

// check validates a decoded tree and turns it into the typed doc.
func check(root any, content string) (Doc, error) {
	v := &validator{ids: map[string]string{}}
	if err := v.document(root); err != nil {
		return Doc{}, err
	}
	var doc Doc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return Doc{}, fmt.Errorf("document: %w", err)
	}
	return doc, nil
}

type validator struct {
	ids map[string]string // id -> the path that uses it
}

func at(path, format string, args ...any) error {
	return fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...))
}

func idx(path string, i int) string { return path + "[" + strconv.Itoa(i) + "]" }

func (v *validator) document(root any) error {
	obj, err := object(root, "document", "tabs")
	if err != nil {
		return err
	}
	tabs, err := list(obj["tabs"], "tabs")
	if err != nil {
		return err
	}
	if len(tabs) == 0 {
		return at("tabs", "a doc needs at least one tab")
	}
	if err := limits(tabs); err != nil {
		return err
	}
	for i, t := range tabs {
		if err := v.tab(t, idx("tabs", i)); err != nil {
			return err
		}
	}
	return nil
}

func (v *validator) tab(raw any, path string) error {
	obj, err := object(raw, path, "id", "title", "children", "blocks")
	if err != nil {
		return err
	}
	if err := v.id(obj, path); err != nil {
		return err
	}
	if err := title(obj["title"], path+".title"); err != nil {
		return err
	}
	if c, ok := obj["children"]; ok {
		if l, err := list(c, path+".children"); err != nil {
			return err
		} else if len(l) > 0 {
			return at(path+".children", "nested tabs are not supported yet; leave it empty")
		}
	}
	blocks, err := list(obj["blocks"], path+".blocks")
	if err != nil {
		return err
	}
	return v.blocks(blocks, path+".blocks", nil)
}

func title(raw any, path string) error {
	s, ok := raw.(string)
	switch {
	case !ok && raw == nil:
		return at(path, "required")
	case !ok:
		return at(path, "must be text")
	case strings.TrimSpace(s) == "":
		return at(path, "must not be empty")
	case strings.ContainsAny(s, "\r\n"):
		return at(path, "must be one line")
	case strings.Contains(s, "-->"):
		return at(path, "must not contain -->")
	case utf8.RuneCountInString(s) > MaxTitleRunes:
		return at(path, "is %d characters; at most %d", utf8.RuneCountInString(s), MaxTitleRunes)
	}
	return noNUL(s, path)
}

func noNUL(s, path string) error {
	if strings.ContainsRune(s, 0) {
		return at(path, "must not contain a NUL character")
	}
	return nil
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// id checks the id of a tab or a block and that no other one has it.
func (v *validator) id(obj map[string]any, path string) error {
	raw, ok := obj["id"]
	if !ok {
		return at(path+".id", "required")
	}
	s, ok := raw.(string)
	if !ok || len(s) > MaxIDLength || !idPattern.MatchString(s) {
		return at(path+".id", "must be 1 to %d letters, digits, - or _, starting with a letter or digit", MaxIDLength)
	}
	if other, dup := v.ids[s]; dup {
		return at(path+".id", "%q is already used by %s", s, other)
	}
	v.ids[s] = path
	return nil
}

// blocks validates a list of blocks; inside is the kind of block that holds
// them (nil at the top of a tab).
func (v *validator) blocks(blocks []any, path string, inside *string) error {
	for i, b := range blocks {
		if err := v.block(b, idx(path, i), inside); err != nil {
			return err
		}
	}
	return nil
}

var blockFields = map[string][]string{
	Paragraph:    {"content"},
	Heading:      {"level", "content"},
	BulletList:   {"items"},
	NumberedList: {"start", "items"},
	TaskList:     {"items"},
	Quote:        {"blocks"},
	Code:         {"lang", "text"},
	Rule:         {},
	Table:        {"header", "rows"},
	Image:        {"src", "alt", "size"},
	Pending:      {"intent"},
}

// quotable are the blocks a quote may hold: all but a pending placeholder,
// which stands for a part of the page itself.
var quotable = map[string]bool{Paragraph: true, Heading: true, BulletList: true, NumberedList: true, TaskList: true, Quote: true, Code: true, Rule: true, Table: true, Image: true}

func blockTypes() string {
	names := make([]string, 0, len(blockFields))
	for n := range blockFields {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (v *validator) block(raw any, path string, inside *string) error {
	obj, ok := raw.(map[string]any)
	if !ok {
		return at(path, "must be an object")
	}
	typ, err := blockType(obj, path)
	if err != nil {
		return err
	}
	if inside != nil && !quotable[typ] {
		return at(path+".type", "a %s cannot hold a %s block", *inside, typ)
	}
	if _, err := object(obj, path, append([]string{"id", "type"}, blockFields[typ]...)...); err != nil {
		return err
	}
	if err := v.id(obj, path); err != nil {
		return err
	}
	return v.body(typ, obj, path)
}

func blockType(obj map[string]any, path string) (string, error) {
	raw, ok := obj["type"]
	if !ok {
		return "", at(path+".type", "required; one of %s", blockTypes())
	}
	typ, _ := raw.(string)
	if _, known := blockFields[typ]; !known {
		return "", at(path+".type", "unknown block type %q; the types are: %s", fmt.Sprint(raw), blockTypes())
	}
	return typ, nil
}

func (v *validator) body(typ string, obj map[string]any, path string) error {
	switch typ {
	case Paragraph:
		return inlines(obj["content"], path+".content")
	case Heading:
		if err := checkHeading(obj, path); err != nil {
			return err
		}
		return inlines(obj["content"], path+".content")
	case BulletList, NumberedList, TaskList:
		return v.listBlock(typ, obj, path)
	case Quote:
		return v.quote(obj, path)
	case Code:
		return checkCode(obj, path)
	case Table:
		return checkTable(obj, path)
	case Image:
		return checkImage(obj, path)
	case Pending:
		return checkPending(obj, path)
	}
	return nil // rule
}

func checkHeading(obj map[string]any, path string) error {
	n, ok := whole(obj["level"])
	if !ok || n < 1 || n > 6 {
		return at(path+".level", "must be 1 to 6, a whole number")
	}
	return nil
}

// whole reads a JSON number that is a whole number.
func whole(raw any) (int64, bool) {
	n, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

func (v *validator) quote(obj map[string]any, path string) error {
	blocks, err := list(obj["blocks"], path+".blocks")
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return at(path+".blocks", "a quote needs at least one block")
	}
	typ := Quote
	return v.blocks(blocks, path+".blocks", &typ)
}

var langPattern = regexp.MustCompile(`^[A-Za-z0-9_+#.-]+$`)

func checkCode(obj map[string]any, path string) error {
	if raw, ok := obj["text"]; ok {
		s, isText := raw.(string)
		if !isText {
			return at(path+".text", "must be text")
		}
		if err := noNUL(s, path+".text"); err != nil {
			return err
		}
	}
	if raw, ok := obj["lang"]; ok {
		s, _ := raw.(string)
		if s != "" && (len(s) > MaxLangRunes || !langPattern.MatchString(s)) {
			return at(path+".lang", "must be a language name such as go or c++: at most %d letters, digits or _+#.-", MaxLangRunes)
		}
	}
	return nil
}

func checkImage(obj map[string]any, path string) error {
	src, ok := obj["src"].(string)
	if !ok || src == "" {
		return at(path+".src", "required: the address of the picture")
	}
	if err := imageSource(src); err != nil {
		return at(path+".src", "%s", err)
	}
	if raw, ok := obj["alt"]; ok {
		alt, isText := raw.(string)
		switch {
		case !isText:
			return at(path+".alt", "must be text")
		case utf8.RuneCountInString(alt) > MaxAltRunes:
			return at(path+".alt", "is %d characters; at most %d", utf8.RuneCountInString(alt), MaxAltRunes)
		}
		if err := noNUL(alt, path+".alt"); err != nil {
			return err
		}
	}
	if raw, ok := obj["size"]; ok {
		if size, isText := raw.(string); !isText || (size != "" && ImageSizes[size] == 0) {
			return at(path+".size", "must be one of %s", imageSizeNames())
		}
	}
	return nil
}

// imageSizeNames lists the sizes in the order of their widths, for a refusal.
func imageSizeNames() string {
	names := make([]string, 0, len(ImageSizes))
	for n := range ImageSizes {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return ImageSizes[names[i]] < ImageSizes[names[j]] })
	return strings.Join(names, ", ")
}

func checkPending(obj map[string]any, path string) error {
	intent, ok := obj["intent"].(string)
	switch {
	case !ok || strings.TrimSpace(intent) == "":
		return at(path+".intent", "required: say what this part will hold")
	case strings.ContainsAny(intent, "\r\n"):
		return at(path+".intent", "must be one line")
	case strings.Contains(intent, "-->"):
		return at(path+".intent", "must not contain -->")
	case utf8.RuneCountInString(intent) > MaxIntentRunes:
		return at(path+".intent", "is %d characters; at most %d", utf8.RuneCountInString(intent), MaxIntentRunes)
	}
	return noNUL(intent, path+".intent")
}

// object checks raw is an object whose fields are all among allowed.
func object(raw any, path string, allowed ...string) (map[string]any, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, at(path, "must be an object")
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		known := false
		for _, a := range allowed {
			known = known || a == k
		}
		if !known {
			return nil, at(path, "unknown field %q; the fields here are: %s", k, strings.Join(allowed, ", "))
		}
	}
	return obj, nil
}

func list(raw any, path string) ([]any, error) {
	if raw == nil {
		return nil, at(path, "required: a list")
	}
	l, ok := raw.([]any)
	if !ok {
		return nil, at(path, "must be a list")
	}
	return l, nil
}
