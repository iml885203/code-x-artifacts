package designsourceedit

import (
	"fmt"
	"sync"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// Scope binds a cached read to current authority context without pretending an
// ordinary autosave retires the live epoch. The saved version stays signed in
// ReadBase.Basis; the service separately enforces its saved-version contract.
type Scope struct{ ArtifactID, File, Format, LiveEpoch, Principal string }

// ScopeOf selects identity/epoch fields from the server's current context.
func ScopeOf(b Basis) Scope { return Scope{b.ArtifactID, b.File, b.Format, b.LiveEpoch, b.Principal} }

// ReadBase is an exact authenticated source snapshot, never a guessed fallback.
// Strings and value-only context make returned reads independent of the book.
type ReadBase struct {
	Basis  Basis
	Source string
	// Absent distinguishes an absent file from a present empty source.
	Absent bool
}

type retainedBase struct {
	ReadBase
	at       time.Time
	sequence uint64
}

// ReadBases is a bounded explicit revision registry. It has no account/client
// last-read pointer, disk persistence, password, auth token or new permission.
// Losing a read requires reconciliation of the caller-retained draft.
type ReadBases struct {
	mu       sync.Mutex
	issuer   *RevisionIssuer
	ttl      time.Duration
	limit    int
	now      func() time.Time
	sequence uint64
	entries  map[string]retainedBase
}

// NewReadBases creates one registry per service lifetime. The existing service
// selects its retention/entry bound; tests replace only clock and small bounds.
func NewReadBases(ttl time.Duration, limit int, now func() time.Time) (*ReadBases, error) {
	if ttl <= 0 || limit < 1 {
		return nil, fmt.Errorf("read bases require positive retention and capacity")
	}
	issuer, err := NewRevisionIssuer()
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &ReadBases{issuer: issuer, ttl: ttl, limit: limit, now: now, entries: map[string]retainedBase{}}, nil
}

func (b *ReadBases) expired(entry retainedBase, now time.Time) bool {
	return now.Before(entry.at) || now.Sub(entry.at) >= b.ttl
}

// Remember authenticates a complete source read. A second read produces an
// independent key, including when bytes and context happen to be identical.
func (b *ReadBases) Remember(basis Basis, source string) (string, error) {
	return b.remember(basis, source, false)
}

// RememberAbsent records authoritative absence from a coherent source snapshot.
func (b *ReadBases) RememberAbsent(basis Basis) (string, error) {
	if err := designfmt.CheckFramePath(basis.File); err != nil {
		return "", ErrSourceBase
	}
	return b.remember(basis, "", true)
}

func (b *ReadBases) remember(basis Basis, source string, absent bool) (string, error) {
	if b == nil {
		return "", ErrSourceBase
	}
	maxBytes := designfmt.MaxFrameBytes
	if basis.File == "design.json" {
		maxBytes = designfmt.MaxManifestBytes
	}
	if len(source) > maxBytes {
		return "", ErrSourceBase
	}
	token, err := b.issuer.Issue(basis, source)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	for key, entry := range b.entries {
		if b.expired(entry, now) {
			delete(b.entries, key)
		}
	}
	if _, exists := b.entries[token]; !exists && len(b.entries) >= b.limit {
		var oldest string
		var sequence uint64
		for key, entry := range b.entries {
			if oldest == "" || entry.sequence < sequence {
				oldest = key
				sequence = entry.sequence
			}
		}
		delete(b.entries, oldest)
	}
	b.sequence++
	b.entries[token] = retainedBase{ReadBase{basis, source, absent}, now, b.sequence}
	return token, nil
}

// Load refuses missing/expired/evicted/restarted or mismatched proofs. It never
// substitutes the newest read, a saved version, or another client's snapshot.
func (b *ReadBases) Load(token string, scope Scope) (ReadBase, error) {
	if b == nil {
		return ReadBase{}, ErrSourceBase
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, exists := b.entries[token]
	if !exists {
		return ReadBase{}, ErrSourceBase
	}
	if b.expired(entry, b.now()) {
		delete(b.entries, token)
		return ReadBase{}, ErrSourceBase
	}
	if ScopeOf(entry.Basis) != scope || b.issuer.Verify(token, entry.Basis, entry.Source) != nil {
		return ReadBase{}, ErrSourceBase
	}
	return entry.ReadBase, nil
}

// ForgetArtifact retires all proofs when source authority or permissions change.
// Fresh per-read nonces prevent an identical later read from reviving old input.
func (b *ReadBases) ForgetArtifact(id string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, entry := range b.entries {
		if entry.Basis.ArtifactID == id {
			delete(b.entries, token)
		}
	}
}
