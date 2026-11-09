package designfmt

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Token names are CSS custom properties, including the two leading dashes.
var (
	tokenName     = regexp.MustCompile(`^--[A-Za-z_][A-Za-z0-9_-]{0,79}$`)
	tokenFunction = regexp.MustCompile(`([A-Za-z_-][A-Za-z0-9_-]*)\s*\(`)
)

// ValidateToken holds a shared variable to a single inert CSS value. Tokens
// cannot fetch resources or escape the stylesheet injected into every frame.
func ValidateToken(name, value string) error {
	if !tokenName.MatchString(name) {
		return fmt.Errorf("tokens[%q]: name must be -- followed by 1 to 80 ASCII letters, digits, underscores or dashes, starting with a letter or underscore", name)
	}
	if strings.TrimSpace(value) == "" || len(value) > 1000 {
		return fmt.Errorf("tokens[%q]: value must be a nonempty CSS string of at most 1000 bytes", name)
	}
	if !safeTokenSyntax(value) || !balancedToken(value) {
		return fmt.Errorf("tokens[%q]: value must be a single CSS value without URLs, escapes, markup, comments or !important", name)
	}
	return nil
}

func safeTokenSyntax(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(";{}<>\\@!", r) {
			return false
		}
	}
	if strings.Contains(value, "/*") || strings.Contains(value, "*/") {
		return false
	}
	for _, call := range tokenFunction.FindAllStringSubmatch(value, -1) {
		switch strings.ToLower(call[1]) {
		case "calc", "min", "max", "clamp", "var", "env", "rgb", "rgba", "hsl", "hsla", "hwb", "lab", "lch", "oklab", "oklch", "color", "color-mix":
		default:
			return false
		}
	}
	return true
}

func balancedToken(value string) bool {
	depth := 0
	var quote rune
	for _, r := range value {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && quote == 0
}

func (p *parser) tokens(raw any) map[string]string {
	obj, ok := raw.(map[string]any)
	if !ok {
		p.add("design.json: tokens must be an object of CSS custom property names and string values")
		return nil
	}
	out := map[string]string{}
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value, ok := obj[name].(string)
		if !ok {
			p.add("tokens[%q]: value must be a CSS string", name)
			continue
		}
		if err := ValidateToken(name, value); err != nil {
			p.add("%s", err)
			continue
		}
		out[name] = value
	}
	return out
}

// TokenCSS declares the validated manifest's tokens without resolving bindings.
func TokenCSS(tokens map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	// Match the kit's explicit theme selector specificity; this sheet follows it.
	b.WriteString(":root:root{")
	for _, name := range names {
		b.WriteString(name + ":" + tokens[name] + ";")
	}
	b.WriteString("}")
	return b.String()
}
