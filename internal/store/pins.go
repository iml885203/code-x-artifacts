package store

import (
	"context"
	"time"
)

// SetPin pins or unpins an artifact for one person. Both are idempotent.
func (s *Store) SetPin(ctx context.Context, username, artifactID string, pinned bool) error {
	if pinned {
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO pins (username, artifact_id, pinned_at) VALUES (?, ?, ?)`, username, artifactID, time.Now().Unix())
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM pins WHERE username = ? AND artifact_id = ?`, username, artifactID)
	return err
}

// Pinned is the ids of the artifacts the person has pinned; a non-empty
// artifactID narrows it to that one.
func (s *Store) Pinned(ctx context.Context, username, artifactID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id FROM pins WHERE username = ?1 AND (?2 = '' OR artifact_id = ?2)`, username, artifactID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
