package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

type (
	oauthClients struct{ st *store.Store }
	oauthClient  struct {
		ID        string   `json:"client_id"`
		Name      string   `json:"client_name"`
		Redirects []string `json:"redirect_uris"`
	}
)

func (c oauthClients) GetByID(ctx context.Context, id string) (oauth2.ClientInfo, error) {
	b, err := c.st.OAuthGet(ctx, "client", id)
	if err != nil {
		return nil, err
	}
	var v oauthClient
	if err = json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &models.Client{ID: v.ID, Domain: id, Public: true}, nil
}

func (c oauthClients) client(ctx context.Context, id string) (oauthClient, error) {
	b, err := c.st.OAuthGet(ctx, "client", id)
	var v oauthClient
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, err
}

// The library sees credentials only for this request. SQLite stores their SHA256
// indexes and a token model with all bearer values removed.
type (
	oauthTokens   struct{ st *store.Store }
	oauthClaimKey struct{}
	oauthClaim    struct {
		kind, secret string
		token        *models.Token
	}
)

func oauthHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func oauthSecret(prefix string) string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}

func (t oauthTokens) Create(ctx context.Context, ti oauth2.TokenInfo) error {
	v := *ti.(*models.Token)
	code, access, refresh := v.Code, v.Access, v.Refresh
	v.Code = ""
	v.Access = ""
	v.Refresh = ""
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		kind, key string
		expires   time.Time
	}{{"code", code, v.CodeCreateAt.Add(v.CodeExpiresIn)}, {"access", access, v.AccessCreateAt.Add(v.AccessExpiresIn)}, {"refresh", refresh, v.RefreshCreateAt.Add(v.RefreshExpiresIn)}} {
		if item.key != "" {
			if err = t.st.OAuthPut(ctx, item.kind, oauthHash(item.key), b, item.expires); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t oauthTokens) load(ctx context.Context, kind, key string) (oauth2.TokenInfo, error) {
	if claim, ok := ctx.Value(oauthClaimKey{}).(oauthClaim); ok && claim.kind == kind && claim.secret == key {
		return claim.token, nil
	}
	b, err := t.st.OAuthGet(ctx, kind, oauthHash(key))
	if err != nil {
		return nil, err
	}
	return t.decode(ctx, kind, key, b)
}

func (t oauthTokens) decode(ctx context.Context, kind, key string, b []byte) (*models.Token, error) {
	var v models.Token
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	g, err := t.st.OAuthGrant(ctx, v.Extension.Get("grant"))
	if err != nil || g.Revoked {
		return nil, store.ErrNotFound
	}
	switch kind {
	case "code":
		v.Code = key
	case "access":
		v.Access = key
	case "refresh":
		v.Refresh = key
	}
	return &v, nil
}

func (t oauthTokens) GetByCode(ctx context.Context, key string) (oauth2.TokenInfo, error) {
	return t.load(ctx, "code", key)
}

func (t oauthTokens) GetByAccess(ctx context.Context, key string) (oauth2.TokenInfo, error) {
	return t.load(ctx, "access", key)
}

func (t oauthTokens) GetByRefresh(ctx context.Context, key string) (oauth2.TokenInfo, error) {
	return t.load(ctx, "refresh", key)
}

func (t oauthTokens) RemoveByCode(ctx context.Context, key string) error {
	return t.st.OAuthDelete(ctx, "code", oauthHash(key))
}

func (t oauthTokens) RemoveByAccess(ctx context.Context, key string) error {
	return t.st.OAuthDelete(ctx, "access", oauthHash(key))
}

func (t oauthTokens) RemoveByRefresh(ctx context.Context, key string) error {
	return t.st.OAuthDelete(ctx, "refresh", oauthHash(key))
}

// take atomically spends codes/refresh tokens before the library exchange. It
// prevents two concurrent requests from each obtaining credentials from one code.
func (t oauthTokens) take(ctx context.Context, kind, key string) (context.Context, error) {
	var b []byte
	var err error
	if kind == "refresh" || kind == "code" {
		var until time.Time
		if kind == "code" {
			until = time.Now().Add(oauthRefreshLife)
		}
		b, err = t.st.OAuthTakeRemember(ctx, kind, oauthHash(key), "used-"+kind, until)
	} else {
		b, err = t.st.OAuthTake(ctx, kind, oauthHash(key))
	}
	if err != nil {
		if (kind == "refresh" || kind == "code") && errors.Is(err, store.ErrNotFound) {
			if prior, e := t.st.OAuthGet(ctx, "used-"+kind, oauthHash(key)); e == nil {
				var token models.Token
				if e := json.Unmarshal(prior, &token); e == nil {
					if g, e := t.st.OAuthGrant(ctx, token.Extension.Get("grant")); e == nil {
						_ = t.st.RevokeOAuthGrant(ctx, g.Owner, g.ID)
					}
				}
			}
		}
		return ctx, err
	}
	v, err := t.decode(ctx, kind, key, b)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, oauthClaimKey{}, oauthClaim{kind, key, v}), nil
}

type oauthGenerator struct{}

func (oauthGenerator) Token(_ context.Context, _ *oauth2.GenerateBasic, refresh bool) (string, string, error) {
	r := ""
	if refresh {
		r = oauthSecret("car_")
	}
	return oauthSecret("caa_"), r, nil
}

type oauthCodeGenerator struct{}

func (oauthCodeGenerator) Token(_ context.Context, _ *oauth2.GenerateBasic) (string, error) {
	return oauthSecret("cac_"), nil
}
