// Package designsourceedit provides conditional source operations. It does not
// authorize callers, persist changes, admit executable code, or broadcast rooms.
package designsourceedit

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// ErrSourceBase requires a fresh read and explicit reconciliation of the retained
// caller draft. A missing proof must never fall back to a saved or cached base.
var ErrSourceBase = errors.New("design source base is missing, stale or mismatched; read current source and reconcile the retained draft")

// Basis is server context captured with a source read. LiveEpoch changes on
// replacement/restore/protocol change; ordinary independent edits may share it.
// Principal is the authenticated account, never an agent's self-reported name.
type Basis struct {
	ArtifactID   string `json:"artifact"`
	File         string `json:"file"`
	Format       string `json:"format"`
	SavedVersion int    `json:"version"`
	LiveEpoch    string `json:"epoch"`
	Principal    string `json:"principal"`
}

// RevisionIssuer authenticates read bases without retaining a last-read cache.
// Its process-local key deliberately makes pre-restart proofs unusable. Tokens
// prove bytes and context only; current access must still be checked by service.
type RevisionIssuer struct{ key []byte }

// NewRevisionIssuer creates one issuer per service lifetime, not per request.
func NewRevisionIssuer() (*RevisionIssuer, error) {
	issuer := &RevisionIssuer{key: make([]byte, 32)}
	if _, err := rand.Read(issuer.key); err != nil {
		return nil, fmt.Errorf("source revision key: %w", err)
	}
	return issuer, nil
}

func (b Basis) valid() bool {
	return b.ArtifactID != "" && b.Principal != "" && b.LiveEpoch != "" && b.SavedVersion > 0 &&
		(b.Format == "1" || b.Format == "2") && (b.File == "design.json" || designfmt.CheckFramePath(b.File) == nil)
}

// Digest hashes exact UTF-8 source bytes, without normalization.
func Digest(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func (r *RevisionIssuer) signature(b Basis, source string, nonce []byte) []byte {
	payload, _ := json.Marshal(struct {
		Domain       string `json:"domain"`
		Basis        Basis  `json:"basis"`
		SourceSHA256 string `json:"sourceSHA256"`
		Nonce        string `json:"nonce"`
	}{"pictor-source-base/1", b, Digest(source), base64.RawURLEncoding.EncodeToString(nonce)})
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

// Issue binds a source read to exact artifact/file/format/version/epoch/account.
func (r *RevisionIssuer) Issue(b Basis, source string) (string, error) {
	if r == nil || len(r.key) != 32 || !b.valid() {
		return "", ErrSourceBase
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("source read nonce: %w", err)
	}
	return "sb1." + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(r.signature(b, source, nonce)), nil
}

// Verify checks caller-supplied base bytes, not the current room's source. The
// operation's per-unit comparisons decide which independent changes can merge.
func (r *RevisionIssuer) Verify(token string, b Basis, source string) error {
	if r == nil || len(r.key) != 32 || !b.valid() || len(token) != 70 || !strings.HasPrefix(token, "sb1.") {
		return ErrSourceBase
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrSourceBase
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(nonce) != 16 {
		return ErrSourceBase
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, r.signature(b, source, nonce)) {
		return ErrSourceBase
	}
	return nil
}
