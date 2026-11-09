package store

import (
	"context"
	"database/sql"
)

// Opens: when each person last opened each artifact, for their own "Recently
// opened" list. Only the latest open per artifact is kept, nobody else sees
// it, and it goes with the artifact.
func migration23Opens(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS opens (
			username    TEXT NOT NULL,
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			opened_at   INTEGER NOT NULL, -- unix milliseconds
			PRIMARY KEY (username, artifact_id)
		);
		CREATE INDEX IF NOT EXISTS opens_by_person ON opens(username, opened_at DESC)`)
	return err
}
