// Package kind defines what an artifact kind is and holds the registry of the
// kinds this build understands. A kind answers four questions about content:
// is it valid, what page shows it, how is it written well, and (optionally)
// how is it exported.
// Permissions, sharing, shared data and the bridge are not the kind's business;
// they work on any artifact and live outside this package.
package kind

import (
	"errors"
	"fmt"
	"html"
	"slices"
	"sort"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

// Default is the kind of an artifact published without naming one, and of
// every artifact that predates kinds.
const Default = "html"

// Kind is implemented once per artifact kind.
type Kind interface {
	// Name is the stable identifier stored with each artifact and sent by
	// publishers. It is lowercase and never changes once released.
	Name() string
	// FormatVersions lists the content format versions this build can validate
	// and render, oldest first; the last is the one new artifacts get. A kind
	// whose content has no versioned format (html) returns nil: its only
	// format is the empty string. A released version is never removed, because
	// artifacts published under it must keep rendering with it (principle 7).
	FormatVersions() []string
	// Validate refuses content this kind cannot show in the given format
	// version, including content over the kind's size limit. The error text is
	// returned to the publisher, so it must say what to fix.
	Validate(content, format string) error
	// Render returns the HTML document that /raw/{id} serves for the content,
	// built with the format version the artifact was published under, not the
	// newest one. The server adds the sandbox headers and injects the bridge
	// afterwards, so a kind cannot loosen the sandbox or leave the bridge out.
	Render(content, format string) (string, error)
	// Guide is the kind's authoring manual for a publisher that has never seen
	// it: what valid content is, what the page can and cannot do, and what
	// makes it good. Agents receive it through the MCP `kinds` tool, so it is
	// written to be acted on without any other document.
	Guide() string
}

// Page is a rendered document together with the text of each inline script it
// contains, exactly as it appears between the tags.
type Page struct {
	HTML          string
	InlineScripts []string
	// ScriptPaths are the paths on this site, each ending in a slash, that the
	// page loads scripts from besides its inline ones (the runtime libraries
	// under /vendor/). The server allows exactly these in script-src.
	ScriptPaths []string

	// titleAt is where the text of <title> goes in HTML; the title is not part
	// of any inline script, so naming a cached page never changes its hashes.
	titleAt int
}

// Titled returns the page with title (escaped) as the text of its <title>.
// The scripts are shared, not copied, and are the same ones.
func (p Page) Titled(title string) Page {
	if p.titleAt > 0 {
		p.HTML = p.HTML[:p.titleAt] + html.EscapeString(title) + p.HTML[p.titleAt:]
		p.titleAt = 0
	}
	return p
}

// PageRenderer is implemented by kinds whose pages are generated entirely by
// the platform, so every inline script is known when the page is made. The
// server then serves such a page with a script-src that allows only those
// scripts (by hash), the bridge and the platform's own files, which turns
// cross-user script injection into an error instead of code (docs/script-src.md).
// A kind whose page is the author's own markup (html) cannot offer this.
type PageRenderer interface {
	Kind
	// RenderPage is Render plus the inline scripts of the page it returns.
	RenderPage(content, format string) (Page, error)
}

// Reviewer is implemented by kinds that can tell a publisher something about
// what it just published that the publisher cannot see for itself, such as a
// slide that will not fit its canvas. The notes come back in the publish
// response; they never refuse a publish (that is Validate's job).
type Reviewer interface {
	Kind
	// Review returns the notes for content that Validate accepted. previous is
	// the content of the version being replaced, or empty for a new artifact.
	Review(content, format, previous string) []string
}

// Composer is implemented by kinds whose stored content is not exactly what a
// publisher writes: a doc is written as blocks that may lack ids, and the store
// keeps them with ids, in one canonical form.
type Composer interface {
	Kind
	// Compose returns the content to store for a publish in the given format
	// version. written is the publisher's content argument and structured its
	// blocks argument (empty when absent); previous is the content of the
	// version being replaced when it has the same format, else empty. The error
	// text is returned to the publisher. The result is still passed to Validate.
	Compose(format, written string, structured []byte, previous string) (string, error)
	// Written is the inverse: the stored content as a publisher writes it, so
	// that what read returns is what publish takes.
	Written(content, format string) (string, error)
}

// Bundle is what a version holds: its content and the text of its files. A
// live room holds a bundle, and a version is cut from it. Files is nil for a
// kind whose room holds no files.
type Bundle struct {
	Content string
	Files   files.Set
}

// FileKind is implemented by kinds whose content can come with files (a script,
// a stylesheet, data) at relative paths that the page refers to
// (docs/multi-file.md). A kind that does not implement it is refused any files
// rather than having them ignored.
type FileKind interface {
	Kind
	// AcceptedFileTypes lists the extensions a file of this kind may have,
	// lower case with the dot.
	AcceptedFileTypes() []string
}

// FileValidator is implemented by FileKinds whose content cannot be judged
// without the files of the same version (react: the component imports its
// siblings; design: every frame the manifest names needs its file). The service
// asks them through ValidateFiles whenever a version has files, and when a
// version is restored.
type FileValidator interface {
	FileKind
	// ValidateFiles is Validate for content that comes with files.
	ValidateFiles(content, format string, fs files.Set) error
}

// Bundler is implemented by FileValidators that compile their files into the
// page (react), so the page itself needs the files too.
type Bundler interface {
	FileValidator
	// RenderPageFiles is RenderPage for content that comes with files.
	RenderPageFiles(content, format string, fs files.Set) (Page, error)
}

// FileNormalizer is implemented by FileKinds that store their files in one
// canonical form (design: a frame parsed and written back once, with an id on
// every element). The service applies it to each file a publish sends, after
// the content has been validated, and tells the publisher which files changed.
type FileNormalizer interface {
	FileKind
	// NormalizeFile returns the form in which the file at path is stored. It is
	// idempotent: normalising its own result changes nothing.
	NormalizeFile(format, path, text string) (string, error)
}

// FileReviewer is Reviewer for kinds whose notes depend on the files: it looks
// at the files in changed (the paths this publish sent), within the whole set.
type FileReviewer interface {
	FileKind
	ReviewFiles(content, format string, fs files.Set, changed []string) []string
}

// FrameKind is implemented by FileKinds whose HTML files are not pages of the
// author's own but documents the platform frames itself (design: one frame per
// file). Such a file is served with a policy of its own, in which the only
// script is the one the kind adds, instead of the bridge.
type FrameKind interface {
	FileKind
	// RenderFrame returns the document served for the HTML file at path, with
	// the text of its inline scripts, which are the ones the policy allows.
	RenderFrame(path, markup string) (Page, error)
}

// Exporter is implemented by kinds whose content can be downloaded in other
// formats.
type Exporter interface {
	Kind
	// Formats lists the format names Export accepts.
	Formats() []string
	// Export converts content published under the content format version
	// `format` into the export format `exportFormat`. origin is the address
	// people reach this server at (https://host, no trailing slash), so an
	// export that leaves the server can name its files by an address that works
	// there; a kind with no files to name ignores it.
	Export(content, format, exportFormat, origin string) (Download, error)
}

// File is the bytes of a file that belongs to an artifact.
type File struct {
	Data        []byte
	ContentType string
}

// FileExporter is implemented by exporters whose download holds the artifact's
// own files (the pictures of a doc in a Word file). open finds the file an
// address in the content points at, or says it cannot; it may be nil, and the
// export then leaves the files out. The server never fetches anything from
// another server for an export.
type FileExporter interface {
	Exporter
	ExportWithFiles(content, format, exportFormat, origin string, open func(src string) (File, bool)) (Download, error)
}

// BundleExporter is implemented by exporters of kinds whose artifacts are made
// of files (a design: one file per frame), where the page alone is not the
// artifact. The server hands it the text of every file of the version being
// exported; open finds the uploads and platform fonts an address in a file points at,
// as FileExporter's does. An artifact with files is exported only by a kind
// that implements this.
type BundleExporter interface {
	Exporter
	ExportBundle(title, content, format, exportFormat, origin string, fs files.Set, open func(src string) (File, bool)) (Download, error)
}

// BrowserExporter is implemented by kinds with an export the browser makes and
// the server cannot: the viewer asks the page to print and the person saves the
// result as PDF. The names are not formats of the export route (it answers 404
// for them), and an agent has no path to one by design: it has `read`.
type BrowserExporter interface {
	Kind
	// BrowserFormats lists the formats the viewer produces in the browser.
	BrowserFormats() []string
}

// Download is one exported file.
type Download struct {
	ContentType string
	Extension   string
	Body        []byte
}

// all is the registry. Adding a kind means adding it here (docs/adding-a-kind.md).
var all = []Kind{HTML{}, Doc{}, React{}, Slides{}, Design{}, Board{}}

// Lookup returns the kind with the given name.
func Lookup(name string) (Kind, error) {
	for _, k := range all {
		if k.Name() == name {
			return k, nil
		}
	}
	return nil, &UnknownError{Name: name}
}

// CurrentFormat is the content format version a new artifact of the kind gets:
// the newest it lists, or "" for a kind without versioned content.
func CurrentFormat(k Kind) string {
	versions := k.FormatVersions()
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}

// HasFormat reports whether the kind can render content in the format version.
func HasFormat(k Kind, format string) bool {
	versions := k.FormatVersions()
	if len(versions) == 0 {
		return format == ""
	}
	return slices.Contains(versions, format)
}

// Add registers a kind for the life of the process and returns a function that
// removes it. Production kinds belong in all; this exists so tests can drive
// the service and the server with a kind that has several format versions
// before any shipped kind does.
func Add(k Kind) (remove func()) {
	all = append(all, k)
	return func() {
		for i := range all {
			if all[i] == k {
				all = append(all[:i], all[i+1:]...)
				return
			}
		}
	}
}

// Names lists the kinds this build understands, sorted.
func Names() []string {
	names := make([]string, len(all))
	for i, k := range all {
		names[i] = k.Name()
	}
	sort.Strings(names)
	return names
}

// NamesWithFiles lists the kinds whose content can come with files, sorted.
func NamesWithFiles() []string {
	var names []string
	for _, k := range all {
		if _, ok := k.(FileKind); ok {
			names = append(names, k.Name())
		}
	}
	sort.Strings(names)
	return names
}

// sizeError is a refusal because the content is over the kind's size limit.
type sizeError struct{ msg string }

func (e sizeError) Error() string { return e.msg }

func tooLarge(format string, args ...any) error { return sizeError{fmt.Sprintf(format, args...)} }

// IsTooLarge reports whether Validate refused content for being over the
// kind's size limit, which a publisher can fix by sending the content from a
// file instead of in the request (docs/large-publish.md).
func IsTooLarge(err error) bool {
	_, ok := errors.AsType[sizeError](err)
	return ok
}

// UnknownError reports a kind this build does not implement.
type UnknownError struct{ Name string }

func (e *UnknownError) Error() string {
	return fmt.Sprintf("unknown kind %q; the kinds on this server are: %s", e.Name, strings.Join(Names(), ", "))
}
