package store

import (
	"context"
	"database/sql"
	"fmt"
)

// A comment may point at a place (anchor: a JSON object whose meaning belongs
// to the artifact's kind; empty for a comment on the whole page) and says
// whether the author's agent should act on it (for_agent, true unless the
// commenter said otherwise).
func migration19CommentAnchors(ctx context.Context, db *sql.DB) error {
	for column, definition := range map[string]string{
		"anchor":    "TEXT NOT NULL DEFAULT ''",
		"for_agent": "INTEGER NOT NULL DEFAULT 1",
	} {
		var present int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = ?`, column).Scan(&present); err != nil {
			return err
		}
		if present > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE comments ADD COLUMN %s %s`, column, definition)); err != nil {
			return err
		}
	}
	return nil
}
