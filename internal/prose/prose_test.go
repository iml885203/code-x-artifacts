// Package prose holds the scan that keeps the product's name written "CodeXArtifacts"
// wherever a person or an agent reads it as a sentence.
package prose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The lowercase word is an identifier everywhere it is glued to other
// identifier characters: the binary and the repo (`pictor`, scripts/pictor), the
// module path, host names (artifacts.example.com, pictor.example), env vars
// (PICTOR_TOKEN is upper case and never matches), `window.pictor`, the
// `pictor:tab` comment directives, the plugin (pictor@marketplace) and the
// kit file (pictor.css), and the name an MCP client gives the server (mcp add pictor).
// Code spans, fenced code and URLs are identifiers by
// construction and are removed before the scan.
var (
	fence      = regexp.MustCompile("(?s)(```|~~~).*?(```|~~~)")
	span       = regexp.MustCompile("`[^`\n]*`")
	url        = regexp.MustCompile(`https?://\S+|\]\([^)]*\)`)
	mcpName    = regexp.MustCompile(`mcp add (--transport http )?pictor`)
	htmlTag    = regexp.MustCompile(`<[^>\n]*>`)
	word       = regexp.MustCompile(`pictor`)
	identChars = "_/:@-"
)

// mask blanks the parts of text that are identifiers by construction, keeping
// every offset where it was.
func mask(text string, markdown bool) string {
	b := []byte(text)
	res := []*regexp.Regexp{url, htmlTag, mcpName}
	if markdown {
		res = []*regexp.Regexp{fence, span, url, htmlTag, mcpName}
	}
	for _, re := range res {
		for _, m := range re.FindAllIndex(b, -1) {
			for i := m[0]; i < m[1]; i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
		}
	}
	return string(b)
}

// hit is one lowercase "pictor" used as the product's name: its offset in the
// text it was found in.
type hit struct{ at int }

func hits(text string) []hit { return hitsIn(text, true) }

// In front-end source a backtick opens a template string, not a code span.
func hitsIn(text string, markdown bool) []hit {
	masked := mask(text, markdown)
	var out []hit
	for _, m := range word.FindAllStringIndex(masked, -1) {
		start, end := m[0], m[1]
		before, after := byte(' '), byte(' ')
		if start > 0 {
			before = masked[start-1]
		}
		if end < len(masked) {
			after = masked[end]
		}
		if isAlnum(before) || strings.ContainsRune(identChars+"./$", rune(before)) {
			continue
		}
		if isAlnum(after) || strings.ContainsRune(identChars+"?", rune(after)) {
			continue
		}
		// pictor.db is an identifier; "by pictor." ends a sentence.
		if after == '.' && end+1 < len(masked) && isAlnum(masked[end+1]) {
			continue
		}
		// A quoted name is a literal: realm="pictor".
		if (before == '"' || before == '\'') && after == before {
			continue
		}
		out = append(out, hit{start})
	}
	return out
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func around(text string, at int) string {
	from, to := max(0, at-30), min(len(text), at+36)
	return strings.ReplaceAll(text[from:to], "\n", " ")
}

func root(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "internal")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no repository root above the test")
		}
		dir = parent
	}
}

// skipDir names the directories that are not the repository's own prose: tools, vendored packages, build output
// and scratch space. .cache holds the Go module and build caches in CI (thousands of other people's files).
func skipDir(name string) bool {
	switch name {
	case ".git", ".work", ".local", ".tools", ".claude", ".cache", ".tmp", "node_modules", "dist", "bin", "testdata", "interop":
		return true
	}
	return false
}

// isSource says a front-end file is read whole: its comments and the text it
// shows are prose, and what is not prose (`window.pictor`, URLs, the pictor-boot
// block) is glued to identifier characters. Tests name hosts and fixtures freely.
func isSource(rel string) bool {
	return (strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".tsx")) &&
		!strings.HasSuffix(rel, ".test.ts") && !strings.HasSuffix(rel, ".test.tsx") && !strings.HasSuffix(rel, ".spec.ts")
}

// A span is a stretch of a file that is read as prose.
type span2 struct{ from, to int }

// proseSpans says which parts of a file are prose: all of a Markdown or locale
// file, and the string literals of Go code that hold a space (one without is an
// identifier, a key or a path, not a sentence).
func proseSpans(t *testing.T, rel string, data []byte) []span2 {
	t.Helper()
	switch {
	case strings.HasSuffix(rel, ".md"), isSource(rel) && strings.HasPrefix(rel, "web/src/"), strings.HasSuffix(rel, ".json") && strings.Contains(rel, "i18n/locales/"):
		return []span2{{0, len(data)}}
	case strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") && strings.HasPrefix(rel, "internal/"):
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []span2
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.ContainsAny(lit.Value, " \n") {
				out = append(out, span2{fset.Position(lit.Pos()).Offset, fset.Position(lit.End()).Offset})
			}
			return true
		})
		return out
	}
	return nil
}

type found struct {
	path string
	at   int // offset in the file
	text string
}

func scanRepo(t *testing.T) []found {
	t.Helper()
	base := root(t)
	var all []found
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".md") && !strings.HasSuffix(rel, ".json") && !strings.HasSuffix(rel, ".go") && !isSource(rel) {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: a file found by walking the repository
		if err != nil {
			return err
		}
		for _, sp := range proseSpans(t, rel, data) {
			text := string(data[sp.from:sp.to])
			for _, h := range hitsIn(text, !isSource(rel)) {
				all = append(all, found{rel, sp.from + h.at, around(text, h.at)})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func TestTheProductIsNamedCodeXArtifactsInProse(t *testing.T) {
	var lines []string
	for _, f := range scanRepo(t) {
		lines = append(lines, f.path+": ..."+f.text+"...")
	}
	if len(lines) > 0 {
		t.Errorf("the product is written CodeXArtifacts in prose (identifiers, code spans and URLs may stay lowercase):\n%s", strings.Join(lines, "\n"))
	}
}

func TestTheScanTellsProseFromIdentifiers(t *testing.T) {
	for text, want := range map[string]int{
		"CodeXArtifacts hosts pages":                   0,
		"pictor hosts pages":                           1,
		"call pictor's API":                            1,
		"hosted by pictor.":                            1,
		"the `pictor` binary and `window.pictor`":      0,
		"https://artifacts.example.com/api/mcp":        0,
		"artifacts.example.com and pictor.example":     0,
		"scripts/pictor and /kit/v2/pictor.css":        0,
		"<!-- pictor:tab Title -->":                    0,
		"window.pictor and pictor.db":                  0,
		"```\nawait pictor.whoami()\n```\nthen pictor": 1,
		"a [link](https://x/pictor/y) to pictor":       1,
		`Bearer realm="pictor"`:                        0,
		"set PICTOR_TOKEN":                             0,
		"install pictor@example-marketplace":           0,
	} {
		if got := len(hits(text)); got != want {
			t.Errorf("%q: %d hits, want %d", text, got, want)
		}
	}
}
