package designsource

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// StyleDeclaration fixes one property name and keeps typed holes in its value.
// Neither a property name nor a declaration boundary can be supplied by data.
type StyleDeclaration struct {
	Name     string
	Segments []Segment
}

const markerPrefix = "pictorholeq"

func parseStyle(source string) ([]StyleDeclaration, error) {
	parts, e := segments(source)
	if e != nil {
		return nil, e
	}
	if strings.Contains(source, markerPrefix) {
		return nil, fmt.Errorf("reserved CSS binding marker")
	}
	markers := map[string]string{}
	var prepared strings.Builder
	for _, s := range parts {
		if s.Path == "" {
			prepared.WriteString(s.Literal)
		} else {
			marker := fmt.Sprintf("%s%dq", markerPrefix, len(markers))
			markers[marker] = s.Path
			prepared.WriteString(marker)
		}
	}
	declarations, e := cssDeclarations(prepared.String())
	if e != nil {
		return nil, e
	}
	var out []StyleDeclaration
	for _, d := range declarations {
		if strings.Contains(d.Name, markerPrefix) {
			return nil, fmt.Errorf("CSS property names cannot be bound")
		}
		value := d.Value
		for marker, path := range markers {
			value = strings.ReplaceAll(value, marker, "{{"+path+"}}")
		}
		types, e := cssHoleTypes(d.Value, markers)
		if e != nil {
			return nil, e
		}
		segs, e := segments(value)
		if e != nil {
			return nil, e
		}
		setCSSValueTypes(segs, markers, types)
		out = append(out, StyleDeclaration{d.Name, segs})
	}
	return out, nil
}

type cssDeclaration struct{ Name, Value string }

func cssDeclarations(source string) ([]cssDeclaration, error) {
	p := css.NewParser(parse.NewInputString(source), true)
	var out []cssDeclaration
	for {
		grammar, _, name := p.Next()
		if grammar == css.ErrorGrammar {
			if !errors.Is(p.Err(), io.EOF) {
				return nil, p.Err()
			}
			return out, nil
		}
		if grammar == css.CommentGrammar {
			continue
		}
		if grammar != css.DeclarationGrammar && grammar != css.CustomPropertyGrammar {
			return nil, fmt.Errorf("style must contain declarations only")
		}
		var value strings.Builder
		for _, token := range p.Values() {
			if strings.Contains(string(token.Data), markerPrefix) && (token.TokenType == css.StringToken || token.TokenType == css.URLToken) {
				return nil, fmt.Errorf("CSS holes cannot appear inside quoted strings or url(); bind a complete value")
			}
			value.Write(token.Data)
		}
		out = append(out, cssDeclaration{string(name), value.String()})
	}
}

// ResolveStyle resolves each typed value and checks the resulting declaration
// grammar and existing drawing/address rules before DOM style assignment.
func ResolveStyle(declarations []StyleDeclaration, data map[string]any) (string, error) {
	var out strings.Builder
	for _, d := range declarations {
		var value strings.Builder
		for _, segment := range d.Segments {
			if segment.Path == "" {
				value.WriteString(segment.Literal)
				continue
			}
			v, e := Lookup(data, segment.Path)
			if e != nil {
				return "", e
			}
			if segment.ValueType == "css-number" {
				if _, ok := v.(float64); !ok {
					if _, ok := v.(json.Number); !ok {
						return "", fmt.Errorf("CSS dimension binding requires a finite number")
					}
				}
			}
			text, e := scalar(v)
			if e != nil {
				return "", e
			}
			if e := cssValue(text); e != nil {
				return "", e
			}
			value.WriteString(text)
		}
		declaration := d.Name + ":" + value.String() + ";"
		parsed, e := cssDeclarations(declaration)
		if e != nil {
			return "", e
		}
		if len(parsed) != 1 || parsed[0].Name != d.Name {
			return "", fmt.Errorf("CSS data changed declaration structure")
		}
		if issues := designfmt.CheckAttribute("div", "style", declaration); len(issues) > 0 {
			return "", fmt.Errorf("%s", strings.Join(issues, "; "))
		}
		out.WriteString(declaration)
	}
	return out.String(), nil
}

func cssValue(value string) error {
	// Data is one CSS value, never declaration syntax, comments, strings or escape
	// sequences. Functions/tokens such as var(--ink), rgb(...) and dimensions are
	// parsed normally and addresses still pass designfmt after substitution.
	if strings.ContainsAny(value, ";{}@!\\\"'") || strings.Contains(value, "/*") {
		return fmt.Errorf("CSS binding requires one typed CSS value")
	}
	parsed, e := cssDeclarations("color:" + value + ";")
	if e != nil {
		return e
	}
	if len(parsed) != 1 || parsed[0].Name != "color" {
		return fmt.Errorf("CSS binding is not one value")
	}
	return nil
}

var cssUnit = regexp.MustCompile(`^(?:px|em|rem|ex|ch|lh|rlh|vw|vh|vmin|vmax|svw|svh|lvw|lvh|dvw|dvh|cm|mm|in|pt|pc|q|deg|rad|grad|turn|s|ms|fr|%)$`)

func cssHoleTypes(value string, markers map[string]string) (map[string]string, error) {
	out := map[string]string{}
	lexer := css.NewLexer(parse.NewInputString(value))
	for {
		kind, raw := lexer.Next()
		if kind == css.ErrorToken {
			if !errors.Is(lexer.Err(), io.EOF) {
				return nil, lexer.Err()
			}
			return out, nil
		}
		for marker := range markers {
			token := string(raw)
			if !strings.Contains(token, marker) {
				continue
			}
			if kind != css.IdentToken {
				return nil, fmt.Errorf("CSS hole must be a value token or numeric dimension")
			}
			if token == marker {
				out[marker] = "css-value"
				continue
			}
			if strings.HasPrefix(token, marker) && cssUnit.MatchString(strings.TrimPrefix(token, marker)) {
				out[marker] = "css-number"
				continue
			}
			return nil, fmt.Errorf("CSS hole cannot change token structure")
		}
	}
}

func setCSSValueTypes(segs []Segment, markers, types map[string]string) {
	for i := range segs {
		if segs[i].Path != "" {
			segs[i].ValueType = "css-value"
			for marker, path := range markers {
				if path == segs[i].Path && types[marker] == "css-number" {
					segs[i].ValueType = "css-number"
				}
			}
		}
	}
}
