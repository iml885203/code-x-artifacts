package store

import (
	"context"
	"database/sql"
)

// Directory lookup metadata is separate from sign-in profiles and access rules.
func migration21Directory(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS directory_people(username TEXT PRIMARY KEY,display_name TEXT NOT NULL,active INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS directory_status(id INTEGER PRIMARY KEY CHECK(id=1),scope TEXT NOT NULL,snapshot_scope TEXT NOT NULL,attempted_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,complete INTEGER NOT NULL);`)
	return err
}
