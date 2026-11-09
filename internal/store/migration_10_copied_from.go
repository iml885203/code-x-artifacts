package store

import (
	"context"
	"database/sql"
)

// Where a copy came from. No foreign key: a copy outlives its source, and
// keeps the title the source had when it was copied.
func migration10CopiedFrom(ctx context.Context, db *sql.DB) error {
	if err := addArtifactColumn(ctx, db, "copied_from", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return addArtifactColumn(ctx, db, "copied_from_title", "TEXT NOT NULL DEFAULT ''")
}
