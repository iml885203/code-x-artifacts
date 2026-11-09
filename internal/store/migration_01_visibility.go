package store

import (
	"context"
	"database/sql"
)

// Artifacts that existed before visibility was introduced were visible to
// the whole organisation; the column default keeps them that way. New rows
// always name their visibility explicitly.
func migration01Visibility(ctx context.Context, db *sql.DB) error {
	return addArtifactColumn(ctx, db, "visibility", "TEXT NOT NULL DEFAULT 'organisation'")
}
