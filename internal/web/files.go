package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/operationlog"
)

// readFile answers GET /api/artifacts/{id}/files/{path...}[?version=N] with the
// text of one file of the page, the same answer as the MCP read tool's file
// argument. It is text, never a document, so it runs nothing wherever it is opened.
func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleArtifact(w, r); !ok {
		return
	}
	version := 0
	if q := r.URL.Query().Get("version"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 {
			jsonError(w, http.StatusBadRequest, "version must be a number from 1")
			return
		}
		version = n
	}
	read, err := s.svc.ReadSourceFile(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("path"), version)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !read.Present {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"path": read.Ref.Path, "present": false, "sourceBase": read.SourceBase})
		return
	}
	ref, text := read.Ref, read.Text
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	if read.SourceBase != "" {
		h.Set("X-Pictor-Source-Base", read.SourceBase)
	}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Pictor-Content-Type", ref.ContentType)
	h.Set("ETag", `"`+ref.SHA256+`"`)
	_, _ = w.Write([]byte(text)) // served as text/plain with nosniff under a sandbox that allows nothing, never as a document (TestReadingAFileOverTheAPIIsPlainTextThatRunsNothing)
}

// Maintain does the housekeeping the server owes its data: it deletes the
// blobs of publishes that stored their bytes and then failed, and the publish
// sessions that expired with the bytes uploaded to them, when it starts and
// then once a day, until ctx ends. A failure is logged and tried again next
// time; it costs disk space, never correctness.
func (s *Server) Maintain(ctx context.Context, every time.Duration) {
	ctx = operationlog.Context(ctx, s.log)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if err := s.store.SweepActivity(ctx, time.Now()); err != nil && ctx.Err() == nil {
			s.log.Error("sweep agent activity", "err", err)
		}
		if err := s.store.SweepUsage(ctx, time.Now()); err != nil && ctx.Err() == nil {
			s.log.Error("sweep usage counts", "err", err)
		}
		if _, err := s.svc.SweepBlobs(ctx, time.Now()); err != nil && ctx.Err() == nil {
			s.log.Error("sweep unreferenced file blobs", "err", err)
		}
		if _, err := s.svc.SweepPublishSessions(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("sweep expired publish sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// routesFiles registers the files of a version.
func (s *Server) routesFiles(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/files/{path...}", s.api(s.readFile))
}
