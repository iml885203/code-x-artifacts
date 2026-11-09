package store

import (
	"context"
	"database/sql"
)

// Comments on an artifact, in one thread per artifact (a reply names the
// comment it answers). seq orders them and is what "unread" is measured in;
// comment_reads remembers, per person, the newest comment they have seen.
func migration08Comments(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS comments (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			id          TEXT NOT NULL UNIQUE,
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			author      TEXT NOT NULL,
			body        TEXT NOT NULL,
			parent_id   TEXT NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS comments_by_artifact ON comments (artifact_id, seq);
		CREATE TABLE IF NOT EXISTS comment_reads (
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			username    TEXT NOT NULL,
			seen_seq    INTEGER NOT NULL,
			PRIMARY KEY (artifact_id, username)
		)`)
	return err
}
