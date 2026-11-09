package web

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// assetView is an asset as the API shows it; URL is what to put in the
// artifact. The MCP tools show the same fields.
type assetView struct {
	store.Asset
	URL string `json:"url"`
}

func (s *Server) assetView(r *http.Request, id string, a store.Asset) assetView {
	return assetView{Asset: a, URL: baseURL(r) + assets.Path(id, a.Name)}
}

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Assets(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	views := make([]assetView, len(list))
	for i, a := range list {
		views[i] = s.assetView(r, r.PathValue("id"), a)
	}
	writeJSON(w, http.StatusOK, views)
}

// putAsset takes the file as the raw request body, whatever Content-Type it
// claims: the type is decided from the bytes. The artifact is checked first, so
// a body problem never tells a stranger the artifact exists.
func (s *Server) putAsset(w http.ResponseWriter, r *http.Request) {
	a, ok := s.editArtifact(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, assets.MaxBytes))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		jsonError(w, http.StatusRequestEntityTooLarge, "a file can be at most 15 MB; send the file itself as the request body")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, "could not read the request body: send the file itself as the body")
		return
	}
	up, err := s.svc.UploadAsset(r.Context(), currentViewer(r), a.ID, r.PathValue("name"), data)
	if err != nil {
		s.fail(w, err)
		return
	}
	status := http.StatusCreated
	if up.Replaced {
		status = http.StatusOK
	}
	writeJSON(w, status, s.assetView(r, a.ID, up.Asset))
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteAsset(r.Context(), currentViewer(r), r.PathValue("id"), r.PathValue("name")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fileViewer is who a request for a file is made by. Four kinds of caller come
// here, because the readers are an <img>, a font or a fetch inside a sandboxed
// page as often as a person's browser or an agent:
//   - a personal token, for agents and scripts;
//   - a grant in the path, which a page was handed when it was served;
//   - the session cookie, for someone who opens the file directly;
//   - nobody, for a public artifact.
//
// Whatever the caller, the service decides; a credential that does not check
// out is not an error, it is just not enough, so every refusal looks the same.
func (s *Server) fileViewer(r *http.Request, grant string) service.Viewer {
	now := time.Now()
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if username, err := s.store.TokenUser(r.Context(), bearer); err == nil {
			return s.svc.Viewer(username)
		}
		return s.svc.Anonymous()
	}
	if grant != "" {
		if visibility, ok := s.sessions.openGrant(grant, r.PathValue("id"), now); ok {
			return s.svc.Granted(service.Grant{ArtifactID: r.PathValue("id"), Visibility: visibility})
		}
		return s.svc.Anonymous()
	}
	if username, ok := s.sessionUser(r); ok {
		return s.svc.Viewer(username)
	}
	return s.svc.Anonymous()
}

// serveFile answers GET /files/{id}/{name} and GET /files/{id}/{grant}/{name}.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	// Files are fetched by pages in an opaque origin (Origin: null), by fonts
	// and by fetch(). The answer carries no credentials of its own and a
	// browser never lets a cross-origin reader use the cookie with a wildcard,
	// so opening it to every origin shows nothing the reader could not
	// already fetch.
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("X-Content-Type-Options", "nosniff")

	rec, f, err := s.svc.OpenAsset(r.Context(), s.fileViewer(r, r.PathValue("grant")), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		s.fail(w, err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			s.log.Error("close asset file", "id", r.PathValue("id"), "name", rec.Name, "err", err)
		}
	}()
	h.Set("Content-Type", rec.ContentType)
	h.Set("Content-Disposition", `inline; filename="`+rec.Name+`"`)
	h.Set("ETag", `"`+rec.SHA256+`"`)
	// private: what a reader may fetch depends on who they are. A day is long
	// enough to matter and short enough that a replaced file shows up.
	h.Set("Cache-Control", "private, max-age=86400")
	if t, ok := assets.TypeOf(rec.ContentType); ok && t.Active {
		// An SVG opened on its own would run its scripts as this site. Under
		// a sandbox policy with nothing allowed it is a picture and no more;
		// as an <img> it never ran code in the first place.
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	}
	http.ServeContent(w, r, rec.Name, rec.CreatedAt, f)
}

// assetGrants lets a page load its own files. The page is served to a viewer
// who may see it, so every reference to the artifact's own files is given the
// grant that proves it. Authors write the plain URL that upload returned and
// never see this. The same grant has to reach the page and every inline script
// the CSP pins by hash, so it is applied before the hashes are taken.
func (s *Server) assetGrants(a store.Artifact) *strings.Replacer {
	prefix := assets.PathPrefix(a.ID)
	return strings.NewReplacer(prefix, prefix+s.sessions.grant(a.ID, a.Visibility, time.Now())+"/")
}

// routesAssets registers uploaded files.
func (s *Server) routesAssets(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/assets", s.api(s.listAssets))
	mux.HandleFunc("PUT /api/artifacts/{id}/assets/{name}", s.api(s.putAsset))
	mux.HandleFunc("DELETE /api/artifacts/{id}/assets/{name}", s.api(s.deleteAsset))
	mux.HandleFunc("GET /files/{id}/{name}", s.serveFile)
	mux.HandleFunc("GET /files/{id}/{grant}/{name}", s.serveFile)
}
