package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// maxRequestBytes caps a publish request that carries no files. A kind's own
// size limit (checked by Kind.Validate) can only be lower than this. A request
// with a files field may be as large as files.MaxRequestBytes.
const maxRequestBytes = 10 << 20

type artifactInput struct {
	SourceBases map[string]string `json:"sourceBases,omitempty"`
	Title       string            `json:"title"`
	// Kind is optional: a new artifact is html and an update keeps the kind
	// the artifact already has.
	Kind string `json:"kind,omitempty"`
	// Format is optional: a new artifact gets the kind's current content
	// format version and an update keeps the one it has.
	Format string `json:"format,omitempty"`
	// Content is the artifact's content in the format of its kind. HTML is the
	// name this field had when html was the only kind; it is accepted for html
	// artifacts so existing publishers keep working.
	Content string `json:"content,omitempty"`
	HTML    string `json:"html,omitempty"`
	// Blocks is the content of a doc as structured blocks (docs/doc-format.md),
	// instead of Content.
	Blocks json.RawMessage `json:"blocks,omitempty"`
	// BaseVersion is the version an update started from; see
	// service.PublishInput.BaseVersion.
	BaseVersion int `json:"baseVersion,omitempty"`
	// Visibility is optional: a new artifact defaults to private and an
	// update keeps whatever the artifact already has.
	Visibility store.Visibility `json:"visibility,omitempty"`
	// Files changes the files that come with the page; see
	// service.PublishInput.Files.
	Files map[string]*string `json:"files,omitempty"`
}

type artifactView struct {
	SourceBases map[string]string `json:"sourceBases,omitempty"`
	store.Artifact
	URL string `json:"url"`
	// CanChange is whether the caller may change who it is shared with and who
	// edits it, or delete it, as Viewer.CanChange answers, so a front end shows
	// controls without working out who the author is.
	CanChange bool `json:"canChange"`
	// CanEdit is whether the caller may publish new versions, restore and
	// manage files, as Viewer.CanEdit answers; the author and editors may.
	CanEdit bool `json:"canEdit"`
	// Role is "author", "editor" or "reader", as Viewer.Role answers.
	Role string `json:"role"`
	// CanComment is whether the caller may read and write its comments, as
	// Viewer.CanComment answers: a signed-in person who can see it.
	CanComment bool `json:"canComment"`
	// Review is what the artifact's kind says about content it just accepted
	// (a slide that will not fit, a slide count that changed); only a publish
	// carries it.
	Review []string `json:"review,omitempty"`
	// Rewritten is the text of each file as the kind stored it, for the files
	// that differ from what was sent (a design's frames get a data-pid on every
	// element); only a publish carries it.
	Rewritten map[string]string `json:"rewritten,omitempty"`
	// Live is true when the publish went into the live copy that people are
	// editing and no version was saved; only a publish carries it.
	Live bool `json:"live,omitempty"`
}

func (s *Server) view(r *http.Request, a store.Artifact) artifactView {
	return artifactView{Artifact: a, URL: baseURL(r) + "/a/" + a.ID, CanChange: currentViewer(r).CanChange(a), CanEdit: currentViewer(r).CanEdit(a), Role: currentViewer(r).Role(a), CanComment: currentViewer(r).CanComment(a)}
}

func (s *Server) publishedView(r *http.Request, p service.Published) artifactView {
	v := s.view(r, p.Artifact)
	v.Review = p.Review
	v.Rewritten = p.Rewritten
	v.Live = p.Live
	return v
}

// decodeArtifact reads a publish request body. What makes it valid is the
// service's business, so the HTTP API and the MCP tools cannot disagree.
func (s *Server) decodeArtifact(w http.ResponseWriter, r *http.Request) (service.PublishInput, bool) {
	const shape = "body must be JSON {title, kind?, format?, content or blocks, baseVersion?, visibility?, files?, sourceBases?}; under 10 MB, or under 64 MB when it carries files; a page that is already a file goes up with POST /api/publish-sessions instead"
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, files.MaxRequestBytes))
	if err != nil {
		jsonError(w, http.StatusBadRequest, shape)
		return service.PublishInput{}, false
	}
	var in artifactInput
	if err := json.Unmarshal(body, &in); err != nil {
		jsonError(w, http.StatusBadRequest, shape)
		return service.PublishInput{}, false
	}
	if in.Files == nil && len(body) > maxRequestBytes {
		jsonError(w, http.StatusBadRequest, shape)
		return service.PublishInput{}, false
	}
	if in.Content != "" && in.HTML != "" {
		jsonError(w, http.StatusBadRequest, "send content or html, not both; html is the old name of content")
		return service.PublishInput{}, false
	}
	if in.HTML != "" {
		in.Content = in.HTML
	}
	if string(in.Blocks) == "null" {
		in.Blocks = nil
	}
	return service.PublishInput{Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content, Blocks: in.Blocks, BaseVersion: in.BaseVersion, Visibility: in.Visibility, Files: in.Files, SourceBases: in.SourceBases}, true
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.List(r.Context(), currentViewer(r), service.ListFilter{Order: service.Order(r.URL.Query().Get("order"))})
	if err != nil {
		s.fail(w, err)
		return
	}
	views := make([]artifactView, len(list))
	for i, a := range list {
		views[i] = s.view(r, a)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	in, ok := s.decodeArtifact(w, r)
	if !ok {
		return
	}
	a, err := s.svc.Create(r.Context(), currentViewer(r), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.publishedView(r, a))
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	a, ok := s.visibleArtifact(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if a.Kind != "design" {
		writeJSON(w, http.StatusOK, s.view(r, a))
		return
	}
	read, err := s.svc.ReadSource(r.Context(), currentViewer(r), a.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	view := s.view(r, read.Artifact)
	view.SourceBases = read.SourceBases
	writeJSON(w, http.StatusOK, struct {
		artifactView
		Content string `json:"content"`
	}{artifactView: view, Content: read.Artifact.Content})
}

// visibleArtifact loads the artifact named in the path. Whoever may not see it
// gets the same 404 as for an id that does not exist, so a 404 never says
// whether an artifact is there. Every route that touches an artifact or its
// shared db starts here (TestWhoCannotSeeAnArtifactCannotTellItExists).
func (s *Server) visibleArtifact(w http.ResponseWriter, r *http.Request) (store.Artifact, bool) {
	a, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return store.Artifact{}, false
	}
	return a, true
}

// ownArtifact is visibleArtifact for changes: someone who can see the artifact
// but did not author it is told so with a 403. Routes that read a body check
// ownership first, so a body error never tells a stranger the artifact exists.
func (s *Server) ownArtifact(w http.ResponseWriter, r *http.Request) (store.Artifact, bool) {
	a, err := s.svc.Own(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return store.Artifact{}, false
	}
	return a, true
}

// editArtifact is visibleArtifact for publishing, restoring and files: the
// author and editors pass, anyone else who can see it gets a 403.
func (s *Server) editArtifact(w http.ResponseWriter, r *http.Request) (store.Artifact, bool) {
	a, err := s.svc.Edit(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return store.Artifact{}, false
	}
	return a, true
}

// writableArtifact is the same early check for writes to the shared db.
func (s *Server) writableArtifact(w http.ResponseWriter, r *http.Request) bool {
	if err := s.svc.Writable(r.Context(), currentViewer(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return false
	}
	return true
}

func (s *Server) patchArtifact(w http.ResponseWriter, r *http.Request) {
	a, ok := s.ownArtifact(w, r)
	if !ok {
		return
	}
	var in struct {
		Visibility store.Visibility `json:"visibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, service.VisibilityHint)
		return
	}
	a, err := s.svc.SetVisibility(r.Context(), currentViewer(r), a.ID, in.Visibility)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r, a))
}

// patchEditors changes who may edit: {add?, remove?, organisationEdit?}.
func (s *Server) patchEditors(w http.ResponseWriter, r *http.Request) {
	a, ok := s.ownArtifact(w, r)
	if !ok {
		return
	}
	var in struct {
		Add              []string `json:"add"`
		Remove           []string `json:"remove"`
		OrganisationEdit *bool    `json:"organisationEdit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON {add?: [accounts], remove?: [accounts], organisationEdit?: true|false}")
		return
	}
	a, err := s.svc.Share(r.Context(), currentViewer(r), a.ID, service.ShareInput{AddEditors: in.Add, RemoveEditors: in.Remove, OrganisationEdit: in.OrganisationEdit})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r, a))
}

func (s *Server) updateArtifact(w http.ResponseWriter, r *http.Request) {
	a, ok := s.editArtifact(w, r)
	if !ok {
		return
	}
	in, ok := s.decodeArtifact(w, r)
	if !ok {
		return
	}
	published, err := s.svc.Update(r.Context(), currentViewer(r), a.ID, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.publishedView(r, published))
}

// copyArtifact makes the caller's own copy of an artifact they can see. The body
// is optional: {"version": N} copies that version instead of the newest.
func (s *Server) copyArtifact(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int `json:"version"`
	}
	if body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10)); err != nil || (len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &in) != nil) {
		jsonError(w, http.StatusBadRequest, "body is optional; when sent it must be JSON {version?}")
		return
	}
	a, err := s.svc.Copy(r.Context(), currentViewer(r), r.PathValue("id"), in.Version)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.view(r, a))
}

func (s *Server) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Delete(r.Context(), currentViewer(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleArtifact(w, r); !ok {
		return
	}
	me, err := s.svc.Me(r.Context(), currentViewer(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, me)
}

// routesArtifacts registers the artifact itself: list, publish, update, share, copy, delete, whoami.
func (s *Server) routesArtifacts(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts", s.api(s.listArtifacts))
	mux.HandleFunc("POST /api/artifacts", s.api(s.createArtifact))
	mux.HandleFunc("GET /api/artifacts/{id}", s.api(s.getArtifact))
	mux.HandleFunc("PUT /api/artifacts/{id}", s.api(s.updateArtifact))
	mux.HandleFunc("PATCH /api/artifacts/{id}", s.api(s.patchArtifact))
	mux.HandleFunc("PATCH /api/artifacts/{id}/editors", s.api(s.patchEditors))
	mux.HandleFunc("POST /api/artifacts/{id}/copy", s.api(s.copyArtifact))
	mux.HandleFunc("DELETE /api/artifacts/{id}", s.api(s.deleteArtifact))
	mux.HandleFunc("GET /api/artifacts/{id}/me", s.openAPI(s.whoami))
}
