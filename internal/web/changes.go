package web

import (
	"net/http"
	"strconv"
)

// changesSince answers what happened to the artifact since ?sinceVersion=N or
// ?since=<RFC 3339>, the same as the changes_since tool. The artifact is checked
// before the query is read, so a malformed query never tells a stranger that
// the artifact exists.
func (s *Server) changesSince(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleArtifact(w, r); !ok {
		return
	}
	version := 0
	if text := r.URL.Query().Get("sinceVersion"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "sinceVersion must be a version number from 1; or send since, a time in RFC 3339")
			return
		}
		version = n
	}
	out, err := s.svc.ChangesSince(r.Context(), currentViewer(r), r.PathValue("id"), version, r.URL.Query().Get("since"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// routesChanges registers what changed since a version or a time.
func (s *Server) routesChanges(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/changes", s.openAPI(s.changesSince))
}
