package designroom

import (
	"sort"
	"strings"
)

// ownedProps are the inline style properties the editor has controls for: the
// ones people change at once in different places, so each is its own entry in
// the room and two people changing different ones keep both. Everything else in
// a style attribute stays in the element's skeleton, as written.
var ownedProps = ownedSet()

// Owned reports whether the editor owns a style property.
func Owned(prop string) bool { return ownedProps[prop] }

const (
	slotOpen  = ""
	slotClose = ""
)

// splitDeclarations splits a style attribute at its top-level semicolons, as
// written: the pieces joined by ";" are the attribute again. A semicolon inside
// parentheses or a string (url(data:…;base64,…), content: ";") does not split.
func splitDeclarations(style string) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(style); i++ {
		c := style[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth = max(depth-1, 0)
		case c == ';' && depth == 0:
			parts = append(parts, style[start:i])
			start = i + 1
		}
	}
	return append(parts, style[start:])
}

func propertyOf(declaration string) (name string, valueAt int, ok bool) {
	colon := strings.IndexByte(declaration, ':')
	if colon < 0 {
		return "", 0, false
	}
	return strings.ToLower(strings.TrimSpace(declaration[:colon])), colon + 1, true
}

// splitStyle separates a style attribute into its skeleton, which holds every
// declaration as written with the value of each owned one replaced by a slot,
// and the values of the owned ones. A property that is written twice (a
// fallback: display:-webkit-box;display:flex) is not owned for that element, so
// the attribute comes back exactly.
func splitStyle(style string) (skeleton string, owned map[string]string) {
	parts := splitDeclarations(style)
	count := map[string]int{}
	for _, p := range parts {
		if name, _, ok := propertyOf(p); ok {
			count[name]++
		}
	}
	owned = map[string]string{}
	for i, p := range parts {
		name, at, ok := propertyOf(p)
		if !ok || !ownedProps[name] || (count[name] != 1 && !overrideAfterShorthand(name, parts, i)) || coveredByLaterShorthand(name, parts[i+1:]) || strings.TrimSpace(p[at:]) == "" {
			continue
		}
		// A picture the editor cannot hold to its grammar (a gradient, a picture from elsewhere) stays in the skeleton as written.
		if pictureSet[name] && ValidateStyleValue(name, p[at:]) != nil {
			continue
		}
		owned[name] = p[at:]
		parts[i] = p[:at] + slotOpen + name + slotClose
	}
	return strings.Join(parts, ";"), owned
}

// joinStyle is the style attribute for a skeleton and the owned values: each
// slot takes its value, a declaration whose value was removed goes, and a value
// the skeleton has no slot for is added at the end. ok is false when nothing is
// left, so the attribute is left out.
func joinStyle(skeleton string, owned map[string]string, hadStyle bool) (style string, ok bool) {
	parts := splitDeclarations(skeleton)
	if skeleton == "" {
		parts = nil // an element with no style of its own: its values are the whole attribute
	}
	used := map[string]bool{}
	var out []string
	slotted := false
	for _, p := range parts {
		open := strings.Index(p, slotOpen)
		if open < 0 {
			out = append(out, p)
			continue
		}
		slotted = true
		name := p[open+len(slotOpen) : strings.Index(p, slotClose)]
		used[name] = true
		if v, has := owned[name]; has && v != "" {
			out = append(out, p[:open]+v)
		}
	}
	for _, name := range sortedKeys(owned) {
		if used[name] || owned[name] == "" {
			continue
		}
		added := name + ":" + owned[name]
		if n := len(out); n > 0 && out[n-1] == "" {
			out = append(out[:n-1], added, "") // before the trailing semicolon
		} else {
			out = append(out, added)
		}
	}
	joined := strings.Join(out, ";")
	if joined == "" && (slotted || !hadStyle) {
		return "", false
	}
	return joined, true
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
