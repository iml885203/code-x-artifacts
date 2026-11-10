package auth

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// OIDCLogout is a verified subject-wide event; it contains no bearer token.
type OIDCLogout struct {
	Issuer, Subject, ID string
	IssuedAt            int64
	Expires             time.Time
}

// VerifyLogout deliberately accepts only subject-wide logout+jwt notifications,
// as emitted by Pocket ID. ID/access tokens and sid-bearing events are not
// interchangeable with this credential type. All remote fetches share OIDC's
// bounded, trusted transport and signing-key cache.
func (o *OIDC) VerifyLogout(ctx context.Context, raw string) (OIDCLogout, error) {
	invalid := errors.New("invalid OIDC logout notification")
	if len(raw) == 0 || len(raw) > 16384 {
		return OIDCLogout{}, invalid
	}
	signed, err := jose.ParseSigned(raw, o.logoutAlgorithms)
	if err != nil || len(signed.Signatures) != 1 || signed.Signatures[0].Protected.ExtraHeaders[jose.HeaderType] != "logout+jwt" {
		return OIDCLogout{}, invalid
	}
	before := o.failures.Load()
	payload, err := o.logoutKeys.VerifySignature(ctx, raw)
	if err != nil {
		return OIDCLogout{}, o.protocolFailure(before, "invalid OIDC logout signature")
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return OIDCLogout{}, invalid
	}
	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	id, _ := claims["jti"].(string)
	iat, exp, validTimes := logoutTimes(claims, time.Now())
	if !logoutIdentity(issuer, subject, id, o.cfg.Issuer) || !validTimes {
		return OIDCLogout{}, invalid
	}
	if !logoutEvent(claims) || !logoutAudience(claims["aud"], o.cfg.ClientID) {
		return OIDCLogout{}, invalid
	}
	return OIDCLogout{Issuer: issuer, Subject: subject, ID: id, IssuedAt: int64(iat), Expires: time.Unix(int64(math.Ceil(exp)), 0)}, nil
}

func logoutEvent(claims map[string]any) bool {
	if _, ok := claims["nonce"]; ok {
		return false
	}
	if _, ok := claims["sid"]; ok {
		return false
	}
	events, ok := claims["events"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = events["http://schemas.openid.net/event/backchannel-logout"].(map[string]any)
	return ok
}

func logoutAudience(raw any, client string) bool {
	if value, ok := raw.(string); ok {
		return value == client
	}
	audiences, ok := raw.([]any)
	if !ok {
		return false
	}
	found := false
	for _, aud := range audiences {
		value, ok := aud.(string)
		if !ok {
			return false
		}
		found = found || value == client
	}
	return found
}

func logoutTimes(claims map[string]any, at time.Time) (float64, float64, bool) {
	iat, iatOK := claims["iat"].(float64)
	exp, expOK := claims["exp"].(float64)
	now := float64(at.UnixNano()) / float64(time.Second)
	valid := iatOK && expOK && iat > 0 && iat <= now+60 && iat >= now-300 && exp > now && exp > iat && exp-iat <= 300
	return iat, exp, valid
}

func logoutIdentity(issuer, subject, id, configured string) bool {
	return issuer == configured && subject != "" && len(subject) <= 255 && id != "" && len(id) <= 256
}

func logoutSigningAlgorithms(advertised []string) []jose.SignatureAlgorithm {
	if len(advertised) == 0 {
		return []jose.SignatureAlgorithm{jose.RS256}
	}
	supported := []jose.SignatureAlgorithm{jose.RS256, jose.RS384, jose.RS512, jose.ES256, jose.ES384, jose.ES512, jose.PS256, jose.PS384, jose.PS512, jose.EdDSA}
	out := []jose.SignatureAlgorithm{}
	for _, algorithm := range supported {
		for _, value := range advertised {
			if string(algorithm) == value {
				out = append(out, algorithm)
				break
			}
		}
	}
	return out
}
