// Package files holds the rules for the files that come with an artifact's
// page: which paths and types are accepted, how a publish's changes apply to
// the files of the version before it, and the limits. The rules are pure; the
// bytes live in internal/assets (blobs) and the manifest in internal/store.
// See docs/multi-file.md.
package files

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// Limits bound the files of one publish and one version. They are fixed in
// code so a tool description can tell an agent the numbers.
type Limits struct {
	// FileBytes is the largest single file.
	FileBytes int
	// PerPublish is how many files one publish may add, replace or remove.
	PerPublish int
	// PerVersion is how many files one version may hold.
	PerVersion int
	// TotalBytes is the size of all the files of one version together.
	TotalBytes int64
	// Accept is the extensions a file may have, lower case with the dot. Nil
	// means Types().
	Accept []string
}

// DefaultLimits are the numbers docs/multi-file.md documents.
var DefaultLimits = Limits{FileBytes: 16 << 20, PerPublish: 255, PerVersion: 511, TotalBytes: 256 << 20}

// MaxRequestBytes is the largest publish request that carries files. A
// request without files keeps the smaller limit of the HTTP API.
const MaxRequestBytes = 64 << 20

// Path rules.
const (
	maxDepth      = 5
	maxPathLength = 200
	// EntryPage is the name of the page a version is entered through; it is
	// the artifact's content, never a file.
	EntryPage = "index.html"
)

// Set is the text of each file of a version, by path.
type Set map[string]string

var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".json": "application/json; charset=utf-8",
	// Sources a kind compiles into its page. They are stored as text and
	// never served as files: see Servable.
	".tsx": "text/plain; charset=utf-8",
	".ts":  "text/plain; charset=utf-8",
	".jsx": "text/plain; charset=utf-8",
}

// Types lists the extensions a page's files may have.
func Types() []string { return []string{".html", ".js", ".mjs", ".css", ".json"} }

// SourceTypes lists the extensions of sources that a kind compiles into its
// page (TypeScript and JSX): a kind that bundles its files accepts them as well.
func SourceTypes() []string { return []string{".tsx", ".ts", ".jsx"} }

// Servable reports whether a file of this path is something a page can load.
// Sources that are only ever imported are not: asking for one is a missing file.
func Servable(p string) bool {
	return !slices.Contains(SourceTypes(), strings.ToLower(path.Ext(p)))
}

// TypesText names the accepted extensions for a refusal.
func TypesText(accept []string) string { return strings.Join(accept, ", ") }

func (l Limits) accept() []string {
	if l.Accept != nil {
		return l.Accept
	}
	return Types()
}

// ContentTypeOf is what a file of this path is served as, or "" for a path
// whose extension is not one of the known types.
func ContentTypeOf(p string) string { return contentTypes[strings.ToLower(path.Ext(p))] }

// Rewritable reports whether a file of this path can hold the URL of an
// uploaded asset, so a copy has to rewrite it (everything but JSON).
func Rewritable(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".html", ".css", ".js", ".mjs", ".tsx", ".ts", ".jsx":
		return true
	}
	return false
}

// CheckPath refuses a path that is malformed, escapes, uses a type that is not
// accepted or names the entry page. It never rewrites a path: what an agent
// wrote is what must be stored, or its relative references would break.
func CheckPath(p string, accept []string) error {
	switch {
	case p == "":
		return fmt.Errorf("a file path is empty; send paths like app.js or styles/site.css")
	case strings.HasPrefix(p, "/"):
		return fmt.Errorf("file path %q must be relative (app.js, not /app.js): the page refers to it by the same relative path", p)
	case len(p) > maxPathLength:
		return fmt.Errorf("file path %.40q… is %d characters; a path is at most %d", p, len(p), maxPathLength)
	}
	segments := strings.Split(p, "/")
	if len(segments) > maxDepth {
		return fmt.Errorf("file path %q is %d folders deep; a path has at most %d levels", p, len(segments), maxDepth)
	}
	for _, seg := range segments {
		if err := checkSegment(p, seg); err != nil {
			return err
		}
	}
	ext := strings.ToLower(path.Ext(p))
	if !slices.Contains(accept, ext) {
		return fmt.Errorf("file %q is not an accepted type: its extension must be one of %s", p, TypesText(accept))
	}
	if strings.EqualFold(p, EntryPage) {
		return fmt.Errorf("%s is the page itself: send it as content, not as a file", EntryPage)
	}
	return nil
}

// checkSegment checks one part of the path p.
func checkSegment(p, seg string) error {
	switch {
	case seg == "":
		return fmt.Errorf("file path %q has an empty part; write it like styles/site.css", p)
	case seg == "..":
		return fmt.Errorf("file path %q uses ..; paths cannot leave the artifact's files", p)
	case strings.HasPrefix(seg, "."):
		return fmt.Errorf("file path %q has a part that starts with a dot; names cannot start with a dot", p)
	}
	for _, r := range seg {
		if !nameRune(r) {
			return fmt.Errorf("file path %q has %q in it; a path has only letters, digits, dot, dash and underscore, separated by /", p, string(r))
		}
	}
	return nil
}

func nameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
}

// Check validates one file and returns the content type it is served with.
func Check(p, content string, accept []string) (string, error) {
	if err := CheckPath(p, accept); err != nil {
		return "", err
	}
	switch {
	case !utf8.ValidString(content):
		return "", fmt.Errorf("file %q is not UTF-8 text; only text files are accepted (images and fonts are uploaded as assets)", p)
	case strings.ContainsRune(content, 0):
		return "", fmt.Errorf("file %q contains a NUL byte; only text files are accepted", p)
	}
	if strings.ToLower(path.Ext(p)) == ".json" && !json.Valid([]byte(content)) {
		return "", fmt.Errorf("file %q is not valid JSON; fix it, a page that fetches it would fail", p)
	}
	return ContentTypeOf(p), nil
}

// Change is what a publish's files do to the files of the version before it.
type Change struct {
	// Put maps each added or replaced path to the content type it is served as.
	Put map[string]string
	// Remove is the paths taken out, sorted.
	Remove []string
	// Paths is every path of the new version, sorted.
	Paths []string
}

// Apply works out the files of the next version. base maps each path of the
// version being built on to its size; patch is what the publisher sent: a text
// replaces or adds the file, nil removes it, and whatever is not named is
// carried over. Every refusal names the limit or the path, and what to send.
func Apply(base map[string]int64, patch map[string]*string, lim Limits) (Change, error) {
	if len(patch) > lim.PerPublish {
		return Change{}, fmt.Errorf("this publish changes %d files; at most %d files in one publish, so send the rest in a later publish", len(patch), lim.PerPublish)
	}
	final := make(map[string]int64, len(base)+len(patch))
	for p, n := range base {
		final[p] = n
	}
	ch := Change{Put: map[string]string{}}
	for _, p := range sortedKeys(patch) {
		text := patch[p]
		if text == nil {
			if _, ok := base[p]; !ok {
				return Change{}, fmt.Errorf("cannot remove %q: this artifact has no such file; its files are %s", p, listOf(base))
			}
			delete(final, p)
			ch.Remove = append(ch.Remove, p)
			continue
		}
		if len(*text) > lim.FileBytes {
			return Change{}, fmt.Errorf("file %q is %d bytes; a file can be at most %d bytes", p, len(*text), lim.FileBytes)
		}
		ct, err := Check(p, *text, lim.accept())
		if err != nil {
			return Change{}, err
		}
		ch.Put[p] = ct
		final[p] = int64(len(*text))
	}
	if err := checkCollisions(final); err != nil {
		return Change{}, err
	}
	var total int64
	for _, n := range final {
		total += n
	}
	if len(final) > lim.PerVersion {
		return Change{}, fmt.Errorf("this version would hold %d files; a version holds at most %d files. Remove files that are no longer used by sending them as null", len(final), lim.PerVersion)
	}
	if total > lim.TotalBytes {
		return Change{}, fmt.Errorf("this version's files would add up to %d bytes; a version's files are at most %d bytes in all", total, lim.TotalBytes)
	}
	ch.Paths = sortedKeys(final)
	return ch, nil
}

func checkCollisions(paths map[string]int64) error {
	seen := make(map[string]string, len(paths))
	for _, p := range sortedKeys(paths) {
		lower := strings.ToLower(p)
		if other, ok := seen[lower]; ok {
			return fmt.Errorf("file paths %q and %q differ only in case (case is ignored when paths are compared); keep one", other, p)
		}
		seen[lower] = p
	}
	return nil
}

func listOf[V any](m map[string]V) string {
	if len(m) == 0 {
		return "none"
	}
	return strings.Join(sortedKeys(m), ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
