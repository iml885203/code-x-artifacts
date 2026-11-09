//go:build ignore

// Lists declared Go and frontend tests for the AGENTS invariant-reference gate.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/scanner"
)

func main() {
	paths, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "--", "*_test.go", "*.test.ts", "*.test.tsx", "*.spec.ts", "*.spec.tsx").Output()
	if err != nil {
		fail(err)
	}
	for _, path := range strings.Split(strings.TrimSpace(string(paths)), "\n") {
		if path == "" {
			continue
		}
		if !strings.HasSuffix(path, "_test.go") {
			frontend(path)
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			fail(err)
		}
		aliases := map[string]bool{}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				fail(err)
			}
			if name == "testing" {
				alias := "testing"
				if imp.Name != nil {
					alias = imp.Name.Name
				}
				aliases[alias] = true
			}
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || fn.Type.TypeParams != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Type.Results != nil || len(fn.Type.Params.List) != 1 {
				continue
			}
			param := fn.Type.Params.List[0]
			if len(param.Names) > 1 {
				continue
			}
			pointer, ok := param.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			selector, ok := pointer.X.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "T" {
				continue
			}
			pkg, ok := selector.X.(*ast.Ident)
			if ok && aliases[pkg.Name] {
				fmt.Println(fn.Name.Name)
			}
		}
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "verify-agents-md:", err); os.Exit(2) }

// Only literal titles passed to test/it count. Scanning skips comments and
// strings containing example test calls, without requiring Node dependencies.
func frontend(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	var scan scanner.Scanner
	scan.Init(strings.NewReader(string(raw)))
	scan.Error = func(*scanner.Scanner, string) {} // TypeScript is not Go; only identifiers and quoted literals matter.
	for tok := scan.Scan(); tok != scanner.EOF; tok = scan.Scan() {
		if tok != scanner.Ident || (scan.TokenText() != "test" && scan.TokenText() != "it") {
			continue
		}
		tok = scan.Scan()
		if tok == '.' {
			scan.Scan() // only/skip/concurrent modifiers keep the same title contract.
			tok = scan.Scan()
		}
		if tok != '(' {
			continue
		}
		tok = scan.Scan()
		if tok != scanner.String && tok != scanner.Char && tok != scanner.RawString {
			continue
		}
		title := scan.TokenText()
		if strings.HasPrefix(title, "'") {
			title = `"` + strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(title, "'"), "'"), `"`, `\"`) + `"`
		}
		if name, err := strconv.Unquote(title); err == nil {
			fmt.Println(name)
		}
	}
}
