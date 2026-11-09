package store

import (
	"context"
	"database/sql"
)

// Files an artifact references. Only the record lives here; the bytes are
// on the data volume, under the key in blob (internal/assets).
func migration07Assets(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS assets (
			artifact_id  TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			name         TEXT NOT NULL,
			blob         TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size         INTEGER NOT NULL,
			sha256       TEXT NOT NULL,
			created_by   TEXT NOT NULL,
			created_at   INTEGER NOT NULL,
			PRIMARY KEY (artifact_id, name)
		)`)
	return err
}
