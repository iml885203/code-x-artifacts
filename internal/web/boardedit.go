package web

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/iml885203/code-x-artifacts/internal/boardedit"
	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
)

// boardRead is a board as it is read to be edited: as it is now, with the
// hashes board_edit checks.
type boardRead struct {
	Version int               `json:"version"`
	Content string            `json:"content"`
	Hashes  map[string]string `json:"hashes"`
}

func (s *Server) getBoard(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if a.Kind != "board" {
		jsonError(w, http.StatusBadRequest, "this artifact is a "+a.Kind+", not a board")
		return
	}
	if a, err = s.svc.Current(r.Context(), a); err != nil {
		s.fail(w, err)
		return
	}
	b, err := boardfmt.Parse(a.Content)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, boardRead{Version: a.Version, Content: a.Content, Hashes: boardedit.Hashes(b)})
}

func (s *Server) editBoard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.editArtifact(w, r); !ok {
		return
	}
	var in struct {
		Ops []boardedit.Op `json:"ops"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil || json.Unmarshal(body, &in) != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON {ops: [{op: add|change|delete, ...}]}; under 10 MB")
		return
	}
	out, err := s.svc.BoardEdit(r.Context(), currentViewer(r), r.PathValue("id"), in.Ops)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// routesBoardEdit registers reading a board with its hashes and changing some of its elements.
func (s *Server) routesBoardEdit(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/board", s.api(s.getBoard))
	mux.HandleFunc("POST /api/artifacts/{id}/board-edit", s.api(s.editBoard))
}
