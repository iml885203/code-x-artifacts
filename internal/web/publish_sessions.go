package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A publish session carries a page that already exists as a file: begin
// announces it, the bytes go to one-use URLs, commit publishes
// (docs/large-publish.md). Begin and commit are API calls like any other; the
// upload is the one route that takes no credential, because its URL is one.

const (
	beginShape = `body must be JSON {id?, title, kind?, format?, baseVersion?, visibility?, files: [{path, sha256, size}], remove?: [paths]}; the page itself is the file with path "content"`
	// maxBeginBytes bounds a begin request: a publish names at most 255 files.
	maxBeginBytes = 1 << 20
	// uploadReadTime is how long an upload may take to arrive.
	uploadReadTime = 5 * time.Minute
)

type beginInput struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Kind        string              `json:"kind"`
	Format      string              `json:"format"`
	BaseVersion int                 `json:"baseVersion"`
	Visibility  store.Visibility    `json:"visibility"`
	Files       []store.SessionFile `json:"files"`
	Remove      []string            `json:"remove"`
}

func (s *Server) beginPublish(w http.ResponseWriter, r *http.Request) {
	var in beginInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBeginBytes)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, beginShape)
		return
	}
	begun, err := s.svc.BeginPublish(r.Context(), currentViewer(r), service.BeginInput{
		ArtifactID: in.ID, Title: in.Title, Kind: in.Kind, Format: in.Format, BaseVersion: in.BaseVersion,
		Visibility: in.Visibility, Files: in.Files, Remove: in.Remove,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, begun.View(baseURL(r)))
}

func (s *Server) commitPublish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceBases map[string]string `json:"sourceBases,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBeginBytes)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		jsonError(w, http.StatusBadRequest, "body must be JSON {sourceBases?: {path: sourceBase}}")
		return
	}
	p, created, err := s.svc.CommitPublish(r.Context(), currentViewer(r), r.PathValue("session"), in.SourceBases)
	if err != nil {
		s.fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, s.publishedView(r, p))
}

// putUpload takes the bytes for one upload URL as the raw request body. The
// secret in the path is the whole credential: no cookie or token is read, so
// there is nothing for a cross-site request to ride on.
func (s *Server) putUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTime)); err != nil {
		s.log.Debug("no read deadline for an upload", "err", err)
	}
	body := http.MaxBytesReader(w, r.Body, int64(s.svc.FileLimits().FileBytes)+1)
	if err := s.svc.ReceiveUpload(r.Context(), r.PathValue("secret"), body); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"uploaded": true})
}

// refuseUploadMethod answers any other method on an upload URL with what to send.
func refuseUploadMethod(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", http.MethodPut)
	jsonError(w, http.StatusMethodNotAllowed, "send the file as the body of a PUT: curl --fail-with-body -X PUT --data-binary @file URL")
}

// routesPublishSessions registers publishing from a file.
func (s *Server) routesPublishSessions(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/publish-sessions", s.api(s.beginPublish))
	mux.HandleFunc("POST /api/publish-sessions/{session}/commit", s.api(s.commitPublish))
	mux.HandleFunc("PUT /api/uploads/{secret}", s.putUpload)
	mux.HandleFunc("/api/uploads/{secret}", refuseUploadMethod)
}
