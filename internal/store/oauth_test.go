package store

import (
	"bytes"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOAuthRecordsSurviveRestartExpireAndStayInTheirNamespace(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "oauth.db")
	st := openStore(ctx, t, path)
	future := time.Now().Add(time.Hour)
	for _, kind := range []string{"client", "code", "refresh"} {
		oauthStorageOK(t, st.OAuthPut(ctx, kind, "hashed-key", []byte(kind), future))
	}
	oauthStorageOK(t, st.OAuthPut(ctx, "client", "durable", []byte("client metadata"), time.Time{}))
	oauthStorageOK(t, st.OAuthPut(ctx, "code", "expired", []byte("expired code"), time.Now().Add(-time.Hour)))
	if _, err := st.OAuthGet(ctx, "code", "expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired read: %v", err)
	}
	if _, err := st.OAuthTake(ctx, "code", "expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired consume: %v", err)
	}
	oauthStorageOK(t, st.Close())
	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	if got, err := st.OAuthGet(ctx, "client", "durable"); err != nil || !bytes.Equal(got, []byte("client metadata")) {
		t.Fatalf("durable after restart: %q %v", got, err)
	}
	for _, kind := range []string{"client", "code", "refresh"} {
		if got, err := st.OAuthGet(ctx, kind, "hashed-key"); err != nil || string(got) != kind {
			t.Fatalf("%s after restart: %q %v", kind, got, err)
		}
	}
	oauthStorageOK(t, st.OAuthDelete(ctx, "code", "hashed-key"))
	oauthStorageOK(t, st.OAuthDelete(ctx, "code", "hashed-key"))
	if _, err := st.OAuthGet(ctx, "code", "hashed-key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted record: %v", err)
	}
	if _, err := st.OAuthGet(ctx, "client", "hashed-key"); err != nil {
		t.Fatalf("code deletion removed client: %v", err)
	}
}

func TestOAuthCredentialConsumptionIsAtomicAcrossConnections(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "oauth.db")
	first := openStore(ctx, t, path)
	defer func() { _ = first.Close() }()
	second := openStore(ctx, t, path)
	defer func() { _ = second.Close() }()
	for _, kind := range []string{"code", "refresh"} {
		oauthStorageOK(t, first.OAuthPut(ctx, kind, "hashed-one-use-key", []byte("safe metadata"), time.Now().Add(time.Hour)))
		results := make(chan error, 20)
		var wg sync.WaitGroup
		for n := range 20 {
			wg.Add(1)
			st := first
			if n%2 == 1 {
				st = second
			}
			go func() {
				defer wg.Done()
				data, err := st.OAuthTake(ctx, kind, "hashed-one-use-key")
				if err == nil && string(data) != "safe metadata" {
					t.Errorf("wrong payload %q", data)
				}
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if !errors.Is(err, ErrNotFound) {
				t.Fatalf("unexpected consume error: %v", err)
			}
		}
		if success != 1 {
			t.Fatalf("%s consumed %d times, want one", kind, success)
		}
	}
}

func TestOAuthGrantsKeepOwnersAndRevocationAcrossRestart(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "oauth.db")
	st := openStore(ctx, t, path)
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, g := range []OAuthGrant{
		{ID: "alice-first", Owner: "alice", ClientID: "client-a", ClientName: "Codex", Resource: "https://example.test/api/mcp", CreatedAt: now},
		{ID: "alice-second", Owner: "alice", ClientID: "client-b", ClientName: "Other agent", Resource: "https://example.test/api/mcp", CreatedAt: now.Add(time.Second)},
		{ID: "bob-first", Owner: "bob", ClientID: "client-a", ClientName: "Codex", Resource: "https://example.test/api/mcp", CreatedAt: now},
	} {
		oauthStorageOK(t, st.CreateOAuthGrant(ctx, g))
	}
	if err := st.RevokeOAuthGrant(ctx, "bob", "alice-first"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign revoke: %v", err)
	}
	if err := st.RevokeOAuthGrant(ctx, "alice", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing revoke: %v", err)
	}
	for range 2 {
		oauthStorageOK(t, st.RevokeOAuthGrant(ctx, "alice", "alice-first"))
	}
	if err := st.CreateOAuthGrant(ctx, OAuthGrant{ID: "alice-first", Owner: "alice", ClientID: "attacker", CreatedAt: now}); err == nil {
		t.Fatal("existing revoked grant was replaced")
	}
	oauthStorageOK(t, st.Close())
	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	checkOAuthAliceGrantsAfterRestart(t, st, now)
	checkOAuthOtherOwnersGrants(t, st)
}

func TestOAuthRefreshConsumptionRemembersTheGrantBeforeAnyReplay(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "oauth-remember.db")
	first := openStore(ctx, t, path)
	defer func() { _ = first.Close() }()
	second := openStore(ctx, t, path)
	defer func() { _ = second.Close() }()
	payload := []byte(`{"grant":"grant-a","owner":"alice"}`)
	oauthStorageOK(t, first.OAuthPut(ctx, "refresh", "hashed-refresh", payload, time.Now().Add(time.Hour)))
	results := make(chan error, 20)
	var wg sync.WaitGroup
	for n := range 20 {
		wg.Add(1)
		st := first
		if n%2 == 1 {
			st = second
		}
		go func() {
			defer wg.Done()
			data, err := st.OAuthTakeRemember(ctx, "refresh", "hashed-refresh", "used-refresh")
			if err == nil {
				if !bytes.Equal(data, payload) {
					t.Errorf("wrong consumed payload: %q", data)
				}
			} else if errors.Is(err, ErrNotFound) {
				remembered, readErr := st.OAuthGet(ctx, "used-refresh", "hashed-refresh")
				if readErr != nil || !bytes.Equal(remembered, payload) {
					t.Errorf("replay lost grant tombstone: %q %v", remembered, readErr)
				}
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("refresh consumed %d times, want once", successes)
	}
	checkOAuthRememberFailureRollback(t, first, payload)
}

func oauthStorageOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func checkOAuthAliceGrantsAfterRestart(t *testing.T, st *Store, now time.Time) {
	t.Helper()
	list, err := st.OAuthGrants(t.Context(), "alice")
	if err != nil || len(list) != 2 {
		t.Fatalf("alice grants: %+v %v", list, err)
	}
	if list[0].ID != "alice-second" || list[0].Revoked || list[1].ID != "alice-first" || !list[1].Revoked {
		t.Fatalf("grant order/revocation: %+v", list)
	}
	g, err := st.OAuthGrant(t.Context(), "alice-first")
	if err != nil || g.Owner != "alice" || g.ClientID != "client-a" || !g.Revoked || !g.CreatedAt.Equal(now) {
		t.Fatalf("grant after restart: %+v %v", g, err)
	}
	if _, err := st.OAuthGrant(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing grant lookup: %v", err)
	}
}

func checkOAuthOtherOwnersGrants(t *testing.T, st *Store) {
	t.Helper()
	bob, err := st.OAuthGrants(t.Context(), "bob")
	if err != nil || len(bob) != 1 || bob[0].ID != "bob-first" || bob[0].Revoked {
		t.Fatalf("bob grants: %+v %v", bob, err)
	}
	empty, err := st.OAuthGrants(t.Context(), "new-user")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty grants: %+v %v", empty, err)
	}
}

func checkOAuthRememberFailureRollback(t *testing.T, st *Store, payload []byte) {
	t.Helper()
	oauthStorageOK(t, st.OAuthPut(t.Context(), "refresh", "not-consumed", payload, time.Now().Add(time.Hour)))
	if _, err := st.OAuthTakeRemember(t.Context(), "refresh", "not-consumed", "refresh"); err == nil {
		t.Fatal("same namespace allowed replay")
	}
	if _, err := st.OAuthGet(t.Context(), "refresh", "not-consumed"); err != nil {
		t.Fatalf("invalid consumption changed record: %v", err)
	}
	// A storage error cannot spend the credential without recording the tombstone.
	if _, err := st.db.ExecContext(t.Context(), `CREATE TRIGGER reject_oauth_tombstone BEFORE INSERT ON oauth_records WHEN NEW.kind='reject-tombstone' BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OAuthTakeRemember(t.Context(), "refresh", "not-consumed", "reject-tombstone"); err == nil {
		t.Fatal("failing tombstone insertion succeeded")
	}
	if got, err := st.OAuthGet(t.Context(), "refresh", "not-consumed"); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("failed tombstone consumed credential: %q %v", got, err)
	}
}

func TestOAuthReplayTombstoneCanOutliveTheCode(t *testing.T) {
	st := openStore(t.Context(), t, filepath.Join(t.TempDir(), "oauth-code-tombstone.db"))
	defer func() { _ = st.Close() }()
	expires := time.Now().Add(5 * time.Minute)
	rememberUntil := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Millisecond)
	oauthStorageOK(t, st.OAuthPut(t.Context(), "code", "hashed-code", []byte(`{"grant":"issued-grant"}`), expires))
	_, err := st.OAuthTakeRemember(t.Context(), "code", "hashed-code", "used-code", rememberUntil)
	oauthStorageOK(t, err)
	var rememberedExpiry int64
	oauthStorageOK(t, st.db.QueryRowContext(t.Context(), `SELECT expires_at FROM oauth_records WHERE kind='used-code' AND key='hashed-code'`).Scan(&rememberedExpiry))
	if rememberedExpiry != rememberUntil.UnixMilli() {
		t.Fatalf("code replay tombstone expires at %d, want %d", rememberedExpiry, rememberUntil.UnixMilli())
	}
	if _, err := st.OAuthGet(t.Context(), "code", "hashed-code"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("code remained usable: %v", err)
	}
}
