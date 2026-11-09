package kind

import (
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// PreviewRenderer is implemented by kinds that draw a card of the library
// differently from their page. The server asks for it only for a preview.
type PreviewRenderer interface {
	Kind
	// RenderPreview is RenderPage for a card: the same inputs, a smaller page.
	RenderPreview(content, format string) (Page, error)
}

// RenderPreview implements PreviewRenderer: the first frame of the design in
// manifest order (Frames skips pages and rows that hold none), in the one
// script-less frame the card loads however many frames the design has, scaled
// to the width of the card and clipped by it. A design with no frame is an empty
// page. RenderPage, and so the overview, print and the full viewer, is untouched.
func (Design) RenderPreview(content, format string) (Page, error) {
	if format != designFormat {
		return Page{}, fmt.Errorf("a design artifact has no content format %q", format)
	}
	m, problems := designfmt.Parse(content)
	if len(problems) > 0 {
		return Page{}, fmt.Errorf("design.json cannot be drawn: %s", problems[0])
	}
	return previewPage(m), nil
}

// previewPage draws the preview of a manifest. Parse accepts no design without a
// frame, but a room can project one (a page whose frames were all deleted), and
// that is an empty card, not an error.
func previewPage(m designfmt.Manifest) Page {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>")
	titleAt := b.Len()
	b.WriteString("</title>\n<style>\n" + designPreviewCSS + "</style>\n</head>\n<body>\n")
	if frames := m.Frames(); len(frames) > 0 {
		f := frames[0].Frame
		fmt.Fprintf(&b, "<div class=\"thumb\" style=\"aspect-ratio: %d / %d\">", f.W, f.H)
		fmt.Fprintf(&b, "<iframe src=\"%s\" title=\"%s\" sandbox=\"allow-scripts\" tabindex=\"-1\" style=\"width: %dpx; height: %dpx; transform: scale(tan(atan2(100cqw, %dpx)))\"></iframe></div>\n", esc(f.File), esc(f.Name), f.W, f.H, f.W)
	}
	b.WriteString("</body>\n</html>\n")
	return Page{HTML: b.String(), titleAt: titleAt}
}

const designPreviewCSS = `html, body { margin: 0; overflow: hidden; }
.thumb { container-type: inline-size; position: relative; overflow: hidden; width: 100%; background: #fff; }
.thumb iframe { position: absolute; left: 0; top: 0; border: 0; transform-origin: 0 0; pointer-events: none; }
`
