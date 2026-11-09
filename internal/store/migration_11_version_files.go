package store

import (
	"context"
	"database/sql"
)

// The files of each version (docs/multi-file.md): one row per path. Only
// the manifest lives here; the bytes are blobs on the data volume named by
// their sha256. Deleting a version's row would delete its files, so the
// foreign key is the same cascade as the version itself.
func migration11VersionFiles(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS version_files (
			artifact_id  TEXT NOT NULL,
			version      INTEGER NOT NULL,
			path         TEXT NOT NULL,
			sha256       TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size         INTEGER NOT NULL,
			PRIMARY KEY (artifact_id, version, path),
			FOREIGN KEY (artifact_id, version) REFERENCES artifact_versions (artifact_id, number) ON DELETE CASCADE
		)`)
	return err
}
