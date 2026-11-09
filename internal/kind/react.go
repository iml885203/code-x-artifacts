package kind

import (
	"crypto/sha256"
	_ "embed" // react_theme.css, react_guide.md
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/jsruntime"
)

//go:embed react_theme.css
var reactTheme string

//go:embed react_guide.md
var reactGuide string

// MaxReactBytes is the largest React component source.
const MaxReactBytes = 1 << 20

// maxBundleBytes is the largest script or stylesheet the compile may produce.
// Files make it possible to build one far bigger than the source that was
// allowed before, and the page carries it inline.
const maxBundleBytes = 16 << 20

// bundleLimit is the limit a React kind enforces: its own when set (tests), else maxBundleBytes.
func (r React) bundleLimit() int {
	if r.MaxBundleBytes > 0 {
		return r.MaxBundleBytes
	}
	return maxBundleBytes
}

// maxReactProblems bounds the error an agent reads; the first ones are the ones to fix.
const maxReactProblems = 20

// React is one TSX file whose default export is a component, optionally with
// sibling files it imports (./Chart, ./lib/format.ts, ./data.json,
// ./styles.css): the platform compiles them all into the page's one script
// with esbuild when it is published (nothing is stored but the sources) and
// serves it with the libraries of the runtime the artifact was published under. Its format version is that runtime's name, so the libraries
// a page sees never change under it (principle 7).
//
// The page the kind produces is made of platform-owned scripts only, and
// RenderPage names them so the server can pin script-src to their hashes
// (docs/script-src.md).
type React struct {
	// MaxBundleBytes is the largest compiled script or stylesheet; the 16 MB
	// default when zero. Tests that probe the limit set a small one.
	MaxBundleBytes int
}

// Name implements Kind.
func (React) Name() string { return "react" }

// FormatVersions implements Kind: one format per embedded runtime.
func (React) FormatVersions() []string { return []string{string(jsruntime.V1)} }

// Guide implements Kind.
func (React) Guide() string { return reactGuide }

// BrowserFormats implements BrowserExporter: the page prints itself to PDF in
// the browser, as an html page does.
func (React) BrowserFormats() []string { return []string{"pdf"} }

// Validate implements Kind: the component has to compile in the format's runtime.
func (r React) Validate(content, format string) error { return r.ValidateFiles(content, format, nil) }

// AcceptedFileTypes implements FileKind: the types every page's files may have,
// and the sources the component may import.
func (React) AcceptedFileTypes() []string { return append(files.Types(), files.SourceTypes()...) }

// ValidateFiles implements Bundler: the component and the files it imports have
// to compile together.
func (r React) ValidateFiles(content, format string, fs files.Set) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("a react artifact needs content: the source of one component, with `export default function App() { ... }`")
	}
	if len(content) > MaxReactBytes {
		return tooLarge("a react artifact can be at most %d MB of source, this one is %d bytes", MaxReactBytes>>20, len(content))
	}
	_, err := compileReact(content, format, fs, r.bundleLimit())
	return err
}

// Render implements Kind.
func (r React) Render(content, format string) (string, error) {
	p, err := r.RenderPage(content, format)
	return p.Titled("Page").HTML, err
}

// RenderPage implements PageRenderer.
func (r React) RenderPage(content, format string) (Page, error) {
	return compileReact(content, format, nil, r.bundleLimit())
}

// RenderPageFiles implements Bundler.
func (r React) RenderPageFiles(content, format string, fs files.Set) (Page, error) {
	return compileReact(content, format, fs, r.bundleLimit())
}

// compiled remembers the page of each (content, files, runtime) triple:
// publishing validates, and every view after that renders, and the compile is
// the same work each time. Entries never go stale because the key is the
// content and the files.
var compiled = struct {
	sync.Mutex
	pages map[[sha256.Size]byte]Page
}{pages: map[[sha256.Size]byte]Page{}}

const compiledCacheEntries = 256

// reactCompiles counts compiles that missed the cache; a test reads it to
// prove a second view of the same content does not compile again.
var reactCompiles atomic.Int64

// compileKey names a compile: the runtime, the component and every file, so a
// change to any of them is a different entry.
func compileKey(content, format string, fs files.Set, limit int) [sha256.Size]byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d\x00", limit) + format + "\x00" + content))
	paths := make([]string, 0, len(fs))
	for p := range fs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		h.Write([]byte("\x00" + p + "\x00" + fs[p]))
	}
	var key [sha256.Size]byte
	copy(key[:], h.Sum(nil))
	return key
}

func compileReact(content, format string, fs files.Set, limit int) (Page, error) {
	key := compileKey(content, format, fs, limit)
	compiled.Lock()
	page, ok := compiled.pages[key]
	compiled.Unlock()
	if ok {
		return page, nil
	}
	rt, err := reactRuntime(format)
	if err != nil {
		return Page{}, err
	}
	reactCompiles.Add(1)
	js, css, err := compileComponent(content, fs, rt, limit)
	if err != nil {
		return Page{}, err
	}
	page, err = assembleReactPage(js, css, rt)
	if err != nil {
		return Page{}, err
	}
	compiled.Lock()
	if len(compiled.pages) >= compiledCacheEntries {
		for k := range compiled.pages { // any entry: they all cost one compile to bring back
			delete(compiled.pages, k)
			break
		}
	}
	compiled.pages[key] = page
	compiled.Unlock()
	return page, nil
}

func reactRuntime(format string) (*jsruntime.Runtime, error) {
	if !slices.Contains(React{}.FormatVersions(), format) {
		return nil, fmt.Errorf("a react artifact has no content format %q; the ones this server has are: %s", format, strings.Join(React{}.FormatVersions(), ", "))
	}
	return jsruntime.Load(jsruntime.Version(format))
}

// reactProblem is one compile error written so an agent can act on it.
type reactProblem struct {
	line, column int
	text, source string
}

func (p reactProblem) String() string {
	if p.line == 0 {
		return p.text
	}
	return fmt.Sprintf("line %d:%d: %s\n    %s", p.line, p.column, p.text, strings.TrimSpace(p.source))
}

// reactCompileError carries every problem found, not just the first, so the
// agent fixes them in one round.
type reactCompileError struct{ problems []reactProblem }

func (e *reactCompileError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "the component does not compile (%d problem(s)):", len(e.problems))
	for i, p := range e.problems {
		if i == maxReactProblems {
			fmt.Fprintf(&b, "\n- ... and %d more", len(e.problems)-maxReactProblems)
			break
		}
		b.WriteString("\n- " + p.String())
	}
	return b.String()
}

const (
	reactEntry     = "pictor:entry"
	reactComponent = "pictor:component"
)

// reactEntrySource mounts the component under an error boundary: a component
// that throws shows its message and stack in the page, not a blank frame.
const reactEntrySource = `
import App from "pictor:component";
import { createRoot } from "react-dom/client";
import { createElement, Component } from "react";
class Boundary extends Component {
  constructor(props) { super(props); this.state = { error: null }; }
  static getDerivedStateFromError(error) { return { error }; }
  render() {
    if (!this.state.error) return this.props.children;
    return createElement("pre", { style: { padding: 16, whiteSpace: "pre-wrap", color: "var(--danger)" } }, String((this.state.error && this.state.error.stack) || this.state.error));
  }
}
createRoot(document.getElementById("root")).render(createElement(Boundary, null, createElement(App)));
`

func specifierList(rt *jsruntime.Runtime) string {
	names := make([]string, 0, len(rt.Manifest.Specifiers))
	for s := range rt.Manifest.Specifiers {
		names = append(names, s)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// siblingNamespace is where esbuild finds the artifact's own files: not "file",
// which it keeps for real paths on a disk, and there is none here.
const siblingNamespace = "sibling"

// sourceExtensions are tried, in this order, when an import names a file
// without its extension (what esbuild does by default).
var sourceExtensions = []string{".tsx", ".ts", ".jsx", ".js", ".mjs", ".json", ".css"}

// loaders say how each kind of file an import may reach is compiled.
var loaders = map[string]api.Loader{
	".tsx": api.LoaderTSX, ".ts": api.LoaderTS, ".jsx": api.LoaderJSX,
	".js": api.LoaderJS, ".mjs": api.LoaderJS, ".json": api.LoaderJSON, ".css": api.LoaderCSS,
}

// resolver decides what an import may mean. A relative import is one of the
// artifact's own files, found from the importing file's folder and never outside
// the artifact's files; any other name must be a module of the runtime. In the
// validating pass every runtime module becomes a stub that exports exactly the
// names the real one has, so esbuild itself reports `import { Fire } from
// "lucide-react"` with its line, instead of the page going blank in the browser.
func resolver(src string, fs files.Set, rt *jsruntime.Runtime, validate bool) api.Plugin {
	return api.Plugin{Name: "pictor", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: `.*`}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			switch {
			case a.Path == reactEntry || a.Path == reactComponent:
				return api.OnResolveResult{Path: a.Path, Namespace: "pictor"}, nil
			case a.Kind == api.ResolveCSSURLToken:
				// A url() in a stylesheet is the page's own business, resolved by
				// the browser against the page's address.
				return api.OnResolveResult{Path: a.Path, External: true}, nil
			case strings.HasPrefix(a.Path, "./") || strings.HasPrefix(a.Path, "../"):
				return resolveSibling(a, fs)
			case strings.HasPrefix(a.Path, "/") || strings.Contains(a.Path, "://"):
				return api.OnResolveResult{}, fmt.Errorf("cannot import %q: import one of the artifact's files by a relative path (./Chart), or only from: %s", a.Path, specifierList(rt))
			}
			if _, ok := rt.Manifest.Specifiers[a.Path]; !ok {
				return api.OnResolveResult{}, fmt.Errorf("cannot import %q: this server provides only: %s", a.Path, specifierList(rt))
			}
			if validate {
				return api.OnResolveResult{Path: a.Path, Namespace: "stub"}, nil
			}
			return api.OnResolveResult{Path: a.Path, External: true}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "pictor"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			contents := src
			loader := api.LoaderTSX
			if a.Path == reactEntry {
				contents, loader = reactEntrySource, api.LoaderJS
			}
			return api.OnLoadResult{Contents: &contents, Loader: loader, ResolveDir: "/"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: siblingNamespace}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			loader, ok := loaders[strings.ToLower(path.Ext(a.Path))]
			if !ok {
				return api.OnLoadResult{}, fmt.Errorf("cannot import %q: only .tsx, .ts, .jsx, .js, .mjs, .json and .css files can be imported", a.Path)
			}
			contents := fs[a.Path]
			return api.OnLoadResult{Contents: &contents, Loader: loader, ResolveDir: "/"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "stub"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			var sb strings.Builder
			for _, n := range rt.Manifest.Specifiers[a.Path].Exports {
				if n == "default" {
					sb.WriteString("export default 0;")
				} else {
					fmt.Fprintf(&sb, "export const %s = 0;", n)
				}
			}
			contents := sb.String()
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderJS}, nil
		})
	}}
}

// resolveSibling finds the file a relative import names. The importing file is
// the component (at the root of the artifact's files) or another file, and
// the import is read from its folder.
func resolveSibling(a api.OnResolveArgs, fs files.Set) (api.OnResolveResult, error) {
	dir := ""
	if a.Namespace == siblingNamespace {
		dir = path.Dir(a.Importer)
	}
	target := path.Join(dir, a.Path)
	if target == ".." || strings.HasPrefix(target, "../") {
		return api.OnResolveResult{}, fmt.Errorf("cannot import %q: it leaves the artifact's files; an import stays inside them", a.Path)
	}
	candidates := []string{target}
	for _, ext := range sourceExtensions {
		candidates = append(candidates, target+ext)
	}
	for _, c := range candidates {
		if _, ok := fs[c]; ok {
			return api.OnResolveResult{Path: c, Namespace: siblingNamespace}, nil
		}
	}
	if len(fs) == 0 {
		return api.OnResolveResult{}, fmt.Errorf("cannot import %q: this artifact has no files besides the component. Publish the file in files (for example {%q: \"...\"}), or import only from the runtime's modules", a.Path, target+".tsx")
	}
	names := make([]string, 0, len(fs))
	for p := range fs {
		names = append(names, p)
	}
	sort.Strings(names)
	return api.OnResolveResult{}, fmt.Errorf("cannot import %q: there is no file %q (nor %s); the artifact's files are: %s", a.Path, target, strings.Join(sourceExtensions[:3], ", ")+"…", strings.Join(names, ", "))
}

func reactProblems(msgs []api.Message) *reactCompileError {
	e := &reactCompileError{}
	for _, m := range msgs {
		p := reactProblem{text: m.Text}
		if m.Location != nil {
			p.line, p.column, p.source = m.Location.Line, m.Location.Column+1, m.Location.LineText
		}
		if strings.Contains(p.text, "No matching export") {
			p.text = strings.Replace(strings.Replace(p.text, "No matching export in ", "", 1), `"stub:`, `"`, 1)
			p.text = strings.Replace(p.text, " for import ", " has no export named ", 1)
		}
		// The component's default export is how the entry finds it.
		if strings.Contains(p.text, `named "default"`) && strings.Contains(p.text, reactComponent) {
			p = reactProblem{text: "the file needs `export default function App() { ... }`: the component CodeXArtifacts mounts"}
		}
		e.problems = append(e.problems, p)
	}
	return e
}

// compileComponent returns the page's module script and the stylesheet the
// imported CSS files make (empty when none is imported). Two builds: the first
// only validates (syntax, specifiers, named imports, the default export) and
// its output is thrown away; the second leaves the runtime's modules as
// imports for the page's import map to resolve.
func compileComponent(src string, fs files.Set, rt *jsruntime.Runtime, limit int) (js, css string, err error) {
	common := api.BuildOptions{
		EntryPoints: []string{reactEntry},
		Bundle:      true,
		Write:       false,
		// Imported CSS needs somewhere to go; nothing is written.
		Outdir:   "/out",
		Format:   api.FormatESModule,
		LogLevel: api.LogLevelSilent,
		JSX:      api.JSXAutomatic,
		Target:   api.ES2022,
	}
	check := common
	check.Plugins = []api.Plugin{resolver(src, fs, rt, true)}
	if r := api.Build(check); len(r.Errors) > 0 {
		return "", "", reactProblems(r.Errors)
	}
	emit := common
	emit.Plugins = []api.Plugin{resolver(src, fs, rt, false)}
	emit.MinifyWhitespace, emit.MinifySyntax = true, true
	emit.LegalComments = api.LegalCommentsNone
	r := api.Build(emit)
	if len(r.Errors) > 0 {
		return "", "", reactProblems(r.Errors)
	}
	for _, f := range r.OutputFiles {
		switch strings.ToLower(path.Ext(f.Path)) {
		case ".js":
			js = string(f.Contents)
		case ".css":
			css = string(f.Contents)
		}
	}
	if js == "" || len(r.OutputFiles) > 2 {
		return "", "", fmt.Errorf("compile react component: %d output files, want one script and at most one stylesheet", len(r.OutputFiles))
	}
	if len(js) > limit || len(css) > limit {
		return "", "", fmt.Errorf("the compiled component is %d bytes of script and %d of style; the page carries it inline, so each is at most %d MB. Import less, or move data to a .json file the page fetches", len(js), len(css), limit>>20)
	}
	// esbuild already writes `</script` as `<\/script` in strings, regexps and
	// templates. `<!--` it leaves alone, and followed by `<script` it puts the
	// HTML parser in a state where `</script>` no longer closes the element. A
	// module cannot contain it outside a literal (it would be a syntax error),
	// so `\x3C` there means the same text.
	js = strings.ReplaceAll(js, "<!--", `\x3C!--`)
	if strings.Contains(strings.ToLower(js), "</script") {
		return "", "", errors.New("the compiled component contains `</script`, which would end the page's script; remove it from the source")
	}
	if strings.Contains(strings.ToLower(css), "</style") {
		return "", "", errors.New("an imported stylesheet contains `</style`, which would end the page's style element; remove it from the file")
	}
	return js, css, nil
}

// reactErrorScript shows an error that happens before React mounts (a throw
// while the component's module runs), which the error boundary cannot see.
const reactErrorScript = `addEventListener("error",function(e){var r=document.getElementById("root");if(r&&!r.firstChild){var p=document.createElement("pre");p.style.cssText="padding:16px;white-space:pre-wrap;color:var(--danger)";p.textContent=String(e.error&&e.error.stack||e.message);r.appendChild(p)}});`

// assembleReactPage is the document around the compiled component: the kit in
// a layer below Tailwind's (so the kit's plain-element styles never outrank a
// utility class), the import map from the runtime's manifest, Tailwind's
// browser build, the theme, and the component's script.
func assembleReactPage(js, css string, rt *jsruntime.Runtime) (Page, error) {
	base := "/vendor/" + string(rt.Manifest.Runtime) + "/"
	imports := make(map[string]string, len(rt.Manifest.Specifiers))
	for s, m := range rt.Manifest.Specifiers {
		imports[s] = base + m.File
	}
	importMap, err := json.Marshal(map[string]any{"imports": imports})
	if err != nil {
		return Page{}, fmt.Errorf("encode import map: %w", err)
	}
	tailwind, ok := rt.Manifest.Scripts["tailwind"]
	if !ok {
		return Page{}, fmt.Errorf("runtime %s has no tailwind script", rt.Manifest.Runtime)
	}
	scripts := []string{string(importMap), reactErrorScript, js}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<meta name=\"color-scheme\" content=\"dark light\">\n")
	b.WriteString("<title>")
	titleAt := b.Len()
	b.WriteString("</title>\n")
	b.WriteString("<style>@layer theme, kit, base, components, utilities;\n@import url(\"/kit/v5/pictor.css\") layer(kit);</style>\n")
	b.WriteString("<script type=\"importmap\">" + scripts[0] + "</script>\n")
	b.WriteString("<script src=\"" + base + tailwind + "\"></script>\n")
	b.WriteString("<style type=\"text/tailwindcss\">\n" + reactTheme + "</style>\n")
	if css != "" {
		// The stylesheets the component imports, as the author wrote them.
		b.WriteString("<style>\n" + css + "</style>\n")
	}
	b.WriteString("<script>" + scripts[1] + "</script>\n</head>\n<body>\n<div id=\"root\"></div>\n")
	b.WriteString("<script type=\"module\">" + scripts[2] + "</script>\n</body>\n</html>\n")
	return Page{HTML: b.String(), InlineScripts: scripts, ScriptPaths: []string{"/vendor/"}, titleAt: titleAt}, nil
}
