package store

import (
	"context"
	"database/sql"
)

// Which version of its kind's runtime an artifact was published against.
// Everything published before this column existed is unversioned, which
// is what the default records, so nothing about it changes.
func migration03Format(ctx context.Context, db *sql.DB) error {
	return addArtifactColumn(ctx, db, "format", "TEXT NOT NULL DEFAULT ''")
}
