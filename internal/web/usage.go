package web

import (
	"net/http"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

// usage is the usage summary (service.Usage): ?from=&to= (YYYY-MM-DD),
// ?period=week|day, and ?by=department,kind (or by repeated).
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var by []string
	for _, v := range q["by"] {
		for b := range strings.SplitSeq(v, ",") {
			if b = strings.TrimSpace(b); b != "" {
				by = append(by, b)
			}
		}
	}
	out, err := s.svc.Usage(r.Context(), currentViewer(r), service.UsageQuery{From: q.Get("from"), To: q.Get("to"), Period: q.Get("period"), By: by})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// routesUsage registers the usage summary, for a person or their agent.
func (s *Server) routesUsage(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/usage", s.api(s.usage))
}
