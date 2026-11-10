package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
)

func oidcHarness(t *testing.T, p *oidcProvider, change ...func(*auth.OIDCConfig)) *harness {
	t.Helper()
	cfg := p.config("https://localhost")
	for _, f := range change {
		f(&cfg)
	}
	login, err := auth.NewOIDC(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return newHarness(t, func(cfg *Config) { cfg.OIDC = login; cfg.OAuthOrigin = "https://localhost" })
}

func oidcRequest(h *harness, c *http.Client, path string) (response, string) {
	return oidcHTTP(h, c, "GET", path, "", nil)
}

func oidcHTTP(h *harness, c *http.Client, method, path, body string, headers map[string]string) (response, string) {
	h.t.Helper()
	r, err := http.NewRequestWithContext(h.t.Context(), method, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	r.Host = "localhost"
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	if method != "GET" {
		r.Header.Set("Origin", "https://localhost")
	}
	if c == nil {
		c = h.client(nil)
	}
	res, err := c.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode, Header: res.Header, cookies: res.Cookies()}, string(data)
}

func oidcAttempt(t *testing.T, h *harness, next string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := h.client(jar)
	res, b := oidcRequest(h, browser, "/auth/oidc/start?next="+url.QueryEscape(next))
	if res.StatusCode != 303 {
		t.Fatalf("start %d %s", res.StatusCode, b)
	}
	authURL := res.Header.Get("Location")
	provider := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	redirect := oidcFollow(t, provider, authURL)
	if redirect.StatusCode != 303 {
		t.Fatalf("provider status %d", redirect.StatusCode)
	}
	callback, err := url.Parse(redirect.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return browser, callback.RequestURI()
}

func oidcBrowser(t *testing.T, h *harness, next string) (*http.Client, string) {
	t.Helper()
	browser, callback := oidcAttempt(t, h, next)
	res, b := oidcRequest(h, browser, callback)
	if res.StatusCode != 303 {
		t.Fatalf("callback %d %s", res.StatusCode, b)
	}
	res, b = oidcRequest(h, browser, "/")
	if res.StatusCode != 200 {
		t.Fatalf("gallery %d", res.StatusCode)
	}
	return browser, bootOf(t, b).User
}

func TestOIDCSignInKeepsIdentityAndExistingArtifactPermissions(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	alice, user := oidcBrowser(t, h, "/connect")
	if !strings.HasPrefix(user, "oidc:") {
		t.Fatalf("identity %q", user)
	}
	who := h.agent(h.token(user))
	if got := who.ok("whoami", nil)["username"]; got != user {
		t.Fatalf("MCP identity %v", got)
	}
	made := who.ok("publish", map[string]any{"title": "OIDC private report", "content": page})
	id := made["id"].(string)
	p.mu.Lock()
	p.Name = "Renamed Alice"
	p.mu.Unlock()
	_, again := oidcBrowser(t, h, "/")
	if again != user {
		t.Fatal("profile rename changed account")
	}
	p.mu.Lock()
	p.Subject = "bob-subject"
	p.Name = "Bob OIDC"
	p.mu.Unlock()
	bob, bobUser := oidcBrowser(t, h, "/")
	if bobUser == user {
		t.Fatal("same email linked different subject")
	}
	other := h.agent(h.token(bobUser))
	if reason := other.refused("read", map[string]any{"id": id}); reason == "" {
		t.Fatal("foreign private artifact readable")
	}
	other.refused("publish", map[string]any{"id": id, "title": "stolen", "content": page})
	res, _ := oidcHTTP(h, bob, "GET", "/api/artifacts/"+id, "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 404 {
		t.Fatalf("private HTTP status %d", res.StatusCode)
	}
	res, body := oidcHTTP(h, alice, "GET", "/api/artifacts/"+id+"/people?q=Bob", "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 200 || !strings.Contains(body, bobUser) {
		t.Fatalf("known OIDC colleague not found %d %s", res.StatusCode, body)
	}
	assertOIDCMentionSurvivesReload(t, h, h.token(user), body)
	res, body = oidcHTTP(h, alice, "PATCH", "/api/artifacts/"+id+"/editors", `{"add":["`+bobUser+`"]}`, map[string]string{"X-Pictor": "1", "Content-Type": "application/json"})
	if res.StatusCode != 200 {
		t.Fatalf("share %d %s", res.StatusCode, body)
	}
	other.ok("read", map[string]any{"id": id})
	res, _ = oidcHTTP(h, alice, "POST", "/logout", "", nil)
	if res.StatusCode != 303 {
		t.Fatal("logout")
	}
	res, _ = oidcHTTP(h, alice, "GET", "/connect", "", nil)
	if res.StatusCode != 303 {
		t.Fatal("logout left browser session")
	}
}

func TestOIDCRejectsInvalidTokensAndRestrictedIdentities(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]any{"missing issued at": {"iat": nil}, "future issued at": {"iat": time.Now().Add(time.Hour).Unix()}, "wrong issuer": {"iss": "https://other.example"}, "wrong audience": {"aud": "another-client"}, "wrong nonce": {"nonce": "other"}, "expired": {"exp": time.Now().Add(-time.Hour).Unix()}, "missing subject": {"sub": ""}, "wrong authorized party": {"azp": "other"}, "malformed authorized party": {"azp": 42}, "multiple audiences without party": {"aud": []string{"test-client", "other"}}, "missing required groups": {"groups": nil}, "wrong group shape": {"groups": "artists"}, "unpermitted group": {"groups": []string{"outsiders"}}}
	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := newOIDCProvider(t)
			p.Claims = claims
			h := oidcHarness(t, p)
			browser, callback := oidcAttempt(t, h, "/connect")
			res, _ := oidcRequest(h, browser, callback)
			if res.StatusCode == 303 {
				t.Fatal("invalid token signed in")
			}
			res, _ = h.do(browser, "GET", "/connect", "", nil)
			if res.StatusCode != 303 {
				t.Fatal("invalid token created session")
			}
		})
	}
}

func TestOIDCTransactionsAreBoundToTheBrowserExpireAndCannotReplay(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	browser, callback := oidcAttempt(t, h, "//evil.example")
	foreign := h.client(nil)
	res, _ := oidcRequest(h, foreign, callback)
	if res.StatusCode != 400 {
		t.Fatal("unbound browser accepted")
	}
	res, _ = oidcRequest(h, browser, callback)
	if res.StatusCode != 303 || res.Header.Get("Location") != "/" {
		t.Fatal("safe same-browser destination not preserved")
	}
	res, _ = oidcRequest(h, browser, callback)
	if res.StatusCode != 400 {
		t.Fatal("callback replay accepted")
	}
	browser, callback = oidcAttempt(t, h, "/")
	u, _ := url.Parse(callback)
	key := oidcDigest(u.Query().Get("state"))
	raw, err := h.st.OAuthGet(t.Context(), "oidc-login", key)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.OAuthPut(t.Context(), "oidc-login", key, raw, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	res, _ = oidcRequest(h, browser, callback)
	if res.StatusCode != 400 {
		t.Fatal("expired transaction accepted")
	}
}

func TestOIDCProviderFailureDenialAndPasswordFallbackDoNotSignIn(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	p.mu.Lock()
	p.Deny = true
	p.mu.Unlock()
	browser, callback := oidcAttempt(t, h, "/connect")
	res, body := oidcRequest(h, browser, callback)
	if res.StatusCode != 401 || bootOf(t, body).Login.Error != "login.error.oidcDenied" {
		t.Fatal("denial did not render retry")
	}
	p.mu.Lock()
	p.Deny = false
	p.mu.Unlock()
	browser, callback = oidcAttempt(t, h, "/connect")
	p.server.Close()
	res, body = oidcRequest(h, browser, callback)
	if res.StatusCode != 503 || bootOf(t, body).Login.Error != "login.error.oidcUnavailable" {
		t.Fatal("unavailable provider did not render actionable retry error")
	}
	res, _ = h.do(nil, "POST", "/login", "username=alice&password=pw-a", map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if res.StatusCode != 401 {
		t.Fatal("password fallback bypassed OIDC mode")
	}
	res, _ = h.do(nil, "GET", "/api/artifacts", "", map[string]string{"Authorization": "Bearer provider-token-not-for-our-api"})
	if res.StatusCode != 401 {
		t.Fatal("upstream token accepted as API credential")
	}
}

func TestOIDCAdmissionIntersectsSubjectAndGroupRestrictions(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p, func(cfg *auth.OIDCConfig) {
		cfg.TrustAppAssignment = true
		cfg.AllowedSubjects = []string{"permitted-subject"}
	})
	browser, callback := oidcAttempt(t, h, "/")
	res, _ := oidcRequest(h, browser, callback)
	if res.StatusCode != 401 {
		t.Fatal("trust assignment bypassed subject restriction")
	}
	p.mu.Lock()
	p.Subject = "permitted-subject"
	p.mu.Unlock()
	_, _ = oidcBrowser(t, h, "/")
}

func TestOIDCConfigurationRequiresTrustedEndpointsAndExplicitAdmission(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	for _, change := range []func(*auth.OIDCConfig){func(c *auth.OIDCConfig) { c.Issuer = "http://untrusted.example" }, func(c *auth.OIDCConfig) { c.Origin = "https://app.example/path" }, func(c *auth.OIDCConfig) { c.AllowedGroups = nil }, func(c *auth.OIDCConfig) { c.ClientID = "" }} {
		cfg := p.config("https://localhost")
		change(&cfg)
		if _, err := auth.NewOIDC(t.Context(), cfg); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestOIDCPasswordLoginCannotTakeOverReservedIdentity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res, _ := h.do(nil, "POST", "/login", "username=oidc%3Atest&password=pw-a", map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if res.StatusCode != 401 {
		t.Fatal("reserved identity passed password route")
	}
}

func TestOIDCPendingMCPAuthorizationSurvivesLoginWithoutImplicitConsent(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	client := oauthClientFor(t, h, "Waiting agent")
	q := url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {"http://127.0.0.1:9876/callback"}, "resource": {"https://localhost/api/mcp"}, "state": {"original-agent-state"}, "code_challenge": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "code_challenge_method": {"S256"}}
	next := "/oauth/authorize?" + q.Encode()
	browser, callback := oidcAttempt(t, h, next)
	res, _ := oidcRequest(h, browser, callback)
	if res.StatusCode != 303 || res.Header.Get("Location") != next {
		t.Fatal("pending request lost")
	}
	res, body := oidcRequest(h, browser, next)
	if res.StatusCode != 200 || !strings.Contains(body, `"page":"oauth"`) {
		t.Fatalf("consent %d %s", res.StatusCode, body)
	}
	var payload struct {
		OAuth oauthBoot `json:"oauth"`
	}
	matches := bootBlock.FindStringSubmatch(body)
	if err := json.Unmarshal([]byte(matches[1]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OAuth.Fields["state"] != "original-agent-state" {
		t.Fatal("MCP binding lost")
	}
}

func TestOIDCUntrustedTokenSignatureCannotCreateASession(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	p.BadSignature = true
	h := oidcHarness(t, p)
	browser, callback := oidcAttempt(t, h, "/")
	res, _ := oidcRequest(h, browser, callback)
	if res.StatusCode != 401 {
		t.Fatal("untrusted signing key accepted")
	}
}

func assertOIDCMentionSurvivesReload(t *testing.T, h *harness, token, searchBody string) {
	t.Helper()
	var found struct{ People []struct{ User, Name string } }
	if err := json.Unmarshal([]byte(searchBody), &found); err != nil {
		t.Fatal(err)
	}
	if len(found.People) != 1 {
		t.Fatal("expected the known colleague")
	}
	person := found.People[0]
	markdown := "Hello [@" + person.Name + "](mention:" + person.User + ")"
	doc := h.publishDoc(token, markdown)
	agent := h.agent(token)
	first := agent.ok("read", map[string]any{"id": doc.ID})
	if !strings.Contains(first["content"].(string), "mention:"+person.User) {
		t.Fatal("saving lost the discovered colleague mention")
	}
	restarted := newHarnessOn(t, h.st)
	loaded := restarted.agent(token).ok("read", map[string]any{"id": doc.ID, "view": "blocks"})
	data, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), person.User) {
		t.Fatal("reloading lost the stored mention identity")
	}
}

func TestOIDCTemporaryTokenAndKeyFailuresOfferProviderRetry(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"token", "keys"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			p := newOIDCProvider(t)
			h := oidcHarness(t, p)
			p.mu.Lock()
			if stage == "token" {
				p.TokenStatus = 503
			} else {
				p.KeyStatus = 503
			}
			p.mu.Unlock()
			browser, callback := oidcAttempt(t, h, "/connect")
			res, body := oidcRequest(h, browser, callback)
			if res.StatusCode != 503 || bootOf(t, body).Login.Error != "login.error.oidcUnavailable" || bootOf(t, body).Login.OIDC == nil {
				t.Fatal("temporary provider failure did not preserve explicit retry")
			}
		})
	}
}

func TestOIDCProviderCredentialsNeverBecomeApplicationBearerCredentials(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	browser, _ := oidcBrowser(t, h, "/")
	p.mu.Lock()
	idToken := p.LastIDToken
	p.mu.Unlock()
	if idToken == "" {
		t.Fatal("fixture issued no ID token")
	}
	_, body := oidcRequest(h, browser, "/")
	for _, secret := range []string{idToken, "provider-token-not-for-our-api"} {
		if strings.Contains(body, secret) {
			t.Fatal("provider credential exposed in frontend")
		}
		for _, endpoint := range []struct{ method, path string }{{"GET", "/api/artifacts"}, {"POST", "/api/mcp"}} {
			res, _ := oidcHTTP(h, nil, endpoint.method, endpoint.path, "{}", map[string]string{"Authorization": "Bearer " + secret, "Content-Type": "application/json"})
			if res.StatusCode != 401 {
				t.Fatalf("upstream credential accepted at %s", endpoint.path)
			}
		}
	}
}

func TestOIDCValidFractionalIssuedAtCreatesTheBoundAccount(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	// RFC 7519 NumericDate permits fractional seconds, not just integers.
	p.Claims = map[string]any{"iat": float64(time.Now().Unix()) - 0.5}
	h := oidcHarness(t, p)
	_, user := oidcBrowser(t, h, "/connect")
	if !strings.HasPrefix(user, "oidc:") {
		t.Fatal("valid fractional issued-at did not create the bound account")
	}
}
