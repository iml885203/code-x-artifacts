package web

import "net/http"

// recordOpen notes that the artifact was opened in the viewer: it counts in the
// usage, and goes on a signed-in person's own "Recently opened" list
// (GET /api/artifacts?order=opened).
func (s *Server) recordOpen(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.Open(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r, a))
}

// routesOpens registers the viewer's opens. Only a browser opens a page in the
// viewer, signed in or as the anonymous reader of a public link; what an agent
// reads is not an open, so a token is refused.
func (s *Server) routesOpens(mux *http.ServeMux) {
	open := s.openAPI(s.recordOpen)
	mux.HandleFunc("POST /api/artifacts/{id}/open", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			jsonError(w, http.StatusUnauthorized, "only the viewer in a browser records an open; an agent's read is not one")
			return
		}
		open(w, r)
	})
}
