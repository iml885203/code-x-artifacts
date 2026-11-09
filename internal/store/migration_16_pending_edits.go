package store

import (
	"context"
	"database/sql"
)

// Rooms with edits that no version holds yet, and who made them: what the
// snapshotter needs to turn a quiet room into a version, and what survives a
// restart while the edits wait for the room to go quiet.
func migration16PendingEdits(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS rt_pending (
			room TEXT PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS rt_pending_authors (
			seq      INTEGER PRIMARY KEY AUTOINCREMENT,
			room     TEXT NOT NULL REFERENCES rt_pending(room) ON DELETE CASCADE,
			username TEXT NOT NULL,
			UNIQUE (room, username)
		)`)
	return err
}
