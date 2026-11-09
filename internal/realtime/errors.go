package realtime

import (
	"errors"
	"fmt"
)

// Why a connection ended. Peer.Err returns one of these (or the reason a
// caller gave to Disconnect), so a transport can pick a close code and a
// message from it.
var (
	// ErrShutdown means the hub is closing.
	ErrShutdown = errors.New("the realtime hub is shutting down")
	// ErrRoomReset means the room was dropped from memory so it will be loaded
	// again from disk: an update could not be kept, or a change was refused after
	// it had been applied. Reconnecting gets the saved state.
	ErrRoomReset = errors.New("the room was reset; reconnect to continue")
	// ErrSlowPeer means the connection did not keep up with the room.
	ErrSlowPeer = errors.New("the connection is too slow to keep up")
	// ErrPeerClosed is returned for a message sent on a connection that has ended.
	ErrPeerClosed = errors.New("the connection is closed")
)

// EpochError says a client's local state belongs to a log that has since been
// rebuilt: it must drop its copy and load the room again.
type EpochError struct {
	// Current is the room's epoch now.
	Current string
}

func (e *EpochError) Error() string {
	return fmt.Sprintf("the document was rebuilt (epoch %s); start over from the server's copy", e.Current)
}

// RefusedError says a change was not kept because the document it would have
// made is one the hub's Validate refused.
type RefusedError struct{ Err error }

func (e *RefusedError) Error() string { return "the change was refused: " + e.Err.Error() }
func (e *RefusedError) Unwrap() error { return e.Err }
