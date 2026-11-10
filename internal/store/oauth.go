package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// oauthSchema keeps the OAuth persistence tables beside their operations.
const oauthSchema = `
CREATE TABLE IF NOT EXISTS oauth_records (
 kind TEXT NOT NULL,
 key TEXT NOT NULL,
 data BLOB NOT NULL,
 expires_at INTEGER NOT NULL,
 PRIMARY KEY (kind, key)
);
CREATE INDEX IF NOT EXISTS oauth_record_expiry ON oauth_records(expires_at);
CREATE TABLE IF NOT EXISTS oauth_grants (
 id TEXT PRIMARY KEY,
 owner TEXT NOT NULL,
 client_id TEXT NOT NULL,
 client_name TEXT NOT NULL,
 resource TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 revoked INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS oauth_grant_owner ON oauth_grants(owner, created_at DESC);
`

// OAuthPut persists an opaque protocol record. The caller hashes credential keys
// and removes plaintext credentials from data before calling this method. A zero
// expiration keeps durable client metadata; expired transient records are swept
// on writes and cannot be read or consumed.
func (s *Store) OAuthPut(ctx context.Context, kind, key string, data []byte, expires time.Time) error {
	exp := int64(0)
	if !expires.IsZero() {
		exp = expires.UnixMilli()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM oauth_records WHERE expires_at > 0 AND expires_at <= ?`, time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oauth_records(kind,key,data,expires_at) VALUES(?,?,?,?) ON CONFLICT(kind,key) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at`, kind, key, data, exp); err != nil {
		return err
	}
	return tx.Commit()
}

// OAuthGet returns one unexpired record without consuming it.
func (s *Store) OAuthGet(ctx context.Context, kind, key string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM oauth_records WHERE kind=? AND key=? AND (expires_at=0 OR expires_at>?)`, kind, key, time.Now().UnixMilli()).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}

// OAuthTake atomically consumes a record. Concurrent exchanges cannot both use
// the same authorization code or refresh credential, even across Store handles.
func (s *Store) OAuthTake(ctx context.Context, kind, key string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `DELETE FROM oauth_records WHERE kind=? AND key=? AND (expires_at=0 OR expires_at>?) RETURNING data`, kind, key, time.Now().UnixMilli()).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}

// OAuthTakeRemember consumes a credential and remembers its scrubbed metadata in
// another namespace in the same transaction. A concurrent replay can therefore
// always find its grant reference after the first use succeeds. The tombstone
// has the original expiry, optionally extended through rememberUntil, and never
// stores the plaintext credential.
func (s *Store) OAuthTakeRemember(ctx context.Context, kind, key, rememberKind string, rememberUntil ...time.Time) ([]byte, error) {
	if kind == rememberKind {
		return nil, errors.New("OAuth consumption requires a distinct tombstone namespace")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var data []byte
	var expires int64
	err = tx.QueryRowContext(ctx, `DELETE FROM oauth_records WHERE kind=? AND key=? AND (expires_at=0 OR expires_at>?) RETURNING data,expires_at`, kind, key, time.Now().UnixMilli()).Scan(&data, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(rememberUntil) > 0 && rememberUntil[0].UnixMilli() > expires {
		expires = rememberUntil[0].UnixMilli()
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oauth_records(kind,key,data,expires_at) VALUES(?,?,?,?) ON CONFLICT(kind,key) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at`, rememberKind, key, data, expires); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return data, nil
}

// OAuthDelete removes one protocol record; deleting an absent record succeeds.
func (s *Store) OAuthDelete(ctx context.Context, kind, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_records WHERE kind=? AND key=?`, kind, key)
	return err
}

// OAuthGrant is the nonsecret authorization a user gave one client for a resource.
// Revocation is durable, and does not remove the reference used by retained history.
type OAuthGrant struct {
	ID         string    `json:"id"`
	Owner      string    `json:"-"`
	ClientID   string    `json:"clientId"`
	ClientName string    `json:"clientName"`
	Resource   string    `json:"resource"`
	CreatedAt  time.Time `json:"createdAt"`
	Revoked    bool      `json:"revoked"`
}

// CreateOAuthGrant records a new authorization. It never replaces an existing
// grant or clears a revocation, even if the caller reuses its identifier.
func (s *Store) CreateOAuthGrant(ctx context.Context, g OAuthGrant) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_grants(id,owner,client_id,client_name,resource,created_at,revoked) VALUES(?,?,?,?,?,?,?)`, g.ID, g.Owner, g.ClientID, g.ClientName, g.Resource, g.CreatedAt.UnixMilli(), g.Revoked)
	return err
}

// OAuthGrant returns the durable grant, including its revocation status. This is
// an internal lookup, not a user-facing authorization check; lists and mutations
// below constrain ownership explicitly.
func (s *Store) OAuthGrant(ctx context.Context, id string) (OAuthGrant, error) {
	var g OAuthGrant
	var at int64
	err := s.db.QueryRowContext(ctx, `SELECT id,owner,client_id,client_name,resource,created_at,revoked FROM oauth_grants WHERE id=?`, id).Scan(&g.ID, &g.Owner, &g.ClientID, &g.ClientName, &g.Resource, &at, &g.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthGrant{}, ErrNotFound
	}
	g.CreatedAt = time.UnixMilli(at).UTC()
	return g, err
}

// OAuthGrants lists only this owner's nonsecret authorizations, newest first.
func (s *Store) OAuthGrants(ctx context.Context, owner string) ([]OAuthGrant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,owner,client_id,client_name,resource,created_at,revoked FROM oauth_grants WHERE owner=? ORDER BY created_at DESC,id DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []OAuthGrant{}
	for rows.Next() {
		var g OAuthGrant
		var at int64
		if err := rows.Scan(&g.ID, &g.Owner, &g.ClientID, &g.ClientName, &g.Resource, &at, &g.Revoked); err != nil {
			return nil, err
		}
		g.CreatedAt = time.UnixMilli(at).UTC()
		list = append(list, g)
	}
	return list, rows.Err()
}

// RevokeOAuthGrant revokes only an owned grant. Missing and foreign grants are
// indistinguishable; repeating an owned revocation succeeds.
func (s *Store) RevokeOAuthGrant(ctx context.Context, owner, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE oauth_grants SET revoked=1 WHERE id=? AND owner=?`, id, owner)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
