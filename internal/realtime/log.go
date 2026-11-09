package realtime

import (
	"context"
	"errors"
)

// ErrNoRoom reports that a room's artifact does not exist. A caller answers it
// like any missing artifact.
var ErrNoRoom = errors.New("no such room")

// ErrEpochMismatch reports an append to a log whose epoch is no longer the one
// the room was loaded under: the log was rebuilt behind the room's back.
var ErrEpochMismatch = errors.New("the room's log has a different epoch")

// Snapshot is what a room is loaded from.
type Snapshot struct {
	// Epoch names the lineage of the log. It changes when the log is rebuilt
	// rather than merely compacted, so a client that holds state from another
	// epoch must start over: merging its history into the new one would
	// duplicate the content.
	Epoch string
	// Updates are the log's rows in the order they were appended.
	Updates [][]byte
}

// Log is the room's durable update log. Every call is atomic, and a row that
// Append has returned for is on disk: the hub acknowledges an update only
// after that.
type Log interface {
	// Load returns the room's log. A room with no log yet gets a log with
	// the epoch newEpoch and no rows. It returns ErrNoRoom when the room's
	// artifact does not exist.
	Load(ctx context.Context, room, newEpoch string) (Snapshot, error)
	// Append adds one update, provided the room still has the given epoch.
	Append(ctx context.Context, room, epoch string, update []byte) error
	// Replace makes state the room's whole log, as one row, under the given
	// epoch. Compaction passes the epoch the room already has; rebuilding the
	// log passes a new one.
	Replace(ctx context.Context, room, epoch string, state []byte) error
}
