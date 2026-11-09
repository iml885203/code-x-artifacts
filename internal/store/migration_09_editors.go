package store

import (
	"context"
	"database/sql"
)

// Who besides the author may publish new versions: accounts the author
// names, and a switch for everyone in the organisation.
func migration09Editors(ctx context.Context, db *sql.DB) error {
	if err := addArtifactColumn(ctx, db, "organisation_edit", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS artifact_editors (
			artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			username    TEXT NOT NULL,
			added_at    INTEGER NOT NULL,
			PRIMARY KEY (artifact_id, username)
		)`)
	return err
}
