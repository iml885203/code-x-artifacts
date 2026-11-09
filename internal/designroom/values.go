package designroom

import (
	"fmt"
	"strings"
	"unicode"
)

// A style entry of the room becomes a declaration in a frame's file, so what a
// person (or a modified client, or an agent) writes there is held to a grammar
// that cannot end the declaration or the rule, load anything or run anything.
// The grammar is a superset of what the properties panel writes
// (web/src/design/properties/css.ts) and of what authored frames use in the
// properties the editor owns: lengths, colours, keywords, functions of a small
// set, quoted font names. Check applies it to every style entry of the room.

const maxValueLen = 1000

var (
	globalKeywords = []string{"inherit", "initial", "unset", "revert", "revert-layer"}
	// The keywords of the properties that take only a few.
	enumerated = map[string][]string{
		"display":           {"none", "flex", "inline-flex", "block", "inline", "inline-block", "grid", "inline-grid", "contents", "table", "list-item", "flow-root"},
		"position":          {"static", "relative", "absolute", "fixed", "sticky"},
		"flex-wrap":         {"nowrap", "wrap", "wrap-reverse"},
		"flex-direction":    {"row", "row-reverse", "column", "column-reverse"},
		"align-items":       {"normal", "stretch", "center", "start", "end", "flex-start", "flex-end", "baseline", "self-start", "self-end"},
		"justify-content":   {"normal", "stretch", "center", "start", "end", "flex-start", "flex-end", "left", "right", "space-between", "space-around", "space-evenly"},
		"font-style":        {"normal", "italic", "oblique"},
		"text-align":        {"left", "right", "center", "justify", "start", "end"},
		"border-style":      {"none", "hidden", "dotted", "dashed", "solid", "double", "groove", "ridge", "inset", "outset"},
		"font-weight":       {"normal", "bold", "bolder", "lighter"},
		"object-fit":        {"fill", "contain", "cover", "none", "scale-down"},
		"background-repeat": {"repeat", "no-repeat", "repeat-x", "repeat-y", "space", "round"},
	}
	lengthProps = setOf2(`left top width height gap flex-basis font-size line-height letter-spacing border-width
padding padding-top padding-right padding-bottom padding-left margin margin-top margin-right margin-bottom margin-left
background-size background-position
border-radius border-top-left-radius border-top-right-radius border-bottom-right-radius border-bottom-left-radius`)
	colourProps = setOf2(`color background-color border-color`)
	numberProps = setOf2(`opacity flex-grow font-weight`)
	// The functions a value may call; their arguments are held to a small alphabet.
	functions = setOf2(`calc min max clamp var env fit-content minmax rgb rgba hsl hsla hwb lab lch oklab oklch color color-mix`)
)

func setOf2(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// ValidateStyleValue says whether a style property of an element may hold value.
// The empty value takes the declaration away and is always allowed.
func ValidateStyleValue(prop, value string) error {
	if !Owned(prop) && prop != "display" {
		return fmt.Errorf("the style property %q is not one the editor owns", prop)
	}
	v := strings.TrimSpace(value)
	if i := strings.LastIndex(strings.ToLower(v), "!important"); i >= 0 && strings.TrimSpace(v[i+len("!important"):]) == "" {
		v = strings.TrimSpace(v[:i])
	}
	if v == "" {
		return nil
	}
	if prop == "background-image" {
		if err := checkBackgroundImage(v); err != nil {
			return fmt.Errorf("%s: %w", prop, err)
		}
		return nil
	}
	if err := checkAlphabet(v); err != nil {
		return fmt.Errorf("%s: %w", prop, err)
	}
	if err := checkByProperty(prop, v); err != nil {
		return fmt.Errorf("%s: %w", prop, err)
	}
	return nil
}

// checkByProperty holds a value to the grammar of its property.
func checkByProperty(prop, v string) error {
	switch {
	case prop == "flex-wrap":
		return checkTokens(v, 1, func(t string) bool { return in(enumerated[prop], t) || isStyleVariable(t) })
	case isBorderSideProperty(prop):
		return checkBorderSideValue(prop, v)
	case prop == "font-family":
		return checkFontFamily(v)
	case prop == "box-shadow":
		return checkShadow(v)
	case prop == "font-weight":
		return checkTokens(v, 1, func(t string) bool { return isNumber(t) || in(enumerated[prop], t) })
	case numberProps[prop]:
		return checkTokens(v, 1, isNumber)
	case lengthProps[prop]:
		return checkTokens(v, 5, isLength)
	case colourProps[prop]:
		return checkTokens(v, 4, isColour)
	case enumerated[prop] != nil:
		return checkTokens(v, 4, func(t string) bool { return in(enumerated[prop], t) })
	}
	return nil
}

// checkAlphabet refuses what could end a declaration or a rule, start markup or a
// comment, load an address or write an escape, whatever the property.
func checkAlphabet(v string) error {
	if len(v) > maxValueLen {
		return fmt.Errorf("a value is at most %d bytes", maxValueLen)
	}
	lower := strings.ToLower(v)
	for _, bad := range []string{";", "{", "}", "<", ">", "\\", "@", "/*", "*/", "url(", "image(", "image-set(", "expression(", "javascript:", "\n", "\r", "\x00"} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("%q cannot be in a value", bad)
		}
	}
	depth := 0
	for _, r := range v {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("a closing parenthesis has no opening one")
			}
		case unicode.IsControl(r):
			return fmt.Errorf("a control character cannot be in a value")
		}
	}
	if depth != 0 {
		return fmt.Errorf("a parenthesis is not closed")
	}
	return nil
}

func in(list []string, t string) bool {
	t = strings.ToLower(t)
	for _, k := range list {
		if k == t {
			return true
		}
	}
	for _, k := range globalKeywords {
		if k == t {
			return true
		}
	}
	return false
}

// tokens splits at the spaces outside parentheses and quotes.
func tokens(v string) []string {
	var out []string
	depth, start := 0, -1
	var quote rune
	for i, r := range v {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
			if start < 0 {
				start = i
			}
		case r == '(':
			depth++
			if start < 0 {
				start = i
			}
		case r == ')':
			depth--
		case unicode.IsSpace(r) && depth == 0:
			if start >= 0 {
				out = append(out, v[start:i])
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		out = append(out, v[start:])
	}
	return out
}

func checkTokens(v string, most int, ok func(string) bool) error {
	list := tokens(v)
	if len(list) > most {
		return fmt.Errorf("%q has %d parts, at most %d", v, len(list), most)
	}
	for _, t := range list {
		if !ok(t) {
			return fmt.Errorf("%q is not a value this property takes", t)
		}
	}
	return nil
}

func isNumber(t string) bool {
	t = strings.TrimSuffix(t, "%")
	return numberLike(t) || isFunction(t)
}

// numberLike is an optionally signed decimal number.
func numberLike(t string) bool {
	if t == "" {
		return false
	}
	i, digits, dot := 0, 0, false
	if t[0] == '+' || t[0] == '-' {
		i++
	}
	for ; i < len(t); i++ {
		switch {
		case t[i] >= '0' && t[i] <= '9':
			digits++
		case t[i] == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digits > 0
}

// isLength is a number with an optional unit, a keyword (auto, normal, fit-content…),
// the slash of a radius, or a function of the small set.
func isLength(t string) bool {
	if t == "/" || isFunction(t) {
		return true
	}
	i := 0
	if t != "" && (t[0] == '+' || t[0] == '-') {
		i++
	}
	j := i
	for j < len(t) && (t[j] >= '0' && t[j] <= '9' || t[j] == '.') {
		j++
	}
	if j > i {
		return numberLike(t[:j]) && isUnit(t[j:])
	}
	return isIdent(t)
}

func isUnit(s string) bool {
	if s == "%" {
		return true
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return len(s) <= 5
}

func isIdent(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		if !isIdentRune(r) {
			return false
		}
	}
	return true
}

// isFunction is a call of a function of the small set whose arguments are made of
// numbers, units, names, commas, operators, nested calls and colours.
func isFunction(t string) bool {
	open := strings.IndexByte(t, '(')
	if open <= 0 || !strings.HasSuffix(t, ")") || !functions[strings.ToLower(t[:open])] {
		return false
	}
	for _, r := range t[open+1 : len(t)-1] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" .,%+-*/()#_", r) {
			return false
		}
	}
	return true
}

func isColour(t string) bool {
	if strings.HasPrefix(t, "#") {
		h := t[1:]
		if len(h) != 3 && len(h) != 4 && len(h) != 6 && len(h) != 8 {
			return false
		}
		for _, r := range h {
			if !isHexRune(r) {
				return false
			}
		}
		return true
	}
	return isFunction(t) || isIdent(t)
}

func checkShadow(v string) error {
	if strings.EqualFold(v, "none") {
		return nil
	}
	var layers []string
	depth, start := 0, 0
	for i, r := range v {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				layers = append(layers, v[start:i])
				start = i + 1
			}
		}
	}
	layers = append(layers, v[start:])
	for _, layer := range layers {
		err := checkTokens(strings.TrimSpace(layer), 8, func(t string) bool { return isLength(t) || isColour(t) })
		if err != nil {
			return err
		}
	}
	return nil
}

// checkFontFamily is a comma separated list of names: quoted, or words, or var().
func checkFontFamily(v string) error {
	var items []string
	var quote rune
	start := 0
	depth := 0
	for i, r := range v {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			items = append(items, strings.TrimSpace(v[start:i]))
			start = i + 1
		}
	}
	if quote != 0 {
		return fmt.Errorf("a quote is not closed")
	}
	items = append(items, strings.TrimSpace(v[start:]))
	for _, it := range items {
		if !fontName(it) {
			return fmt.Errorf("%q is not a font name", it)
		}
	}
	return nil
}

func fontName(s string) bool {
	if s == "" {
		return false
	}
	if q := s[0]; q == '"' || q == '\'' {
		if len(s) < 2 || s[len(s)-1] != q {
			return false
		}
		return !strings.ContainsRune(s[1:len(s)-1], rune(q))
	}
	if isFunction(s) {
		return true
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// ValidateVisibility says whether a hidden entry holds a state the editor writes.
func ValidateVisibility(value string) error {
	switch value {
	case "", "0", "1":
		return nil
	}
	return fmt.Errorf("a visibility is 0 or 1, not %q", value)
}

func isIdentRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_'
}

func isHexRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
}

func isBorderSideProperty(prop string) bool {
	for _, side := range []string{"top", "right", "bottom", "left"} {
		for _, part := range []string{"width", "style", "color"} {
			if prop == "border-"+side+"-"+part {
				return true
			}
		}
	}
	return false
}

func isStyleVariable(value string) bool {
	return strings.HasPrefix(strings.ToLower(value), "var(") && isFunction(value)
}

func isBorderWidth(value string) bool {
	return in([]string{"thin", "medium", "thick"}, value) ||
		isFunction(value) || value != "/" && !isIdent(value) && isLength(value)
}

func checkBorderSideValue(prop, value string) error {
	switch {
	case strings.HasSuffix(prop, "-width"):
		return checkTokens(value, 1, isBorderWidth)
	case strings.HasSuffix(prop, "-style"):
		return checkTokens(value, 1, func(t string) bool { return in(enumerated["border-style"], t) || isStyleVariable(t) })
	default:
		return checkTokens(value, 1, isColour)
	}
}
