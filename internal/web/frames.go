package web

import (
	"io"
	"net/http"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// frameSandbox is all a frame is allowed: to run the script the platform put in
// it, in an opaque origin. No forms, popups, modals or downloads, and no bridge:
// a frame has nothing to ask the site.
const frameSandbox = "sandbox allow-scripts"

// frameCSP is the policy of a frame. It is stricter than a page's: nothing
// loads but pictures and fonts of this artifact (or inline), styles are inline,
// and the only script is the kind's own, by hash. 'unsafe-eval' and 'self' are
// not there, so another artifact's file or a string turned into code cannot run.
// It has no frame-ancestors: a frame is shown inside the overview, whose origin
// is opaque, and 'self' does not match an opaque ancestor. Embedding gives a
// site nothing, since the address that carries the grant is the credential.
func frameCSP(origin, artifactID string, scripts []string) string {
	hashes := make([]string, len(scripts))
	for i, sc := range scripts {
		hashes[i] = scriptHash(sc)
	}
	files := origin + assets.PathPrefix(artifactID)
	return strings.Join([]string{
		frameSandbox,
		"default-src 'none'",
		"script-src " + strings.Join(hashes, " "),
		"style-src 'unsafe-inline'",
		"img-src data: " + files,
		"font-src data: " + origin + "/static/fonts/ " + files,
		"base-uri 'none'",
	}, "; ")
}

// writeFrame sends one HTML file of a design as the frame it is: the kind's
// document under the frame's policy, without the bridge. The asset URLs in the
// markup already carry the grant (writeFile).
func (s *Server) writeFrame(w http.ResponseWriter, r *http.Request, a store.Artifact, fk kind.FrameKind, path, markup string) {
	markup, err := kind.DesignKitFrame(markup, platformKit)
	if err != nil {
		s.serverError(w, err)
		return
	}
	page, err := fk.RenderFrame(path, kind.DesignTokenFrame(a.Content, markup))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if isPreview(r) {
		page.HTML = inject(page.HTML, s.thumbnailScript)
		page.InlineScripts = append(page.InlineScripts, s.thumbnailScript)
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	policy := frameCSP(baseURL(r), a.ID, page.InlineScripts)
	if isPreview(r) {
		policy += "; connect-src " + baseURL(r) + "/static/fonts/ " + baseURL(r) + assets.PathPrefix(a.ID)
	}
	h.Set("Content-Security-Policy", policy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex")
	h.Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, page.HTML)
}
