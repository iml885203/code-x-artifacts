package web

import (
	"encoding/json"
	"net/http"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Comments(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// addComment checks the artifact before it reads the body, so a malformed body
// never tells a stranger that the artifact exists.
func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.svc.Comments(r.Context(), currentViewer(r), id); err != nil {
		s.fail(w, err)
		return
	}
	var in struct {
		Body     string          `json:"body"`
		ParentID string          `json:"parentId"`
		Anchor   json.RawMessage `json:"anchor"`
		ForAgent *bool           `json:"forAgent"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON {body, parentId?, anchor?, forAgent?}")
		return
	}
	c, err := s.svc.AddComment(r.Context(), currentViewer(r), id, service.NewComment{Body: in.Body, ReplyTo: in.ParentID, Anchor: in.Anchor, ForAgent: in.ForAgent})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) markCommentsSeen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.svc.Comments(r.Context(), currentViewer(r), id); err != nil {
		s.fail(w, err)
		return
	}
	var in struct {
		UpTo string `json:"upTo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON {upTo}: the id of the newest comment shown")
		return
	}
	if err := s.svc.MarkCommentsSeen(r.Context(), currentViewer(r), id, in.UpTo); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteComment(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("comment")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// routesComments registers comments.
func (s *Server) routesComments(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/comments", s.api(s.listComments))
	mux.HandleFunc("POST /api/artifacts/{id}/comments", s.api(s.addComment))
	mux.HandleFunc("POST /api/artifacts/{id}/comments/seen", s.api(s.markCommentsSeen))
	mux.HandleFunc("DELETE /api/artifacts/{id}/comments/{comment}", s.api(s.deleteComment))
}
