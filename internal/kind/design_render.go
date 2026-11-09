package kind

import (
	_ "embed" // design_overview.css
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

//go:embed design_overview.css
var designOverviewCSS string

// Render implements Kind.
func (d Design) Render(content, format string) (string, error) {
	p, err := d.RenderPage(content, format)
	return p.HTML, err
}

// RenderPage implements PageRenderer: the overview of the design, every frame
// as a thumbnail in a script-less frame of its own (the frame's page, at the
// address of its file next to this one), scaled to the width of their cell by CSS
// alone, so the page runs no script and a card of the gallery, whose preview has
// no allow-scripts, shows the same thumbnails (#49).
func (Design) RenderPage(content, format string) (Page, error) {
	if format != designFormat {
		return Page{}, fmt.Errorf("a design artifact has no content format %q", format)
	}
	m, problems := designfmt.Parse(content)
	if len(problems) > 0 {
		return Page{}, fmt.Errorf("design.json cannot be drawn: %s", problems[0])
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<meta name=\"color-scheme\" content=\"dark light\">\n<title>")
	titleAt := b.Len()
	b.WriteString("</title>\n<style>@layer kit;\n@import url(\"/kit/v1/pictor.css\") layer(kit);</style>\n<style>\n")
	b.WriteString(designOverviewCSS)
	writePrintPages(&b, m)
	b.WriteString("</style>\n</head>\n")
	writeOverviewBody(&b, m, "allow-scripts")
	b.WriteString("</html>\n")
	return Page{HTML: b.String(), titleAt: titleAt}, nil
}

// writeOverviewBody writes the <body> of the overview: every page, its rows, every
// frame as a thumbnail. sandbox is the sandbox attribute of the thumbnails' frames,
// empty for none.
func writeOverviewBody(b *strings.Builder, m designfmt.Manifest, sandbox string) {
	b.WriteString("<body>\n<main>\n")
	for _, page := range m.Pages {
		b.WriteString("<section class=\"page\">\n<h2>" + esc(page.Name) + "</h2>\n")
		for _, row := range page.Rows {
			b.WriteString("<div class=\"row\">\n")
			for _, f := range row {
				writeThumbnail(b, f, sandbox)
			}
			b.WriteString("</div>\n")
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</main>\n</body>\n")
}

// writePrintPages writes one named @page per frame size, so each printed page is
// as large as its frame; writeThumbnail puts every figure on the page of its size.
func writePrintPages(b *strings.Builder, m designfmt.Manifest) {
	seen := map[[2]int]bool{}
	for _, p := range m.Frames() {
		size := [2]int{p.Frame.W, p.Frame.H}
		if seen[size] {
			continue
		}
		seen[size] = true
		fmt.Fprintf(b, "@page %s { size: %dpx %dpx; margin: 0; }\n", printPageName(p.Frame), p.Frame.W, p.Frame.H)
	}
}

func printPageName(f designfmt.Frame) string { return fmt.Sprintf("s%dx%d", f.W, f.H) }

// writeThumbnail draws one frame: a box with the frame's proportions whose width
// in the row is proportional to the frame's own width, so every frame in a row is
// shown at the same scale.
func writeThumbnail(b *strings.Builder, f designfmt.Frame, sandbox string) {
	fmt.Fprintf(b, "<figure class=\"frame\" style=\"flex: %d 1 0; page: %s\">\n", f.W, printPageName(f))
	fmt.Fprintf(b, "<div class=\"thumb\" data-w=\"%d\" style=\"aspect-ratio: %d / %d; --w: %dpx; --h: %dpx\">", f.W, f.W, f.H, f.W, f.H)
	// The scale is the cell's width over the frame's: tan(atan2(a, b)) is a / b
	// as a number, which calc() cannot divide lengths into. The frame is not lazy:
	// the overview is what the browser prints, and a frame that has not loaded
	// prints as a blank page.
	if sandbox != "" {
		sandbox = " sandbox=\"" + sandbox + "\""
	}
	fmt.Fprintf(b, "<iframe src=\"%s\" title=\"%s\"%s tabindex=\"-1\" style=\"width: %dpx; height: %dpx; transform: scale(tan(atan2(100cqw, %dpx)))\"></iframe></div>\n", esc(f.File), esc(f.Name), sandbox, f.W, f.H, f.W)
	fmt.Fprintf(b, "<figcaption>%s <span>%d × %d</span></figcaption>\n</figure>\n", esc(f.Name), f.W, f.H)
}
