package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const oidcSchema = `
CREATE TABLE IF NOT EXISTS oidc_identities (
 issuer TEXT NOT NULL,
 subject TEXT NOT NULL,
 username TEXT NOT NULL UNIQUE REFERENCES users(username),
 PRIMARY KEY (issuer, subject)
);
`

// ResolveOIDCUser records an authenticated external identity and its current
// profile in one transaction. Only issuer/subject select the account; profile
// names and email cannot link accounts. The reserved oidc: namespace is never a
// password-login account, and its random identifier reveals no provider subject.
// Admission and group mapping belong to the authenticator, before this call.
func (s *Store) ResolveOIDCUser(ctx context.Context, issuer, subject string, profile User) (User, error) {
	if !validOIDCIdentity(issuer, subject) {
		return User{}, errors.New("invalid OIDC identity")
	}
	profile.DisplayName = BoundedLabel(strings.TrimSpace(profile.DisplayName), 100)
	if profile.DisplayName == "" {
		profile.DisplayName = "OIDC user"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// The first statement writes, so concurrent Store handles serialize before
	// looking up the mapping; two first sign-ins cannot select different users.
	candidate, err := insertOIDCCandidate(ctx, tx)
	if err != nil {
		return User{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oidc_identities(issuer,subject,username) VALUES(?,?,?) ON CONFLICT(issuer,subject) DO NOTHING`, issuer, subject, candidate); err != nil {
		return User{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT username FROM oidc_identities WHERE issuer=? AND subject=?`, issuer, subject).Scan(&profile.Username); err != nil {
		return User{}, err
	}
	if profile.Username != candidate {
		if _, err = tx.ExecContext(ctx, `DELETE FROM users WHERE username=?`, candidate); err != nil {
			return User{}, err
		}
	}
	if err = updateOIDCProfile(ctx, tx, profile); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return profile, nil
}

func insertOIDCCandidate(ctx context.Context, tx *sql.Tx) (string, error) {
	for range 3 {
		var raw [16]byte
		rand.Read(raw[:])
		candidate := "oidc:" + hex.EncodeToString(raw[:])
		result, err := tx.ExecContext(ctx, `INSERT INTO users(username,display_name,email,groups_json,department,last_login) VALUES(?, 'OIDC user', '', '[]', '', ?) ON CONFLICT(username) DO NOTHING`, candidate, time.Now().Unix())
		if err != nil {
			return "", err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if n == 1 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("allocate OIDC account: identifier collision")
}

func updateOIDCProfile(ctx context.Context, tx *sql.Tx, profile User) error {
	groups, err := json.Marshal(profile.Groups)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET display_name=?,email=?,groups_json=?,department=?,last_login=? WHERE username=?`, profile.DisplayName, profile.Email, string(groups), profile.Department, time.Now().Unix(), profile.Username)
	return err
}

func validOIDCIdentity(issuer, subject string) bool {
	return issuer != "" && len(issuer) <= 2048 && subject != "" && len(subject) <= 255 && utf8.ValidString(issuer) && utf8.ValidString(subject)
}
