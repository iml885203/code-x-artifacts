package store

import (
	"context"
	"database/sql"
)

// Pins: the artifacts a person keeps at the top of their overview. A pin is
// the person's own (nobody else sees it) and goes with the artifact.
func migration18Pins(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS pins (
			username    TEXT NOT NULL,
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			pinned_at   INTEGER NOT NULL,
			PRIMARY KEY (username, artifact_id)
		)`)
	return err
}
