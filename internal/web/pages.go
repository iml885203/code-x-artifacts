package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.renderShell(w, r, pageData{page: "login", login: &loginBoot{Next: safeNext(r.URL.Query().Get("next"))}})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.FormValue("next"))
	signed, err := s.auth.Authenticate(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		msg := "login.error.invalid"
		switch {
		case errors.Is(err, auth.ErrNotAllowed):
			msg = "login.error.notAllowed"
		case !errors.Is(err, auth.ErrInvalidCredentials):
			s.log.Error("login", "err", err)
			msg = "login.error.unavailable"
		}
		s.renderShell(w, r, pageData{page: "login", status: http.StatusUnauthorized, login: &loginBoot{Next: next, Error: msg, Username: r.FormValue("username")}})
		return
	}
	user := signed.User
	if err := s.store.SaveUser(r.Context(), user); err != nil {
		s.log.Error("save user", "err", err)
		http.Error(w, "sign-in failed", http.StatusInternalServerError)
		return
	}
	if signed.DirectoryError != nil {
		signed.Directory.Complete = false
		signed.Directory.People = nil
		s.log.Warn("directory lookup incomplete", "err", signed.DirectoryError)
	}
	if err := s.store.RecordDirectory(r.Context(), signed.Directory); err != nil {
		s.log.Error("record directory lookup", "err", err)
	}
	s.log.Info("login", "user", user.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.sessions.issue(user.Username, time.Now()), Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(s.sessions.ttl.Seconds()),
	})
	http.Redirect(w, r, next, http.StatusSeeOther) // safeNext reduces next to a path on this site (TestLoginOnlyRedirectsWithinTheSite)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, `/\`) {
		return "/"
	}
	return next
}

// gallery is the shell: the app lists artifacts through GET /api/artifacts, the
// same call agents make, so what the gallery shows is what the API answers.
func (s *Server) gallery(w http.ResponseWriter, r *http.Request) {
	s.renderShell(w, r, pageData{page: "gallery"})
}

// viewer is the shell with the artifact in its boot block, so the page opens
// with what the viewer is allowed to see already in it, anonymous readers of a
// public artifact included (they cannot call the artifact API).
func (s *Server) viewer(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		if refusal, ok := errors.AsType[*service.Error](err); ok && refusal.Code == service.NotFound {
			// Missing and not visible look the same, down to the page.
			s.renderShell(w, r, pageData{page: "error", status: http.StatusNotFound})
			return
		}
		s.fail(w, err)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex")
	view := s.view(r, a)
	s.renderShell(w, r, pageData{page: "viewer", artifact: &view})
}

// exportOption is one entry of the Export menu: the format name, which the
// viewer turns into a label.
type exportOption struct {
	Format string `json:"format"`
	// Browser says the viewer makes this one itself (it asks the page to print)
	// instead of fetching it from the export route.
	Browser bool `json:"browser,omitempty"`
}

// exportOptions is the formats every kind that exports declares: those the
// export route serves, then those the browser makes.
func exportOptions() map[string][]exportOption {
	options := map[string][]exportOption{}
	for _, name := range kind.Names() {
		k, err := kind.Lookup(name)
		if err != nil {
			continue // Names lists registered kinds; a failed lookup is impossible
		}
		if x, ok := k.(kind.Exporter); ok {
			for _, f := range x.Formats() {
				options[name] = append(options[name], exportOption{Format: f})
			}
		}
		if x, ok := k.(kind.BrowserExporter); ok {
			for _, f := range x.BrowserFormats() {
				options[name] = append(options[name], exportOption{Format: f, Browser: true})
			}
		}
	}
	return options
}

func (s *Server) connectPage(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Tokens(r.Context(), currentUser(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderShell(w, r, pageData{page: "connect", connect: &connectBoot{BaseURL: baseURL(r), Tokens: list}})
}

// asksForAPage reports whether a request is a person opening an address, so a
// miss gets the site's not-found page. The API and the other methods keep the
// plain status a program reads.
func asksForAPage(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.HasPrefix(r.URL.Path, "/api/")
}

// notFoundPage is the page for an address no route serves: the same error page
// a missing artifact gets, with the header of whoever is signed in.
func (s *Server) notFoundPage(w http.ResponseWriter, r *http.Request) {
	if username, ok := s.sessionUser(r); ok {
		r = s.signedIn(r, username)
	}
	s.renderShell(w, r, pageData{page: "error", status: http.StatusNotFound})
}

// routesPages registers the pages people open: sign-in, language, gallery, viewer, kit reference and connect.
func (s *Server) routesPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.form(s.logout))
	mux.HandleFunc("POST /lang", s.setLanguage)

	mux.HandleFunc("GET /{$}", s.page(s.gallery))
	mux.HandleFunc("GET /recent", s.page(s.gallery))
	mux.HandleFunc("GET /a/{id}", s.openPage(s.viewer))
	mux.HandleFunc("GET /kit", s.page(s.kitReference))
	mux.HandleFunc("GET /connect", s.page(s.connectPage))
	// The Tokens page was folded into Connect; old links and bookmarks still land.
	mux.HandleFunc("GET /tokens", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/connect#tokens", http.StatusSeeOther)
	})
}
