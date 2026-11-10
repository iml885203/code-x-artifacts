package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/mcptools"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

type viewerKey struct{}

func currentViewer(r *http.Request) service.Viewer {
	return r.Context().Value(viewerKey{}).(service.Viewer)
}

// currentUser is the signed-in person, or "" for the anonymous viewer that
// only openPage and openAPI let through.
func currentUser(r *http.Request) string { return currentViewer(r).Username }

// signedIn attaches the authenticated person to the request, so every access
// decision in the request sees the same viewer.
func (s *Server) signedIn(r *http.Request, username string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), viewerKey{}, s.svc.Viewer(username)))
}

func (s *Server) asAnonymous(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), viewerKey{}, s.svc.Anonymous()))
}

func (s *Server) sessionUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return s.sessions.verify(r.Context(), c.Value, time.Now())
}

// page serves a signed-in person's browser; anyone else is sent to sign in.
func (s *Server) page(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, ok := s.sessionUser(r)
		if !ok {
			signInRedirect(w, r)
			return
		}
		h(w, s.signedIn(r, username))
	}
}

func signInRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
}

// openPage is page for the routes that may serve the anonymous viewer: with no
// session it lets the request through only when the artifact in the path is
// one an anonymous viewer may see. Every other case, including an id that does
// not exist, gets the sign-in redirect page gives, so it reveals nothing.
func (s *Server) openPage(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, ok := s.sessionUser(r)
		if !ok {
			if !s.anonymousMaySee(r) {
				signInRedirect(w, r)
				return
			}
			h(w, s.asAnonymous(r))
			return
		}
		h(w, s.signedIn(r, username))
	}
}

// anonymousMaySee reports whether the artifact named in the path is open to
// the anonymous viewer.
func (s *Server) anonymousMaySee(r *http.Request) bool {
	_, err := s.svc.Artifact(r.Context(), s.svc.Anonymous(), r.PathValue("id"))
	return err == nil
}

// form accepts a cookie-authenticated POST only from this site's own pages,
// which also refuses a form submitted from inside an artifact (Origin: null).
func (s *Server) form(h http.HandlerFunc) http.HandlerFunc {
	return s.page(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		h(w, r)
	})
}

// api accepts a personal token (scripts, CI, agents) or the viewer's cookie.
// A cookie call must carry X-Pictor, a header no other origin can send without
// a CORS preflight this service never answers.
func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return s.authenticated(h, func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, http.StatusUnauthorized, "sign in or send a token")
	})
}

// browserAPI is api for what only a person at a browser may do: it ignores
// Authorization, so a token cannot make more tokens (a stolen one would
// otherwise outlive its revocation).
func (s *Server) browserAPI(h http.HandlerFunc) http.HandlerFunc {
	api := s.api(h)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Authorization")
		api(w, r)
	}
}

// openAPI is api for the reads that may serve the anonymous viewer (whoami and
// the shared db): a request with no credentials goes through only when the
// artifact in the path is open to anonymous viewers. Sending a token or cookie
// still means the usual checks, so a wrong token is an error, not anonymity.
func (s *Server) openAPI(h http.HandlerFunc) http.HandlerFunc {
	return s.authenticated(h, func(w http.ResponseWriter, r *http.Request) {
		if s.anonymousMaySee(r) {
			h(w, s.asAnonymous(r))
			return
		}
		jsonError(w, http.StatusUnauthorized, "sign in or send a token")
	})
}

// mcpEndpoint serves agents. A personal or resource-bound OAuth token opens it: a cookie is
// ambient authority that any page in the browser could ride, and an agent has
// no use for one.
func (s *Server) mcpEndpoint() http.Handler {
	h := s.mcp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			s.oauthChallenge(w, false)
			jsonError(w, http.StatusUnauthorized, "send a personal token as Authorization: Bearer <token>; create one on the Tokens page")
			return
		}
		p, err := s.store.AuthenticateToken(r.Context(), bearer)
		if err != nil && strings.HasPrefix(bearer, "caa_") && s.oauth != nil && baseURL(r) == s.oauth.origin {
			p, err = s.oauthAuthenticate(r.Context(), bearer)
		}
		if err != nil {
			s.oauthChallenge(w, true)
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		p.Transport = "mcp"
		caller := mcptools.Caller{Viewer: s.svc.Viewer(p.Owner), BaseURL: baseURL(r), Provenance: p}
		h.ServeHTTP(w, r.WithContext(mcptools.WithCaller(r.Context(), caller)))
	})
}

// authenticated runs h as the token's or cookie's owner and calls unauthenticated
// when the request carries neither.
func (s *Server) authenticated(h, unauthenticated http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			p, err := s.store.AuthenticateToken(r.Context(), bearer)
			if err != nil {
				jsonError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			p.Transport = "http"
			provenance := service.WithTokenProvenance(r.Context(), p)
			signed := service.AgentRequest(s.signedIn(r.WithContext(provenance), p.Owner))
			s.withTokenUse(w, signed, p, h)
			return
		}
		username, ok := s.sessionUser(r)
		if !ok {
			unauthenticated(w, r)
			return
		}
		if r.Header.Get("X-Pictor") == "" || (r.Header.Get("Origin") != "" && !sameOrigin(r)) {
			jsonError(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
		h(w, s.signedIn(r, username))
	}
}

// baseURL is the address people use: TLS ends at the ingress, which says so
// in X-Forwarded-Proto.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func sameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Host != "" && origin.Host == r.Host
}
