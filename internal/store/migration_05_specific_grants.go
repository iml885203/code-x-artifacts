package store

import (
	"context"
	"database/sql"
)

// Sharing with named groups or people is not an audience any more. Such an
// artifact cannot keep its audience, so closed by default applies: it
// becomes private. The grants table goes with it.
func migration05SpecificGrants(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `UPDATE artifacts SET visibility = 'private' WHERE visibility = 'specific'`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS artifact_grants`)
	return err
}
