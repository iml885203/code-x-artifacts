package store

import (
	"context"
	"database/sql"
)

// The people who edited a version. A published or restored version has one,
// the person who did it; a version cut from a live room (docs/doc-room.md) has
// everyone who edited since the one before. Every version that existed before
// gets the author it already had, so a reader needs no second case.
func migration15VersionAuthors(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS version_authors (
			artifact_id TEXT NOT NULL,
			version     INTEGER NOT NULL,
			position    INTEGER NOT NULL,
			username    TEXT NOT NULL,
			PRIMARY KEY (artifact_id, version, username),
			FOREIGN KEY (artifact_id, version) REFERENCES artifact_versions(artifact_id, number) ON DELETE CASCADE
		)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO version_authors (artifact_id, version, position, username)
		SELECT artifact_id, number, 0, author FROM artifact_versions`)
	return err
}
