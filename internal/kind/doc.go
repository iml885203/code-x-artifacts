package kind

import (
	_ "embed" // doc_guide.md, doc.css
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docx"
)

//go:embed doc_guide.md
var docGuide string

//go:embed doc.css
var docCSS string

// MaxDocBytes is the largest Markdown document a publisher may send.
const MaxDocBytes = 2 << 20

// docFormat is structured content: tabs of blocks with stable ids, as JSON
// (docs/doc-format.md, package docfmt). Markdown is what a publisher writes and
// what a reader gets back, never what is stored.
const docFormat = "2"

// Doc is a document the platform draws as a reading page. The content is data,
// not a program: the blocks are validated and drawn from escaped strings, so
// what the sandbox protects against is already unlikely to be there.
type Doc struct{}

// Name implements Kind.
func (Doc) Name() string { return "doc" }

// FormatVersions implements Kind. Format 1 (Markdown) was converted to this one
// when it was removed, so there is no other.
func (Doc) FormatVersions() []string { return []string{docFormat} }

// Validate implements Kind.
func (Doc) Validate(content, format string) error {
	if format != docFormat {
		return fmt.Errorf("a doc artifact has no content format %q; the only one is %q (blocks); send Markdown as content or blocks as blocks", format, docFormat)
	}
	return docfmt.Validate(content)
}

// checkMarkdown refuses Markdown that is empty, too big, not text, or the wrong
// thing altogether.
func checkMarkdown(content string) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("a doc artifact needs content: the document's Markdown, or blocks")
	}
	if len(content) > MaxDocBytes {
		return tooLarge("a doc artifact can be at most %d MB of Markdown, this one is %d bytes", MaxDocBytes>>20, len(content))
	}
	if !utf8.ValidString(content) {
		return errors.New("a doc artifact must be UTF-8 text; this content is not valid UTF-8")
	}
	if strings.ContainsRune(content, 0) {
		return errors.New("a doc artifact is text; this content contains a NUL byte")
	}
	if looksLikeHTMLPage(content) {
		return errors.New("this content is a complete HTML page, not Markdown; publish it with kind html, or write the document as Markdown")
	}
	return nil
}

// looksLikeHTMLPage catches the one mistake worth a dedicated message: an agent
// sending a finished page to the document kind, where raw HTML is dropped and
// the reader would see an empty document.
func looksLikeHTMLPage(content string) bool {
	head := strings.ToLower(strings.TrimSpace(content))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

// Render implements Kind.
func (Doc) Render(content, format string) (string, error) {
	if format != docFormat {
		return "", fmt.Errorf("doc has no content format %q", format)
	}
	d, err := docfmt.Parse(content)
	if err != nil {
		return "", fmt.Errorf("render doc: %w", err)
	}
	return docfmt.RenderPage(d, docCSS), nil
}

// RenderPage implements PageRenderer. The page has no script of its own, and
// declaring none (an empty list, not nil) is what makes the server's
// script-src allow the bridge alone, so a rendering bug that let document text
// become markup could still not make it run (docs/script-src.md).
func (d Doc) RenderPage(content, format string) (Page, error) {
	page, err := d.Render(content, format)
	return Page{HTML: page, InlineScripts: []string{}}, err
}

// Compose implements Composer: the blocks sent, given ids where they lack one,
// or the Markdown sent, converted, with the ids of the version it replaces kept
// for the blocks that did not change.
func (Doc) Compose(format, written string, structured []byte, previous string) (string, error) {
	switch {
	case format != docFormat:
		return "", fmt.Errorf("a doc artifact has no content format %q; the only one is %q (blocks)", format, docFormat)
	case len(structured) > 0 && strings.TrimSpace(written) != "":
		return "", errors.New("send content or blocks, not both")
	case len(structured) > 0:
		return docfmt.Normalize(structured, docfmt.NewID)
	}
	if err := checkMarkdown(written); err != nil {
		return "", err
	}
	var before *docfmt.Doc
	if d, err := docfmt.Parse(previous); previous != "" && err == nil {
		before = &d
	}
	d, err := docfmt.FromMarkdown(written, before, docfmt.NewID)
	if err != nil {
		return "", err
	}
	return docfmt.Canonical(d)
}

// Written implements Composer: the content as a publisher writes it, which is
// Markdown, so an agent that reads it, changes it and sends it back loses
// nothing but what Markdown cannot hold (which a note at the end names).
func (Doc) Written(content, format string) (string, error) {
	if format != docFormat {
		return "", fmt.Errorf("a doc artifact has no content format %q", format)
	}
	d, err := docfmt.Parse(content)
	if err != nil {
		return "", fmt.Errorf("read doc: %w", err)
	}
	return docfmt.ToMarkdown(d), nil
}

// Guide implements Kind.
func (Doc) Guide() string { return docGuide }

// Formats implements Exporter.
func (Doc) Formats() []string { return []string{"md", "docx"} }

// BrowserFormats implements BrowserExporter: a doc prints to PDF from the
// browser (its page has a print stylesheet); the server has no PDF, because
// that would mean a headless browser as a dependency.
func (Doc) BrowserFormats() []string { return []string{"pdf"} }

// Export implements Exporter: the Markdown the blocks convert to (see Written),
// where a picture uploaded to the doc is written by its full address so the file
// opens in an editor or anywhere else, and what Markdown cannot hold is named at
// the end; or a Word file (internal/docx), with the pictures that are not in the
// doc's own files as links.
func (d Doc) Export(content, format, exportFormat, origin string) (Download, error) {
	return d.ExportWithFiles(content, format, exportFormat, origin, nil)
}

// ExportWithFiles implements FileExporter: a Word file holds the pictures
// uploaded to the doc, which open finds.
func (Doc) ExportWithFiles(content, format, exportFormat, origin string, open func(src string) (File, bool)) (Download, error) {
	if exportFormat != "md" && exportFormat != "docx" {
		return Download{}, fmt.Errorf("doc artifacts export as md or docx, not %q", exportFormat)
	}
	if format != docFormat {
		return Download{}, fmt.Errorf("a doc artifact has no content format %q", format)
	}
	d, err := docfmt.Parse(content)
	if err != nil {
		return Download{}, fmt.Errorf("read doc: %w", err)
	}
	if exportFormat == "docx" {
		var images docx.Images
		if open != nil {
			images = func(src string) (docx.Image, bool) {
				f, ok := open(src)
				return docx.Image{Data: f.Data, Type: f.ContentType}, ok
			}
		}
		body, err := docx.Write(d, docfmt.Title(d), images)
		if err != nil {
			return Download{}, fmt.Errorf("write docx: %w", err)
		}
		return Download{ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Extension: "docx", Body: body}, nil
	}
	docfmt.AddressFiles(&d, origin)
	return Download{ContentType: "text/markdown; charset=utf-8", Extension: "md", Body: []byte(docfmt.ToMarkdown(d))}, nil
}
