package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

// distFS is the front end as `vite build` leaves it. It is built, not
// committed: only the placeholder that keeps the folder in git is. `make web`
// builds it, the Dockerfile builds it in its own stage, and
// TestTheBundleEmbeddedInTheBinaryIsBuilt fails when it is missing.
//
//go:embed all:dist
var distFS embed.FS

func embeddedBundle() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(fmt.Sprintf("embedded dist: %v", err)) // a constant path inside an embed.FS
	}
	return sub
}

// bundle names the files of one build the shell has to load. Paths are
// relative to the bundle's root and carry the content hash in their name.
type bundle struct {
	// Entry is the script that starts the app.
	Entry string
	// CSS is the stylesheets the entry needs: its own and those of every chunk
	// it imports, since a module shared with a lazy chunk takes its styles
	// into the shared chunk.
	CSS []string
	// Preload is the chunks the entry imports, so the browser fetches them
	// beside the entry instead of after it.
	Preload []string
}

type manifestChunk struct {
	File    string   `json:"file"`
	IsEntry bool     `json:"isEntry"`
	CSS     []string `json:"css"`
	Imports []string `json:"imports"`
}

// loadBundle reads Vite's manifest. A bundle without a manifest or an entry is
// an error here, so a server that was built without its front end says so
// instead of serving a page that cannot start.
func loadBundle(fsys fs.FS) (bundle, error) {
	raw, err := fs.ReadFile(fsys, ".vite/manifest.json")
	if err != nil {
		return bundle{}, fmt.Errorf("front-end bundle has no manifest: %w", err)
	}
	var chunks map[string]manifestChunk
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return bundle{}, fmt.Errorf("parse front-end manifest: %w", err)
	}
	var entry *manifestChunk
	for _, c := range chunks {
		if c.IsEntry {
			entry = &c
			break
		}
	}
	if entry == nil {
		return bundle{}, errors.New("front-end manifest has no entry chunk")
	}
	b := bundle{Entry: entry.File, CSS: append([]string(nil), entry.CSS...)}
	seen := map[string]bool{}
	var preload func(imports []string)
	preload = func(imports []string) {
		for _, name := range imports {
			if seen[name] {
				continue
			}
			seen[name] = true
			b.Preload = append(b.Preload, chunks[name].File)
			b.CSS = append(b.CSS, chunks[name].CSS...)
			preload(chunks[name].Imports)
		}
	}
	preload(entry.Imports)
	return b, nil
}

// assetsHandler serves the built files under /assets/. Their names carry a
// content hash, so a browser may keep them for good.
func assetsHandler(fsys fs.FS) http.Handler {
	files, err := fs.Sub(fsys, "assets")
	if err != nil {
		panic(fmt.Sprintf("bundle assets: %v", err)) // a constant path
	}
	h := http.StripPrefix("/assets/", http.FileServerFS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		if info, err := fs.Stat(files, name); err != nil || info.IsDir() {
			http.NotFound(w, r) // also refuses a directory listing
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}
