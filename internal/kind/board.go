package kind

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

//go:embed board_guide.md
var boardGuide string

// Board is a whiteboard for drawing ideas: flows, architecture and discussion
// sketches, as Excalidraw elements (internal/boardfmt). People draw on it live
// through its room (internal/boardroom); a Design is for how a page looks.
type Board struct{}

// Name implements Kind.
func (Board) Name() string { return "board" }

// FormatVersions implements Kind.
func (Board) FormatVersions() []string { return []string{boardfmt.Format} }

// Validate implements Kind.
func (Board) Validate(content, format string) error { return boardfmt.Validate(content, format) }

// Render implements Kind: the board drawn read-only (boardfmt.SVG), fitted to
// the page, with no script. It is what a reader without the editor sees (an
// older version, a full page, someone not signed in) and the gallery card.
func (Board) Render(content, format string) (string, error) {
	if err := boardfmt.Validate(content, format); err != nil {
		return "", fmt.Errorf("render board: %w", err)
	}
	b, _ := boardfmt.Parse(content)
	return `<!doctype html>
<html><head><meta charset="utf-8"><title></title>
<style>html,body{margin:0;height:100%;background:#fff}svg{display:block;width:100%;height:100%}</style></head>
<body>` + boardfmt.SVG(b) + `</body></html>
`, nil
}

// RenderPage implements PageRenderer: the page has no script of its own, so
// script-src allows the bridge alone.
func (k Board) RenderPage(content, format string) (Page, error) {
	page, err := k.Render(content, format)
	return Page{HTML: page, InlineScripts: []string{}}, err
}

// Compose implements Composer: a skeleton (labels on shapes, arrows from and to
// shapes) is expanded into the elements the editor draws (boardfmt.Expand), and
// a board is stored in the form a room gives back (boardfmt.Canonical), so a
// version cut from a room and one published are the same text for the same board.
func (Board) Compose(format, written string, structured []byte, _ string) (string, error) {
	if len(structured) > 0 {
		return "", errors.New(`a board has no blocks; send its {"version": 1, "elements": [...]} as content`)
	}
	if format != boardfmt.Format {
		return "", fmt.Errorf("a board has no content format %q; the only one is %q", format, boardfmt.Format)
	}
	full, err := boardfmt.Expand(written)
	if err != nil {
		return "", err
	}
	b, err := boardfmt.Parse(full)
	if err != nil {
		return "", err
	}
	return boardfmt.Encode(boardfmt.Canonical(b)), nil
}

// Written implements Composer: what is stored is what a publisher writes.
func (Board) Written(content, _ string) (string, error) { return content, nil }

// Formats implements Exporter: the board as SVG, drawn by the server, which is
// what an agent downloads. A person's export from an open board is the
// editor's own (web/src/board), with its hand-drawn strokes.
func (Board) Formats() []string { return []string{"svg"} }

// BrowserFormats implements BrowserExporter: a PNG is made in the browser.
func (Board) BrowserFormats() []string { return []string{"png"} }

// Export implements Exporter.
func (Board) Export(content, format, exportFormat, _ string) (Download, error) {
	if exportFormat != "svg" {
		return Download{}, fmt.Errorf("a board exports as svg, not %q", exportFormat)
	}
	if err := boardfmt.Validate(content, format); err != nil {
		return Download{}, fmt.Errorf("export board: %w", err)
	}
	b, _ := boardfmt.Parse(content)
	return Download{ContentType: "image/svg+xml", Extension: "svg", Body: []byte(boardfmt.SVG(b))}, nil
}

// Guide implements Kind.
func (Board) Guide() string { return boardGuide }
