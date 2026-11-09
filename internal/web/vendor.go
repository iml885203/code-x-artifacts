package web

import (
	"io"
	"net/http"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/jsruntime"
)

// /vendor/{runtime}/{file} serves the libraries react artifacts import. Like
// the kit, a page in the sandbox reads them as a cross-origin request without
// cookies (module scripts are fetched with CORS), so they are open to any
// origin and need no session; they are public libraries, not anyone's data.
// Every file but the licence notices carries a hash of its content in its name,
// so the answer never changes and is cached for a year.

func serveVendor(w http.ResponseWriter, r *http.Request) {
	rt, err := jsruntime.Load(jsruntime.Version(r.PathValue("runtime")))
	name := r.PathValue("file")
	if err != nil || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	// Only what the manifest lists is a runtime file; the manifest itself is not.
	if _, ok := rt.Manifest.Files[name]; !ok {
		http.NotFound(w, r)
		return
	}
	f, err := rt.FS.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if name == rt.Manifest.Notices {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = io.Copy(w, f) // the client went away; nothing left to tell it
}
