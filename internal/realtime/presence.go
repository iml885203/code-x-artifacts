package realtime

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/reearth/ygo/encoding"
)

// maxClientIDsPerPeer bounds how many awareness identities one connection may
// claim; a browser tab uses one.
const maxClientIDsPerPeer = 8

// maxAwarenessEntries bounds the entries of one awareness message.
const maxAwarenessEntries = 32

type awarenessEntry struct {
	clientID uint64
	clock    uint64
	state    []byte // JSON; "null" means the client has left
}

func decodeAwareness(update []byte) ([]awarenessEntry, error) {
	dec := encoding.NewDecoder(update)
	n, err := dec.ReadVarUint()
	if err != nil {
		return nil, fmt.Errorf("awareness message: %w", err)
	}
	if n > maxAwarenessEntries {
		return nil, &LimitError{Name: "awareness entries per message", Value: int(min(n, math.MaxInt32)), Max: maxAwarenessEntries}
	}
	entries := make([]awarenessEntry, 0, n)
	for range n {
		id, err := dec.ReadVarUint()
		if err != nil {
			return nil, fmt.Errorf("awareness message: %w", err)
		}
		clock, err := dec.ReadVarUint()
		if err != nil {
			return nil, fmt.Errorf("awareness message: %w", err)
		}
		state, err := dec.ReadVarBytes()
		if err != nil {
			return nil, fmt.Errorf("awareness message: %w", err)
		}
		entries = append(entries, awarenessEntry{clientID: id, clock: clock, state: state})
	}
	return entries, nil
}

func encodeAwareness(entries []awarenessEntry) []byte {
	enc := encoding.NewEncoder()
	enc.WriteVarUint(uint64(len(entries)))
	for _, e := range entries {
		enc.WriteVarUint(e.clientID)
		enc.WriteVarUint(e.clock)
		enc.WriteVarBytes(e.state)
	}
	return enc.Bytes()
}

// isLeaving reports whether the state says the client has gone.
func (e awarenessEntry) isLeaving() bool {
	return len(e.state) == 0 || string(e.state) == "null"
}

// withAuthenticatedUser returns the state with its `user` replaced by what the
// server knows about the connection. The name a client reports about itself is
// never shown: a roster that lets people pick their own name lets them be
// someone else. The colour the client picked is kept, since it is only a colour.
//
// `id` is who the entry is, apart from what it is called: the account for a
// person, "<account>/<client>" for an agent. Another list of the same people
// (the viewers) carries the same id, so a page can tell they are one person
// whatever name each list shows.
func withAuthenticatedUser(state []byte, id string, who Identity, agent bool) ([]byte, error) {
	var fields map[string]any
	if err := json.Unmarshal(state, &fields); err != nil {
		return nil, fmt.Errorf("awareness state is not a JSON object: %w", err)
	}
	if fields == nil {
		fields = map[string]any{}
	}
	user := map[string]any{"name": who.display()}
	if id != "" {
		user["id"] = id
	}
	if agent {
		user["agent"] = true
	}
	if reported, ok := fields["user"].(map[string]any); ok {
		if colour, ok := reported["color"].(string); ok && len(colour) <= 32 {
			user["color"] = colour
		}
	}
	fields["user"] = user
	return json.Marshal(fields)
}
