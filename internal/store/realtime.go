package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

// The realtime hub's update log lives next to the artifact it belongs to and
// goes with it: deleting an artifact removes its rooms' rows by cascade.
const realtimeSchema = `
CREATE TABLE IF NOT EXISTS rt_rooms (
	room  TEXT PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE,
	epoch TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS rt_updates (
	seq  INTEGER PRIMARY KEY AUTOINCREMENT,
	room TEXT NOT NULL REFERENCES rt_rooms(room) ON DELETE CASCADE,
	data BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS rt_updates_by_room ON rt_updates (room, seq)`

// RealtimeLog is the store's implementation of realtime.Log.
func (s *Store) RealtimeLog() realtime.Log { return realtimeLog{s.db} }

type realtimeLog struct{ db *sql.DB }

func (l realtimeLog) Load(ctx context.Context, room, newEpoch string) (realtime.Snapshot, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return realtime.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE id = ?`, room).Scan(&exists); err != nil {
		return realtime.Snapshot{}, err
	}
	if exists == 0 {
		return realtime.Snapshot{}, realtime.ErrNoRoom
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rt_rooms (room, epoch) VALUES (?, ?)`, room, newEpoch); err != nil {
		return realtime.Snapshot{}, err
	}
	snap := realtime.Snapshot{}
	if err := tx.QueryRowContext(ctx, `SELECT epoch FROM rt_rooms WHERE room = ?`, room).Scan(&snap.Epoch); err != nil {
		return realtime.Snapshot{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT data FROM rt_updates WHERE room = ? ORDER BY seq`, room)
	if err != nil {
		return realtime.Snapshot{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return realtime.Snapshot{}, err
		}
		snap.Updates = append(snap.Updates, data)
	}
	if err := rows.Err(); err != nil {
		return realtime.Snapshot{}, err
	}
	return snap, tx.Commit()
}

func (l realtimeLog) Append(ctx context.Context, room, epoch string, update []byte) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO rt_updates (room,data) SELECT ?,? WHERE EXISTS(SELECT 1 FROM rt_rooms WHERE room=? AND epoch=?)`, room, update, room, epoch)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return realtime.ErrEpochMismatch
	}
	if err := recordActivity(ctx, tx, room, 0); err != nil {
		return err
	}
	return tx.Commit()
}

func (l realtimeLog) Replace(ctx context.Context, room, epoch string, state []byte) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rt_rooms (room, epoch) VALUES (?, ?) ON CONFLICT (room) DO UPDATE SET epoch = excluded.epoch`, room, epoch); err != nil {
		return fmt.Errorf("set the epoch: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM rt_updates WHERE room = ?`, room); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rt_updates (room, data) VALUES (?, ?)`, room, state); err != nil {
		return err
	}
	return tx.Commit()
}

// RealtimeRows counts the rows of a room's update log.
func (s *Store) RealtimeRows(ctx context.Context, room string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rt_updates WHERE room = ?`, room).Scan(&n)
	return n, err
}
