package web

import (
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
)

// Provider notifications have no browser cookie/Origin authority. Only the
// configured provider's verified signed event can revoke browser sessions.
func (s *Server) oidcBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	if !s.oidcOrigin(w, r) {
		return
	}
	oauthHeaders(w)
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		http.Error(w, "expected logout form", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
	if err := r.ParseForm(); err != nil || len(r.PostForm["logout_token"]) != 1 || r.URL.RawQuery != "" {
		http.Error(w, "invalid logout form", http.StatusBadRequest)
		return
	}
	event, err := s.oidc.VerifyLogout(r.Context(), r.PostForm.Get("logout_token"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrOIDCUnavailable) {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "logout notification could not be verified", status)
		return
	}
	s.browserAdmission.Lock()
	defer s.browserAdmission.Unlock()
	if err := s.store.RevokeOIDCBrowsers(r.Context(), event.Issuer, event.Subject, event.ID, event.IssuedAt, event.Expires, time.Now()); err != nil {
		http.Error(w, "logout temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
