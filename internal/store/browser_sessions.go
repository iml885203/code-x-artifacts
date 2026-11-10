package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const browserSchema = `
CREATE TABLE IF NOT EXISTS browser_sessions (
 hash TEXT PRIMARY KEY, username TEXT NOT NULL REFERENCES users(username), expires INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS browser_sessions_owner ON browser_sessions(username);
CREATE TABLE IF NOT EXISTS oidc_revocations (
 issuer TEXT NOT NULL, subject TEXT NOT NULL, cutoff INTEGER NOT NULL, received INTEGER NOT NULL, expires INTEGER NOT NULL,
 PRIMARY KEY(issuer,subject)
);
CREATE TABLE IF NOT EXISTS oidc_logout_events (
 issuer TEXT NOT NULL, jti TEXT NOT NULL, expires INTEGER NOT NULL, PRIMARY KEY(issuer,jti)
);
`

// BrowserStateLimit bounds each durable authentication table. Capacity refuses
// new authentication/events rather than evicting active revocation protection.
const BrowserStateLimit = 10000

var (
	// ErrBrowserRevoked rejects an OIDC callback covered by a durable cutoff.
	ErrBrowserRevoked = errors.New("browser authentication was revoked; sign in again")
	// ErrBrowserCapacity refuses new state without evicting active protection.
	ErrBrowserCapacity = errors.New("browser authentication storage capacity reached")
)

// BrowserIdentity binds an OIDC session to both the verified ID-token issuance
// and this browser transaction's start. Empty means password authentication.
type BrowserIdentity struct {
	Issuer, Subject string
	IssuedAt        float64
	StartedAt       int64
}

func cleanBrowserState(ctx context.Context, tx *sql.Tx, now time.Time) error {
	for _, statement := range []string{`DELETE FROM browser_sessions WHERE expires<=?`, `DELETE FROM oidc_revocations WHERE expires<=?`, `DELETE FROM oidc_logout_events WHERE expires<=?`} {
		if _, err := tx.ExecContext(ctx, statement, now.Unix()); err != nil {
			return err
		}
	}
	return nil
}

func browserCapacity(ctx context.Context, tx *sql.Tx, table string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		return err
	}
	if n >= BrowserStateLimit {
		return ErrBrowserCapacity
	}
	return nil
}

// CreateBrowserSession serializes with revocation across SQLite connections.
// hash is a keyed digest of a random cookie, never the cookie itself.
func (s *Store) CreateBrowserSession(ctx context.Context, hash, username string, identity BrowserIdentity, now, expires time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// First statement writes, obtaining SQLite's writer lock before cutoff reads.
	if err = cleanBrowserState(ctx, tx, now); err != nil {
		return err
	}
	if identity.Issuer != "" {
		var cutoff, received int64
		err = tx.QueryRowContext(ctx, `SELECT cutoff,received FROM oidc_revocations WHERE issuer=? AND subject=?`, identity.Issuer, identity.Subject).Scan(&cutoff, &received)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && (identity.IssuedAt <= float64(cutoff) || identity.StartedAt <= received) {
			return ErrBrowserRevoked
		}
	}
	if err = browserCapacity(ctx, tx, "browser_sessions"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_sessions(hash,username,expires) VALUES(?,?,?)`, hash, username, expires.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// BrowserSession resolves only active opaque credentials; missing/expired state
// fails closed, including after a local logout or a server restart.
func (s *Store) BrowserSession(ctx context.Context, hash string, now time.Time) (string, error) {
	var username string
	err := s.db.QueryRowContext(ctx, `SELECT username FROM browser_sessions WHERE hash=? AND expires>?`, hash, now.Unix()).Scan(&username)
	return username, err
}

// RevokeBrowserSession invalidates one browser credential, including its copies.
func (s *Store) RevokeBrowserSession(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM browser_sessions WHERE hash=?`, hash)
	return err
}

// RevokeOIDCBrowsers commits cutoff, deletion and replay marker together. Even
// unknown subjects retain a cutoff, protecting callbacks already in flight.
// Identical events do not move the cutoff or revoke a subsequent login.
func (s *Store) RevokeOIDCBrowsers(ctx context.Context, issuer, subject, jti string, issuedAt int64, tokenExpiry, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = cleanBrowserState(ctx, tx, now); err != nil {
		return err
	}
	var seen int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM oidc_logout_events WHERE issuer=? AND jti=?`, issuer, jti).Scan(&seen); err != nil {
		return err
	}
	if seen > 0 {
		return tx.Commit()
	}
	if err = browserCapacity(ctx, tx, "oidc_logout_events"); err != nil {
		return err
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM oidc_revocations WHERE issuer=? AND subject=?`, issuer, subject).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		if err = browserCapacity(ctx, tx, "oidc_revocations"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oidc_revocations(issuer,subject,cutoff,received,expires) VALUES(?,?,?,?,?) ON CONFLICT(issuer,subject) DO UPDATE SET cutoff=max(cutoff,excluded.cutoff),received=max(received,excluded.received),expires=max(expires,excluded.expires)`, issuer, subject, max(now.Unix(), issuedAt), now.UnixNano(), now.Add(8*24*time.Hour).Unix()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM browser_sessions WHERE username IN (SELECT username FROM oidc_identities WHERE issuer=? AND subject=?)`, issuer, subject); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oidc_logout_events(issuer,jti,expires) VALUES(?,?,?)`, issuer, jti, tokenExpiry.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}
