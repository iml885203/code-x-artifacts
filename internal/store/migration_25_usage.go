package store

import (
	"context"
	"database/sql"
)

// Usage: how often each person opened and published artifacts of each kind,
// one count per day. It names no artifact, so deleting one keeps its usage,
// and it is the only place a repeat open is counted. A person's department is
// the one AD reported at their latest sign-in, empty when it reported none.
func migration25Usage(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS usage_daily (
			day      TEXT NOT NULL,    -- YYYY-MM-DD, UTC
			username TEXT NOT NULL,    -- '' for an anonymous open
			kind     TEXT NOT NULL,
			action   TEXT NOT NULL,    -- open, publish
			via      TEXT NOT NULL,    -- web or agent for a publish, '' for an open
			count    INTEGER NOT NULL,
			PRIMARY KEY (day, username, kind, action, via)
		)`); err != nil {
		return err
	}
	var present int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'department'`).Scan(&present); err != nil {
		return err
	}
	if present > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN department TEXT NOT NULL DEFAULT ''`)
	return err
}
