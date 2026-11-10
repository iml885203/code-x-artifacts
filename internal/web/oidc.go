package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const (
	oidcTransactionLife = 10 * time.Minute
	oidcCookie          = "pictor_oidc"
)

type oidcLoginBoot struct {
	ProviderName string `json:"providerName"`
	StartURL     string `json:"startUrl"`
}
type oidcTransaction struct {
	Browser, Nonce, Next string
	StartedAt            int64
}

func (s *Server) routesOIDC(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/oidc/start", s.oidcStart)
	mux.HandleFunc("POST /auth/backchannel-logout", s.oidcBackchannelLogout)
	mux.HandleFunc("GET /auth/oidc/callback", s.oidcCallback)
}

func (s *Server) oidcLogin(next, key string, status int) pageData {
	login := &loginBoot{Next: safeNext(next), Error: key}
	if s.oidc != nil {
		login.OIDC = &oidcLoginBoot{ProviderName: s.oidc.ProviderName(), StartURL: "/auth/oidc/start?next=" + url.QueryEscape(login.Next)}
	}
	return pageData{page: "login", status: status, login: login}
}

func (s *Server) oidcOrigin(w http.ResponseWriter, r *http.Request) bool {
	if s.oidc == nil {
		http.NotFound(w, r)
		return false
	}
	if baseURL(r) != s.oidc.Origin() {
		http.Error(w, "use the configured OIDC origin", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if !s.oidcOrigin(w, r) {
		return
	}
	oauthHeaders(w)
	state, nonce, verifier := oauthSecret("state_"), oauthSecret("nonce_"), oauthSecret("verifier_")
	tx := oidcTransaction{Browser: oidcDigest(verifier), Nonce: nonce, Next: safeNext(r.URL.Query().Get("next")), StartedAt: time.Now().UnixNano()}
	data, _ := json.Marshal(tx)
	if err := s.store.OAuthPut(r.Context(), "oidc-login", oidcDigest(state), data, time.Now().Add(oidcTransactionLife)); err != nil {
		s.renderShell(w, r, s.oidcLogin(tx.Next, "login.error.oidcUnavailable", http.StatusServiceUnavailable))
		return
	}
	// Only the bound browser holds the PKCE verifier. SQLite retains its hash,
	// nonce and destination, never an upstream token or usable PKCE verifier.
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: verifier, Path: "/auth/oidc", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(oidcTransactionLife.Seconds())})
	http.Redirect(w, r, s.oidc.AuthURL(state, nonce, verifier), http.StatusSeeOther)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oidcOrigin(w, r) {
		return
	}
	oauthHeaders(w)
	tx, verifier, err := s.oidcTransaction(r)
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: "/auth/oidc", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if err != nil {
		s.renderShell(w, r, s.oidcLogin("/", "login.error.oidcInvalid", http.StatusBadRequest))
		return
	}
	if r.URL.Query().Get("error") != "" {
		s.renderShell(w, r, s.oidcLogin(tx.Next, "login.error.oidcDenied", http.StatusUnauthorized))
		return
	}
	identity, err := s.oidc.Exchange(r.Context(), r.URL.Query().Get("code"), verifier, tx.Nonce)
	if err != nil {
		key := "login.error.oidcInvalid"
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrOIDCUnavailable) {
			key = "login.error.oidcUnavailable"
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, auth.ErrNotAllowed) {
			key = "login.error.oidcDenied"
		}
		s.renderShell(w, r, s.oidcLogin(tx.Next, key, status))
		return
	}
	user, err := s.store.ResolveOIDCUser(r.Context(), identity.Issuer, identity.Subject, store.User{DisplayName: identity.DisplayName, Email: identity.Email, Groups: identity.Groups})
	if err != nil {
		s.renderShell(w, r, s.oidcLogin(tx.Next, "login.error.oidcUnavailable", http.StatusServiceUnavailable))
		return
	}
	if !s.issueBrowserSession(w, r, user.Username, store.BrowserIdentity{Issuer: identity.Issuer, Subject: identity.Subject, IssuedAt: identity.IssuedAt, StartedAt: tx.StartedAt}) {
		return
	}
	http.Redirect(w, r, tx.Next, http.StatusSeeOther)
}

func (s *Server) oidcTransaction(r *http.Request) (oidcTransaction, string, error) {
	var tx oidcTransaction
	q := r.URL.Query()
	for _, values := range q {
		if len(values) != 1 {
			return tx, "", errors.New("ambiguous OIDC callback")
		}
	}
	state := q.Get("state")
	cookie, err := r.Cookie(oidcCookie)
	if err != nil || len(state) > 256 || len(cookie.Value) > 256 {
		return tx, "", errors.New("invalid OIDC transaction")
	}
	data, err := s.store.OAuthGet(r.Context(), "oidc-login", oidcDigest(state))
	if err != nil || json.Unmarshal(data, &tx) != nil || !hmac.Equal([]byte(tx.Browser), []byte(oidcDigest(cookie.Value))) {
		return tx, "", errors.New("invalid OIDC transaction")
	}
	// Unbound callbacks cannot consume another browser's transaction.
	if _, err = s.store.OAuthTake(r.Context(), "oidc-login", oidcDigest(state)); err != nil {
		return tx, "", err
	}
	return tx, cookie.Value, nil
}

func oidcDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Server) issueBrowserSession(w http.ResponseWriter, r *http.Request, username string, identity store.BrowserIdentity) bool {
	value, err := s.sessions.issue(r.Context(), username, identity, time.Now())
	if err != nil {
		key, status := "login.error.unavailable", http.StatusServiceUnavailable
		if errors.Is(err, store.ErrBrowserRevoked) {
			key, status = "login.error.oidcInvalid", http.StatusUnauthorized
		}
		s.renderShell(w, r, s.oidcLogin("/", key, status))
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(s.sessions.ttl.Seconds())})
	return true
}

func reservedOIDCAccount(username string) bool {
	return strings.HasPrefix(auth.NormalizeUsername(username), "oidc:")
}
