package web

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

//go:embed bridge.js
var bridgeJS string

// previewJS stands where the bridge does in a preview: the same window.pictor,
// with nothing behind it (bridge_preview.js).
//
//go:embed bridge_preview.js
var previewStubJS string

//go:embed preview_capture.js
var previewCaptureJS string

var previewJS = previewStubJS + "\n" + previewCaptureJS

// isPreview says the request is for a card of the overview: the page as it
// looks, with no way to reach the viewer's bridge.
func isPreview(r *http.Request) bool { return r.URL.Query().Has("preview") }

// The artifact runs in an opaque origin: it can run scripts but cannot read
// the viewer's cookies or call the API as the viewer. Everything it needs from
// the service goes through the postMessage bridge the viewer page answers.
// There is no allow-downloads: a page that wants to hand over a file calls
// pictor.download, and the viewer asks the person before anything is saved.
const artifactSandbox = "sandbox allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals"

// raw serves the page of an artifact. With ?version=N it serves that older
// version instead, for its author and editors only, since history can hold what the author
// took out on purpose; it is the same page with the same sandbox and bridge.
// A version that comes with files is not served here but at an address that
// carries a grant (rawGranted), which this redirects to: the page's relative
// URLs then resolve to the grant too.
func (s *Server) raw(w http.ResponseWriter, r *http.Request) {
	a, ok := s.visibleArtifact(w, r)
	if !ok {
		return
	}
	shown := store.Version{Number: a.Version, Title: a.Title, Kind: a.Kind, Format: a.Format, Content: a.Content}
	if q := r.URL.Query().Get("version"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 {
			jsonError(w, http.StatusBadRequest, "version must be a number from 1")
			return
		}
		shown, err = s.svc.Version(r.Context(), currentViewer(r), a.ID, n)
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	has, err := s.svc.VersionHasFiles(r.Context(), a, shown.Number)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if has {
		w.Header().Set("Cache-Control", "no-store")
		to := pagePath(a.ID, s.sessions.pageGrant(a.ID, a.Visibility, shown.Number, time.Now()), "")
		if isPreview(r) {
			to += "?preview"
		}
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	s.servePage(w, r, a, shown)
}

// pagePath is the address of a file of a page, or of the page itself for an
// empty path.
func pagePath(id, grant, path string) string { return "/raw/" + id + "/" + grant + "/" + path }

// servePage writes one version of an artifact as the page it is: rendered by
// its kind, the sandbox and the bridge added by the server.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, a store.Artifact, shown store.Version) {
	k, err := kind.Lookup(shown.Kind)
	if err != nil {
		// Written by a newer version: never guess how to show it.
		s.log.Error("artifact of a kind this version lacks", "id", a.ID, "kind", shown.Kind)
		jsonError(w, http.StatusNotImplemented, "this server cannot render artifacts of kind "+shown.Kind)
		return
	}
	if !kind.HasFormat(k, shown.Format) {
		s.log.Error("artifact of a format this version lacks", "id", a.ID, "kind", shown.Kind, "format", shown.Format)
		jsonError(w, http.StatusNotImplemented, fmt.Sprintf("this server cannot render %s artifacts of format %s", shown.Kind, shown.Format))
		return
	}
	bundled, err := s.svc.BundleSet(r.Context(), a.ID, shown.Number, k)
	if err != nil {
		s.serverError(w, err)
		return
	}
	page, scripts, paths, err := renderPage(k, shown.Title, shown.Content, shown.Format, bundled, isPreview(r))
	if err != nil {
		s.serverError(w, fmt.Errorf("render %s artifact %s version %d: %w", shown.Kind, a.ID, shown.Number, err))
		return
	}
	if len(bundled) > 0 {
		// The page's own files may be loaded as scripts, by path: this
		// artifact's, never another's.
		paths = append(slices.Clone(paths), "/raw/"+a.ID+"/")
	}
	if shown.Kind == "doc" {
		page = s.withoutMissingPictures(r.Context(), a.ID, page)
	}
	grants := s.assetGrants(a)
	page = grants.Replace(page)
	if scripts != nil {
		// A copy: the kind may hand back a slice it keeps in its render cache.
		granted := make([]string, len(scripts))
		for i, sc := range scripts {
			granted[i] = grants.Replace(sc)
		}
		scripts = granted
	}
	// The headers and the bridge belong to the server, not the kind, so no
	// kind can loosen the sandbox or leave a page without window.pictor.
	s.writeDocument(w, r, page, scripts, paths)
}

// writeDocument sends a page with the headers every page of an artifact has.
func (s *Server) writeDocument(w http.ResponseWriter, r *http.Request, page string, scripts, scriptPaths []string) {
	// The sandbox is the same for a preview; only what is injected differs.
	injected := bridgeJS
	if isPreview(r) {
		injected = previewJS
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", artifactCSPWith(baseURL(r), scripts, scriptPaths, injected))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, inject(page, injected)) // the client went away, nothing left to tell it
}

// rawGranted serves a page that comes with files, and the files. The grant in
// the path is the only credential: the page's own requests are made from an
// opaque origin and carry no cookie. It pins one version, so a page that stays
// open while a new version is published keeps reading its own. A bad grant, a
// grant for another artifact or audience, an expired one and a path that is not
// there get the same answer as an artifact that does not exist.
func (s *Server) rawGranted(w http.ResponseWriter, r *http.Request) {
	id, path := r.PathValue("id"), r.PathValue("path")
	h := w.Header()
	// Pages in an opaque origin fetch these as CORS requests, without
	// credentials; and a page that loads a third party's resource must not
	// hand it an address that contains the grant.
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	if isLiveGrant(r.PathValue("grant")) {
		s.rawLive(w, r, id, path)
		return
	}
	audience, version, ok := s.sessions.openPageGrant(r.PathValue("grant"), id, time.Now())
	if !ok {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	g := service.Grant{ArtifactID: id, Visibility: audience, Version: version}
	if path == "" || path == files.EntryPage {
		a, shown, err := s.svc.GrantedPage(r.Context(), g)
		if err != nil {
			s.fail(w, err)
			return
		}
		s.servePage(w, r, a, store.Version{Number: shown.Number, Title: shown.Title, Kind: shown.Kind, Format: shown.Format, Content: shown.Content})
		return
	}
	a, ref, f, err := s.svc.GrantedFile(r.Context(), g, path)
	if err != nil {
		s.fail(w, err)
		return
	}
	body, err := io.ReadAll(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		s.serverError(w, fmt.Errorf("read file %s of artifact %s: %w", path, id, err))
		return
	}
	if a.Kind == "design" {
		_, shown, err := s.svc.GrantedPage(r.Context(), g)
		if err != nil {
			s.fail(w, err)
			return
		}
		a.Content = shown.Content
	}
	s.writeFile(w, r, a, ref, string(body))
}

// writeFile sends one file of a page. A web page is a document like the entry
// page, in the same sandbox and with the bridge (a frame of a design is the
// exception, see writeFrame); anything else is not a
// document at all and says so, so opening it on its own runs nothing. The asset
// URLs in it carry the grant, as in the page.
func (s *Server) writeFile(w http.ResponseWriter, r *http.Request, a store.Artifact, ref store.FileRef, body string) {
	h := w.Header()
	if files.Rewritable(ref.Path) {
		body = s.assetGrants(a).Replace(body)
	}
	if strings.HasPrefix(ref.ContentType, "text/html") {
		if k, err := kind.Lookup(a.Kind); err == nil {
			if fk, ok := k.(kind.FrameKind); ok {
				s.writeFrame(w, r, a, fk, ref.Path, body)
				return
			}
		}
		s.writeDocument(w, r, body, nil, nil)
		return
	}
	sum := sha256.Sum256([]byte(body))
	h.Set("Content-Type", ref.ContentType)
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	// The tag covers the bytes sent, which change with the grant in the asset
	// URLs; the address changes with the grant too, hourly.
	h.Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	h.Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, ref.Path, time.Time{}, strings.NewReader(body))
}

// renderPage returns the document a kind draws and, for a kind whose page is
// generated entirely by the platform, the inline scripts in it and the paths
// it loads scripts from (nil for the kinds whose page is the author's own markup).
func renderPage(k kind.Kind, title, content, format string, bundled files.Set, preview bool) (page string, scripts, scriptPaths []string, err error) {
	if b, ok := k.(kind.Bundler); ok && len(bundled) > 0 {
		p, err := b.RenderPageFiles(content, format, bundled)
		return p.Titled(title).HTML, p.InlineScripts, p.ScriptPaths, err
	}
	if pv, ok := k.(kind.PreviewRenderer); ok && preview {
		p, err := pv.RenderPreview(content, format)
		return p.Titled(title).HTML, p.InlineScripts, p.ScriptPaths, err
	}
	if pr, ok := k.(kind.PageRenderer); ok {
		p, err := pr.RenderPage(content, format)
		return p.Titled(title).HTML, p.InlineScripts, p.ScriptPaths, err
	}
	page, err = k.Render(content, format)
	return page, nil, nil, err
}

// artifactCSP is the policy /raw sends. The sandbox is the boundary and is the
// same for every kind. A page the platform generated also gets a script-src:
// only the paths on this site that the kind says it loads scripts from (the
// runtime libraries under /vendor/), the bridge, and the inline scripts the kind
// declared, by hash. Without it, a string someone else wrote into the shared
// db and the page drew as markup would run as the page, with the viewer's
// bridge (docs/script-src.md). Pages that are the author's markup get none.
//
// It never says 'self': that would allow every script on this site, and a
// script is something an artifact's files can now be, so another artifact's
// file could be loaded into this page by whoever can write markup into it.
func artifactCSP(origin string, scripts, scriptPaths []string) string {
	return artifactCSPWith(origin, scripts, scriptPaths, bridgeJS)
}

// artifactCSPWith is artifactCSP for a page that carries the given script
// where the bridge goes.
func artifactCSPWith(origin string, scripts, scriptPaths []string, injected string) string {
	csp := artifactSandbox
	if scripts != nil {
		sources := make([]string, 0, len(scriptPaths)+1+len(scripts))
		for _, p := range scriptPaths {
			sources = append(sources, origin+p)
		}
		sources = append(sources, scriptHash(injected))
		for _, s := range scripts {
			sources = append(sources, scriptHash(s))
		}
		csp += "; script-src " + strings.Join(sources, " ")
	}
	return csp + "; frame-ancestors 'self'"
}

func scriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

var (
	headOpen = regexp.MustCompile(`(?i)<head[^>]*>`)
	doctype  = regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`)
)

// injectBridge places the bridge before any of the author's scripts without
// pushing content ahead of a doctype, which would drop the page into quirks mode.
func injectBridge(html string) string { return inject(html, bridgeJS) }

// inject puts a script first in the page, after the doctype and the head tag.
func inject(html, script string) string {
	tag := "<script>" + script + "</script>"
	if loc := headOpen.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + tag + html[loc[1]:]
	}
	if loc := doctype.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + tag + html[loc[1]:]
	}
	return tag + html
}

// routesRaw registers the sandboxed page itself and, for a page with files, its files.
func (s *Server) routesRaw(mux *http.ServeMux) {
	mux.HandleFunc("GET /raw/{id}", s.openPage(s.raw))
	// A page that comes with files is served here, where the grant in the path
	// is the only credential: its own requests cannot carry the cookie.
	mux.HandleFunc("GET /raw/{id}/{grant}/{path...}", s.rawGranted)
}
