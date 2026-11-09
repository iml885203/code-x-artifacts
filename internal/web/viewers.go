package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

// maxHeartbeatBytes bounds a heartbeat's body, which holds at most a guest id.
const maxHeartbeatBytes = 1 << 10

// listViewers answers who has the artifact open now, as the viewers field of
// the read tool does. Only signed-in people who can see the artifact get it.
func (s *Server) listViewers(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Viewers(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// heartbeat records that the caller has the artifact open and answers with who
// the caller may see has it open, so a page needs one call each time it is due.
// The body is optional: {"guest": "<the browser's random guest id>"} lists a
// signed-out browser as a guest, and lets a signed-in one take its guest's place.
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Guest string `json:"guest"`
	}
	if body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHeartbeatBytes)); err != nil || (len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &in) != nil) {
		jsonError(w, http.StatusBadRequest, "body is optional; when sent it must be JSON {guest?} of at most 1 KB")
		return
	}
	list, err := s.svc.Heartbeat(r.Context(), currentViewer(r), r.PathValue("id"), in.Guest)
	if service.CodeOf(err) == service.Busy {
		// A full guest list frees up as guests leave: a page tries again once a person would have left.
		w.Header().Set("Retry-After", strconv.Itoa(int(s.svc.ViewerStays().Seconds())))
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	// The page beats at the pace this server forgets people at.
	w.Header().Set("CodeXArtifacts-Heartbeat-Ms", strconv.FormatInt(s.svc.ViewerBeat().Milliseconds(), 10))
	writeJSON(w, http.StatusOK, list)
}

// routesViewers registers who is looking at an artifact. The heartbeat is open
// to the anonymous viewer of a public link, who is listed as a guest; the list
// itself is not.
func (s *Server) routesViewers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/viewers", s.api(s.listViewers))
	mux.HandleFunc("POST /api/artifacts/{id}/viewers", s.openAPI(s.heartbeat))
}
