package store

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func oidcStoreOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func resolveOIDCForTest(t *testing.T, st *Store, issuer, subject string, profile User) User {
	t.Helper()
	u, err := st.ResolveOIDCUser(t.Context(), issuer, subject, profile)
	oidcStoreOK(t, err)
	return u
}

func TestOIDCIdentitySurvivesRestartAndProfileRenamesWithoutChangingOwnership(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oidc.db")
	st := openStore(t.Context(), t, path)
	first := resolveOIDCForTest(t, st, "https://id.example", "subject-one", User{Username: "alice", DisplayName: "Alice", Email: "alice@example.org", Groups: []string{"team"}})
	if !strings.HasPrefix(first.Username, "oidc:") || first.Username == "alice" {
		t.Fatalf("external username selected by profile: %q", first.Username)
	}
	a, err := st.CreateArtifact(t.Context(), "Private report", "html", "", "hello", first.Username, VisibilityPrivate)
	oidcStoreOK(t, err)
	oidcStoreOK(t, st.Close())
	st = openStore(t.Context(), t, path)
	defer func() { _ = st.Close() }()
	renamed := resolveOIDCForTest(t, st, "https://id.example", "subject-one", User{Username: "changed", DisplayName: "Renamed", Email: "new@example.org", Groups: []string{"other"}})
	if renamed.Username != first.Username {
		t.Fatalf("identity changed after restart/rename: %q vs %q", renamed.Username, first.Username)
	}
	got, err := st.User(t.Context(), first.Username)
	oidcStoreOK(t, err)
	if got.DisplayName != "Renamed" || got.Email != "new@example.org" || len(got.Groups) != 1 || got.Groups[0] != "other" {
		t.Fatalf("profile not refreshed: %+v", got)
	}
	owned, err := st.Artifact(t.Context(), a.ID)
	oidcStoreOK(t, err)
	if owned.Author != first.Username {
		t.Fatalf("artifact ownership changed: %+v", owned)
	}
}

func TestOIDCProfilesCannotLinkLocalAccountsOrOtherIssuerSubjects(t *testing.T) {
	t.Parallel()
	st := openStore(t.Context(), t, filepath.Join(t.TempDir(), "oidc.db"))
	defer func() { _ = st.Close() }()
	profile := User{Username: "alice", DisplayName: "Alice", Email: "alice@example.org", Groups: []string{"local"}}
	oidcStoreOK(t, st.SaveUser(t.Context(), profile))
	seen := map[string]bool{"alice": true}
	for _, identity := range []struct{ issuer, subject string }{{"https://id.example", "one"}, {"https://id.example", "two"}, {"https://other.example", "one"}} {
		u := resolveOIDCForTest(t, st, identity.issuer, identity.subject, profile)
		if seen[u.Username] {
			t.Fatalf("identities merged by profile or subject: %+v", u)
		}
		seen[u.Username] = true
	}
	local, err := st.User(t.Context(), "alice")
	oidcStoreOK(t, err)
	if local.DisplayName != profile.DisplayName || local.Email != profile.Email || len(local.Groups) != 1 || local.Groups[0] != "local" {
		t.Fatalf("local profile overwritten: %+v", local)
	}
}

func TestConcurrentOIDCFirstSignInsResolveOneDurableAccount(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oidc.db")
	first := openStore(t.Context(), t, path)
	defer func() { _ = first.Close() }()
	second := openStore(t.Context(), t, path)
	defer func() { _ = second.Close() }()
	users := make(chan string, 12)
	var wg sync.WaitGroup
	for i := range 12 {
		st := first
		if i%2 == 1 {
			st = second
		}
		wg.Go(func() {
			u, err := st.ResolveOIDCUser(t.Context(), "https://id.example", "one", User{DisplayName: "Same person"})
			if err != nil {
				t.Error(err)
				return
			}
			users <- u.Username
		})
	}
	wg.Wait()
	close(users)
	var account string
	for user := range users {
		if account == "" {
			account = user
		}
		if user != account {
			t.Fatalf("one identity got multiple accounts: %q and %q", account, user)
		}
	}
	var count int
	oidcStoreOK(t, first.db.QueryRowContext(t.Context(), `SELECT count(*) FROM users WHERE username LIKE 'oidc:%'`).Scan(&count))
	if count != 1 {
		t.Fatalf("left orphan accounts: %d", count)
	}
}

func TestEmptyOrOversizedOIDCIdentitiesCannotCreateAccounts(t *testing.T) {
	t.Parallel()
	st := openStore(t.Context(), t, filepath.Join(t.TempDir(), "oidc.db"))
	defer func() { _ = st.Close() }()
	for _, identity := range []struct{ issuer, subject string }{{"", "one"}, {"https://id.example", ""}, {"https://id.example", strings.Repeat("s", 256)}, {strings.Repeat("i", 2049), "one"}} {
		if _, err := st.ResolveOIDCUser(t.Context(), identity.issuer, identity.subject, User{DisplayName: "Alice"}); err == nil {
			t.Errorf("accepted invalid identity: lengths %d/%d", len(identity.issuer), len(identity.subject))
		}
	}
}

func TestKnownOIDCProfilesCanBeFoundWithoutInventingACompleteProviderDirectory(t *testing.T) {
	t.Parallel()
	st := openStore(t.Context(), t, filepath.Join(t.TempDir(), "oidc.db"))
	defer func() { _ = st.Close() }()
	known := resolveOIDCForTest(t, st, "https://id.example", "one", User{DisplayName: "Known person"})
	oidcStoreOK(t, st.SaveUser(t.Context(), User{Username: "unlisted-local", DisplayName: "Known local"}))
	people, err := st.SearchUsers(t.Context(), "known", 10)
	oidcStoreOK(t, err)
	if len(people) != 1 || people[0].Username != known.Username || people[0].DisplayName != "Known person" {
		t.Fatalf("known OIDC profile absent or unknown local profile leaked: %+v", people)
	}
	limited, err := st.SearchUsers(t.Context(), "", 0)
	oidcStoreOK(t, err)
	if len(limited) != 0 {
		t.Fatalf("ignored result bound: %+v", limited)
	}
}
