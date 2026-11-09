package store

import (
	"context"
	"database/sql"
)

// Versions: every artifact that existed before has exactly one, number 1,
// built from its current row. The artifacts row keeps holding the newest
// version's content so a rollback to an earlier binary still finds it.
func migration04Versions(ctx context.Context, db *sql.DB) error {
	if err := addArtifactColumn(ctx, db, "version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS artifact_versions (
			artifact_id   TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
			number        INTEGER NOT NULL,
			title         TEXT NOT NULL,
			kind          TEXT NOT NULL,
			format        TEXT NOT NULL,
			content       TEXT NOT NULL,
			author        TEXT NOT NULL,
			restored_from INTEGER NOT NULL DEFAULT 0,
			created_at    INTEGER NOT NULL,
			PRIMARY KEY (artifact_id, number)
		)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_versions (artifact_id, number, title, kind, format, content, author, created_at)
		SELECT id, version, title, kind, format, html, author, updated_at FROM artifacts
		WHERE NOT EXISTS (SELECT 1 FROM artifact_versions v WHERE v.artifact_id = artifacts.id)`)
	return err
}
