package kind

import (
	_ "embed" // design_guide.md, design_inspector.js
	"fmt"
	"regexp"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/files"
)

//go:embed design_guide.md
var designGuide string

// designInspector is the one script a frame runs, pinned by hash in the
// frame's policy. A string, so the server can hash exactly what the document holds.
//
//go:embed design_inspector.js
var designInspector string

// designFormat is the version of design.json and of the rules for its frames.
const designFormat = "1"

// Design is a manifest (design.json, the content) and one static HTML file per
// frame (the artifact's files). The frames are data, not programs: the
// platform draws them in a sandbox where only its own inspector runs.
type Design struct{}

// Name implements Kind.
func (Design) Name() string { return "design" }

// FormatVersions implements Kind.
func (Design) FormatVersions() []string { return []string{designFormat} }

// Validate implements Kind: design.json alone, so every frame it names is a
// frame without a file.
func (d Design) Validate(content, format string) error { return d.ValidateFiles(content, format, nil) }

// ValidateFiles implements FileValidator: design.json and the frames that come
// with it, judged together, every problem named by its place (docs/design-format.md).
func (Design) ValidateFiles(content, format string, fs files.Set) error {
	if format != designFormat {
		return fmt.Errorf("a design artifact has no content format %q; the only one is %q", format, designFormat)
	}
	return designfmt.Validate(content, fs)
}

// NormalizeFile implements FileNormalizer: a frame is stored in its normal
// form, with a data-pid on every element.
func (Design) NormalizeFile(_, _, text string) (string, error) { return designfmt.Normalize(text) }

// ReviewFiles implements FileReviewer.
func (Design) ReviewFiles(content, _ string, fs files.Set, changed []string) []string {
	return designfmt.Review(content, fs, changed)
}

// Guide implements Kind.
func (Design) Guide() string { return designGuide }

// AcceptedFileTypes implements FileKind: a frame is an HTML file.
func (Design) AcceptedFileTypes() []string { return []string{".html"} }

var (
	frameHeadClose = regexp.MustCompile(`(?i)</head\s*>`)
	frameHeadOpen  = regexp.MustCompile(`(?i)<head[^>]*>`)
	frameDoctype   = regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`)
)

// RenderFrame implements FrameKind: the frame's own markup with the inspector
// first in its head, after any doctype, so nothing of the author's runs before it.
func (Design) RenderFrame(_, markup string) (Page, error) {
	tag := "<script>" + designInspector + "</script>"
	html := tag + markup
	if loc := frameHeadOpen.FindStringIndex(markup); loc != nil {
		html = markup[:loc[1]] + tag + markup[loc[1]:]
	} else if loc := frameDoctype.FindStringIndex(markup); loc != nil {
		html = markup[:loc[1]] + tag + markup[loc[1]:]
	}
	return Page{HTML: html, InlineScripts: []string{designInspector}}, nil
}
