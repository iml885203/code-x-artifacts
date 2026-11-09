package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func (s *Server) listDocs(w http.ResponseWriter, r *http.Request) {
	docs, err := s.svc.Docs(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (s *Server) getDoc(w http.ResponseWriter, r *http.Request) {
	d, err := s.svc.Doc(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"), r.PathValue("doc"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ifVersion reads the optional ?ifVersion=N of a document write.
func ifVersion(r *http.Request) (*int, error) {
	raw := r.URL.Query().Get("ifVersion")
	if raw == "" {
		return nil, nil //nolint:nilnil // absent means the write is unconditional
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return nil, errors.New("ifVersion is a version number (0 means the document must not exist yet)")
	}
	return &n, nil
}

func (s *Server) putDoc(w http.ResponseWriter, r *http.Request) {
	if !s.writableArtifact(w, r) {
		return
	}
	cond, err := ifVersion(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	var data json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, service.MaxDocBytes)).Decode(&data); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON under 1 MB")
		return
	}
	d, err := s.svc.PutDoc(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"), r.PathValue("doc"), data, cond)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) updateDoc(w http.ResponseWriter, r *http.Request) {
	if !s.writableArtifact(w, r) {
		return
	}
	cond, err := ifVersion(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	var fields json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, service.MaxDocBytes)).Decode(&fields); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be a JSON object of the fields to change, under 1 MB")
		return
	}
	d, err := s.svc.UpdateDoc(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"), r.PathValue("doc"), fields, cond)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDoc(w http.ResponseWriter, r *http.Request) {
	if !s.writableArtifact(w, r) {
		return
	}
	cond, err := ifVersion(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.svc.DeleteDoc(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"), r.PathValue("doc"), cond); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) queryDocs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filter json.RawMessage `json:"filter"`
		Limit  int             `json:"limit"`
	}
	if _, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, `body is {"filter": {...}, "limit": n}; both are optional`)
		return
	}
	docs, err := s.svc.QueryDocs(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("collection"), in.Filter, in.Limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (s *Server) batchDocs(w http.ResponseWriter, r *http.Request) {
	if !s.writableArtifact(w, r) {
		return
	}
	var in struct {
		Ops []struct {
			Op         string          `json:"op"`
			Collection string          `json:"collection"`
			ID         string          `json:"id"`
			Data       json.RawMessage `json:"data"`
			Fields     json.RawMessage `json:"fields"`
			IfVersion  *int            `json:"ifVersion"`
		} `json:"ops"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, service.MaxBatchBytes+(1<<20))).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, `body is {"ops": [{"op": "set"|"update"|"delete", "collection", "id", "data" (set) or "fields" (update), "ifVersion"}]}`)
		return
	}
	ops := make([]store.DocOp, len(in.Ops))
	for i, o := range in.Ops {
		data := o.Data
		if o.Op == store.OpUpdate {
			data = o.Fields
		}
		ops[i] = store.DocOp{Kind: o.Op, Collection: o.Collection, ID: o.ID, Data: data, IfVersion: o.IfVersion}
	}
	docs, err := s.svc.BatchDocs(r.Context(), currentViewer(r), r.PathValue("id"), ops)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": docs})
}

// routesDB registers the shared database.
func (s *Server) routesDB(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/db/{collection}", s.openAPI(s.listDocs))
	mux.HandleFunc("GET /api/artifacts/{id}/db/{collection}/{doc}", s.openAPI(s.getDoc))
	mux.HandleFunc("PUT /api/artifacts/{id}/db/{collection}/{doc}", s.api(s.putDoc))
	mux.HandleFunc("DELETE /api/artifacts/{id}/db/{collection}/{doc}", s.api(s.deleteDoc))
	mux.HandleFunc("PATCH /api/artifacts/{id}/db/{collection}/{doc}", s.api(s.updateDoc))
	mux.HandleFunc("POST /api/artifacts/{id}/db/{collection}/query", s.openAPI(s.queryDocs))
	mux.HandleFunc("POST /api/artifacts/{id}/db/batch", s.api(s.batchDocs))
}
