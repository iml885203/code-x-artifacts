package web

import (
	_ "embed" // the kit's files
	"io"
	"net/http"
	"strings"
)

// The kit is the stylesheet agents link from the pages they publish. A page
// runs in an opaque origin, so everything it loads is a cross-origin request
// without cookies: the kit and the fonts it names must be served to any origin
// and without a session. The path carries the version and the answer is
// immutable, so a page published today looks the same in ten years; changing
// the look means a new directory, never an edit.

// Every version is a directory that is never edited after it is released. The
// newest is what the guides, the react kind and the /kit reference point at;
// the older ones stay served, byte for byte, for the pages that link them.

//go:embed kit/v1/pictor.css
var kitV1CSS []byte

//go:embed kit/v1/sample.html
var kitV1Sample string

//go:embed kit/v2/pictor.css
var kitV2CSS []byte

//go:embed kit/v2/sample.html
var kitV2Sample string

//go:embed kit/v3/pictor.css
var kitV3CSS []byte

//go:embed kit/v3/sample.html
var kitV3Sample string

//go:embed kit/v4/pictor.css
var kitV4CSS []byte

//go:embed kit/v4/sample.html
var kitV4Sample string

//go:embed kit/v5/pictor.css
var kitCSS []byte

//go:embed kit/v5/sample.html
var kitSample string

const (
	kitStylesheetPath = "/kit/v5/pictor.css"
	kitSamplePath     = "/kit/v5/sample.html"
)

// kitVersion is one released version of the kit.
type kitVersion struct {
	dir    string // "v1": the directory under /kit/
	css    []byte
	sample string
}

var kitVersions = []kitVersion{
	{"v1", kitV1CSS, kitV1Sample},
	{"v2", kitV2CSS, kitV2Sample},
	{"v3", kitV3CSS, kitV3Sample},
	{"v4", kitV4CSS, kitV4Sample},
	{"v5", kitCSS, kitSample},
}

// anyOrigin lets a page in the sandbox (origin null) read what it loads. What it
// wraps is public and the request carries no credentials, so "*" leaks nothing.
func anyOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}

func serveKitStylesheet(css []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(css) // the client went away; nothing left to tell it
	}
}

// serveKitSample is the page the reference shows in each theme. It is a plain
// agent page: it only links the kit, and the theme is the one thing the
// request may force, as "light" or "dark". Any other theme is the system's.
func serveKitSample(sample string) http.HandlerFunc {
	samples := map[string]string{
		"":      sample,
		"light": strings.Replace(sample, "<html ", `<html data-theme="light" `, 1),
		"dark":  strings.Replace(sample, "<html ", `<html data-theme="dark" `, 1),
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox; frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		page, ok := samples[r.URL.Query().Get("theme")]
		if !ok {
			page = samples[""]
		}
		_, _ = io.WriteString(w, page) // the client went away; nothing left to tell it
	}
}

// handleKit serves every released version of the kit and its sample page.
func handleKit(mux *http.ServeMux) {
	for _, v := range kitVersions {
		mux.Handle("GET /kit/"+v.dir+"/pictor.css", anyOrigin(serveKitStylesheet(v.css)))
		mux.Handle("GET /kit/"+v.dir+"/sample.html", serveKitSample(v.sample))
	}
}

// kitReference is the page that shows every component of the kit in both themes.
func (s *Server) kitReference(w http.ResponseWriter, r *http.Request) {
	s.renderShell(w, r, pageData{page: "kit"})
}
