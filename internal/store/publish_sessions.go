package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrTooManySessions reports that a person already has the most publish
// sessions open that one person may.
var ErrTooManySessions = errors.New("too many open publish sessions")

// SessionFile is one file a publish session announced.
type SessionFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// PublishSession is a publish whose bytes are sent separately: what the
// publisher announced, and until when it can be committed.
type PublishSession struct {
	ID          string
	Username    string
	ArtifactID  string
	BaseVersion int
	Title       string
	Kind        string
	Format      string
	Visibility  Visibility
	Files       []SessionFile
	// Removed is the paths the publish takes out.
	Removed   []string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSessionID is the id of a publish session: 128 random bits, so nobody can
// name another person's session by guessing.
func NewSessionID() string { return randomHex(16) }

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreatePublishSession saves a session and one upload slot for each blob in
// send. It returns the secret of each slot by the blob's sha256; only the hash
// of a secret is kept. It refuses with ErrTooManySessions when the person has
// maxOpen sessions that have not expired.
func (s *Store) CreatePublishSession(ctx context.Context, sess PublishSession, send []SessionFile, maxOpen int) (map[string]string, error) {
	removed, err := json.Marshal(sess.Removed)
	if err != nil {
		return nil, fmt.Errorf("encode removed paths: %w", err)
	}
	secrets := make(map[string]string, len(send))
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var open int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publish_sessions WHERE username = ? AND expires_at > ?`, sess.Username, sess.CreatedAt.Unix()).Scan(&open); err != nil {
			return err
		}
		if open >= maxOpen {
			return ErrTooManySessions
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO publish_sessions (id, username, artifact_id, base_version, title, kind, format, visibility, removed, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sess.ID, sess.Username, sess.ArtifactID, sess.BaseVersion, sess.Title, sess.Kind, sess.Format, sess.Visibility, string(removed), sess.CreatedAt.Unix(), sess.ExpiresAt.Unix()); err != nil {
			return err
		}
		for _, f := range sess.Files {
			if _, err := tx.ExecContext(ctx, `INSERT INTO publish_session_files (session_id, path, sha256, size) VALUES (?, ?, ?, ?)`, sess.ID, f.Path, f.SHA256, f.Size); err != nil {
				return err
			}
		}
		for _, f := range send {
			secret := "pup_" + randomHex(32)
			if _, err := tx.ExecContext(ctx, `INSERT INTO publish_uploads (token_hash, session_id, sha256, size) VALUES (?, ?, ?, ?)`, hashToken(secret), sess.ID, f.SHA256, f.Size); err != nil {
				return err
			}
			secrets[f.SHA256] = secret
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// PublishSession loads a session that has not expired and belongs to username.
// Anyone else's session, an expired one and one that does not exist are all
// ErrNotFound.
func (s *Store) PublishSession(ctx context.Context, id, username string, now time.Time) (PublishSession, error) {
	var (
		sess               PublishSession
		removed            string
		created, expiresAt int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, username, artifact_id, base_version, title, kind, format, visibility, removed, created_at, expires_at
		FROM publish_sessions WHERE id = ? AND username = ? AND expires_at > ?`, id, username, now.Unix()).
		Scan(&sess.ID, &sess.Username, &sess.ArtifactID, &sess.BaseVersion, &sess.Title, &sess.Kind, &sess.Format, &sess.Visibility, &removed, &created, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PublishSession{}, ErrNotFound
	}
	if err != nil {
		return PublishSession{}, err
	}
	if err := json.Unmarshal([]byte(removed), &sess.Removed); err != nil {
		return PublishSession{}, fmt.Errorf("decode removed paths of session %s: %w", id, err)
	}
	sess.CreatedAt, sess.ExpiresAt = time.Unix(created, 0).UTC(), time.Unix(expiresAt, 0).UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT path, sha256, size FROM publish_session_files WHERE session_id = ? ORDER BY path`, id)
	if err != nil {
		return PublishSession{}, err
	}
	defer closeRows(rows)
	for rows.Next() {
		var f SessionFile
		if err := rows.Scan(&f.Path, &f.SHA256, &f.Size); err != nil {
			return PublishSession{}, err
		}
		sess.Files = append(sess.Files, f)
	}
	return sess, rows.Err()
}

// UploadedBlobs is the blobs of a session that have arrived.
func (s *Store) UploadedBlobs(ctx context.Context, sessionID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sha256 FROM publish_uploads WHERE session_id = ? AND used_at IS NOT NULL`, sessionID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	got := map[string]bool{}
	for rows.Next() {
		var sha string
		if err := rows.Scan(&sha); err != nil {
			return nil, err
		}
		got[sha] = true
	}
	return got, rows.Err()
}

// Upload is what an upload secret is good for.
type Upload struct {
	SessionID string
	SHA256    string
	Size      int64
}

// OpenUpload finds the slot a secret opens: one that has not been used and
// whose session has not expired. Any other secret is ErrNotFound.
func (s *Store) OpenUpload(ctx context.Context, secret string, now time.Time) (Upload, error) {
	var u Upload
	err := s.db.QueryRowContext(ctx, `SELECT u.session_id, u.sha256, u.size FROM publish_uploads u
		JOIN publish_sessions p ON p.id = u.session_id
		WHERE u.token_hash = ? AND u.used_at IS NULL AND p.expires_at > ?`, hashToken(secret), now.Unix()).Scan(&u.SessionID, &u.SHA256, &u.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	return u, err
}

// UseUpload spends a secret once its bytes are stored. Of two requests that
// race with the same secret exactly one gets nil; the other ErrNotFound.
func (s *Store) UseUpload(ctx context.Context, secret string, now time.Time) error {
	return requireRow(s.db.ExecContext(ctx, `UPDATE publish_uploads SET used_at = ? WHERE token_hash = ? AND used_at IS NULL
		AND session_id IN (SELECT id FROM publish_sessions WHERE expires_at > ?)`, now.Unix(), hashToken(secret), now.Unix()))
}

// ClaimPublishSession takes a session to commit it, so two commits of the same
// session cannot both publish. A session someone else holds is ErrNotFound.
func (s *Store) ClaimPublishSession(ctx context.Context, id, username string, now time.Time) error {
	return requireRow(s.db.ExecContext(ctx, `UPDATE publish_sessions SET claimed = 1 WHERE id = ? AND username = ? AND claimed = 0 AND expires_at > ?`, id, username, now.Unix()))
}

// ReleasePublishSession gives a claimed session back after a commit that did not publish.
func (s *Store) ReleasePublishSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE publish_sessions SET claimed = 0 WHERE id = ?`, id)
	return err
}

// DeletePublishSession removes a session with its files and upload slots.
func (s *Store) DeletePublishSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM publish_sessions WHERE id = ?`, id)
	return err
}

// ExpirePublishSessions deletes the sessions that expired at or before now and
// returns their ids.
func (s *Store) ExpirePublishSessions(ctx context.Context, now time.Time) ([]string, error) {
	var ids []string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM publish_sessions WHERE expires_at <= ?`, now.Unix())
		if err != nil {
			return err
		}
		defer closeRows(rows)
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM publish_sessions WHERE expires_at <= ?`, now.Unix())
		return err
	})
	return ids, err
}

// PublishSessionIDs is every session there is, expired or not.
func (s *Store) PublishSessionIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM publish_sessions`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
