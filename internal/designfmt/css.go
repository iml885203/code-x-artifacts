package designfmt

import (
	"fmt"
	"strings"
)

// allowedProperties is the CSS a frame may use: every property that draws,
// lays out, animates or fonts an element, which covers what an agent writes for
// a design. Custom properties (--x) and the vendor-prefixed ones are allowed as
// a class (see propertyAllowed). A property outside it is refused by name so the
// agent can say what it needs. It is widened by evidence: docs/design-format.md
// ("The width of the allow-list") says what was measured and what each addition
// stands on, so a property is added with the pages that used it, not on a hunch.
var allowedProperties = setOf(`
display position top right bottom left inset inset-block inset-inline inset-block-start inset-block-end inset-inline-start inset-inline-end
z-index float clear overflow overflow-x overflow-y overflow-wrap overflow-clip-margin visibility box-sizing aspect-ratio contain
width height min-width min-height max-width max-height block-size inline-size min-block-size min-inline-size max-block-size max-inline-size
margin margin-top margin-right margin-bottom margin-left margin-block margin-inline margin-block-start margin-block-end margin-inline-start margin-inline-end
padding padding-top padding-right padding-bottom padding-left padding-block padding-inline padding-block-start padding-block-end padding-inline-start padding-inline-end
flex flex-basis flex-direction flex-flow flex-grow flex-shrink flex-wrap order gap row-gap column-gap
grid grid-area grid-auto-columns grid-auto-flow grid-auto-rows grid-column grid-column-end grid-column-start grid-row grid-row-end grid-row-start
grid-template grid-template-areas grid-template-columns grid-template-rows
align-content align-items align-self justify-content justify-items justify-self place-content place-items place-self
columns column-count column-fill column-rule column-rule-color column-rule-style column-rule-width column-span column-width
color font font-family font-feature-settings font-kerning font-size font-size-adjust font-stretch font-style font-variant font-variant-caps
font-variant-ligatures font-variant-numeric font-variation-settings font-weight font-display letter-spacing line-height line-clamp
list-style list-style-position list-style-type list-style-image quotes tab-size text-align text-align-last text-decoration text-decoration-color text-decoration-line
text-decoration-style text-decoration-thickness text-emphasis text-indent text-justify text-overflow text-shadow text-transform text-underline-offset
text-underline-position text-wrap text-wrap-mode text-wrap-style unicode-bidi unicode-range vertical-align white-space white-space-collapse word-break
word-spacing word-wrap writing-mode direction hyphens hanging-punctuation line-break
background background-attachment background-blend-mode background-clip background-color background-image background-origin background-position
background-position-x background-position-y background-repeat background-size
border border-top border-right border-bottom border-left border-color border-style border-width border-radius
border-top-color border-right-color border-bottom-color border-left-color border-top-style border-right-style border-bottom-style border-left-style
border-top-width border-right-width border-bottom-width border-left-width
border-top-left-radius border-top-right-radius border-bottom-left-radius border-bottom-right-radius
border-start-start-radius border-start-end-radius border-end-start-radius border-end-end-radius
border-block border-block-color border-block-style border-block-width border-block-start border-block-end
border-inline border-inline-color border-inline-style border-inline-width border-inline-start border-inline-end
border-collapse border-spacing border-image border-image-outset border-image-repeat border-image-slice border-image-source border-image-width
outline outline-color outline-offset outline-style outline-width box-shadow
opacity filter backdrop-filter mix-blend-mode isolation mask mask-image mask-position mask-repeat mask-size mask-clip mask-composite mask-mode mask-origin mask-type
clip-path clip transform transform-origin transform-style transform-box perspective perspective-origin rotate scale translate backface-visibility
object-fit object-position image-rendering image-orientation
animation animation-delay animation-direction animation-duration animation-fill-mode animation-iteration-count animation-name animation-play-state
animation-timing-function transition transition-delay transition-duration transition-property transition-timing-function transition-behavior will-change
table-layout caption-side empty-cells
fill fill-opacity fill-rule stroke stroke-dasharray stroke-dashoffset stroke-linecap stroke-linejoin stroke-miterlimit stroke-opacity stroke-width
stop-color stop-opacity shape-rendering text-rendering paint-order vector-effect marker marker-start marker-mid marker-end dominant-baseline
alignment-baseline text-anchor baseline-shift
content counter-increment counter-reset counter-set cursor pointer-events user-select resize scroll-behavior scroll-snap-type scroll-snap-align
scroll-margin scroll-padding accent-color caret-color color-scheme appearance forced-color-adjust print-color-adjust zoom
container container-name container-type shape-outside shape-margin shape-image-threshold touch-action src
scroll-margin-top scroll-margin-right scroll-margin-bottom scroll-margin-left scroll-margin-block scroll-margin-inline
scroll-margin-block-start scroll-margin-block-end scroll-margin-inline-start scroll-margin-inline-end
scroll-padding-top scroll-padding-right scroll-padding-bottom scroll-padding-left scroll-padding-block scroll-padding-inline
scroll-padding-block-start scroll-padding-block-end scroll-padding-inline-start scroll-padding-inline-end scroll-snap-stop
overscroll-behavior overscroll-behavior-x overscroll-behavior-y overscroll-behavior-block overscroll-behavior-inline
scrollbar-color scrollbar-width scrollbar-gutter
break-before break-after break-inside page-break-before page-break-after page-break-inside orphans widows
grid-gap grid-row-gap grid-column-gap
x y cx cy r rx ry`)

func setOf(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// propertyAllowed reports whether a frame may set the property: one on the
// list, a custom property, or a vendor-prefixed one, except the one that binds
// behaviour to an element.
func propertyAllowed(name string) bool {
	switch {
	case strings.HasPrefix(name, "--"):
		return true
	case name == "-moz-binding":
		return false
	case strings.HasPrefix(name, "-webkit-"), strings.HasPrefix(name, "-moz-"), strings.HasPrefix(name, "-ms-"), strings.HasPrefix(name, "-o-"):
		return true
	}
	return allowedProperties[name]
}

// allowedAtRules are the at-rules a frame may use, by name without the @.
var allowedAtRules = setOf("media supports keyframes -webkit-keyframes font-face layer container charset page property")

// scopes says what the text inside a pair of braces is.
type scope int

const (
	rules   scope = iota // rules and at-rules: the top of a sheet, the inside of @media
	decls                // declarations (and nested rules): the inside of a rule, @font-face, a keyframe step
	skipped              // the inside of an at-rule that was refused
)

// checkCSS lists what is wrong with a stylesheet (a <style> element's text)
// or, when declarationsOnly, with the value of a style attribute. It parses
// only as far as it needs to find declarations: it never rewrites the CSS, so
// what an author wrote survives untouched.
func checkCSS(src string, declarationsOnly bool) []string {
	return scanCSS(src, declarationsOnly).problems
}

// scanCSS walks a stylesheet or a style attribute's value once.
func scanCSS(src string, declarationsOnly bool) *cssScanner {
	top := rules
	if declarationsOnly {
		top = decls
	}
	sc := &cssScanner{stack: []scope{top}}
	for i := 0; i < len(src); i++ {
		i = sc.next(src, i)
	}
	sc.flush(';')
	return sc
}

// cssScanner walks CSS text one character at a time, collecting the text of the
// segment it is in (a prelude or a declaration) and judging it where it ends.
type cssScanner struct {
	problems []string
	props    []string // the property of every declaration met, in order, lower case except custom properties
	stack    []scope
	seg      strings.Builder
	parens   int
	braces   int // inside the value of a custom property, which may hold {} (--x: {a:b})
	quote    byte
}

// next handles the character at src[i] and returns the index of the last one it used.
func (sc *cssScanner) next(src string, i int) int {
	c := src[i]
	switch {
	case sc.quote != 0:
		sc.seg.WriteByte(c)
		if c == '\\' && i+1 < len(src) {
			i++
			sc.seg.WriteByte(src[i])
		} else if c == sc.quote {
			sc.quote = 0
		}
	case c == '/' && i+1 < len(src) && src[i+1] == '*':
		sc.seg.WriteByte(' ')
		if end := strings.Index(src[i+2:], "*/"); end >= 0 {
			return i + end + 3
		}
		return len(src)
	case c == '"' || c == '\'':
		sc.quote = c
		sc.seg.WriteByte(c)
	default:
		sc.punctuation(c)
	}
	return i
}

// punctuation handles a character outside strings and comments.
func (sc *cssScanner) punctuation(c byte) {
	custom := c == '{' && sc.inCustomValue()
	switch {
	case c == '(':
		sc.parens++
		sc.seg.WriteByte(c)
	case c == ')':
		sc.parens = max(sc.parens-1, 0)
		sc.seg.WriteByte(c)
	case sc.parens > 0, sc.braces > 0 && c != '{' && c != '}':
		sc.seg.WriteByte(c)
	case custom:
		sc.braces++
		sc.seg.WriteByte(c)
	case c == '}' && sc.braces > 0:
		sc.braces--
		sc.seg.WriteByte(c)
	case c == '{' || c == ';':
		sc.flush(c)
	case c == '}':
		sc.flush(c)
		if len(sc.stack) > 1 {
			sc.stack = sc.stack[:len(sc.stack)-1]
		}
	default:
		sc.seg.WriteByte(c)
	}
}

// flush judges the segment that ended with ended: a prelude when it is a brace
// that opened a block, otherwise a declaration or an at-rule statement.
func (sc *cssScanner) flush(ended byte) {
	text := strings.TrimSpace(sc.seg.String())
	sc.seg.Reset()
	if text == "" {
		return
	}
	cur := sc.stack[len(sc.stack)-1]
	switch {
	case ended == '{':
		sc.stack = append(sc.stack, blockScope(text, cur, &sc.problems))
	case cur == decls:
		sc.problems = append(sc.problems, checkDeclaration(text)...)
		if name, _, ok := splitDeclaration(text); ok {
			sc.props = append(sc.props, name)
		}
	case cur == rules:
		sc.problems = append(sc.problems, checkStatement(text)...)
	}
}

// inCustomValue reports that the text so far is "--name:", so a brace after it
// is part of the value and not the start of a block.
func (sc *cssScanner) inCustomValue() bool {
	text := strings.TrimSpace(sc.seg.String())
	return sc.stack[len(sc.stack)-1] == decls && strings.HasPrefix(text, "--") && strings.Contains(text, ":")
}

// blockScope is the scope inside the braces that follow prelude, and notes an
// at-rule that is not allowed.
func blockScope(prelude string, outer scope, problems *[]string) scope {
	if outer == skipped {
		return skipped
	}
	if !strings.HasPrefix(prelude, "@") {
		return decls
	}
	name := atRuleName(prelude)
	switch {
	case !allowedAtRules[name]:
		*problems = append(*problems, fmt.Sprintf("at-rule @%s is not allowed (%s)", name, atRuleHint(name)))
		return skipped
	case name == "font-face":
		return decls
	}
	return rules
}

func atRuleName(prelude string) string {
	name := strings.TrimPrefix(prelude, "@")
	end := strings.IndexFunc(name, func(r rune) bool { return r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') })
	if end >= 0 {
		name = name[:end]
	}
	return strings.ToLower(name)
}

func atRuleHint(name string) string {
	if name == "import" {
		return "a frame loads nothing from outside; put the rules in a <style> in the frame"
	}
	return "use @media, @supports, @keyframes, @font-face, @layer or @container"
}

// checkStatement checks an at-rule without a block (@import, @charset).
func checkStatement(text string) []string {
	if !strings.HasPrefix(text, "@") {
		return nil
	}
	name := atRuleName(text)
	if !allowedAtRules[name] {
		return []string{fmt.Sprintf("at-rule @%s is not allowed (%s)", name, atRuleHint(name))}
	}
	return nil
}

// checkDeclaration checks one "property: value".
func checkDeclaration(text string) []string {
	name, value, ok := splitDeclaration(text)
	if !ok {
		return nil
	}
	var problems []string
	if !propertyAllowed(name) {
		problems = append(problems, fmt.Sprintf("css property %q is not allowed", name))
	}
	return append(problems, checkValue(name, value)...)
}

// splitDeclaration splits "property: value"; the property is lower case unless
// it is a custom property, whose name is case sensitive.
func splitDeclaration(text string) (name, value string, ok bool) {
	colon := strings.IndexByte(text, ':')
	if colon < 0 {
		return "", "", false
	}
	name, value = strings.TrimSpace(text[:colon]), strings.TrimSpace(text[colon+1:])
	if !strings.HasPrefix(name, "--") {
		name = strings.ToLower(name)
	}
	return name, value, true
}

// declaredProperties lists the property of each declaration in a stylesheet or,
// when declarationsOnly, in the value of a style attribute.
func declaredProperties(src string, declarationsOnly bool) []string {
	return scanCSS(src, declarationsOnly).props
}

// checkValue refuses what a value could load or run: an address that is not one
// of the frame's own, expression(), a script URL, and a backslash outside a
// string, which is how a function name is written to slip past the checks.
func checkValue(name, value string) []string {
	var problems []string
	lower := strings.ToLower(value)
	for _, u := range urlsIn(value) {
		if err := checkAddress(u); err != nil {
			problems = append(problems, fmt.Sprintf("css property %q: %s", name, err))
		}
	}
	switch {
	case strings.Contains(lower, "expression("), strings.Contains(lower, "javascript:"):
		problems = append(problems, fmt.Sprintf("css property %q: expression() and javascript: are not allowed", name))
	case hasBareBackslash(value) && !strings.HasPrefix(name, "--"):
		problems = append(problems, fmt.Sprintf("css property %q: a backslash escape outside a string is not allowed (write the text itself)", name))
	}
	return problems
}

// hasBareBackslash reports a backslash that is not inside a quoted string.
func hasBareBackslash(value string) bool {
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
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
		case c == '\\':
			return true
		}
	}
	return false
}

// urlsIn returns the argument of each url( in a value.
func urlsIn(value string) []string {
	var out []string
	lower := strings.ToLower(value)
	for from := 0; ; {
		i := strings.Index(lower[from:], "url(")
		if i < 0 {
			return out
		}
		start := from + i + len("url(")
		end := strings.IndexByte(value[start:], ')')
		if end < 0 {
			return append(out, strings.TrimSpace(value[start:]))
		}
		out = append(out, strings.Trim(strings.TrimSpace(value[start:start+end]), `"'`))
		from = start + end
	}
}
