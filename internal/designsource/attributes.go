package designsource

import (
	"strings"

	"github.com/tdewolff/parse/v2"
	sourcehtml "github.com/tdewolff/parse/v2/html"
)

// x/net/html deliberately drops duplicate attributes, including conflicting
// source PIDs. The source lexer retains their original attribute boundaries;
// it is used only to diagnose those otherwise-lost conflicts in a start token.
// HTML tree construction and attribute value decoding remain with x/net/html.
func duplicateAttributes(start string) []string {
	lexer := sourcehtml.NewLexer(parse.NewInputString(start))
	seen := map[string]bool{}
	var duplicates []string
	for {
		kind, _ := lexer.Next()
		if kind == sourcehtml.ErrorToken {
			return duplicates
		}
		if kind != sourcehtml.AttributeToken {
			continue
		}
		name := strings.ToLower(string(lexer.AttrKey()))
		if seen[name] {
			duplicates = append(duplicates, name)
		}
		seen[name] = true
	}
}
