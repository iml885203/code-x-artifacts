package store

import (
	"context"
	"database/sql"
)

// Every shared document carries a version, from 1, that each write
// advances, so a write can say which version it was built on. A document
// that existed before the column is at version 1.
func migration06DocVersion(ctx context.Context, db *sql.DB) error {
	var present int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('docs') WHERE name = 'version'`).Scan(&present); err != nil || present > 0 {
		return err
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE docs ADD COLUMN version INTEGER NOT NULL DEFAULT 1`)
	return err
}
