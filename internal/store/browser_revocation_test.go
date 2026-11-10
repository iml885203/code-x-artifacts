package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBrowserRevocationSurvivesReopenSerializesCallbacksAndExpiresSafely(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "sessions.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.ResolveOIDCUser(ctx, "https://identity.example", "subject", User{DisplayName: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	identity := BrowserIdentity{Issuer: "https://identity.example", Subject: "subject", IssuedAt: float64(now.Unix()), StartedAt: now.UnixNano()}
	if err = st.CreateBrowserSession(ctx, "digest-old", user.Username, identity, now, now.Add(7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	peer, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Close() }()
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := st.CreateBrowserSession(ctx, "digest-racing", user.Username, identity, now, now.Add(7*24*time.Hour)); err != nil && !errors.Is(err, ErrBrowserRevoked) {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if err := peer.RevokeOIDCBrowsers(ctx, identity.Issuer, identity.Subject, "event", now.Unix(), now.Add(2*time.Minute), now.Add(time.Second)); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	for _, hash := range []string{"digest-old", "digest-racing"} {
		if _, err := st.BrowserSession(ctx, hash, now.Add(2*time.Second)); err == nil {
			t.Fatal("racing callback survived revocation")
		}
	}
	checkBrowserReopenAndReplay(t, st, path, user.Username, identity, now)
}

func checkBrowserReopenAndReplay(t *testing.T, st *Store, path, username string, identity BrowserIdentity, now time.Time) {
	t.Helper()
	ctx := t.Context()
	var err error
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err = st.CreateBrowserSession(ctx, "digest-pending", username, identity, now.Add(2*time.Second), now.Add(time.Hour)); !errors.Is(err, ErrBrowserRevoked) {
		t.Fatal("old callback accepted after reopening", err)
	}
	identity.IssuedAt = float64(now.Add(3 * time.Second).Unix())
	identity.StartedAt = now.Add(3 * time.Second).UnixNano()
	if err = st.CreateBrowserSession(ctx, "digest-new", username, identity, now.Add(3*time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = st.RevokeOIDCBrowsers(ctx, identity.Issuer, identity.Subject, "event", now.Unix(), now.Add(2*time.Minute), now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = st.BrowserSession(ctx, "digest-new", now.Add(4*time.Second)); err != nil {
		t.Fatal("replay revoked fresh session", err)
	}
	if _, err = st.BrowserSession(ctx, "digest-new", now.Add(time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
}

func TestBrowserStorageCapacityFailsClosedAndCleanupRestoresCapacity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "capacity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err = st.SaveUser(ctx, User{Username: "alice", DisplayName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Populate a legitimate boundary state efficiently, then assert operations
	// through the public store API rather than testing each individual insert.
	_, err = st.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<?) INSERT INTO browser_sessions SELECT 'digest-'||x,'alice',? FROM n`, BrowserStateLimit, now.Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateBrowserSession(ctx, "overflow", "alice", BrowserIdentity{}, now, now.Add(time.Hour)); !errors.Is(err, ErrBrowserCapacity) {
		t.Fatal(err)
	}
	if err = st.CreateBrowserSession(ctx, "after-expiry", "alice", BrowserIdentity{}, now.Add(2*time.Hour), now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err = st.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<?) INSERT INTO oidc_revocations SELECT 'issuer','subject-'||x,?,?,? FROM n`, BrowserStateLimit, now.Unix(), now.UnixNano(), now.Add(8*24*time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.RevokeOIDCBrowsers(ctx, "issuer", "unknown", "overflow-event", now.Unix(), now.Add(time.Minute), now); !errors.Is(err, ErrBrowserCapacity) {
		t.Fatal(err)
	}
	// No partial acknowledgment/replay marker may bypass a later real attempt.
	if _, err = st.db.ExecContext(ctx, `DELETE FROM oidc_revocations WHERE subject='subject-1'`); err != nil {
		t.Fatal(err)
	}
	if err = st.RevokeOIDCBrowsers(ctx, "issuer", "unknown", "overflow-event", now.Unix(), now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
}
