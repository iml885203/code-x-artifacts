package web

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// docBlocks is a doc as it is read to be edited (docs/doc-edit.md).
type docBlocks struct {
	Version int               `json:"version"`
	Tabs    []docedit.TabView `json:"tabs"`
	Outline []docedit.Outline `json:"outline"`
}

func (s *Server) getBlocks(w http.ResponseWriter, r *http.Request) {
	read, err := s.svc.ReadDoc(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, docBlocks{Version: read.Artifact.Version, Tabs: docedit.View(read.Doc), Outline: docedit.Outlines(read.Doc)})
}

func (s *Server) editDoc(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.editArtifact(w, r); !ok {
		return
	}
	var in struct {
		BaseVersion int          `json:"baseVersion"`
		Ops         []docedit.Op `json:"ops"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil || json.Unmarshal(body, &in) != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON {baseVersion?, ops: [{op: replace|insert|delete, ...}]}; under 10 MB")
		return
	}
	out, err := s.svc.DocEdit(r.Context(), currentViewer(r), r.PathValue("id"), service.DocEditInput{BaseVersion: in.BaseVersion, Ops: in.Ops})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// routesDocEdit registers reading a doc as blocks and changing some of them.
func (s *Server) routesDocEdit(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/blocks", s.api(s.getBlocks))
	mux.HandleFunc("POST /api/artifacts/{id}/edit", s.api(s.editDoc))
}
