package web

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	oe "github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	osrv "github.com/go-oauth2/oauth2/v4/server"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"golang.org/x/time/rate"
)

var oauthVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

const (
	oauthScope       = "artifacts"
	oauthCodeLife    = 5 * time.Minute
	oauthAccessLife  = time.Hour
	oauthRefreshLife = 30 * 24 * time.Hour
)

type oauthServer struct {
	origin            string
	server            *osrv.Server
	manager           *manage.Manager
	clients           oauthClients
	tokens            oauthTokens
	registrationLimit *rate.Limiter
}
type oauthBoot struct {
	ClientName string            `json:"clientName"`
	ClientID   string            `json:"clientId"`
	Scope      string            `json:"scope"`
	Action     string            `json:"action"`
	Fields     map[string]string `json:"fields"`
}

// ValidateOAuthOrigin rejects request-derived issuers. HTTPS is mandatory except
// for an explicitly configured loopback development origin.
func ValidateOAuthOrigin(origin string) error {
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.ForceQuery || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" || u.String() != origin {
		return errors.New("OAuth origin must be an absolute origin without a path, credentials, query or fragment")
	}
	if !oauthSecureURL(u) {
		return errors.New("OAuth origin requires HTTPS (HTTP is allowed only on loopback)")
	}
	return nil
}

func oauthLoopback(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func newOAuthServer(origin string, st *store.Store) *oauthServer {
	if origin == "" {
		return nil
	}
	if err := ValidateOAuthOrigin(origin); err != nil {
		panic(err)
	}
	o := &oauthServer{origin: origin, clients: oauthClients{st}, tokens: oauthTokens{st}, registrationLimit: rate.NewLimiter(rate.Every(time.Minute), 20)}
	m := manage.NewDefaultManager()
	m.MapClientStorage(o.clients)
	m.MapTokenStorage(o.tokens)
	m.MapAccessGenerate(oauthGenerator{})
	m.MapAuthorizeGenerate(oauthCodeGenerator{})
	m.SetAuthorizeCodeExp(oauthCodeLife)
	m.SetAuthorizeCodeTokenCfg(&manage.Config{AccessTokenExp: oauthAccessLife, RefreshTokenExp: oauthRefreshLife, IsGenerateRefresh: true})
	m.SetRefreshTokenCfg(&manage.RefreshingConfig{AccessTokenExp: oauthAccessLife, IsGenerateRefresh: true, IsRemoveRefreshing: true})
	m.SetValidateURIHandler(func(clientID, redirect string) error {
		c, err := o.clients.client(context.Background(), clientID)
		if err != nil || !slices.Contains(c.Redirects, redirect) {
			return oe.ErrInvalidRedirectURI
		}
		return nil
	})
	m.SetExtractExtensionHandler(func(req *oauth2.TokenGenerateRequest, ti oauth2.ExtendableTokenInfo) {
		if grant, ok := req.Request.Context().Value(oauthGrantKey{}).(store.OAuthGrant); ok {
			ti.SetExtension(url.Values{"grant": {grant.ID}, "resource": {grant.Resource}})
		}
	})
	cfg := osrv.NewConfig()
	cfg.AllowedResponseTypes = []oauth2.ResponseType{oauth2.Code}
	cfg.AllowedGrantTypes = []oauth2.GrantType{oauth2.AuthorizationCode, oauth2.Refreshing}
	cfg.AllowedCodeChallengeMethods = []oauth2.CodeChallengeMethod{oauth2.CodeChallengeS256}
	cfg.ForcePKCE = true
	o.server = osrv.NewServer(cfg, m)
	o.server.SetClientInfoHandler(osrv.ClientFormHandler)
	o.manager = m
	return o
}

type oauthGrantKey struct{}

func (s *Server) oauthAvailable(w http.ResponseWriter, r *http.Request) bool {
	if s.oauth == nil {
		jsonError(w, http.StatusNotFound, "OAuth is not enabled")
		return false
	}
	// Never use an attacker-supplied Host to mint or advertise an issuer/resource.
	if baseURL(r) != s.oauth.origin {
		jsonError(w, http.StatusBadRequest, "use the configured OAuth origin")
		return false
	}
	return true
}

func oauthHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
}

func (s *Server) routesOAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.oauthResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/api/mcp", s.oauthResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.oauthAuthorizationMetadata)
	mux.HandleFunc("POST /oauth/register", s.oauthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.page(s.oauthAuthorize))
	mux.HandleFunc("POST /oauth/authorize", s.form(s.oauthAuthorize))
	mux.HandleFunc("POST /oauth/token", s.oauthToken)
	mux.HandleFunc("GET /api/oauth/grants", s.browserAPI(s.oauthListGrants))
	mux.HandleFunc("DELETE /api/oauth/grants/{id}", s.browserAPI(s.oauthRevokeGrant))
}

func (s *Server) oauthResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if !s.oauthAvailable(w, r) {
		return
	}
	oauthHeaders(w)
	writeJSON(w, 200, map[string]any{"resource": s.oauth.origin + "/api/mcp", "authorization_servers": []string{s.oauth.origin}, "scopes_supported": []string{oauthScope}, "bearer_methods_supported": []string{"header"}, "resource_name": "CodeXArtifacts"})
}

func (s *Server) oauthAuthorizationMetadata(w http.ResponseWriter, r *http.Request) {
	if !s.oauthAvailable(w, r) {
		return
	}
	oauthHeaders(w)
	o := s.oauth.origin
	writeJSON(w, 200, map[string]any{"issuer": o, "authorization_endpoint": o + "/oauth/authorize", "token_endpoint": o + "/oauth/token", "registration_endpoint": o + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{oauthScope}, "authorization_response_iss_parameter_supported": true})
}

func oauthError(w http.ResponseWriter, code string) {
	oauthHeaders(w)
	writeJSON(w, 400, map[string]string{"error": code})
}

type oauthRegistration struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func oauthSecureURL(u *url.URL) bool {
	return u.Scheme == "https" || u.Scheme == "http" && oauthLoopback(u.Hostname())
}

func oauthValidRedirect(redirect string) bool {
	u, err := url.Parse(redirect)
	return err == nil && len(redirect) <= 2048 && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.Opaque == "" && oauthSecureURL(u)
}

func (in oauthRegistration) validationError() string {
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 8 || in.TokenEndpointAuthMethod != "" && in.TokenEndpointAuthMethod != "none" {
		return "invalid_client_metadata"
	}
	for _, g := range in.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			return "invalid_client_metadata"
		}
	}
	for _, v := range in.ResponseTypes {
		if v != "code" {
			return "invalid_client_metadata"
		}
	}
	for _, redirect := range in.RedirectURIs {
		if !oauthValidRedirect(redirect) {
			return "invalid_redirect_uri"
		}
	}
	return ""
}

func oauthParseForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if len(r.URL.RawQuery) > 16<<10 {
		return errors.New("request query too long")
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	for _, vs := range r.Form {
		if len(vs) != 1 {
			return errors.New("duplicate protocol parameter")
		}
	}
	return nil
}

func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	if !s.oauthAvailable(w, r) {
		return
	}
	oauthHeaders(w)
	if !s.oauth.registrationLimit.Allow() {
		w.Header().Set("Retry-After", "60")
		jsonError(w, http.StatusTooManyRequests, "registration rate limit")
		return
	}
	var in oauthRegistration
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		oauthError(w, "invalid_client_metadata")
		return
	}
	if code := in.validationError(); code != "" {
		oauthError(w, code)
		return
	}

	name := store.BoundedLabel(strings.TrimSpace(in.ClientName), 60)
	if name == "" {
		name = "MCP client"
	}
	c := oauthClient{ID: oauthSecret("client_"), Name: name, Redirects: in.RedirectURIs}
	data, _ := json.Marshal(c)
	// Registrations expire after 90 days of inactivity; successful authorization
	// refreshes metadata. No network requests to client-controlled metadata URLs.
	if err := s.store.OAuthPut(r.Context(), "client", c.ID, data, time.Now().Add(90*24*time.Hour)); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.Redirects, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
}

func (o *oauthServer) validateAuthorization(r *http.Request) (*osrv.AuthorizeRequest, oauthClient, string) {
	ar, err := o.server.ValidationAuthorizeRequest(r)
	if err != nil {
		return nil, oauthClient{}, "invalid_request"
	}
	challenge, err := base64.RawURLEncoding.DecodeString(ar.CodeChallenge)
	if err != nil || len(challenge) != 32 || len(ar.State) > 2048 || len(ar.RedirectURI) > 2048 {
		return nil, oauthClient{}, "invalid_request"
	}
	c, err := o.clients.client(r.Context(), ar.ClientID)
	if err != nil || !slices.Contains(c.Redirects, ar.RedirectURI) {
		return nil, c, "invalid_request"
	}
	if ar.Scope != "" && ar.Scope != oauthScope {
		return nil, c, "invalid_scope"
	}
	if r.Form.Get("resource") != o.origin+"/api/mcp" {
		return nil, c, "invalid_target"
	}
	ar.Scope = oauthScope
	return ar, c, ""
}

func (s *Server) oauthIssueCode(r *http.Request, ar *osrv.AuthorizeRequest, c oauthClient) (string, error) {
	g := store.OAuthGrant{ID: oauthSecret("grant_"), Owner: currentUser(r), ClientID: c.ID, ClientName: c.Name, Resource: s.oauth.origin + "/api/mcp", CreatedAt: time.Now().UTC()}
	if err := s.store.CreateOAuthGrant(r.Context(), g); err != nil {
		return "", err
	}
	ctx := context.WithValue(r.Context(), oauthGrantKey{}, g)
	ar.UserID = g.Owner
	ar.Request = r.WithContext(ctx)
	ti, err := s.oauth.server.GetAuthorizeToken(ctx, ar)
	if err != nil {
		_ = s.store.RevokeOAuthGrant(r.Context(), g.Owner, g.ID)
		return "", err
	}
	data, _ := json.Marshal(c)
	if err := s.store.OAuthPut(r.Context(), "client", c.ID, data, time.Now().Add(90*24*time.Hour)); err != nil {
		return "", err
	}
	return ti.GetCode(), nil
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if !s.oauthAvailable(w, r) {
		return
	}
	oauthHeaders(w)
	if err := oauthParseForm(w, r); err != nil {
		oauthError(w, "invalid_request")
		return
	}
	ar, c, code := s.oauth.validateAuthorization(r)
	if code != "" {
		oauthError(w, code)
		return
	}

	fields := map[string]string{}
	for _, k := range []string{"response_type", "client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method", "resource"} {
		fields[k] = r.Form.Get(k)
	}
	canonical := url.Values{}
	for k, v := range fields {
		canonical.Set(k, v)
	}
	if r.Method == http.MethodGet {
		nonce := oauthSecret("consent_")
		record, _ := json.Marshal(map[string]string{"user": currentUser(r), "request": canonical.Encode()})
		if err := s.store.OAuthPut(r.Context(), "consent", oauthHash(nonce), record, time.Now().Add(10*time.Minute)); err != nil {
			s.serverError(w, err)
			return
		}
		fields["csrf"] = nonce
		w.Header().Set("Referrer-Policy", "same-origin")
		s.renderShell(w, r, pageData{page: "oauth", oauth: &oauthBoot{ClientName: c.Name, ClientID: c.ID, Scope: oauthScope, Action: "/oauth/authorize", Fields: fields}})
		return
	}
	b, err := s.store.OAuthTake(r.Context(), "consent", oauthHash(r.Form.Get("csrf")))
	var consent map[string]string
	if err != nil || json.Unmarshal(b, &consent) != nil || !hmac.Equal([]byte(consent["user"]), []byte(currentUser(r))) || !hmac.Equal([]byte(consent["request"]), []byte(canonical.Encode())) {
		oauthError(w, "invalid_request")
		return
	}
	redirect, _ := url.Parse(ar.RedirectURI)
	q := redirect.Query()
	q.Set("state", ar.State)
	q.Set("iss", s.oauth.origin)
	if r.Form.Get("decision") != "allow" {
		q.Set("error", "access_denied")
	} else {
		code, err := s.oauthIssueCode(r, ar, c)
		if err != nil {
			s.serverError(w, err)
			return
		}
		q.Set("code", code)
	}
	redirect.RawQuery = q.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusSeeOther)
}

func (o *oauthServer) validateTokenRequest(r *http.Request) (oauth2.GrantType, *oauth2.TokenGenerateRequest, string) {
	if r.URL.RawQuery != "" {
		return "", nil, "invalid_request"
	}
	if r.Form.Get("resource") != o.origin+"/api/mcp" {
		return "", nil, "invalid_target"
	}
	gt, tgr, err := o.server.ValidationTokenRequest(r)
	if err != nil {
		return "", nil, "invalid_request"
	}
	if gt != oauth2.AuthorizationCode && gt != oauth2.Refreshing {
		return "", nil, "unsupported_grant_type"
	}
	if gt == oauth2.AuthorizationCode && !oauthVerifierPattern.MatchString(tgr.CodeVerifier) {
		return "", nil, "invalid_grant"
	}
	return gt, tgr, ""
}

func (o *oauthServer) exchange(r *http.Request, gt oauth2.GrantType, tgr *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	kind, key := "code", tgr.Code
	if gt == oauth2.Refreshing {
		kind, key = "refresh", tgr.Refresh
	}
	ti, err := o.tokens.load(r.Context(), kind, key)
	if err != nil {
		_, _ = o.tokens.take(r.Context(), kind, key)
		return nil, oe.ErrInvalidGrant
	}
	token := ti.(*models.Token)
	if token.ClientID != tgr.ClientID || tgr.ClientSecret != "" || token.Extension.Get("resource") != r.Form.Get("resource") || tgr.Scope != "" && tgr.Scope != oauthScope {
		return nil, oe.ErrInvalidGrant
	}
	ctx, err := o.tokens.take(r.Context(), kind, key)
	if err != nil {
		return nil, err
	}
	tgr.Request = r.WithContext(ctx)
	return o.server.GetAccessToken(ctx, gt, tgr)
}

func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	if !s.oauthAvailable(w, r) {
		return
	}
	oauthHeaders(w)
	if err := oauthParseForm(w, r); err != nil {
		oauthError(w, "invalid_request")
		return
	}
	gt, tgr, code := s.oauth.validateTokenRequest(r)
	if code != "" {
		oauthError(w, code)
		return
	}
	ti, err := s.oauth.exchange(r, gt, tgr)
	if err != nil {
		oauthError(w, "invalid_grant")
		return
	}
	writeJSON(w, 200, map[string]any{"access_token": ti.GetAccess(), "token_type": "Bearer", "expires_in": int64(ti.GetAccessExpiresIn() / time.Second), "refresh_token": ti.GetRefresh(), "scope": ti.GetScope()})
}

func (s *Server) oauthListGrants(w http.ResponseWriter, r *http.Request) {
	oauthHeaders(w)
	list, err := s.store.OAuthGrants(r.Context(), currentUser(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := []store.OAuthGrant{}
	for _, g := range list {
		if !g.Revoked {
			out = append(out, g)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) oauthRevokeGrant(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeOAuthGrant(r.Context(), currentUser(r), r.PathValue("id")); err != nil {
		s.notFoundOr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) oauthAuthenticate(ctx context.Context, bearer string) (store.TokenProvenance, error) {
	if s.oauth == nil {
		return store.TokenProvenance{}, store.ErrNotFound
	}
	ti, err := s.oauth.manager.LoadAccessToken(ctx, bearer)
	if err != nil {
		return store.TokenProvenance{}, err
	}
	g, err := s.store.OAuthGrant(ctx, ti.(*models.Token).Extension.Get("grant"))
	if err != nil || g.Revoked || g.Resource != s.oauth.origin+"/api/mcp" {
		return store.TokenProvenance{}, store.ErrNotFound
	}
	return store.TokenProvenance{Owner: g.Owner, TokenID: g.ID, TokenLabel: g.ClientName}, nil
}

func (s *Server) oauthChallenge(w http.ResponseWriter, invalid bool) {
	challenge := `Bearer realm="pictor"`
	if invalid {
		challenge += `, error="invalid_token"`
	}
	if s.oauth != nil {
		challenge += fmt.Sprintf(`, resource_metadata=%q, scope=%q`, s.oauth.origin+"/.well-known/oauth-protected-resource/api/mcp", oauthScope)
	}
	w.Header().Set("WWW-Authenticate", challenge)
}
