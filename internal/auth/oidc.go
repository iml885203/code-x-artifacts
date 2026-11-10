package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig selects one operator-trusted provider. HTTPClient optionally supplies
// a test/embedding transport; production never disables normal TLS validation.
type OIDCConfig struct {
	Issuer, ClientID, ClientSecret, Origin, ProviderName string
	Scopes, AllowedSubjects, AllowedGroups               []string
	TrustAppAssignment                                   bool
	GroupsClaim, NameClaim                               string
	GroupMap                                             map[string]string
	HTTPClient                                           *http.Client
}

// ErrOIDCUnavailable distinguishes temporary protocol fetch failures from invalid credentials.
var ErrOIDCUnavailable = errors.New("OIDC provider is temporarily unavailable")

// OIDCIdentity contains verified subject and mapped profile data, never tokens.
type OIDCIdentity struct {
	Issuer, Subject, DisplayName, Email string
	Groups                              []string
}

// OIDC is a single configured provider and its validated protocol endpoints.
type OIDC struct {
	cfg      OIDCConfig
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
	failures *atomic.Uint64
}

// NewOIDC performs bounded discovery at startup and fails closed on invalid
// settings or an unavailable provider. Protocol credentials never reach storage.
func NewOIDC(ctx context.Context, cfg OIDCConfig) (*OIDC, error) {
	if err := validateOIDCConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = "OpenID Connect"
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if cfg.NameClaim == "" {
		cfg.NameClaim = "name"
	}
	base := cfg.HTTPClient
	if base == nil {
		base = &http.Client{}
	}
	client := *base
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("OIDC redirects are not followed") }
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	failures := &atomic.Uint64{}
	client.Transport = oidcBoundedTransport{next: transport, failures: failures}
	ctx = oidc.ClientContext(ctx, &client)
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed; check issuer, TLS and provider availability")
	}
	endpoint := provider.Endpoint()
	var metadata struct {
		JWKS string   `json:"jwks_uri"`
		PKCE []string `json:"code_challenge_methods_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, errors.New("invalid OIDC discovery metadata")
	}
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKS} {
		if err := validateOIDCURL(raw, false); err != nil {
			return nil, errors.New("OIDC discovery contains an unsafe endpoint")
		}
	}
	if len(metadata.PKCE) > 0 && !slices.Contains(metadata.PKCE, "S256") {
		return nil, errors.New("OIDC provider must support PKCE S256")
	}
	scopes := oidcScopes(cfg.Scopes)
	return &OIDC{cfg: cfg, client: &client, failures: failures, oauth: oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.Origin + "/auth/oidc/callback", Endpoint: endpoint, Scopes: scopes}, verifier: provider.VerifierContext(context.WithoutCancel(ctx), &oidc.Config{ClientID: cfg.ClientID})}, nil
}

func oidcScopes(configured []string) []string {
	scopes := []string{oidc.ScopeOpenID}
	for _, scope := range configured {
		if scope != "offline_access" && !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func validateOIDCConfig(cfg OIDCConfig) error {
	if cfg.ClientID == "" || len(cfg.ClientID) > 1024 {
		return errors.New("OIDC client ID is required")
	}
	if err := validateOIDCURL(cfg.Issuer, false); err != nil {
		return fmt.Errorf("OIDC issuer: %w", err)
	}
	u, _ := url.Parse(cfg.Issuer)
	if u.RawQuery != "" {
		return errors.New("OIDC issuer cannot contain a query")
	}
	if err := validateOIDCURL(cfg.Origin, true); err != nil {
		return fmt.Errorf("OIDC origin: %w", err)
	}
	if len(cfg.AllowedSubjects) == 0 && len(cfg.AllowedGroups) == 0 && !cfg.TrustAppAssignment {
		return errors.New("configure allowed OIDC subjects/groups or explicitly trust IdP application assignment")
	}
	if len(cfg.ProviderName) > 120 {
		return errors.New("OIDC provider name is too long")
	}
	return nil
}

func validateOIDCURL(raw string, origin bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || origin && (u.Path != "" || u.RawQuery != "") {
		return errors.New("use an absolute URL without credentials or fragment (origin has no path or query)")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return errors.New("HTTPS is required except for loopback development")
	}
	return nil
}

// Origin is the canonical application origin for callbacks.
func (o *OIDC) Origin() string { return o.cfg.Origin }

// ProviderName is the public login label selected by the operator.
func (o *OIDC) ProviderName() string { return o.cfg.ProviderName }

// AuthURL starts a bound Authorization Code transaction with PKCE S256.
func (o *OIDC) AuthURL(state, nonce, verifier string) string {
	return o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

// Exchange validates the code and signed ID token before returning identity.
func (o *OIDC) Exchange(ctx context.Context, code, verifier, nonce string) (OIDCIdentity, error) {
	ctx = oidc.ClientContext(ctx, o.client)
	before := o.failures.Load()
	token, err := o.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return OIDCIdentity{}, o.protocolFailure(before, "OIDC token exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return OIDCIdentity{}, errors.New("OIDC ID token is missing")
	}
	before = o.failures.Load()
	id, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return OIDCIdentity{}, o.protocolFailure(before, "OIDC ID token validation failed")
	}
	if id.Nonce != nonce || id.Subject == "" || len(id.Subject) > 255 {
		return OIDCIdentity{}, errors.New("OIDC ID token binding is invalid")
	}
	var claims map[string]any
	if err := id.Claims(&claims); err != nil {
		return OIDCIdentity{}, errors.New("OIDC claims are invalid")
	}
	azp, azpOK := claims["azp"].(string)
	_, hasAZP := claims["azp"]
	if len(id.Audience) > 1 && azp == "" || hasAZP && (!azpOK || azp != o.cfg.ClientID) {
		return OIDCIdentity{}, errors.New("OIDC authorized party is invalid")
	}
	if !validOIDCIssuedAt(claims["iat"]) {
		return OIDCIdentity{}, errors.New("OIDC issuance time is invalid")
	}
	return o.profile(id, claims)
}

func (o *OIDC) profile(id *oidc.IDToken, claims map[string]any) (OIDCIdentity, error) {
	groups, valid := oidcGroups(claims[o.cfg.GroupsClaim])
	if !valid && len(o.cfg.AllowedGroups) > 0 {
		return OIDCIdentity{}, ErrNotAllowed
	}
	if !o.allowed(id.Subject, groups) {
		return OIDCIdentity{}, ErrNotAllowed
	}
	name, _ := claims[o.cfg.NameClaim].(string)
	if name == "" || len(name) > 512 {
		name = "OIDC user"
	}
	email, _ := claims["email"].(string)
	if len(email) > 512 {
		email = ""
	}
	mapped := []string{}
	for _, g := range groups {
		if internal, ok := o.cfg.GroupMap[g]; ok && internal != "" && !slices.Contains(mapped, internal) {
			mapped = append(mapped, internal)
		}
	}
	return OIDCIdentity{Issuer: id.Issuer, Subject: id.Subject, DisplayName: name, Email: email, Groups: mapped}, nil
}

func (o *OIDC) allowed(subject string, groups []string) bool {
	if len(o.cfg.AllowedSubjects) > 0 && !slices.Contains(o.cfg.AllowedSubjects, subject) {
		return false
	}
	if len(o.cfg.AllowedGroups) > 0 {
		for _, g := range groups {
			if slices.Contains(o.cfg.AllowedGroups, g) {
				return true
			}
		}
		return false
	}
	return true
}

func oidcGroups(raw any) ([]string, bool) {
	items, ok := raw.([]any)
	if !ok || len(items) > 256 {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		g, ok := item.(string)
		if !ok || g == "" || len(g) > 256 {
			return nil, false
		}
		out = append(out, g)
	}
	return out, true
}

// go-oidc intentionally does not enforce iat. Reject missing/future issuance
// beyond a small clock allowance in addition to its expiry/issuer/audience checks.
func validOIDCIssuedAt(raw any) bool {
	value, ok := raw.(float64)
	return ok && value > 0 && value <= float64(time.Now().Add(time.Minute).Unix())
}

func (o *OIDC) protocolFailure(before uint64, message string) error {
	if o.failures.Load() != before {
		return ErrOIDCUnavailable
	}
	return errors.New(message)
}
