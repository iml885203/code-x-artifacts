package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/iml885203/code-x-artifacts/internal/auth"
)

func logoutToken(t *testing.T, p *oidcProvider, changes map[string]any, typ string) string {
	t.Helper()
	return logoutTokenWithAlgorithm(t, p, changes, typ, jwt.SigningMethodRS256)
}

func logoutTokenWithAlgorithm(t *testing.T, p *oidcProvider, changes map[string]any, typ string, algorithm jwt.SigningMethod) string {
	t.Helper()
	claims := jwt.MapClaims{"iss": p.server.URL, "aud": "test-client", "sub": p.Subject, "iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(), "jti": oauthSecret("event_"), "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}}
	for k, v := range changes {
		claims[k] = v
	}
	token := jwt.NewWithClaims(algorithm, claims)
	token.Header["kid"] = "test-key"
	token.Header["typ"] = typ
	raw, err := token.SignedString(p.key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sendLogout(t *testing.T, h *harness, token string) int {
	t.Helper()
	r, _ := oidcHTTP(h, nil, "POST", "/auth/backchannel-logout", url.Values{"logout_token": {token}}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	return r.StatusCode
}

func TestLocalLogoutRevokesACopiedCookieButPreservesOtherSessionsAndTokens(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.browser("alice", "pw-a")
	other := h.browser("alice", "pw-a")
	token := h.token("alice")
	u, _ := url.Parse(h.srv.URL)
	copied := alice.Jar.Cookies(u)
	r, _ := h.do(alice, "POST", "/logout", "", map[string]string{"Origin": h.srv.URL})
	if r.StatusCode != 303 {
		t.Fatal(r.StatusCode)
	}
	cookie := ""
	for _, c := range copied {
		if c.Name == sessionCookie {
			cookie = c.String()
		}
	}
	r, _ = h.do(nil, "GET", "/api/artifacts", "", map[string]string{"Cookie": cookie, "X-Pictor": "1"})
	if r.StatusCode != 401 {
		t.Fatalf("copied cookie survived logout: %d", r.StatusCode)
	}
	r, _ = h.do(other, "GET", "/api/artifacts", "", map[string]string{"X-Pictor": "1"})
	if r.StatusCode != 200 {
		t.Fatal("other session revoked")
	}
	if _, err := h.st.AuthenticateToken(t.Context(), token); err != nil {
		t.Fatal("PAT revoked")
	}
	// Restart the HTTP server using durable state and the same session key.
	restarted := newHarnessOn(t, h.st)
	r, _ = restarted.do(nil, "GET", "/api/artifacts", "", map[string]string{"Cookie": cookie, "X-Pictor": "1"})
	if r.StatusCode != 401 {
		t.Fatal("revoked session resurrected")
	}
	for _, c := range other.Jar.Cookies(u) {
		if c.Name == sessionCookie {
			cookie = c.String()
		}
	}
	r, _ = restarted.do(nil, "GET", "/api/artifacts", "", map[string]string{"Cookie": cookie, "X-Pictor": "1"})
	if r.StatusCode != 200 {
		t.Fatal("unrevoked session did not survive")
	}
}

func TestLegacyBrowserCookiesRequireExplicitReloginWithoutChangingArtifacts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.browser("alice", "pw-a")
	artifact := h.publish(h.token("alice"), "Existing work", page)
	payload := base64.RawURLEncoding.EncodeToString([]byte("alice|" + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)))
	mac := hmac.New(sha256.New, []byte("test-key"))
	_, _ = mac.Write([]byte(payload))
	cookie := sessionCookie + "=" + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	r, _ := h.do(nil, "GET", "/api/artifacts", "", map[string]string{"Cookie": cookie, "X-Pictor": "1"})
	if r.StatusCode != 401 {
		t.Fatal("legacy cookie accepted")
	}
	r, b := h.do(nil, "GET", "/login", "", map[string]string{"Cookie": cookie})
	if r.StatusCode != 200 || bootOf(t, b).Login.Error != "login.error.sessionChanged" {
		t.Fatal("missing migration notice")
	}
	if _, err := h.st.Artifact(t.Context(), artifact.ID); err != nil {
		t.Fatal(err)
	}
}

func TestOIDCLogoutRevokesOnlyTheSubjectAndRejectsInvalidNotifications(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	alice, user := oidcBrowser(t, h, "/")
	second, _ := oidcBrowser(t, h, "/")
	token := h.token(user)
	p.mu.Lock()
	p.Subject = "other-subject"
	p.mu.Unlock()
	other, _ := oidcBrowser(t, h, "/")
	p.mu.Lock()
	p.Subject = "alice-subject"
	p.mu.Unlock()
	cases := map[string]map[string]any{
		"issuer": {"iss": "https://other.example"}, "audience": {"aud": "other"}, "expired": {"exp": time.Now().Add(-time.Minute).Unix()}, "future": {"iat": time.Now().Add(time.Hour).Unix()}, "old": {"iat": time.Now().Add(-time.Hour).Unix()}, "event": {"events": map[string]any{}}, "event shape": {"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": true}}, "subject": {"sub": ""}, "identifier": {"jti": ""}, "nonce": {"nonce": nil}, "sid only": {"sid": "one", "sub": ""}, "subject and sid": {"sid": "one"}, "null sid": {"sid": nil}, "long life": {"exp": time.Now().Add(time.Hour).Unix()}, "missing iat": {"iat": nil},
	}
	for name, change := range cases {
		if status := sendLogout(t, h, logoutToken(t, p, change, "logout+jwt")); status != 400 {
			t.Fatalf("%s: status %d", name, status)
		}
	}
	if status := sendLogout(t, h, logoutToken(t, p, nil, "JWT")); status != 400 {
		t.Fatal("wrong type accepted")
	}
	if status := sendLogout(t, h, logoutTokenWithAlgorithm(t, p, nil, "logout+jwt", jwt.SigningMethodRS512)); status != 400 {
		t.Fatal("unadvertised signing algorithm accepted")
	}
	// A different provider's key cannot authenticate an event for this issuer.
	wrong := newOIDCProvider(t)
	if status := sendLogout(t, h, logoutToken(t, wrong, map[string]any{"iss": p.server.URL}, "logout+jwt")); status != 400 {
		t.Fatal("wrong signature accepted")
	}
	r, _ := oidcRequest(h, alice, "/")
	if r.StatusCode != 200 {
		t.Fatal("invalid event revoked user")
	}
	event := logoutToken(t, p, nil, "logout+jwt")
	if status := sendLogout(t, h, event); status != 200 {
		t.Fatal(status)
	}
	for _, browser := range []*http.Client{alice, second} {
		r, _ := oidcHTTP(h, browser, "GET", "/api/artifacts", "", map[string]string{"X-Pictor": "1"})
		if r.StatusCode != 401 {
			t.Fatalf("revoked browser: %d", r.StatusCode)
		}
	}
	r, _ = oidcRequest(h, other, "/")
	if r.StatusCode != 200 {
		t.Fatal("unrelated subject revoked")
	}
	if _, err := h.st.AuthenticateToken(t.Context(), token); err != nil {
		t.Fatal("independent PAT revoked")
	}
	checkOIDCReplayAfterRestart(t, h, p, event)
}

func checkOIDCReplayAfterRestart(t *testing.T, h *harness, p *oidcProvider, event string) {
	t.Helper()
	// Future ID-token issuance represents a later fresh login without sleeping.
	p.mu.Lock()
	p.Claims = map[string]any{"iat": time.Now().Add(2 * time.Second).Unix()}
	p.mu.Unlock()
	fresh, _ := oidcBrowser(t, h, "/")
	cfg := p.config("https://localhost")
	login, err := auth.NewOIDC(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newHarnessOn(t, h.st, func(c *Config) { c.OIDC = login })
	if status := sendLogout(t, restarted, event); status != 200 {
		t.Fatal(status)
	}
	// Use the fresh cookie against the restarted server with the same key.
	u, _ := url.Parse("https://localhost")
	cookie := ""
	for _, c := range fresh.Jar.Cookies(u) {
		if c.Name == sessionCookie {
			cookie = c.String()
		}
	}
	r, _ := oidcHTTP(restarted, nil, "GET", "/api/artifacts", "", map[string]string{"X-Pictor": "1", "Cookie": cookie})
	if r.StatusCode != 200 {
		t.Fatalf("identical replay revoked fresh login: %d cookie length %d", r.StatusCode, len(cookie))
	}
	if status := sendLogout(t, restarted, logoutToken(t, p, nil, "logout+jwt")); status != 200 {
		t.Fatal(status)
	}
	r, _ = oidcHTTP(restarted, nil, "GET", "/api/artifacts", "", map[string]string{"X-Pictor": "1", "Cookie": cookie})
	if r.StatusCode != 401 {
		t.Fatal("fresh event did not revoke")
	}
}

func TestOIDCLogoutProtectsUnknownSubjectsAndPendingCallbacks(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	browser, callback := oidcAttempt(t, h, "/")
	if status := sendLogout(t, h, logoutToken(t, p, nil, "logout+jwt")); status != 200 {
		t.Fatal(status)
	}
	r, _ := oidcRequest(h, browser, callback)
	if r.StatusCode != 401 {
		t.Fatalf("pending callback accepted: %d", r.StatusCode)
	}
	r, _ = oidcRequest(h, browser, "/")
	if r.StatusCode != 303 {
		t.Fatal("callback minted session")
	}
}
