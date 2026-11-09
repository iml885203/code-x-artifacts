package store

import (
	"context"
	"database/sql"
)

// The realtime hub's update log (docs/realtime.md); realtimeSchema is next to
// the code that uses the tables.
func migrateRealtime(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, realtimeSchema)
	return err
}
