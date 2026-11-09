// Package assets holds the files an artifact references (images, fonts, PDFs,
// CSV): what a file may be, what its name becomes, and where the bytes live.
// The rules here are pure; Disk is the only part that touches the file system.
package assets

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

// Limits. They are fixed in code because every deployment shares one set of
// rules an agent can be told in a tool description.
const (
	// MaxBytes is the largest single file.
	MaxBytes = 15 << 20
	// MaxFiles and MaxTotalBytes bound what one artifact may hold.
	MaxFiles      = 50
	MaxTotalBytes = 100 << 20
	// MaxNameLength is the longest sanitised name.
	MaxNameLength = 100
)

// Type is a kind of file an artifact may hold.
type Type struct {
	// ContentType is what the file is served as, whatever the upload claimed.
	ContentType string
	// Extensions are the file name endings that match it, lower case.
	Extensions []string
	// Active is true when a browser that opens the file on its own may run
	// something from it, so it is served with a sandbox policy of its own.
	Active bool
}

var (
	types = []Type{
		{ContentType: "image/png", Extensions: []string{".png"}},
		{ContentType: "image/jpeg", Extensions: []string{".jpg", ".jpeg"}},
		{ContentType: "image/gif", Extensions: []string{".gif"}},
		{ContentType: "image/webp", Extensions: []string{".webp"}},
		{ContentType: "image/svg+xml", Extensions: []string{".svg"}, Active: true},
		{ContentType: "application/pdf", Extensions: []string{".pdf"}},
		{ContentType: "font/woff", Extensions: []string{".woff"}},
		{ContentType: "font/woff2", Extensions: []string{".woff2"}},
		{ContentType: "font/ttf", Extensions: []string{".ttf"}},
		{ContentType: "font/otf", Extensions: []string{".otf"}},
		{ContentType: "text/csv; charset=utf-8", Extensions: []string{".csv"}},
	}
	byContentType = func() map[string]Type {
		m := make(map[string]Type, len(types))
		for _, t := range types {
			m[t.ContentType] = t
		}
		return m
	}()
)

// Allowed describes the accepted file types for people and agents.
const Allowed = "PNG, JPEG, GIF, WebP and SVG images, PDF, WOFF, WOFF2, TTF and OTF fonts, and UTF-8 CSV"

// TypeOf returns the type served under contentType, which is one of the
// values Detect returns.
func TypeOf(contentType string) (Type, bool) {
	t, ok := byContentType[contentType]
	return t, ok
}

// Detect decides what a file is from its bytes and checks that its name says
// the same thing. The type the uploader claims is never consulted: an upload
// is what its content is, so a script cannot be passed off as a picture.
func Detect(name string, data []byte) (Type, error) {
	if len(data) == 0 {
		return Type{}, errors.New("the file is empty")
	}
	ext := strings.ToLower(path.Ext(name))
	detected, ok := sniff(ext, data)
	if !ok {
		return Type{}, fmt.Errorf("this file is not an accepted type (%s); its content must really be one of them, whatever the file name says", Allowed)
	}
	if !slices.Contains(detected.Extensions, ext) {
		return Type{}, fmt.Errorf("the content is %s but the name ends in %q; name it with %s", detected.ContentType, ext, strings.Join(detected.Extensions, " or "))
	}
	return detected, nil
}

func sniff(ext string, data []byte) (Type, bool) {
	head := data[:min(len(data), 512)]
	switch ct := http.DetectContentType(head); {
	case isSVG(data):
		return byContentType["image/svg+xml"], true
	case strings.HasPrefix(ct, "text/") && ext == ".csv" && isPlainText(data):
		return byContentType["text/csv; charset=utf-8"], true
	default:
		t, ok := byContentType[ct]
		return t, ok
	}
}

// isPlainText accepts UTF-8 without NUL or control characters other than tab,
// newline and carriage return, which is what a CSV is.
func isPlainText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

const svgNamespace = "http://www.w3.org/2000/svg"

// isSVG reports whether the document's root element is an SVG element.
func isSVG(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	for {
		tok, err := d.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == "svg" && start.Name.Space == svgNamespace
		}
		if _, ok := tok.(xml.Directive); ok {
			// A DOCTYPE can declare entities; an SVG that needs one is not
			// something to host.
			return false
		}
	}
}

// Name turns the name an uploader gave into the name the file is stored and
// served under: no directories, only letters, digits, dot, dash and
// underscore, no leading dot, at most MaxNameLength characters. What it
// returns is what the caller must put in the artifact, so it is always handed
// back to the uploader.
func Name(given string) (string, error) {
	base := given
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	var b strings.Builder
	dash := false
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	name := strings.Trim(b.String(), ".-")
	if len(name) > MaxNameLength {
		ext := path.Ext(name)
		if len(ext) > 10 {
			ext = ""
		}
		name = name[:MaxNameLength-len(ext)] + ext
	}
	if name == "" || !strings.Contains(name, ".") {
		return "", fmt.Errorf("the file name %q has no usable name and extension; send something like chart.png", given)
	}
	return name, nil
}

// Path is where an asset is served, without the host.
func Path(artifactID, name string) string { return PathPrefix(artifactID) + name }

// PathPrefix is the start of every asset path of an artifact.
func PathPrefix(artifactID string) string { return "/files/" + artifactID + "/" }

// Readable is an io.ReadSeekCloser for serving a file.
type Readable interface {
	io.ReadSeeker
	io.Closer
}
