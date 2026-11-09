package web

import "net/http"

// searchPeople answers the Docs editor's @ list: directory colleagues
// whose account or name contains q, for someone who may edit the doc.
func (s *Server) searchPeople(w http.ResponseWriter, r *http.Request) {
	people, err := s.svc.People(r.Context(), currentViewer(r), r.PathValue("id"), r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, err)
		return
	}
	state, err := s.store.DirectoryState(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "directory": state})
}

// routesPeople registers the people search.
func (s *Server) routesPeople(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/people", s.api(s.searchPeople))
}
