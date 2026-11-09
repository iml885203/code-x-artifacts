package designsource

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// ClassContract is parser-derived instance method metadata. It is an input
// contract, not a sandbox or permission to execute a native worker program.
type ClassContract struct {
	ClassName, BaseName             string
	Methods                         []string
	ClassNameSpan, BaseNameSpan     SourceSpan
	ReferencesClass, ReferencesBase bool
}

// ValidateClassicClass validates one classic class and its bounded AST. Importers
// use the same parser with their source class/base names before renaming headers.
func ValidateClassicClass(source, className, baseName string) (ClassContract, error) {
	out := ClassContract{ClassName: className, BaseName: baseName}
	if len(source) > 128<<10 {
		return out, fmt.Errorf("logic source exceeds 128 KiB")
	}
	ast, e := js.Parse(parse.NewInputString(source), js.Options{})
	if e != nil {
		return out, e
	}
	class, e := onlyClass(ast, className, baseName)
	if e != nil {
		return out, e
	}
	methods, e := classMethods(class)
	if e != nil {
		return out, e
	}
	visitor := &logicVisitor{}
	js.Walk(visitor, ast)
	if visitor.err != nil {
		return out, visitor.err
	}
	for _, global := range ast.Undeclared {
		if !safeGlobals[string(global.Name())] && string(global.Name()) != baseName {
			return out, fmt.Errorf("global %q is unavailable in CodeXArtifacts logic", global.Name())
		}
	}
	for _, member := range class.List {
		refs := &classReferences{className: className, baseName: baseName}
		js.Walk(refs, member.Method)
		out.ReferencesClass = out.ReferencesClass || refs.class
		out.ReferencesBase = out.ReferencesBase || refs.base
	}
	out.ClassNameSpan, out.BaseNameSpan = classHeaderSpans(source)
	for name := range methods {
		out.Methods = append(out.Methods, name)
	}
	sort.Strings(out.Methods)
	return out, nil
}

func onlyClass(ast *js.AST, name, base string) (*js.ClassDecl, error) {
	var found *js.ClassDecl
	for _, statement := range ast.List {
		if _, comment := statement.(*js.Comment); comment {
			continue
		}
		c, ok := statement.(*js.ClassDecl)
		if !ok || found != nil {
			return nil, fmt.Errorf("logic must contain exactly one class declaration and no other top-level program")
		}
		found = c
	}
	if found == nil || found.Name == nil || string(found.Name.Name()) != name {
		return nil, fmt.Errorf("logic requires class %s", name)
	}
	parent, ok := found.Extends.(*js.Var)
	if !ok || string(parent.Name()) != base {
		return nil, fmt.Errorf("class %s must extend %s", name, base)
	}
	return found, nil
}

var safeGlobals = words("undefined NaN Infinity Promise Math JSON Object Array String Number Boolean BigInt Date Intl Map Set WeakMap WeakSet RegExp Error TypeError RangeError SyntaxError URIError EvalError AggregateError parseInt parseFloat isNaN isFinite encodeURI encodeURIComponent decodeURI decodeURIComponent")

type logicVisitor struct {
	depth int
	err   error
}

func (v *logicVisitor) Enter(node js.INode) js.IVisitor {
	if v.err != nil {
		return nil
	}
	v.depth++
	if v.depth > 128 {
		v.err = fmt.Errorf("logic AST nesting exceeds 128")
		v.depth--
		return nil
	}
	switch n := node.(type) {
	case *js.WithStmt:
		v.err = fmt.Errorf("with statements are unsupported")
	case *js.ImportStmt, *js.ExportStmt, *js.ImportMetaExpr:
		v.err = fmt.Errorf("module imports and exports are unsupported")
	case *js.LiteralExpr:
		if n.TokenType == js.ImportToken {
			v.err = fmt.Errorf("dynamic imports are unsupported")
		}
	case *js.IndexExpr:
		if forbiddenIndex(n.Y) {
			v.err = fmt.Errorf("prototype traversal is unsupported")
		}
	case *js.Var:
		name := string(n.Name())
		if name == "eval" || name == "Function" {
			v.err = fmt.Errorf("dynamic code generation is unsupported")
		}
	case *js.DotExpr:
		if !safeName(n.Y.String()) {
			v.err = fmt.Errorf("prototype traversal is unsupported")
		}
	}
	return v
}
func (v *logicVisitor) Exit(js.INode) { v.depth-- }

type classReferences struct {
	className, baseName string
	class, base         bool
}

func (v *classReferences) Enter(n js.INode) js.IVisitor {
	if name, ok := n.(*js.Var); ok {
		v.class = v.class || string(name.Name()) == v.className
		v.base = v.base || string(name.Name()) == v.baseName
	}
	return v
}
func (v *classReferences) Exit(js.INode) {}
func classHeaderSpans(source string) (SourceSpan, SourceSpan) {
	lexer := js.NewLexer(parse.NewInputString(source))
	offset := 0
	identifiers := []SourceSpan{}
	for len(identifiers) < 2 {
		kind, raw := lexer.Next()
		span := SourceSpan{"", offset, offset + len(raw)}
		offset += len(raw)
		if kind == js.ErrorToken {
			break
		}
		if kind == js.IdentifierToken {
			identifiers = append(identifiers, span)
		}
	}
	if len(identifiers) != 2 {
		return SourceSpan{}, SourceSpan{}
	}
	return identifiers[0], identifiers[1]
}

func classMethods(class *js.ClassDecl) (map[string]bool, error) {
	methods := map[string]bool{}
	for _, member := range class.List {
		if member.StaticBlock != nil || member.Method == nil {
			return nil, fmt.Errorf("class supports instance methods only; fields and static blocks are unsupported")
		}
		m := member.Method
		name := string(m.Name.Literal.Data)
		if m.Static || m.Get || m.Set || m.Name.IsComputed() || m.Name.Private != nil || m.Name.Literal.TokenType != js.IdentifierToken || !safeName(name) {
			return nil, fmt.Errorf("class methods require unreserved literal instance names")
		}
		if methods[name] {
			return nil, fmt.Errorf("duplicate class method %q", name)
		}
		methods[name] = true
	}

	return methods, nil
}

func forbiddenIndex(expr js.IExpr) bool {
	literal, ok := expr.(*js.LiteralExpr)
	if !ok || literal.TokenType != js.StringToken {
		return false
	}
	var b strings.Builder
	if e := literal.JSON(&b); e != nil {
		return false
	}
	var value string
	if e := json.Unmarshal([]byte(b.String()), &value); e != nil {
		return false
	}
	return value == "constructor" || value == "prototype" || value == "__proto__"
}
