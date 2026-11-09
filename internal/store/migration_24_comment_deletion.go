package store

import (
	"context"
	"database/sql"
)

// Deleted roots keep thread identity for other people's replies, without keeping the deleted content.
func migration24CommentDeletion(ctx context.Context, db *sql.DB) error {
	var present int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = 'deleted'`).Scan(&present); err != nil {
		return err
	}
	if present != 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE comments ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`)
	return err
}
