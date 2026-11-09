package store

import (
	"context"
	"database/sql"
)

// Publish sessions (docs/large-publish.md): a publish whose bytes travel out of
// band. The session records what was announced, the uploads record which blobs
// may still be sent and which have arrived; the bytes themselves wait on the
// data volume until the commit.
func migration17PublishSessions(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS publish_sessions (
			id           TEXT PRIMARY KEY,
			username     TEXT NOT NULL,
			artifact_id  TEXT NOT NULL,
			base_version INTEGER NOT NULL,
			title        TEXT NOT NULL,
			kind         TEXT NOT NULL,
			format       TEXT NOT NULL,
			visibility   TEXT NOT NULL,
			removed      TEXT NOT NULL,
			claimed      INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL,
			expires_at   INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS publish_sessions_user ON publish_sessions (username);
		CREATE INDEX IF NOT EXISTS publish_sessions_expiry ON publish_sessions (expires_at);
		CREATE TABLE IF NOT EXISTS publish_session_files (
			session_id TEXT NOT NULL REFERENCES publish_sessions(id) ON DELETE CASCADE,
			path       TEXT NOT NULL,
			sha256     TEXT NOT NULL,
			size       INTEGER NOT NULL,
			PRIMARY KEY (session_id, path)
		);
		CREATE TABLE IF NOT EXISTS publish_uploads (
			token_hash TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES publish_sessions(id) ON DELETE CASCADE,
			sha256     TEXT NOT NULL,
			size       INTEGER NOT NULL,
			used_at    INTEGER,
			UNIQUE (session_id, sha256)
		)`)
	return err
}
