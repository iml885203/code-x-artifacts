package store

import (
	"context"
	"time"
)

// RecordOpen notes that the person opened the artifact at the given time and
// keeps only their keep most recent opens. An open always sorts after the
// person's earlier ones, even within the same instant.
func (s *Store) RecordOpen(ctx context.Context, username, artifactID string, at time.Time, keep int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO opens (username, artifact_id, opened_at)
		VALUES (?1, ?2, MAX(?3, COALESCE((SELECT MAX(opened_at) + 1 FROM opens WHERE username = ?1), 0)))
		ON CONFLICT (username, artifact_id) DO UPDATE SET opened_at = excluded.opened_at`,
		username, artifactID, at.UnixMilli()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM opens WHERE username = ?1 AND artifact_id NOT IN (
			SELECT artifact_id FROM opens WHERE username = ?1 ORDER BY opened_at DESC LIMIT ?2)`,
		username, keep); err != nil {
		return err
	}
	return tx.Commit()
}

// Opened is when the person last opened each artifact they opened; a non-empty
// artifactID narrows it to that one.
func (s *Store) Opened(ctx context.Context, username, artifactID string) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, opened_at FROM opens WHERE username = ?1 AND (?2 = '' OR artifact_id = ?2)`, username, artifactID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = time.UnixMilli(at).UTC()
	}
	return out, rows.Err()
}
