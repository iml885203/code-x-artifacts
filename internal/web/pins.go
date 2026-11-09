package web

import "net/http"

// setPin pins (PUT) or unpins (DELETE) an artifact for the caller; both are idempotent.
func (s *Server) setPin(pinned bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.svc.Pin(r.Context(), currentViewer(r), r.PathValue("id"), pinned)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.view(r, a))
	}
}

// routesPins registers pinning.
func (s *Server) routesPins(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/artifacts/{id}/pin", s.api(s.setPin(true)))
	mux.HandleFunc("DELETE /api/artifacts/{id}/pin", s.api(s.setPin(false)))
}
