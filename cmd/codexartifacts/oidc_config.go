package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/auth"
)

// oidcSettings reads operator configuration, never request-selected issuers.
func oidcSettings() (auth.OIDCConfig, error) {
	trust, err := parseBool("OIDC_TRUST_APP_ASSIGNMENT")
	if err != nil {
		return auth.OIDCConfig{}, err
	}
	cfg := auth.OIDCConfig{Issuer: os.Getenv("OIDC_ISSUER"), ClientID: os.Getenv("OIDC_CLIENT_ID"), Origin: os.Getenv("OIDC_ORIGIN"), ProviderName: os.Getenv("OIDC_PROVIDER_NAME"), Scopes: splitList(os.Getenv("OIDC_SCOPES")), AllowedSubjects: splitList(os.Getenv("OIDC_ALLOWED_SUBJECTS")), AllowedGroups: splitList(os.Getenv("OIDC_ALLOWED_GROUPS")), TrustAppAssignment: trust, GroupsClaim: os.Getenv("OIDC_GROUPS_CLAIM"), NameClaim: os.Getenv("OIDC_NAME_CLAIM")}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"profile", "email"}
	}
	if file := os.Getenv("OIDC_CLIENT_SECRET_FILE"); file != "" {
		secret, err := readOIDCSecret(file)
		if err != nil {
			return auth.OIDCConfig{}, err
		}
		cfg.ClientSecret = secret
	}
	if raw := os.Getenv("OIDC_GROUP_MAP"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.GroupMap); err != nil {
			return auth.OIDCConfig{}, errors.New("OIDC_GROUP_MAP must be a JSON object mapping provider groups to application groups")
		}
	}
	return cfg, nil
}

func readOIDCSecret(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // G304: private file explicitly selected by the operator, not a request parameter.
	if err != nil {
		return "", errors.New("cannot read OIDC client secret file")
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 {
		return "", errors.New("OIDC client secret file must be at most 16 KiB")
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("OIDC client secret file is empty")
	}
	return secret, nil
}

func configuredLogin(ctx context.Context, authMode string) (auth.Authenticator, *auth.OIDC, error) {
	var err error
	var authenticator auth.Authenticator
	var oidcLogin *auth.OIDC
	switch authMode {
	case "local":
		authenticator, err = auth.LoadLocal(cmp.Or(os.Getenv("PICTOR_USERS_FILE"), ".local/users.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("%w; run codexartifacts init first, or set PICTOR_USERS_FILE", err)
		}
	case "ldap":
		if os.Getenv("LDAP_URL") == "" || os.Getenv("LDAP_DOMAIN") == "" || os.Getenv("LDAP_BASE_DN") == "" {
			return nil, nil, fmt.Errorf("LDAP_URL, LDAP_DOMAIN and LDAP_BASE_DN are required for LDAP authentication")
		}
		authenticator = &auth.LDAP{URL: os.Getenv("LDAP_URL"), Domain: os.Getenv("LDAP_DOMAIN"), BaseDN: os.Getenv("LDAP_BASE_DN"), AllowedGroups: splitList(os.Getenv("LDAP_ALLOWED_GROUPS"))}
	case "oidc":
		settings, configErr := oidcSettings()
		if configErr != nil {
			return nil, nil, configErr
		}
		oidcLogin, err = auth.NewOIDC(ctx, settings)
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("PICTOR_AUTH must be local, ldap or oidc")
	}
	return authenticator, oidcLogin, nil
}
