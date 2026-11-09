package store

import (
	"context"
	"database/sql"
)

// A room is pending from its first edit that no version holds until a version
// is cut from it. The marks are the store's half of the snapshotter
// (internal/roomkind): who edited, and which rooms to look at.

// MarkPending notes that username edited the room since the last version. An
// empty username marks the room without naming anyone.
func (s *Store) MarkPending(ctx context.Context, room, username string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rt_pending (room) SELECT id FROM artifacts WHERE id = ?`, room); err != nil {
			return err
		}
		if username == "" {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rt_pending_authors (room, username) SELECT room, ? FROM rt_pending WHERE room = ?`, username, room)
		return err
	})
}

// PendingRooms lists the rooms that have edits no version holds.
func (s *Store) PendingRooms(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT room FROM rt_pending ORDER BY room`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var rooms []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

// PendingAuthors lists who edited the room since the last version, the first
// to edit first.
func (s *Store) PendingAuthors(ctx context.Context, room string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT username FROM rt_pending_authors WHERE room = ? ORDER BY seq`, room)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var who []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		who = append(who, username)
	}
	return who, rows.Err()
}

// ClearPending forgets the room's pending edits: a version now holds them.
func (s *Store) ClearPending(ctx context.Context, room string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rt_pending WHERE room = ?`, room)
	return err
}

// MarkPendingAgent notes that an agent of by's account, the client named,
// wrote into the room since the last version.
func (s *Store) MarkPendingAgent(ctx context.Context, room, by, client string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rt_pending (room) SELECT id FROM artifacts WHERE id = ?`, room); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rt_pending_agents (room, username, client) SELECT room, ?, ? FROM rt_pending WHERE room = ?`, by, client, room)
		return err
	})
}

// PendingAgents lists the agents that wrote into the room since the last
// version, the first to write first.
func (s *Store) PendingAgents(ctx context.Context, room string) ([]VersionAgent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT username, client FROM rt_pending_agents WHERE room = ? ORDER BY seq`, room)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []VersionAgent
	for rows.Next() {
		var ag VersionAgent
		if err := rows.Scan(&ag.By, &ag.Client); err != nil {
			return nil, err
		}
		out = append(out, ag)
	}
	return out, rows.Err()
}
