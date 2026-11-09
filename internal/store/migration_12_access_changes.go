package store

import (
	"context"
	"database/sql"
)

// What the audience and the editors became, each time the author changed
// either, so changes_since can say who changed them and when.
func migration12AccessChanges(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS access_changes (
			seq               INTEGER PRIMARY KEY AUTOINCREMENT,
			artifact_id       TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			changed_by        TEXT NOT NULL,
			visibility        TEXT NOT NULL,
			editors_json      TEXT NOT NULL,
			organisation_edit INTEGER NOT NULL,
			created_at        INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS access_changes_by_artifact ON access_changes (artifact_id, created_at)`)
	return err
}
