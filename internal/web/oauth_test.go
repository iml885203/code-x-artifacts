package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// The configured public origin is independent of test listener addresses; requests
// carry that Host exactly as a reverse proxy would do.
func oauthHarness(t *testing.T) *harness {
	return newHarness(t, func(c *Config) { c.OAuthOrigin = "https://localhost" })
}

func oauthDo(h *harness, c *http.Client, method, path, body string, headers map[string]string) (response, string) {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	req.Host = "localhost"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c == nil {
		c = h.client(nil)
	}
	if c.Jar != nil {
		u, _ := url.Parse(h.srv.URL)
		for _, cookie := range c.Jar.Cookies(u) {
			req.AddCookie(cookie)
		}
		c2 := *c
		c2.Jar = nil
		c = &c2
	}
	res, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode, Header: res.Header}, string(b)
}

func oauthClientFor(t *testing.T, h *harness, name string) string {
	t.Helper()
	res, b := oauthDo(h, nil, "POST", "/oauth/register", `{"client_name":"`+name+`","redirect_uris":["http://127.0.0.1:9876/callback"],"token_endpoint_auth_method":"none"}`, map[string]string{"Content-Type": "application/json"})
	if res.StatusCode != 201 {
		t.Fatalf("register: %d %s", res.StatusCode, b)
	}
	return decode[map[string]any](t, b)["client_id"].(string)
}

func oauthAuthorizeCode(t *testing.T, h *harness, client, username, decision string) (string, string) {
	t.Helper()
	browser := h.browser(username, map[string]string{"alice": "pw-a", "bob": "pw-b"}[username])
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {"http://127.0.0.1:9876/callback"}, "resource": {"https://localhost/api/mcp"}, "state": {"original-state"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}}
	res, b := oauthDo(h, browser, "GET", "/oauth/authorize?"+q.Encode(), "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("consent: %d %s", res.StatusCode, b)
	}
	var boot struct {
		OAuth oauthBoot `json:"oauth"`
	}
	start := strings.Index(b, `id="pictor-boot"`)
	if start < 0 {
		start = strings.Index(b, `id="boot"`)
	}
	if start < 0 {
		t.Fatalf("no boot %s", b)
	}
	start += strings.Index(b[start:], ">") + 1
	end := strings.Index(b[start:], "</script>")
	if err := json.Unmarshal([]byte(b[start:start+end]), &boot); err != nil {
		t.Fatal(err)
	}
	for k, v := range boot.OAuth.Fields {
		q.Set(k, v)
	}
	q.Set("decision", decision)
	res, b = oauthDo(h, browser, "POST", "/oauth/authorize", q.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "https://localhost"})
	if res.StatusCode != 303 {
		t.Fatalf("consent post: %d %s", res.StatusCode, b)
	}
	u, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("state") != "original-state" || u.Query().Get("iss") != "https://localhost" {
		t.Fatalf("redirect identity %s", u)
	}
	if decision != "allow" {
		if u.Query().Get("error") != "access_denied" || u.Query().Get("code") != "" {
			t.Fatalf("denial %s", u)
		}
	}
	return u.Query().Get("code"), verifier
}

func oauthExchange(h *harness, q url.Values) (response, string) {
	return oauthDo(h, nil, "POST", "/oauth/token", q.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
}

func oauthCodeForm(client, code, verifier string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {client}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {"http://127.0.0.1:9876/callback"}, "resource": {"https://localhost/api/mcp"}}
}

func TestOAuthDiscoveryConsentPKCEAndGrantRevocation(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	res, b := oauthDo(h, nil, "GET", "/.well-known/oauth-authorization-server", "", nil)
	if res.StatusCode != 200 || !strings.Contains(b, `"code_challenge_methods_supported":["S256"]`) {
		t.Fatalf("metadata %d %s", res.StatusCode, b)
	}
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, nil)
	if res.StatusCode != 401 || !strings.Contains(res.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("challenge %v", res)
	}
	client := oauthClientFor(t, h, "Claude Code")
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	res, b = oauthExchange(h, oauthCodeForm(client, code, verifier))
	oauthRequireStatus(t, res, 200, b)
	tokens := decode[map[string]any](t, b)
	access := tokens["access_token"].(string)
	refresh := tokens["refresh_token"].(string)
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode == 401 {
		t.Fatal("OAuth bearer rejected")
	}
	res, _ = oauthDo(h, nil, "GET", "/api/artifacts", "", map[string]string{"Authorization": "Bearer " + access})
	oauthRequireStatus(t, res, 401, b)
	renew := url.Values{"grant_type": {"refresh_token"}, "client_id": {client}, "refresh_token": {refresh}, "resource": {"https://localhost/api/mcp"}}
	res, b = oauthExchange(h, renew)
	oauthRequireStatus(t, res, 200, b)
	next := decode[map[string]any](t, b)
	if next["refresh_token"] == refresh {
		t.Fatal("refresh not rotated")
	}
	grants, err := h.st.OAuthGrants(t.Context(), "alice")
	if err != nil || len(grants) != 1 {
		t.Fatalf("grants %v %v", grants, err)
	}
	// A replay revokes the family, including its replacement access and refresh.
	res, _ = oauthExchange(h, renew)
	oauthRequireStatus(t, res, 400, b)
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + next["access_token"].(string)})
	oauthRequireStatus(t, res, 401, b)

	res, _ = oauthExchange(h, oauthCodeForm(client, code, verifier))
	oauthRequireStatus(t, res, 400, b)
	// Independent grants and PATs survive revoking this family.
	code, verifier = oauthAuthorizeCode(t, h, client, "alice", "allow")
	res, b = oauthExchange(h, oauthCodeForm(client, code, verifier))
	oauthRequireStatus(t, res, 200, b)
	other := decode[map[string]any](t, b)["access_token"].(string)
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + other})
	if res.StatusCode == 401 {
		t.Fatal("independent grant refused")
	}
	pat := h.token("alice")
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + pat})
	if res.StatusCode == 401 {
		t.Fatal("PAT regression")
	}
}

func TestOAuthRejectsWrongBindingsAndUnsafeRegistration(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Codex")
	other := oauthClientFor(t, h, "Other")
	for _, change := range []string{"pkce", "redirect", "resource", "client"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
			q := oauthCodeForm(client, code, verifier)
			switch change {
			case "pkce":
				q.Set("code_verifier", strings.Repeat("b", 43))
			case "redirect":
				q.Set("redirect_uri", "http://127.0.0.1:9876/other")
			case "resource":
				q.Set("resource", "https://elsewhere.test/api/mcp")
			case "client":
				q.Set("client_id", other)
			}
			res, b := oauthExchange(h, q)
			if res.StatusCode != 400 {
				t.Fatalf("binding %d %s", res.StatusCode, b)
			}
		})
	}
	for _, redirect := range []string{"http://evil.test/callback", "javascript:alert(1)", "https://ok.test/callback#fragment", "https://user:secret@ok.test/callback"} {
		res, _ := oauthDo(h, nil, "POST", "/oauth/register", `{"redirect_uris":["`+redirect+`"]}`, nil)
		if res.StatusCode != 400 {
			t.Errorf("unsafe redirect %s accepted", redirect)
		}
	}
	if code, _ := oauthAuthorizeCode(t, h, client, "alice", "deny"); code != "" {
		t.Fatal("denied consent has code")
	}
	res, _ := h.do(nil, "GET", "/.well-known/oauth-authorization-server", "", nil)
	if res.StatusCode != 400 {
		t.Fatal("unconfigured request Host accepted")
	}
}

func TestOAuthOneCodeCannotBeExchangedConcurrently(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Race")
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wg.Go(func() { res, _ := oauthExchange(h, oauthCodeForm(client, code, verifier)); results <- res.StatusCode })
	}
	wg.Wait()
	close(results)
	ok := 0
	for status := range results {
		if status == 200 {
			ok++
		}
	}
	if ok > 1 {
		t.Fatalf("successful exchanges %d", ok)
	}
	grants, _ := h.st.OAuthGrants(t.Context(), "alice")
	if len(grants) != 1 || !grants[0].Revoked {
		t.Fatal("code replay did not revoke grant")
	}
}

func TestOAuthOriginValidation(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"https://example.com", "http://localhost:8080", "http://127.0.0.1:8000", "http://[::1]:9000", ""} {
		if err := ValidateOAuthOrigin(s); err != nil {
			t.Errorf("valid %s: %v", s, err)
		}
	}
	for _, s := range []string{"http://example.com", "https://example.com/", "https://example.com?q=1", "https://u:p@example.com", "https://example.com#f", "//example.com", "https://example.com/extra", "https://example.com?", "https://:443"} {
		if err := ValidateOAuthOrigin(s); err == nil {
			t.Errorf("invalid %s accepted", s)
		}
	}
}

func TestOAuthExpiredAccessAndRevokedGrantFailAfterRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oauth.db")
	st, err := store.Open(t.Context(), path)
	oauthRequireNoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	h := newHarnessOn(t, st, func(c *Config) { c.OAuthOrigin = "https://localhost" })
	client := oauthClientFor(t, h, "Persist")
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	res, b := oauthExchange(h, oauthCodeForm(client, code, verifier))
	if res.StatusCode != 200 {
		t.Fatalf("exchange %s", b)
	}
	token := decode[map[string]any](t, b)["access_token"].(string)
	h.srv.Close()
	if err := h.st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(t.Context(), path)
	oauthRequireNoError(t, err)
	h.st = st
	restart := newHarnessOn(t, st, func(c *Config) { c.OAuthOrigin = "https://localhost" })
	res, _ = oauthDo(restart, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode == 401 {
		t.Fatal("grant lost on restart")
	}
	raw, err := h.st.OAuthGet(t.Context(), "access", oauthHash(token))
	oauthRequireNoError(t, err)
	if strings.Contains(string(raw), token) {
		t.Fatal("plaintext token persisted")
	}
	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	info["AccessCreateAt"] = time.Now().Add(-2 * time.Hour)
	raw, _ = json.Marshal(info)
	if err := h.st.OAuthPut(t.Context(), "access", oauthHash(token), raw, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	res, _ = oauthDo(restart, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != 401 {
		t.Fatal("expired access accepted")
	}
	info["AccessCreateAt"] = time.Now()
	raw, _ = json.Marshal(info)
	if err := h.st.OAuthPut(t.Context(), "access", oauthHash(token), raw, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := oauthAgent(restart, token).ok("whoami", nil); got["username"] != "alice" {
		t.Fatal("restored active token wrong owner")
	}
	grants, _ := h.st.OAuthGrants(t.Context(), "alice")
	if err := h.st.RevokeOAuthGrant(t.Context(), "bob", grants[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("foreign revoke")
	}
	if err := h.st.RevokeOAuthGrant(t.Context(), "alice", grants[0].ID); err != nil {
		t.Fatal(err)
	}
	restart.srv.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(t.Context(), path)
	oauthRequireNoError(t, err)
	final := newHarnessOn(t, st, func(c *Config) { c.OAuthOrigin = "https://localhost" })
	res, _ = oauthDo(final, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != 401 {
		t.Fatal("revoked grant resurrected after reopen")
	}
}

func TestOAuthConsentCannotBeForgedAndGrantManagementIsBrowserOwnerOnly(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Owned client")
	alice := h.browser("alice", "pw-a")
	bob := h.browser("bob", "pw-b")
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {"http://127.0.0.1:9876/callback"}, "resource": {"https://localhost/api/mcp"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "decision": {"allow"}, "csrf": {"invented"}}
	res, _ := oauthDo(h, alice, "POST", "/oauth/authorize", q.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "https://localhost"})
	if res.StatusCode != 400 {
		t.Fatal("forged consent accepted")
	}
	res, _ = oauthDo(h, alice, "POST", "/oauth/authorize", q.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": "null"})
	if res.StatusCode != 403 {
		t.Fatal("opaque origin consent accepted")
	}
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	res, b := oauthExchange(h, oauthCodeForm(client, code, verifier))
	if res.StatusCode != 200 {
		t.Fatalf("exchange %s", b)
	}
	access := decode[map[string]any](t, b)["access_token"].(string)
	grants, _ := h.st.OAuthGrants(t.Context(), "alice")
	id := grants[0].ID
	res, b = oauthDo(h, bob, "GET", "/api/oauth/grants", "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 200 || b != "[]\n" {
		t.Fatalf("foreign grants %d %s", res.StatusCode, b)
	}
	res, _ = oauthDo(h, bob, "DELETE", "/api/oauth/grants/"+id, "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 404 {
		t.Fatal("foreign owner revocation accepted")
	}
	res, _ = oauthDo(h, nil, "DELETE", "/api/oauth/grants/"+id, "", map[string]string{"Authorization": "Bearer " + h.token("alice")})
	if res.StatusCode != 401 {
		t.Fatal("PAT can revoke OAuth grants")
	}
	res, _ = oauthDo(h, alice, "DELETE", "/api/oauth/grants/"+id, "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 204 {
		t.Fatal("owner revoke failed")
	}
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode != 401 {
		t.Fatal("revoked bearer accepted")
	}
	res, b = oauthDo(h, alice, "GET", "/api/oauth/grants", "", map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 200 || b != "[]\n" {
		t.Fatal("revoked grant remains connected")
	}
	res, _ = oauthDo(h, alice, "POST", "/api/mcp", `{}`, map[string]string{"X-Pictor": "1"})
	if res.StatusCode != 401 {
		t.Fatal("browser cookie accepted as MCP credential")
	}
}

type oauthHostTransport struct{ next http.RoundTripper }

func (t oauthHostTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = "localhost"
	return t.next.RoundTrip(r)
}

func oauthAgent(h *harness, token string) *agent {
	h.t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-test", Version: "1"}, nil)
	session, err := client.Connect(h.t.Context(), &mcp.StreamableClientTransport{Endpoint: h.srv.URL + "/api/mcp", HTTPClient: &http.Client{Transport: oauthHostTransport{bearerTransport{token: token, next: h.srv.Client().Transport}}}}, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = session.Close() })
	return &agent{t: h.t, session: session}
}

func TestOAuthMCPUsesTheGrantOwnerAndExistingPrivateArtifactPermissions(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Two owners")
	connect := func(owner string) *agent {
		code, verifier := oauthAuthorizeCode(t, h, client, owner, "allow")
		res, b := oauthExchange(h, oauthCodeForm(client, code, verifier))
		if res.StatusCode != 200 {
			t.Fatalf("%s exchange %s", owner, b)
		}
		return oauthAgent(h, decode[map[string]any](t, b)["access_token"].(string))
	}
	alice, bob := connect("alice"), connect("bob")
	for owner, a := range map[string]*agent{"alice": alice, "bob": bob} {
		if got := a.ok("whoami", nil); got["username"] != owner {
			t.Fatalf("wrong owner %+v", got)
		}
	}
	made := alice.ok("publish", map[string]any{"title": "OAuth private", "content": page})
	if made["author"] != "alice" || made["visibility"] != "private" {
		t.Fatalf("publish %+v", made)
	}
	id := made["id"].(string)
	if got := alice.ok("read", map[string]any{"id": id}); got["content"] != page {
		t.Fatalf("read %+v", got)
	}
	if got := alice.ok("list", map[string]any{"scope": "mine"}); got["total"].(float64) != 1 {
		t.Fatalf("list %+v", got)
	}
	if got := bob.ok("list", map[string]any{"scope": "all"}); got["total"].(float64) != 0 {
		t.Fatalf("private leak %+v", got)
	}
	if got := bob.refused("read", map[string]any{"id": id}); !strings.Contains(got, "no artifact") {
		t.Fatalf("private refusal %s", got)
	}
	bob.refused("publish", map[string]any{"id": id, "title": "Forbidden", "content": page})
}

func TestOAuthSimultaneousRefreshReplayRevokesTheWinningFamily(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Concurrent refresh")
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	res, b := oauthExchange(h, oauthCodeForm(client, code, verifier))
	if res.StatusCode != 200 {
		t.Fatalf("exchange %s", b)
	}
	original := decode[map[string]any](t, b)
	q := url.Values{"grant_type": {"refresh_token"}, "client_id": {client}, "resource": {"https://localhost/api/mcp"}, "refresh_token": {original["refresh_token"].(string)}}
	type result struct {
		status int
		body   string
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { <-start; res, b := oauthExchange(h, q); results <- result{res.StatusCode, b} })
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for result := range results {
		if result.status == 200 {
			success++
			tokens := decode[map[string]any](t, result.body)
			res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + tokens["access_token"].(string)})
			if res.StatusCode != 401 {
				t.Fatal("concurrent replay left winner bearer valid")
			}
		} else if result.status != 400 {
			t.Fatalf("unexpected replay status %d %s", result.status, result.body)
		}
	}
	if success > 1 {
		t.Fatalf("multiple successful renewals %d", success)
	}
	grants, _ := h.st.OAuthGrants(t.Context(), "alice")
	if len(grants) != 1 || !grants[0].Revoked {
		t.Fatalf("grant not revoked %+v", grants)
	}
}

func TestOAuthAuthorizationCodeReplayRevokesPreviouslyIssuedTokens(t *testing.T) {
	t.Parallel()
	h := oauthHarness(t)
	client := oauthClientFor(t, h, "Code replay")
	code, verifier := oauthAuthorizeCode(t, h, client, "alice", "allow")
	q := oauthCodeForm(client, code, verifier)
	res, b := oauthExchange(h, q)
	if res.StatusCode != 200 {
		t.Fatalf("exchange %s", b)
	}
	tokens := decode[map[string]any](t, b)
	res, _ = oauthExchange(h, q)
	if res.StatusCode != 400 {
		t.Fatal("code replay accepted")
	}
	res, _ = oauthDo(h, nil, "POST", "/api/mcp", `{}`, map[string]string{"Authorization": "Bearer " + tokens["access_token"].(string)})
	if res.StatusCode != 401 {
		t.Fatal("code replay left issued token live")
	}
	res, _ = oauthExchange(h, url.Values{"grant_type": {"refresh_token"}, "client_id": {client}, "resource": {"https://localhost/api/mcp"}, "refresh_token": {tokens["refresh_token"].(string)}})
	if res.StatusCode != 400 {
		t.Fatal("code replay left refresh live")
	}
}

func oauthRequireStatus(t *testing.T, res response, want int, body string) {
	t.Helper()
	if res.StatusCode != want {
		t.Fatalf("HTTP %d, want %d: %s", res.StatusCode, want, body)
	}
}

func oauthRequireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
