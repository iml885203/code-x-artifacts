package store

import (
	"context"
	"database/sql"
)

// revisionActivity suppresses a history entry for a publish that saves identical content and metadata.
// The existing version behavior is unchanged; activity describes a meaningful change, not a save attempt.
func revisionActivity(ctx context.Context, tx *sql.Tx, id string, number int, rev Revision) (context.Context, error) {
	if _, ok := ctx.Value(activityKey{}).(activityWrite); !ok {
		return ctx, nil
	}
	var title, format, content string
	if err := tx.QueryRowContext(ctx, `SELECT title,format,content FROM artifact_versions WHERE artifact_id=? AND number=?`, id, number).Scan(&title, &format, &content); err != nil {
		return ctx, err
	}
	if title != rev.Title || format != rev.Format || content != rev.Content {
		return ctx, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT path,sha256 FROM version_files WHERE artifact_id=? AND version=?`, id, number)
	if err != nil {
		return ctx, err
	}
	defer closeRows(rows)
	old := map[string]string{}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return ctx, err
		}
		old[path] = hash
	}
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	if len(old) != len(rev.Files) {
		return ctx, nil
	}
	for _, f := range rev.Files {
		if old[f.Path] != f.SHA256 {
			return ctx, nil
		}
	}
	return context.WithValue(ctx, activityKey{}, nil), nil
}
