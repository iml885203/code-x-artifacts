package web

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

const sessionCookie = "pictor_session"

type sessions struct {
	key   []byte
	store *store.Store
	ttl   time.Duration
}

// Browser cookies are opaque and versioned; legacy signed payloads are refused.
func (s sessions) issue(ctx context.Context, username string, identity store.BrowserIdentity, now time.Time) (string, error) {
	var raw [32]byte
	rand.Read(raw[:])
	value := "bs_" + base64.RawURLEncoding.EncodeToString(raw[:])
	if err := s.store.CreateBrowserSession(ctx, s.sign("browser:"+value), username, identity, now, now.Add(s.ttl)); err != nil {
		return "", err
	}
	return value, nil
}

func (s sessions) verify(ctx context.Context, value string, now time.Time) (string, bool) {
	if s.store == nil || !validBrowserCookie(value) {
		return "", false
	}
	username, err := s.store.BrowserSession(ctx, s.sign("browser:"+value), now)
	return username, err == nil
}

func validBrowserCookie(value string) bool {
	if len(value) != 46 || !strings.HasPrefix(value, "bs_") {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value[3:])
	return err == nil && len(raw) == 32
}

func (s sessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// A grant is the token a page's own requests carry to read its artifact's
// files: the page runs in an opaque origin and cannot send the session cookie
// (TestAnArtifactsFilesLoadFromInsideItsSandbox). It is signed over the
// artifact and the audience the artifact has right now, so it names nothing
// else and dies when the audience changes. Its expiry is rounded to the hour,
// which keeps the URL the same all hour and lets the browser cache the file.
const (
	grantLife  = 6 * time.Hour
	grantRound = time.Hour
)

// grant returns the token for an artifact in its current audience.
func (s sessions) grant(artifactID string, visibility store.Visibility, now time.Time) string {
	exp := strconv.FormatInt(now.Truncate(grantRound).Add(grantLife).Unix(), 10)
	return exp + "." + s.sign(grantPayload(artifactID, visibility, exp))
}

// openGrant checks a token against the artifact it was presented for. It does
// not know the artifact's audience (that is what the service decides), so it
// reports the audiences the signature is valid for, and the service compares.
func (s sessions) openGrant(token, artifactID string, now time.Time) (store.Visibility, bool) {
	exp, sig, ok := strings.Cut(token, ".")
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if !ok || err != nil || now.Unix() > expiry {
		return "", false
	}
	for _, v := range []store.Visibility{store.VisibilityPrivate, store.VisibilityOrganisation, store.VisibilityPublic} {
		if hmac.Equal([]byte(sig), []byte(s.sign(grantPayload(artifactID, v, exp)))) {
			return v, true
		}
	}
	return "", false
}

// pageGrant is the grant for an artifact's page and its files (docs/multi-file.md):
// the same kind of token, signed over the version as well, so the address it
// opens only ever reads that version. It carries its own payload prefix, so it
// is never accepted for an uploaded asset or the other way round. The version
// is written in the token ("{version}.{exp}.{sig}") and covered by the
// signature: the address names a grant and the grant names the version, so a
// request cannot choose one.
func (s sessions) pageGrant(artifactID string, visibility store.Visibility, version int, now time.Time) string {
	exp := strconv.FormatInt(now.Truncate(grantRound).Add(grantLife).Unix(), 10)
	return strconv.Itoa(version) + "." + exp + "." + s.sign(pagePayload(artifactID, visibility, version, exp))
}

// openPageGrant checks a page grant against the artifact it was presented for.
// It returns the version the grant was made for and the audiences its signature
// is valid for, as openGrant does; the service compares the audience.
func (s sessions) openPageGrant(token, artifactID string, now time.Time) (store.Visibility, int, bool) {
	num, rest, ok := strings.Cut(token, ".")
	version, err := strconv.Atoi(num)
	if !ok || err != nil || version < 1 {
		return "", 0, false
	}
	exp, sig, ok := strings.Cut(rest, ".")
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if !ok || err != nil || now.Unix() > expiry {
		return "", 0, false
	}
	for _, v := range []store.Visibility{store.VisibilityPrivate, store.VisibilityOrganisation, store.VisibilityPublic} {
		if hmac.Equal([]byte(sig), []byte(s.sign(pagePayload(artifactID, v, version, exp)))) {
			return v, version, true
		}
	}
	return "", 0, false
}

func pagePayload(artifactID string, visibility store.Visibility, version int, exp string) string {
	return "file:" + artifactID + ":" + string(visibility) + ":" + strconv.Itoa(version) + ":" + exp
}

// grantPayload cannot be mistaken for a session payload: those are base64 and
// never contain a colon.
func grantPayload(artifactID string, visibility store.Visibility, exp string) string {
	return "asset:" + artifactID + ":" + string(visibility) + ":" + exp
}

// liveGrant is the grant a person who may join a design's room gets for its
// frames as the room holds them now (docs/design-room.md): unsaved work, so it
// stands for the account it was made for, and the service checks that account's
// right to see the room on every request. The account and the expiry are sealed
// (AES-GCM under a key derived from the session key, the artifact as additional
// data): the frame's own script can read its address, and must not learn whose
// it is. The nonce is derived from what is sealed, so the address is the same
// all hour, like the other grants. Its prefix keeps it apart from page and asset
// grants.
func (s sessions) liveGrant(artifactID, account string, now time.Time) string {
	exp := strconv.FormatInt(now.Truncate(grantRound).Add(grantLife).Unix(), 10)
	plain := []byte(exp + "\n" + account)
	aead := s.liveSealer()
	nonce := s.mac("live-nonce:" + artifactID + ":" + string(plain))[:aead.NonceSize()]
	sealed := aead.Seal(slices.Clone(nonce), nonce, plain, []byte("live:"+artifactID))
	return liveGrantPrefix + base64.RawURLEncoding.EncodeToString(sealed)
}

const liveGrantPrefix = "live."

// isLiveGrant says the grant segment of an address is a live grant.
func isLiveGrant(token string) bool { return strings.HasPrefix(token, liveGrantPrefix) }

// openLiveGrant checks a live grant against the artifact it was presented for
// and returns the account it was made for.
func (s sessions) openLiveGrant(token, artifactID string, now time.Time) (string, bool) {
	rest, ok := strings.CutPrefix(token, liveGrantPrefix)
	if !ok {
		return "", false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(rest)
	aead := s.liveSealer()
	if err != nil || len(sealed) < aead.NonceSize() {
		return "", false
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("live:"+artifactID))
	if err != nil {
		return "", false
	}
	exp, account, ok := strings.Cut(string(plain), "\n")
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if !ok || err != nil || now.Unix() > expiry || account == "" {
		return "", false
	}
	return account, true
}

// liveSealer is the cipher live grants are sealed with: its key is derived from
// the session key, so a deployment has no second secret to keep.
func (s sessions) liveSealer() cipher.AEAD {
	block, err := aes.NewCipher(s.mac("live-grant-key"))
	if err != nil {
		panic(fmt.Sprintf("a 32-byte key is always a valid AES key: %v", err))
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("AES always has a GCM mode: %v", err))
	}
	return aead
}

func (s sessions) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
