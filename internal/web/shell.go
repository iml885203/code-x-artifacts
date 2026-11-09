package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/iml885203/code-x-artifacts/internal/i18n"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// pickLanguage is the language a person chose with the switcher, else the one
// their browser asks for.
func (s *Server) pickLanguage(r *http.Request) *i18n.Lang {
	picked := ""
	if c, err := r.Cookie(languageCookie); err == nil {
		picked = c.Value
	}
	return s.langs.Choose(picked, r.Header.Get("Accept-Language"))
}

// shellBoot is what the React app reads from the page before it starts. It
// carries the whole catalogue of the language served, so the app has the text
// of the language the server picked and no copy of its own.
type shellBoot struct {
	DisplayName string `json:"displayName,omitempty"`
	Page        string `json:"page"`
	Lang        string `json:"lang"`
	User        string `json:"user"`
	Initials    string `json:"initials"`
	// Langs is every language this build has and Here the path the language
	// switcher returns to.
	Langs   []string          `json:"langs"`
	Here    string            `json:"here"`
	Strings map[string]string `json:"strings"`
	// PublicLinks is whether this deployment lets an author choose "public".
	PublicLinks bool `json:"publicLinks"`
	// FrameSandbox is the sandbox attribute of the iframe that shows an
	// artifact. It is the same list /raw sends as its CSP, so the two cannot drift.
	FrameSandbox string `json:"frameSandbox"`
	// Exports maps each kind to the formats its exporter declares; a kind that
	// cannot export is absent. The viewer's Export menu lists exactly these, so
	// the front end never keeps a list of its own.
	Exports map[string][]exportOption `json:"exports"`

	// The rest is what one page needs and nothing else.
	Artifact *artifactView `json:"artifact,omitempty"`
	Login    *loginBoot    `json:"login,omitempty"`
	Connect  *connectBoot  `json:"connect,omitempty"`
	// Status is the HTTP status of the error page.
	Status int `json:"status,omitempty"`
}

type loginBoot struct {
	// Next is where signing in leads, Error the catalogue key of what went
	// wrong (empty on a fresh page) and Username what was typed, so a failed
	// attempt does not make a person type it again.
	Next     string `json:"next"`
	Error    string `json:"error"`
	Username string `json:"username"`
}

// connectBoot is what the "Connect your agent" page needs: the address the
// commands it writes point at, and the person's tokens (without secrets). A new
// token is made by POST /api/tokens and never travels in a page.
type connectBoot struct {
	BaseURL string        `json:"baseUrl"`
	Tokens  []store.Token `json:"tokens"`
}

// pageData is the part of a shell page that differs from page to page.
type pageData struct {
	page     string
	status   int
	artifact *artifactView
	login    *loginBoot
	connect  *connectBoot
}

// descriptionKey is the catalogue entry that describes each page, for the meta
// description and the share preview. None of them says anything about an
// artifact: a description is what a stranger's link preview shows.
var descriptionKey = map[string]string{
	"login": "meta.description.login", "gallery": "meta.description.gallery", "viewer": "meta.description.viewer",
	"connect": "meta.description.connect", "kit": "meta.description.kit", "error": "meta.description.error",
}

// renderShell serves the page the React app mounts on. The viewer may be
// nobody (the sign-in page, an anonymous reader of a public artifact).
func (s *Server) renderShell(w http.ResponseWriter, r *http.Request, d pageData) {
	if s.bundleErr != nil {
		s.serverError(w, s.bundleErr)
		return
	}
	lang := s.pickLanguage(r)
	user := ""
	if v, ok := r.Context().Value(viewerKey{}).(service.Viewer); ok {
		user = v.Username
	}
	displayName := ""
	if user != "" {
		profile, err := s.svc.Me(r.Context(), s.svc.Viewer(user))
		if err != nil {
			s.serverError(w, err)
			return
		}
		displayName = profile.DisplayName
	}
	here := r.URL.RequestURI()
	if d.login != nil {
		here = "/login?next=" + url.QueryEscape(d.login.Next)
	}
	status := http.StatusOK
	if d.status != 0 {
		status = d.status
	}
	title := pageTitle(lang, d)
	s.writePage(w, lang, status, d.page, s.shell, map[string]any{
		"L":           lang,
		"Title":       title,
		"Description": lang.T(descriptionKey[d.page]),
		"Image":       baseURL(r) + "/static/og-image.png",
		"Bundle":      s.bundle,
		"Boot": shellBoot{
			DisplayName: displayName, Page: d.page, Lang: lang.Tag, User: user, Initials: initials(user), Langs: s.langs.Tags(), Here: here, Strings: lang.Prefixed(""),
			PublicLinks: s.publicLinks, FrameSandbox: strings.TrimPrefix(artifactSandbox, "sandbox "), Exports: exportOptions(),
			Artifact: d.artifact, Login: d.login, Connect: d.connect, Status: d.status,
		},
	})
}

// writePage executes a template into a buffer first, so a failing template is a
// 500 and never half a page, then writes the headers every page carries. The
// status is written after the headers, so a page with a non-200 status has them.
func (s *Server) writePage(w http.ResponseWriter, lang *i18n.Lang, status int, name string, t *template.Template, data map[string]any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		s.log.Error("render", "page", name, "err", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", lang.Tag)
	w.Header().Add("Vary", "Accept-Language, Cookie")
	// data: images are the icons a board's editor (Excalidraw) draws on its
	// canvas, and blob: images the pictures it shrinks before uploading them,
	// made by the page from bytes it already has; neither can run script or
	// reach another site. The whole shell has them, and the board's one style
	// (boardTextStyle), because the app moves between pages without loading a
	// new document.
	w.Header().Set("Content-Security-Policy", shellCSP)
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w) // the client went away; nothing left to tell it
}

// languageCookie holds the language a person picked with the switcher. It wins
// over Accept-Language. Only the server reads it, so it is HttpOnly.
const languageCookie = "lang"

// setLanguage remembers the language a person picked and sends them back where
// they were. It needs no session, because the sign-in page has a switcher too,
// but it holds to the same rule as every form: only from this site's own pages.
func (s *Server) setLanguage(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	tag := r.FormValue("lang")
	if s.langs.Get(tag) == nil {
		http.Error(w, "unknown language", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: languageCookie, Value: tag, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 60 * 60,
	})
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther) // safeNext reduces next to a path on this site (TestSwitchingLanguageReturnsToTheSamePageAndOnlyWithinTheSite)
}

// boardTextStyle is the one style attribute the app sets: a board's editor puts
// it on every text when it exports an SVG. The policy allows this exact value
// by its hash and no other inline style.
const boardTextStyle = "white-space: pre;"

// shellCSP is the policy of every page of the app.
var shellCSP = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-hashes' 'sha256-" + hashOf(boardTextStyle) + "'; frame-src 'self'; frame-ancestors 'none'"

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// initials is the short mark of a person on the account button: the first
// letters of the parts of an account name like ada.lovelace.
func initials(username string) string {
	parts := strings.FieldsFunc(username, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var out []rune
	for _, p := range parts {
		out = append(out, unicode.ToUpper([]rune(p)[0]))
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 1 {
		if r := []rune(parts[0]); len(r) > 1 {
			out = append(out, unicode.ToUpper(r[1]))
		}
	}
	return string(out)
}

// routesStatic registers the files the shell and pages load, and the health check.
func (s *Server) routesStatic(mux *http.ServeMux) {
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/fonts/", anyOrigin(http.StripPrefix("/static/", http.FileServerFS(static))))
	handleKit(mux)
	mux.Handle("GET /vendor/{runtime}/{file}", anyOrigin(http.HandlerFunc(serveVendor)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.Handle("GET /assets/", assetsHandler(s.assets))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
}
