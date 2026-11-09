package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// designData is what the canvas needs to draw a design in the shell: its
// manifest, and the address its frames are served at. The shell cannot read the
// redirect that /raw/{id} makes to that address, and the frames are separate
// documents, so it is told.
type designData struct {
	// Version is the version the manifest and the address are for.
	Version  int             `json:"version"`
	Manifest json.RawMessage `json:"manifest"`
	// Base is the address of the version's files, ending in a slash: a frame's
	// file is at Base + "frames/<name>.html". It carries a grant for exactly
	// this version.
	Base string `json:"base"`
	// Files is where a frame loads the artifact's uploaded files from, ending in
	// a slash: a file is at Files + "<name>". The markup of a served frame is
	// given it (assetGrants); the canvas gives it to what it builds in a frame.
	Files string `json:"files"`
	// Live is, for the newest version and someone who may join the design's
	// room, the address its frames are served at as the room holds them now,
	// ending in a slash; it does not change with the version. Empty otherwise.
	Live string `json:"live,omitempty"`
}

// designData answers GET /api/artifacts/{id}/design[?version=N]: the newest
// version, or an older one for the people who may read history, as /raw does.
func (s *Server) designData(w http.ResponseWriter, r *http.Request) {
	a, ok := s.visibleArtifact(w, r)
	if !ok {
		return
	}
	if a.Kind != "design" {
		jsonError(w, http.StatusBadRequest, "this artifact is not a design")
		return
	}
	shown := store.Version{Number: a.Version, Format: a.Format, Content: a.Content}
	if q := r.URL.Query().Get("version"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 {
			jsonError(w, http.StatusBadRequest, "version must be a number from 1")
			return
		}
		if shown, err = s.svc.Version(r.Context(), currentViewer(r), a.ID, n); err != nil {
			s.fail(w, err)
			return
		}
	}
	if !kind.HasFormat(kind.Design{}, shown.Format) {
		jsonError(w, http.StatusNotImplemented, "this server cannot draw designs of format "+shown.Format)
		return
	}
	out := designData{
		Version:  shown.Number,
		Manifest: json.RawMessage(shown.Content),
		Base:     pagePath(a.ID, s.sessions.pageGrant(a.ID, a.Visibility, shown.Number, time.Now()), ""),
		Files:    assets.PathPrefix(a.ID) + s.sessions.grant(a.ID, a.Visibility, time.Now()) + "/",
	}
	if v := currentViewer(r); !r.URL.Query().Has("version") && v.CanSeeLive(a) {
		out.Live = pagePath(a.ID, s.sessions.liveGrant(a.ID, v.Username, time.Now()), "")
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// routesDesign registers what the canvas reads.
func (s *Server) routesDesign(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/design", s.openAPI(s.designData))
}
