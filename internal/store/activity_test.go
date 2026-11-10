package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActivitySurvivesRestartAndRevocationWithoutSharingAnotherOwnersHistory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "activity.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, p := activityIdentity(t, st, "Laptop")
	p.Transport, p.Client, p.ClientVersion = "mcp", "Codex", "1"
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	a, err := st.CreateArtifactFrom(ActivityContext(ctx, p, "publish", now), NewArtifact{Title: "Private", Kind: "html", Format: "1", Content: "hello", Author: "alice", Visibility: VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.DeleteToken(ctx, "alice", p.TokenID); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rows := activityRows(t, st, "alice", ActivityFilter{Limit: 50}, now)
	if len(rows) != 1 || rows[0].ArtifactID != a.ID || rows[0].TokenID != p.TokenID || rows[0].Client != "Codex" || !rows[0].Revoked {
		t.Fatalf("retained activity: %+v", rows)
	}
	other := activityRows(t, st, "bob", ActivityFilter{Limit: 50}, now)
	if len(other) != 0 {
		t.Fatalf("other owner: %+v %v", other, err)
	}
}

func TestActivityCoalescesConnectionsAndExpiresWithStableCursorsAndAnOwnerCap(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "activity.db"))
	defer func() { _ = st.Close() }()
	_, p := activityIdentity(t, st, "one")
	var err error
	p.Transport = "mcp"
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for range 2 {
		if err = st.RecordConnection(ctx, p, at); err != nil {
			t.Fatal(err)
		}
	}
	rows := activityRows(t, st, "alice", ActivityFilter{Limit: 100}, at)
	if len(rows) != 1 {
		t.Fatalf("coalesced: %d %v", len(rows), err)
	}
	p.Client = "Codex"
	if err = st.RecordConnection(ctx, p, at); err != nil {
		t.Fatal(err)
	}
	first := activityRows(t, st, "alice", ActivityFilter{Limit: 1}, at)
	if len(first) != 1 {
		t.Fatal(err)
	}
	next := activityRows(t, st, "alice", ActivityFilter{Limit: 100, BeforeID: first[0].ID, BeforeTime: first[0].OccurredAt}, at)
	if len(next) != 1 || first[0].ID == next[0].ID {
		t.Fatalf("cursor: %+v %v", next, err)
	}
	// Seed a full private history in one fixture transaction; admission, not artifact creation,
	// is the boundary under test. A normal connection must trim even an overfull store.
	if _, err = st.db.ExecContext(ctx, `WITH RECURSIVE fixture(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM fixture WHERE n<1005)
 INSERT INTO agent_activity(id,owner,token_id,token_label,transport,client,client_version,occurred_at,operation,artifact_id,version)
 SELECT printf('fixture%04d',n),?,?,?,?,?,?,?+n*1000,'publish','',0 FROM fixture`, p.Owner, p.TokenID, p.TokenLabel, p.Transport, p.Client, p.ClientVersion, at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = st.RecordConnection(ctx, p, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows = activityRows(t, st, "alice", ActivityFilter{Limit: 1100}, at.Add(time.Hour))
	if len(rows) != 1000 {
		t.Fatalf("cap: %d %v", len(rows), err)
	}
	if err = st.SweepActivity(ctx, at.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows = activityRows(t, st, "alice", ActivityFilter{Limit: 100}, at.Add(31*24*time.Hour))
	if len(rows) != 0 {
		t.Fatalf("expiry: %d %v", len(rows), err)
	}
}

func TestActivityFailureRollsBackTheContentAndRealtimeLogWithItsMetadata(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "activity.db"))
	defer func() { _ = st.Close() }()
	_, p := activityIdentity(t, st, "Laptop")
	var err error
	p.Transport = "http"
	a, err := st.CreateArtifact(ctx, "a", "html", "1", "before", "alice", VisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.db.ExecContext(ctx, `CREATE TRIGGER fail_activity BEFORE INSERT ON agent_activity BEGIN SELECT RAISE(ABORT,'activity storage unavailable');END`); err != nil {
		t.Fatal(err)
	}
	admitted := ActivityContext(ctx, p, "publish", time.Now())
	if err = st.UpdateArtifact(admitted, a.ID, 1, Revision{Title: "a", Format: "1", Content: "after", Author: "alice"}, ""); err == nil {
		t.Fatal("failed activity admitted a version")
	}
	saved, err := st.Artifact(ctx, a.ID)
	if err != nil || saved.Content != "before" || saved.Version != 1 {
		t.Fatalf("partial version: %+v %v", saved, err)
	}
	log := st.RealtimeLog()
	snap, err := log.Load(ctx, a.ID, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	if err = log.Append(admitted, a.ID, snap.Epoch, []byte{1, 2, 3}); err == nil {
		t.Fatal("failed activity admitted a room update")
	}
	if n, err := st.RealtimeRows(ctx, a.ID); err != nil || n != 0 {
		t.Fatalf("partial live log: %d %v", n, err)
	}
}

func TestActivityUsesHistoricalTokenLabelsAndThrottlesReadsWithoutMakingHistory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "activity.db"))
	defer func() { _ = st.Close() }()
	secret, p := activityIdentity(t, st, "Original")
	var err error
	p.Transport = "mcp"
	at := time.Now().UTC().Truncate(time.Millisecond)
	if err = st.RecordConnection(ctx, p, at); err != nil {
		t.Fatal(err)
	}
	if _, err = st.db.ExecContext(ctx, `UPDATE tokens SET name='Renamed' WHERE id=?`, p.TokenID); err != nil {
		t.Fatal(err)
	}
	p, err = st.AuthenticateToken(ctx, secret)
	if err != nil {
		t.Fatal(err)
	}
	p.Transport = "http"
	for _, d := range []time.Duration{time.Second, 30 * time.Second, 61 * time.Second} {
		if err = st.TouchTokenUse(ctx, p, at.Add(d)); err != nil {
			t.Fatal(err)
		}
	}
	rows := activityRows(t, st, "alice", ActivityFilter{Limit: 100}, at.Add(time.Minute*2))
	if len(rows) != 1 || rows[0].TokenLabel != "Original" {
		t.Fatalf("history labels: %+v %v", rows, err)
	}
	tokens, err := st.ActivityTokens(ctx, "alice", at.Add(time.Minute*2))
	if err != nil || len(tokens) != 1 || tokens[0].Name != "Renamed" || !tokens[0].LastUsed.Equal(at.Add(61*time.Second)) || !tokens[0].LastConnected.Equal(at) {
		t.Fatalf("throttled summaries: %+v %v", tokens, err)
	}
}

func TestActivityStoresNoCredentialOrArtifactPayload(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "activity.db"))
	defer func() { _ = st.Close() }()
	secret, p := activityIdentity(t, st, "Laptop")
	var err error
	p.Transport = "mcp"
	p.Client = "Codex"
	p.ClientVersion = "1"
	payload := "PRIVATE_CONTENT_SENTINEL"
	if _, err = st.CreateArtifactFrom(ActivityContext(ctx, p, "publish", time.Now()), NewArtifact{Title: "PRIVATE_TITLE_SENTINEL", Kind: "html", Format: "1", Content: payload, Author: "alice", Visibility: VisibilityPrivate}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`SELECT * FROM agent_activity`, `SELECT * FROM agent_token_usage`} {
		rows, err := st.db.QueryContext(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err = rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			text := fmt.Sprint(values)
			if strings.Contains(text, secret) || strings.Contains(text, hashToken(secret)) || strings.Contains(text, payload) || strings.Contains(text, "PRIVATE_TITLE_SENTINEL") {
				t.Fatalf("sensitive history metadata: %s", columns)
			}
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func activityIdentity(t *testing.T, st *Store, name string) (string, TokenProvenance) {
	t.Helper()
	secret, err := st.CreateToken(t.Context(), "alice", name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.AuthenticateToken(t.Context(), secret)
	if err != nil {
		t.Fatal(err)
	}
	return secret, p
}

func activityRows(t *testing.T, st *Store, owner string, f ActivityFilter, at time.Time) []ActivityEntry {
	t.Helper()
	rows, err := st.Activity(t.Context(), owner, f, at)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSelfReportedMetadataHasABoundedPrintableUnicodeLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value string
		limit int
		want  string
	}{
		{value: " a\nb\t", limit: 40, want: "ab"},
		{value: "一二三四", limit: 3, want: "一二三"},
		{value: strings.Repeat("v", 80), limit: 40, want: strings.Repeat("v", 40)},
	} {
		if got := BoundedLabel(tc.value, tc.limit); got != tc.want {
			t.Errorf("bounded metadata %q, want %q", got, tc.want)
		}
	}
}

func TestConcurrentSuccessfulReadsUpdateUsageOnlyOncePerThrottleWindow(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "activity.db"))
	defer func() { _ = st.Close() }()
	_, p := activityIdentity(t, st, "Laptop")
	p.Transport = "http"
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if err := st.TouchTokenUse(ctx, p, at); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TABLE fixture_usage_updates(n INTEGER);
 CREATE TRIGGER fixture_usage_update AFTER UPDATE ON agent_token_usage BEGIN INSERT INTO fixture_usage_updates VALUES(1);END`); err != nil {
		t.Fatal(err)
	}
	touchConcurrentActivity(t, st, p, at.Add(time.Second))
	touchConcurrentActivity(t, st, p, at.Add(time.Minute))
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fixture_usage_updates`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("usage writes across two read batches: %d, want one after expiry", count)
	}
	if rows := activityRows(t, st, "alice", ActivityFilter{Limit: 100}, at.Add(time.Minute)); len(rows) != 0 {
		t.Fatalf("reads made history rows: %+v", rows)
	}
}

func touchConcurrentActivity(t *testing.T, st *Store, p TokenProvenance, at time.Time) {
	t.Helper()
	var calls sync.WaitGroup
	failures := make(chan error, 16)
	for range 16 {
		calls.Add(1)
		go func() { defer calls.Done(); failures <- st.TouchTokenUse(t.Context(), p, at) }()
	}
	calls.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestOAuthActivityTreatsOnlyTheOwnersActiveGrantAsLive(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "oauth-activity.db")
	st := openStore(ctx, t, path)
	at := time.Now().UTC().Truncate(time.Millisecond)
	grant := OAuthGrant{ID: "oauth-alice", Owner: "alice", ClientID: "client-a", ClientName: "Codex OAuth", Resource: "https://example.test/api/mcp", CreatedAt: at}
	if err := st.CreateOAuthGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateOAuthGrant(ctx, OAuthGrant{ID: "oauth-bob", Owner: "bob", ClientID: "client-b", ClientName: "Bob OAuth", CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	p := TokenProvenance{Owner: "alice", TokenID: grant.ID, TokenLabel: grant.ClientName, Transport: "mcp", Client: "Codex"}
	if err := st.RecordConnection(ctx, p, at); err != nil {
		t.Fatal(err)
	}
	// History from a different owner must not be made live by Alice's grant ID.
	other := p
	other.Owner = "bob"
	if err := st.RecordConnection(ctx, other, at); err != nil {
		t.Fatal(err)
	}
	checkOAuthActiveActivity(t, st, grant, at)
	if err := st.RevokeOAuthGrant(ctx, "alice", grant.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st = openStore(ctx, t, path)
	defer func() { _ = st.Close() }()
	checkOAuthRevokedActivity(t, st, grant, at)
}

func checkOAuthActiveActivity(t *testing.T, st *Store, grant OAuthGrant, at time.Time) {
	t.Helper()
	rows := activityRows(t, st, "alice", ActivityFilter{Limit: 50}, at)
	if len(rows) != 1 || rows[0].Revoked {
		t.Fatalf("live OAuth history: %+v", rows)
	}
	bobRows := activityRows(t, st, "bob", ActivityFilter{Limit: 50}, at)
	if len(bobRows) != 1 || !bobRows[0].Revoked {
		t.Fatalf("foreign grant ID should not be live: %+v", bobRows)
	}
	checkOAuthActiveActivitySummaries(t, st, grant, at)
}

func checkOAuthActiveActivitySummaries(t *testing.T, st *Store, grant OAuthGrant, at time.Time) {
	t.Helper()
	list, err := st.ActivityTokens(t.Context(), "alice", at)
	if err != nil || len(list) != 1 || list[0].ID != grant.ID || list[0].Name != grant.ClientName || list[0].Revoked || list[0].LastConnected == nil {
		t.Fatalf("live grant summary: %+v %v", list, err)
	}
	bobList, err := st.ActivityTokens(t.Context(), "bob", at)
	if err != nil || len(bobList) != 2 {
		t.Fatalf("owner-scoped summaries: %+v %v", bobList, err)
	}
	for _, item := range bobList {
		if item.ID == grant.ID && !item.Revoked {
			t.Fatalf("foreign grant summary marked live: %+v", item)
		}
	}
}

func checkOAuthRevokedActivity(t *testing.T, st *Store, grant OAuthGrant, at time.Time) {
	t.Helper()
	rows := activityRows(t, st, "alice", ActivityFilter{Limit: 50}, at)
	if len(rows) != 1 || !rows[0].Revoked {
		t.Fatalf("revoked history after restart: %+v", rows)
	}
	list, err := st.ActivityTokens(t.Context(), "alice", at)
	if err != nil || len(list) != 1 || !list[0].Revoked || list[0].Name != grant.ClientName {
		t.Fatalf("revoked summary: %+v %v", list, err)
	}
	list, err = st.ActivityTokens(t.Context(), "alice", at.Add(31*24*time.Hour))
	if err != nil || len(list) != 0 {
		t.Fatalf("expired revoked summary: %+v %v", list, err)
	}
}
