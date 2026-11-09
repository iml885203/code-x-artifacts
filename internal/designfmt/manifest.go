// Package designfmt is the format of a design artifact (docs/design-format.md):
// design.json, the manifest that names the pages and their frames, and the
// frames themselves, one static HTML file each. It holds the rules as pure
// functions: what a valid design is, with every problem named by its place, the
// canonical form a frame is stored in, and what a publisher cannot see for itself.
package designfmt

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

// The limits of a design, fixed in code so the guide can state the numbers.
const (
	MaxManifestBytes = 256 << 10
	MaxFrameBytes    = 512 << 10
	MaxFrames        = 60
	MaxPages         = 20
	MaxSide          = 8192
	MaxNameLength    = 80
	// FramesDir is where a frame's file lives.
	FramesDir = "frames"
	// Version is the one version of design.json.
	Version = 1
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// Manifest is a design.json that passed Parse.
type Manifest struct {
	Pages  []Page
	Tokens map[string]string
}

// Page is a named list of rows of frames.
type Page struct {
	ID, Name string
	Rows     [][]Frame
}

// Frame is one artboard: its id (unique in the design), name, the file that holds
// its HTML and its size in pixels.
type Frame struct {
	ID, Name, File string
	W, H           int
}

// Frames lists every frame with the page it is on, in reading order.
func (m Manifest) Frames() []Placed {
	var out []Placed
	for _, p := range m.Pages {
		for _, row := range p.Rows {
			for _, f := range row {
				out = append(out, Placed{Page: p, Frame: f})
			}
		}
	}
	return out
}

// Placed is a frame with its page.
type Placed struct {
	Page  Page
	Frame Frame
}

// Label is how a problem names the frame: Page "Home" frame "Hero".
func (p Placed) Label() string { return fmt.Sprintf("Page %q frame %q", p.Page.Name, p.Frame.Name) }

// Parse reads design.json. The manifest it returns holds what could be read, so
// the checks that follow can still look at the frames that are sound; problems
// lists everything wrong, each starting with where.
func Parse(content string) (Manifest, []string) {
	if len(content) > MaxManifestBytes {
		return Manifest{}, []string{fmt.Sprintf("design.json can be at most %d KB, this one is %d bytes", MaxManifestBytes>>10, len(content))}
	}
	var root any
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return Manifest{}, []string{fmt.Sprintf("design.json is not valid JSON: %v", err)}
	}
	if dec.More() {
		return Manifest{}, []string{"design.json has more than one JSON value; it is one object"}
	}
	p := &parser{}
	return p.manifest(root), p.problems
}

type parser struct {
	problems []string
	frameIDs map[string]string // id -> label of the frame that has it
	pageIDs  map[string]bool
	files    map[string]string // file -> label of the frame that uses it
	frames   int
}

func (p *parser) add(format string, args ...any) {
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
}

// object returns v as an object, refusing keys that are not in known.
func (p *parser) object(where string, v any, known ...string) (map[string]any, bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		p.add("%s must be an object", where)
		return nil, false
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !contains(known, k) {
			p.add("%s: unknown field %q (the fields are %s)", where, k, strings.Join(known, ", "))
		}
	}
	return obj, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (p *parser) manifest(root any) Manifest {
	obj, ok := p.object("design.json", root, "version", "pages", "tokens")
	if !ok {
		return Manifest{}
	}
	if v, isNum := obj["version"].(json.Number); !isNum || v.String() != fmt.Sprint(Version) {
		p.add("design.json: version must be %d, the version of this format", Version)
	}
	pages, ok := obj["pages"].([]any)
	switch {
	case !ok || len(pages) == 0:
		p.add("design.json: pages must be a list with at least one page")
		return Manifest{}
	case len(pages) > MaxPages:
		p.add("design.json: a design has at most %d pages, this one has %d", MaxPages, len(pages))
	}
	p.frameIDs, p.pageIDs, p.files = map[string]string{}, map[string]bool{}, map[string]string{}
	var m Manifest
	if raw, has := obj["tokens"]; has {
		m.Tokens = p.tokens(raw)
	}
	for i, raw := range pages {
		m.Pages = append(m.Pages, p.page(i, raw))
	}
	if p.frames > MaxFrames {
		p.add("design.json: a design has at most %d frames, this one has %d", MaxFrames, p.frames)
	}
	return m
}

func (p *parser) page(i int, raw any) Page {
	where := fmt.Sprintf("pages[%d]", i)
	obj, ok := p.object(where, raw, "id", "name", "rows")
	if !ok {
		return Page{}
	}
	pg := Page{ID: p.id(where, obj["id"], p.pageIDs, "page")}
	pg.Name = p.name(where, obj["name"])
	label := where
	if pg.Name != "" {
		label = fmt.Sprintf("Page %q", pg.Name)
	}
	rows, ok := obj["rows"].([]any)
	if !ok || len(rows) == 0 {
		p.add("%s: rows must be a list of rows, each a list of frames (at least one row)", label)
		return pg
	}
	for r, rawRow := range rows {
		row, ok := rawRow.([]any)
		if !ok || len(row) == 0 {
			p.add("%s: rows[%d] must be a list of frames (at least one)", label, r)
			continue
		}
		var frames []Frame
		for f, rawFrame := range row {
			frames = append(frames, p.frame(label, fmt.Sprintf("rows[%d][%d]", r, f), rawFrame))
		}
		pg.Rows = append(pg.Rows, frames)
	}
	return pg
}

func (p *parser) frame(pageLabel, slot string, raw any) Frame {
	p.frames++
	obj, ok := p.object(pageLabel+" "+slot, raw, "id", "name", "file", "w", "h")
	if !ok {
		return Frame{}
	}
	f := Frame{Name: p.name(pageLabel+" "+slot, obj["name"])}
	label := pageLabel + " " + slot
	if f.Name != "" {
		label = fmt.Sprintf("%s frame %q", pageLabel, f.Name)
	}
	f.ID = p.id(label, obj["id"], nil, "frame")
	if f.ID != "" {
		if other, dup := p.frameIDs[f.ID]; dup {
			p.add("%s: frame id %q is already used by %s; a frame id is unique in the design", label, f.ID, other)
		}
		p.frameIDs[f.ID] = label
	}
	f.File = p.file(label, obj["file"])
	f.W, f.H = p.side(label, "w", obj["w"]), p.side(label, "h", obj["h"])
	return f
}

func (p *parser) name(where string, v any) string {
	name, ok := v.(string)
	switch {
	case !ok || strings.TrimSpace(name) == "":
		p.add("%s: name is required, the text people see for it", where)
		return ""
	case len([]rune(name)) > MaxNameLength:
		p.add("%s: name is at most %d characters", where, MaxNameLength)
	}
	return name
}

// id reads a page or frame id; seen, when not nil, is where pages are counted.
func (p *parser) id(where string, v any, seen map[string]bool, what string) string {
	id, ok := v.(string)
	if !ok || !idPattern.MatchString(id) {
		p.add("%s: %s id must be 1 to 40 characters of lower case letters, digits, dash and underscore, starting with a letter or digit (for example \"hero\")", where, what)
		return ""
	}
	if seen != nil {
		if seen[id] {
			p.add("%s: %s id %q is used twice; ids are unique", where, what, id)
		}
		seen[id] = true
	}
	return id
}

func (p *parser) file(where string, v any) string {
	file, ok := v.(string)
	if !ok || file == "" {
		p.add("%s: file is required, the path of the frame's HTML such as frames/hero.html", where)
		return ""
	}
	if err := CheckFramePath(file); err != nil {
		p.add("%s: %v", where, err)
		return file
	}
	if other, dup := p.files[file]; dup {
		p.add("%s: file %s is already the file of %s; each frame has a file of its own", where, file, other)
	}
	p.files[file] = where
	return file
}

func (p *parser) side(where, field string, v any) int {
	n, ok := v.(json.Number)
	if ok {
		if i, err := n.Int64(); err == nil && i >= 1 && i <= MaxSide {
			return int(i)
		}
	}
	p.add("%s: %s must be a whole number of pixels from 1 to %d", where, field, MaxSide)
	return 0
}

var frameFile = regexp.MustCompile(`^` + FramesDir + `/[^/]+\.html$`)

// CheckFramePath refuses a path that is not a frame's: frames/<name>.html, one
// level deep, with the name rules of any file.
func CheckFramePath(p string) error {
	if err := files.CheckPath(p, []string{".html"}); err != nil {
		return err
	}
	if !frameFile.MatchString(p) || path.Dir(p) != FramesDir {
		return fmt.Errorf("file %q is not a frame's: frames are %s/<name>.html", p, FramesDir)
	}
	return nil
}

// Validate checks a design as a whole: the manifest, that every frame has its
// file, and that every frame's HTML follows the rules. fs is the text of the
// version's files by path. The error lists every problem, each with its place,
// in the order a person would fix them; nil means the design opens.
func Validate(content string, fs map[string]string) error {
	m, problems := Parse(content)
	problems = append(problems, checkFiles(m, fs)...)
	if len(problems) == 0 {
		return nil
	}
	const shown = 25
	extra := ""
	if len(problems) > shown {
		extra = fmt.Sprintf("\n… and %d more problems", len(problems)-shown)
		problems = problems[:shown]
	}
	return fmt.Errorf("this design cannot be saved (%d problems), nothing was stored:\n%s%s", len(problems), strings.Join(problems, "\n"), extra)
}

func checkFiles(m Manifest, fs map[string]string) []string {
	var problems []string
	used := map[string]bool{}
	for _, placed := range m.Frames() {
		file := placed.Frame.File
		if file == "" || used[file] {
			continue
		}
		used[file] = true
		text, ok := fs[file]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: file %s is not among this design's files; send it with files", placed.Label(), file))
			continue
		}
		if len(text) > MaxFrameBytes {
			problems = append(problems, fmt.Sprintf("%s: %s is %d bytes; a frame is at most %d KB%s", placed.Label(), file, len(text), MaxFrameBytes>>10, oversizeAdvice(text)))
			continue
		}
		for _, p := range checkSource(text) {
			problems = append(problems, placed.Label()+" "+p)
		}
	}
	paths := make([]string, 0, len(fs))
	for p := range fs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := CheckFramePath(p); err != nil {
			problems = append(problems, fmt.Sprintf("file %s: only frames are files of a design: %v", p, err))
		}
	}
	return problems
}

// oversizeAdvice says what to do about a frame that is too big: nearly always
// it is a picture or a font written into the markup as a data: address.
func oversizeAdvice(text string) string {
	advice := "; upload the image or font with upload_asset (or send it as a file of the design) and reference the returned URL instead of writing it into the frame"
	largest, kind := 0, ""
	for from := 0; ; {
		i := strings.Index(text[from:], "data:")
		if i < 0 {
			break
		}
		start := from + i
		end := start
		for end < len(text) && !strings.ContainsRune("\"')> \t\r\n", rune(text[end])) {
			end++
		}
		if n := end - start; n > largest {
			largest = n
			kind, _, _ = strings.Cut(text[start:end], ";")
			kind, _, _ = strings.Cut(kind, ",")
		}
		from = max(end, start+1)
	}
	if largest == 0 {
		return advice
	}
	return fmt.Sprintf("%s (the largest data: address is %d KB, a %s)", advice, (largest+1023)>>10, kind)
}
