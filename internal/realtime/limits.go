package realtime

import "fmt"

// Limits bound what one connection or one room may cost. A zero field takes
// the default, so a Config that names only what it changes is complete.
type Limits struct {
	// UpdateBytes is the largest single update a peer may send.
	UpdateBytes int
	// PeersPerRoom is how many connections one room holds at once.
	PeersPerRoom int
	// AwarenessBytes is the largest awareness message (cursor, name, selection) a peer may send.
	AwarenessBytes int
	// DocBytes is the size the room's document may reach. The size is the
	// encoded state as of the last compaction plus every update accepted since,
	// so it never under-counts: deleting text does not make room until the
	// next compaction.
	DocBytes int
}

// DefaultLimits are what a deployment gets without asking.
var DefaultLimits = Limits{
	UpdateBytes:    1 << 20,
	PeersPerRoom:   50,
	AwarenessBytes: 4 << 10,
	DocBytes:       2 << 20,
}

func (l Limits) withDefaults() Limits {
	if l.UpdateBytes <= 0 {
		l.UpdateBytes = DefaultLimits.UpdateBytes
	}
	if l.PeersPerRoom <= 0 {
		l.PeersPerRoom = DefaultLimits.PeersPerRoom
	}
	if l.AwarenessBytes <= 0 {
		l.AwarenessBytes = DefaultLimits.AwarenessBytes
	}
	if l.DocBytes <= 0 {
		l.DocBytes = DefaultLimits.DocBytes
	}
	return l
}

// LimitError says which limit a request ran into and by how much. Its message
// is written for the person or agent who has to react to it: it names the
// limit and gives both the current value and the cap.
type LimitError struct {
	// Name is the limit with its unit: "update size (bytes)", "peers in the room".
	Name string
	// Value is what the request amounted to: a size, or a count that includes the request.
	Value int
	// Max is the limit.
	Max int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %d is over the limit of %d", e.Name, e.Value, e.Max)
}

// CheckDocumentSize bounds the encoded log before replay or durable seeding.
func CheckDocumentSize(updates [][]byte, limit int) error {
	size := 0
	for _, update := range updates {
		size += len(update)
	}
	if size > limit {
		return &LimitError{Name: "document size (bytes)", Value: size, Max: limit}
	}
	return nil
}
