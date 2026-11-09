package web

import (
	"encoding/json"
	"net/http"
)

// startNew makes the caller's own blank doc, design or board: {kind, title?}.
func (s *Server) startNew(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, `body must be JSON {kind: "doc" | "design" | "board", title?}`)
		return
	}
	p, err := s.svc.CreateBlank(r.Context(), currentViewer(r), in.Kind, in.Title)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.publishedView(r, p))
}

// renameArtifact gives the artifact a new title: {title}. The author and the
// editors may; whoever cannot see the artifact gets the answer for a missing one.
func (s *Server) renameArtifact(w http.ResponseWriter, r *http.Request) {
	a, ok := s.editArtifact(w, r)
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, `body must be JSON {title}`)
		return
	}
	renamed, err := s.svc.Rename(r.Context(), currentViewer(r), a.ID, in.Title)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r, renamed))
}

// routesCreate registers starting a blank doc or design and renaming an artifact.
func (s *Server) routesCreate(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/new", s.api(s.startNew))
	mux.HandleFunc("PUT /api/artifacts/{id}/title", s.api(s.renameArtifact))
}
