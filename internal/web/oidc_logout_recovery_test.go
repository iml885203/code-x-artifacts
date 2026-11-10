package web

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestOIDCLogoutKeyOutageRetriesWithoutRevokingOrAcknowledging(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	h := oidcHarness(t, p)
	browser, _ := oidcBrowser(t, h, "/")
	token := logoutToken(t, p, nil, "logout+jwt")
	p.mu.Lock()
	p.KeyStatus = 503
	p.mu.Unlock()
	if status := sendLogout(t, h, token); status != 503 {
		t.Fatal(status)
	}
	r, _ := oidcRequest(h, browser, "/")
	if r.StatusCode != 200 {
		t.Fatal("unverified event revoked session")
	}
	p.mu.Lock()
	p.KeyStatus = 0
	p.mu.Unlock()
	if status := sendLogout(t, h, token); status != 200 {
		t.Fatal(status)
	}
	r, _ = oidcRequest(h, browser, "/")
	if r.StatusCode != 303 {
		t.Fatal("retry failed to revoke")
	}
}

func TestOIDCLogoutStorageFailureRollsBackBeforeRetryableResponse(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "logout.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	p := newOIDCProvider(t)
	provider, err := auth.NewOIDC(t.Context(), p.config("https://localhost"))
	if err != nil {
		t.Fatal(err)
	}
	h := newHarnessOn(t, st, func(c *Config) { c.OIDC = provider })
	browser, _ := oidcBrowser(t, h, "/")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(t.Context(), `CREATE TRIGGER reject_logout BEFORE INSERT ON oidc_logout_events BEGIN SELECT RAISE(FAIL,'simulated storage failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	token := logoutToken(t, p, nil, "logout+jwt")
	if status := sendLogout(t, h, token); status != 503 {
		t.Fatal(status)
	}
	r, _ := oidcRequest(h, browser, "/")
	if r.StatusCode != 200 {
		t.Fatal("uncommitted deletion survived failure")
	}
	if _, err = db.ExecContext(t.Context(), `DROP TRIGGER reject_logout`); err != nil {
		t.Fatal(err)
	}
	if status := sendLogout(t, h, token); status != 200 {
		t.Fatal(status)
	}
	r, _ = oidcRequest(h, browser, "/")
	if r.StatusCode != 303 {
		t.Fatal("retry failed to revoke")
	}
}
