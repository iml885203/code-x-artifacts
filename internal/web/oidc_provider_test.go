package web

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/iml885203/code-x-artifacts/internal/auth"
)

// A third-party boundary fixture with real discovery, RSA JWKS, PKCE code
// exchange and signed tokens. Each test owns its provider and identity.
type oidcProvider struct {
	LastIDToken            string
	TokenStatus, KeyStatus int
	server                 *httptest.Server
	key                    *rsa.PrivateKey
	mu                     sync.Mutex
	codes                  map[string]url.Values
	Subject, Name, Email   string
	Groups                 []string
	Claims                 map[string]any
	Deny                   bool
	BadSignature           bool
}

func newOIDCProvider(t *testing.T) *oidcProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &oidcProvider{key: key, codes: map[string]url.Values{}, Subject: "alice-subject", Name: "Alice OIDC", Email: "alice@example.org", Groups: []string{"artists"}}
	p.server = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.server.Close)
	return p
}

func (p *oidcProvider) config(origin string) auth.OIDCConfig {
	return auth.OIDCConfig{Issuer: p.server.URL, ClientID: "test-client", ClientSecret: "test-client-secret", Origin: origin, ProviderName: "Test identity", Scopes: []string{"profile", "email", "groups"}, AllowedGroups: []string{"artists"}, GroupMap: map[string]string{"artists": "SE"}}
}

func (p *oidcProvider) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		writeJSON(w, 200, map[string]any{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}})
	case "/keys":
		if p.KeyStatus > 0 {
			http.Error(w, "temporary key service failure", p.KeyStatus)
			return
		}
		writeJSON(w, 200, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes())}}})
	case "/authorize":
		q := r.URL.Query()
		callback, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			http.Error(w, "invalid callback", 400)
			return
		}
		v := callback.Query()
		v.Set("state", q.Get("state"))
		if p.Deny {
			v.Set("error", "access_denied")
		} else {
			code := oauthSecret("test_code_")
			p.codes[code] = q
			v.Set("code", code)
		}
		callback.RawQuery = v.Encode()
		http.Redirect(w, r, callback.String(), http.StatusSeeOther) //nolint:gosec // G710: synthetic IdP fixture echoes its test-owned registered callback.
	case "/token":
		p.token(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *oidcProvider) token(w http.ResponseWriter, r *http.Request) {
	if p.TokenStatus > 0 {
		http.Error(w, "temporary token service failure", p.TokenStatus)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	q, ok := p.codes[r.Form.Get("code")]
	delete(p.codes, r.Form.Get("code"))
	verifier := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	client, secret, _ := r.BasicAuth()
	if client == "" {
		client = r.Form.Get("client_id")
		secret = r.Form.Get("client_secret")
	}
	if !ok || client != "test-client" || secret != "test-client-secret" || r.Form.Get("redirect_uri") != q.Get("redirect_uri") || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(verifier[:]) {
		writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
		return
	}
	claims := jwt.MapClaims{"iss": p.server.URL, "sub": p.Subject, "aud": "test-client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce"), "name": p.Name, "email": p.Email, "groups": p.Groups}
	for k, v := range p.Claims {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	signKey := p.key
	if p.BadSignature {
		var keyErr error
		signKey, keyErr = rsa.GenerateKey(rand.Reader, 2048)
		if keyErr != nil {
			http.Error(w, "sign failed", 500)
			return
		}
	}
	signed, err := token.SignedString(signKey)
	if err != nil {
		http.Error(w, "sign failed", 500)
		return
	}
	p.LastIDToken = signed
	writeJSON(w, 200, map[string]any{"access_token": "provider-token-not-for-our-api", "token_type": "Bearer", "expires_in": 3600, "id_token": signed})
}

func oidcFollow(t *testing.T, c *http.Client, rawURL string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), "GET", rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode, Header: res.Header, cookies: res.Cookies()}
}
