package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// versionView is a saved version as the API shows it. Content is only there
// when one version is asked for.
type versionView struct {
	store.Version
	Content *string `json:"content,omitempty"`
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Versions(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	views := make([]versionView, len(list))
	for i, v := range list {
		views[i] = versionView{Version: v}
	}
	writeJSON(w, http.StatusOK, views)
}

// versionNumber reads the version in the path; the artifact is checked first
// so a malformed number never tells a stranger that the artifact exists.
func (s *Server) versionNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	if _, ok := s.editArtifact(w, r); !ok {
		return 0, false
	}
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || n < 1 {
		jsonError(w, http.StatusBadRequest, "the version in the path must be a number from 1")
		return 0, false
	}
	return n, true
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	n, ok := s.versionNumber(w, r)
	if !ok {
		return
	}
	v, err := s.svc.Version(r.Context(), currentViewer(r), r.PathValue("id"), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	content, err := service.Written(v.Kind, v.Format, v.Content)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, versionView{Version: v, Content: &content})
}

func (s *Server) restoreVersion(w http.ResponseWriter, r *http.Request) {
	n, ok := s.versionNumber(w, r)
	if !ok {
		return
	}
	// The body is optional: {"baseVersion": N} makes the restore refuse when
	// the artifact has moved on since the caller looked.
	var in struct {
		BaseVersion int `json:"baseVersion"`
	}
	if body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10)); err != nil || (len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &in) != nil) {
		jsonError(w, http.StatusBadRequest, "body is optional; when sent it must be JSON {baseVersion?}")
		return
	}
	a, err := s.svc.Restore(r.Context(), currentViewer(r), r.PathValue("id"), n, in.BaseVersion)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r, a))
}

// routesVersions registers version history.
func (s *Server) routesVersions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/versions", s.api(s.listVersions))
	mux.HandleFunc("GET /api/artifacts/{id}/versions/{version}", s.api(s.getVersion))
	mux.HandleFunc("POST /api/artifacts/{id}/versions/{version}/restore", s.api(s.restoreVersion))
}
