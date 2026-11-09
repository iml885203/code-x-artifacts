package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func (s *Server) agentActivity(w http.ResponseWriter, r *http.Request) {
	q := service.ActivityQuery{TokenID: r.URL.Query().Get("tokenID"), Cursor: r.URL.Query().Get("cursor")}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			jsonError(w, http.StatusBadRequest, "limit must be 1 to 100")
			return
		}
		q.Limit = limit
	}
	out, err := s.svc.AgentActivity(r.Context(), currentViewer(r), q)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) routesAgentActivity(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent-activity", s.api(s.agentActivity))
}

type useResponse struct {
	http.ResponseWriter
	status int
}

func (w *useResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *useResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}
func (w *useResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// withTokenUse observes only a successful API result; auth/protocol errors and reading history never count.
func (s *Server) withTokenUse(w http.ResponseWriter, r *http.Request, p store.TokenProvenance, h http.HandlerFunc) {
	out := &useResponse{ResponseWriter: w}
	h(out, r)
	if r.URL.Path == "/api/agent-activity" || out.status < 200 || out.status >= 300 {
		return
	}
	if err := s.svc.RecordAgentUse(r.Context(), p, time.Now()); err != nil {
		s.log.Error("record token use", "err", err)
	}
}
