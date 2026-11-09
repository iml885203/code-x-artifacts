package store

import (
	"context"
	"database/sql"
)

// Every artifact that existed before kinds was a single HTML page. The
// content column keeps its original name, html, so a rollback to an earlier
// binary still finds it.
func migration02Kind(ctx context.Context, db *sql.DB) error {
	return addArtifactColumn(ctx, db, "kind", "TEXT NOT NULL DEFAULT 'html'")
}
