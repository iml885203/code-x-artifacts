package kind

import (
	_ "embed" // html_guide.md
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/files"
)

//go:embed html_guide.md
var htmlGuide string

// MaxHTMLBytes is the largest HTML artifact (the page itself; its files have a
// limit of their own, see internal/files).
const MaxHTMLBytes = 16 << 20

// HTML is a single self-contained HTML file that renders itself. The platform
// knows nothing about its content beyond the size, so the sandbox is the only
// thing between the page and the viewer.
type HTML struct{}

// Name implements Kind.
func (HTML) Name() string { return "html" }

// FormatVersions implements Kind: html has no versioned format.
func (HTML) FormatVersions() []string { return nil }

// Validate implements Kind.
func (HTML) Validate(content, _ string) error {
	if content == "" {
		return errors.New("an html artifact needs content: the page's HTML")
	}
	if len(content) > MaxHTMLBytes {
		return tooLarge("an html artifact can be at most %d MB, this one is %d bytes", MaxHTMLBytes>>20, len(content))
	}
	return nil
}

// AcceptedFileTypes implements FileKind.
func (HTML) AcceptedFileTypes() []string { return files.Types() }

// Render implements Kind: the page is its own document.
func (HTML) Render(content, _ string) (string, error) { return content, nil }

// Guide implements Kind.
func (HTML) Guide() string { return htmlGuide }

// Formats implements Exporter.
func (HTML) Formats() []string { return []string{"html"} }

// BrowserFormats implements BrowserExporter: the page prints itself to PDF in
// the browser, with its own print stylesheet; the server has no PDF, because
// that would mean a headless browser as a dependency.
func (HTML) BrowserFormats() []string { return []string{"pdf"} }

// Export implements Exporter: the original file, without the bridge.
func (HTML) Export(content, _, exportFormat, _ string) (Download, error) {
	if exportFormat != "html" {
		return Download{}, fmt.Errorf("html artifacts export as html, not %q", exportFormat)
	}
	return Download{ContentType: "text/html; charset=utf-8", Extension: "html", Body: []byte(content)}, nil
}
