package kind

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/files"
)

// Formats implements Exporter: a design downloads as a zip. PDF is made by the
// browser (BrowserFormats), like a doc's.
func (Design) Formats() []string { return []string{"zip"} }

// BrowserFormats implements BrowserExporter: the browser prints the overview,
// whose print stylesheet gives every frame a page of its own size.
func (Design) BrowserFormats() []string { return []string{"pdf"} }

// Export implements Exporter. A design is its frames, so an export without them
// has nothing to hold; the server always goes through ExportBundle.
func (Design) Export(_, _, _, _ string) (Download, error) {
	return Download{}, errors.New("a design is exported with its frames: use ExportBundle")
}

// standaloneVars stand in for the kit's colour and font tokens in the zip's
// index.html, which loads nothing from any server.
const standaloneVars = `:root { --night: #14161a; --text: #e8e8ea; --muted: #9a9ca3; --line: #2b2e35; --font-sans: system-ui, sans-serif; --font-mono: ui-monospace, monospace; }
`

// localRef finds the addresses a frame may use for files that are not in the
// frame: an upload of an artifact (optionally with the address of a server in
// front) and a font of the platform. designfmt accepts no other local address.
var localRef = regexp.MustCompile(`(?:https?://[^/"'()\s,<>\\]+)?(/files/[A-Za-z0-9_-]+/[^"'()\s,<>\\]+|/static/fonts/[A-Za-z0-9._/-]+)`)

// noFile stands in for an address whose file could not be found (another
// artifact's, a deleted upload): nothing is requested and nothing is shown, so
// the zip still makes no request.
const noFile = "data:,"

// ExportBundle implements BundleExporter: a zip whose index.html shows every
// frame and opens from file:// with no request to any server.
//
//	index.html          the overview, thumbnails of frames/…
//	design.json         the manifest, as published
//	frames/<name>.html  the frames, their uploads and fonts rewritten to the files below
//	assets/<name>       the uploads the frames use
//	fonts/<path>        the platform fonts the frames use
//
// open finds the file an address in a frame points at; one it cannot find is
// replaced by an empty data: address instead of being left pointing at a server.
func (Design) ExportBundle(title, content, format, exportFormat, _ string, frames files.Set, open func(src string) (File, bool)) (Download, error) {
	if exportFormat != "zip" {
		return Download{}, fmt.Errorf("design artifacts export as zip, not %q", exportFormat)
	}
	if format != designFormat {
		return Download{}, fmt.Errorf("a design artifact has no content format %q", format)
	}
	m, problems := designfmt.Parse(content)
	if len(problems) > 0 {
		return Download{}, fmt.Errorf("design.json cannot be exported: %s", problems[0])
	}
	entries := map[string][]byte{"design.json": []byte(content)}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>" + html.EscapeString(title) + "</title>\n<style>\n")
	b.WriteString(standaloneVars + designOverviewCSS)
	writePrintPages(&b, m)
	b.WriteString("</style>\n</head>\n")
	writeOverviewBody(&b, m, "")
	b.WriteString("</html>\n")
	entries["index.html"] = []byte(b.String())

	for path, text := range frames {
		text, err := DesignKitFrame(text, open)
		if err != nil {
			return Download{}, err
		}
		entries[path] = []byte(localizeRefs(DesignTokenFrame(content, text), entries, open))
	}
	return zipEntries(entries)
}

// localizeRefs rewrites the files a frame points at to paths relative to the
// frame (frames/x.html reaches the zip's root by ../) and adds those files to entries.
func localizeRefs(text string, entries map[string][]byte, open func(src string) (File, bool)) string {
	return localRef.ReplaceAllStringFunc(text, func(match string) string {
		local := localRef.FindStringSubmatch(match)[1]
		var path string
		if rest, ok := strings.CutPrefix(local, "/static/fonts/"); ok {
			path = "fonts/" + rest
		} else {
			path = "assets/" + local[strings.LastIndex(local, "/")+1:]
		}
		if _, have := entries[path]; !have {
			var f File
			found := false
			if open != nil {
				f, found = open(match)
			}
			if !found {
				return noFile
			}
			entries[path] = f.Data
		}
		return "../" + path
	})
}

// zipEntries writes entries in a fixed order with no time stamps, so the same
// design is the same bytes.
func zipEntries(entries map[string][]byte) (Download, error) {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "index.html") != (names[j] == "index.html") {
			return names[i] == "index.html"
		}
		return names[i] < names[j]
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return Download{}, fmt.Errorf("add %s to the zip: %w", name, err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			return Download{}, fmt.Errorf("write %s into the zip: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return Download{}, fmt.Errorf("finish the zip: %w", err)
	}
	return Download{ContentType: "application/zip", Extension: "zip", Body: buf.Bytes()}, nil
}
