// Package realtime holds the live copy of an artifact: one room per artifact,
// in memory, backed by an append-only update log in SQLite, so that people and
// agents can edit the same artifact at the same moment.
//
// Rooms are in memory and the hub does not talk to other hubs, so the service
// runs as a single replica (docs/realtime.md). The hub knows nothing about
// sockets, sessions or what a document means: a transport hands it an
// Identity it has already authorised and moves bytes between a Peer and the
// wire; the access rules stay in internal/service.
//
// The sync layer is the Yjs CRDT as implemented by github.com/reearth/ygo,
// using its crdt, sync, awareness and encoding packages only. The websocket
// provider that ships with ygo is deliberately not used (docs/realtime.md).
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/reearth/ygo/crdt"
)

// Identity is who a connection is, as the transport authorised it. It is the
// only source of who edited and what the roster shows; nothing a client says
// about itself is used for either.
type Identity struct {
	// Username is the account, normalised the way sign-in does.
	Username string
	// Label is the name shown to the others; the username when empty.
	Label string
	// ReadOnly connections receive every edit and awareness message but their
	// writes are dropped before they reach the room, the log or other peers.
	ReadOnly bool
}

func (i Identity) display() string {
	if i.Label != "" {
		return i.Label
	}
	return i.Username
}

// Update is one accepted change to a room, reported after it is on disk.
type Update struct {
	Room  string
	Epoch string
	// Author is who made it: the connection's identity, or whoever a server-side
	// Edit was made for.
	Author Identity
	// Bytes is the incremental Yjs update.
	Bytes []byte
}

// Config wires a Hub. Only Log is required.
type Config struct {
	// ReceiveGuard admits each peer message before any room lock. Wire the same
	// process registry used by service and socket admission. OnUpdate and Validate
	// callbacks must never acquire outer admission.
	ReceiveGuard func(string) func()
	Log          Log
	Limits       Limits
	// Now is the clock for idle and compaction decisions; tests replace it.
	Now func() time.Time
	// IdleAfter is how long a room with nobody in it stays in memory.
	IdleAfter time.Duration
	// AgentIdle is how long an agent announced with Announce stays in the roster
	// after its last announcement.
	AgentIdle time.Duration
	// CompactAfter is the number of log rows that triggers a compaction.
	CompactAfter int
	// CompactEvery is how often Sweep compacts a room whose log has more than one row.
	CompactEvery time.Duration
	// PeerQueue is how many messages may wait for a slow connection before it is dropped.
	PeerQueue int
	// Validate, when set, is asked about the document after every change that
	// was made to it, before the change is kept: an error refuses the change
	// (the room is reset and the error is returned to whoever made it). It is
	// where a kind says what a document may not become, which an update that
	// is fine alone can still do together with what others sent.
	Validate func(room string, doc *crdt.Doc) error
	// OnUpdate, when set, is told of every accepted update, outside the room's lock.
	OnUpdate func(Update)
	// NewClientID picks the Yjs client id of a room's server-side replica;
	// tests fix it to get reproducible bytes.
	NewClientID func() crdt.ClientID
	Logger      *slog.Logger
}

// Hub owns the rooms.
type Hub struct {
	cfg Config

	// mu guards rooms and closed, and is taken before any room's lock. Loading
	// a room happens under it: the log is local SQLite and a document is capped
	// at Limits.DocBytes, so a load is tens of milliseconds, and it keeps two
	// joiners from loading the same room twice.
	mu     sync.Mutex
	rooms  map[string]*room
	closed bool
}

// New returns a hub with the defaults filled in.
func New(cfg Config) (*Hub, error) {
	if cfg.Log == nil {
		return nil, errors.New("realtime: a Log is required")
	}
	cfg.Limits = cfg.Limits.withDefaults()
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.IdleAfter <= 0 {
		cfg.IdleAfter = 5 * time.Minute
	}
	if cfg.AgentIdle <= 0 {
		cfg.AgentIdle = 15 * time.Second
	}
	if cfg.CompactAfter <= 0 {
		cfg.CompactAfter = 100
	}
	if cfg.CompactEvery <= 0 {
		cfg.CompactEvery = 10 * time.Minute
	}
	if cfg.PeerQueue <= 0 {
		cfg.PeerQueue = 256
	}
	if cfg.NewClientID == nil {
		cfg.NewClientID = crdt.NewClientID
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Hub{cfg: cfg, rooms: map[string]*room{}}, nil
}

func newEpoch() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("realtime: no randomness for an epoch: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// room returns the loaded room, loading it from the log when it is not.
func (h *Hub) room(ctx context.Context, id string) (*room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrShutdown
	}
	if r := h.rooms[id]; r != nil {
		if !r.closed.Load() {
			return r, nil
		}
		delete(h.rooms, id)
	}
	limit, err := h.documentLimit(ctx, id)
	if err != nil {
		return nil, err
	}
	snap, err := h.cfg.Log.Load(ctx, id, newEpoch())
	if err != nil {
		return nil, fmt.Errorf("load room %s: %w", id, err)
	}
	r, err := newRoom(h, id, snap, limit)
	if err != nil {
		return nil, err
	}
	h.rooms[id] = r
	return r, nil
}

// A seeded log may supply a kind-specific working-state budget. The hub stays
// ignorant of kinds, and ordinary logs retain the configured generic limit.
func (h *Hub) documentLimit(ctx context.Context, id string) (int, error) {
	if l, ok := h.cfg.Log.(interface {
		DocumentLimit(context.Context, string) (int, error)
	}); ok {
		return l.DocumentLimit(ctx, id)
	}
	return h.cfg.Limits.DocBytes, nil
}

// run runs fn with the room locked, loading the room first. A room that was
// dropped between the lookup and the lock is looked up again.
func (h *Hub) run(ctx context.Context, id string, fn func(r *room) error) error {
	for range 3 {
		r, err := h.room(ctx, id)
		if err != nil {
			return err
		}
		err = r.run(ctx, func() error { return fn(r) })
		if errors.Is(err, errRoomGone) {
			continue
		}
		return err
	}
	return fmt.Errorf("room %s kept being reset while it was used: %w", id, ErrRoomReset)
}

// Join admits a connection to the room of an artifact and queues the messages
// that start the sync: the server's state vector, its whole state and who is
// present. epoch is the epoch of the state the client already holds, empty for
// a client that holds none; a client from another epoch gets an *EpochError.
// The room is full at Limits.PeersPerRoom, which is a *LimitError.
func (h *Hub) Join(ctx context.Context, roomID string, who Identity, epoch string) (*Peer, error) {
	var p *Peer
	err := h.run(ctx, roomID, func(r *room) error {
		if epoch != "" && epoch != r.epoch {
			return &EpochError{Current: r.epoch}
		}
		if n := len(r.peers); n >= h.cfg.Limits.PeersPerRoom {
			return &LimitError{Name: "peers in the room", Value: n + 1, Max: h.cfg.Limits.PeersPerRoom}
		}
		p = r.addPeerLocked(who)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Epoch returns the epoch of the room, for a client to hold on to.
func (h *Hub) Epoch(ctx context.Context, roomID string) (string, error) {
	var epoch string
	err := h.run(ctx, roomID, func(r *room) error {
		epoch = r.epoch
		return nil
	})
	return epoch, err
}

// Read gives fn the room's document to read. fn must not change it and must
// not keep it: use Edit to change a room, so that the change is saved and sent.
func (h *Hub) Read(ctx context.Context, roomID string, fn func(doc *crdt.Doc) error) error {
	return h.run(ctx, roomID, func(r *room) error { return fn(r.doc) })
}

// ReadEpoch reads source and its generation under the same room lock.
// The callback follows Read's ownership rules and must not call back into the hub.
func (h *Hub) ReadEpoch(ctx context.Context, roomID string, fn func(*crdt.Doc, string) error) error {
	return h.run(ctx, roomID, func(r *room) error { return fn(r.doc, r.epoch) })
}

// Edit changes a room from the server, for an author the caller has already
// authorised (an agent's tool call, a restore). It is the one way to write
// without a connection, and it is saved, sent to every peer and reported
// to OnUpdate exactly like a peer's write.
//
// plan runs first and may read the document freely; it returns the change as a
// function that only writes, or nil for no change. The two steps are separate
// because inside a ygo transaction, looking up a shared type (doc.GetText,
// doc.GetXmlFragment) waits for a lock the transaction itself holds, and the
// call never returns. Everything is looked up and read before the change, and
// the write only uses what the plan fetched.
//
// If the result would not fit the room's limits or cannot be saved, nothing
// is kept: the room is reloaded from its log and Edit returns the reason.
func (h *Hub) Edit(ctx context.Context, roomID string, author Identity, plan func(doc *crdt.Doc) (write func(*crdt.Transaction), err error)) error {
	return h.EditExpect(ctx, roomID, author, "", plan)
}

// EditExpect compares the live epoch inside the mutation lock. Callers retain
// outer artifact admission; this method must never reacquire it.
func (h *Hub) EditExpect(ctx context.Context, roomID string, author Identity, epoch string, plan func(*crdt.Doc) (func(*crdt.Transaction), error)) error {
	return h.run(ctx, roomID, func(r *room) error {
		if epoch != "" && epoch != r.epoch {
			return &EpochError{Current: r.epoch}
		}
		write, err := plan(r.doc)
		if err != nil || write == nil {
			return err
		}
		r.doc.Transact(write, &source{who: author})
		return nil
	})
}

// Replace requires caller-held exclusive artifact admission when used with
// business authorization. Callbacks must not reacquire that admission.
// Replace rebuilds a room's content: build fills a fresh document, which
// becomes the room's whole log under a new epoch. Every connection to the room
// is closed with an *EpochError, because what its client holds belongs to the
// old history and merging it into the new one would duplicate the content.
func (h *Hub) Replace(ctx context.Context, roomID string, build func(doc *crdt.Doc) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrShutdown
	}
	limit, err := h.documentLimit(ctx, roomID)
	if err != nil {
		return err
	}
	doc := crdt.New(crdt.WithClientID(h.cfg.NewClientID()))
	defer doc.Destroy()
	if err := build(doc); err != nil {
		return fmt.Errorf("build room %s: %w", roomID, err)
	}
	state := crdt.EncodeStateAsUpdateV1(doc, nil)
	if err := CheckDocumentSize([][]byte{state}, limit); err != nil {
		return err
	}
	epoch := newEpoch()
	r := h.rooms[roomID]
	if r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	if err := h.cfg.Log.Replace(ctx, roomID, epoch, state); err != nil {
		return fmt.Errorf("replace the log of room %s: %w", roomID, err)
	}
	if r != nil {
		r.unloadLocked(ctx, &EpochError{Current: epoch}, false)
		delete(h.rooms, roomID)
	}
	return nil
}

// Disconnect closes the connections to a room whose identity matches, with
// the given reason (what Peer.Err reports), and returns how many it closed.
// It is how an access change reaches connections that are already open. A
// room that is not loaded has no connections.
func (h *Hub) Disconnect(roomID string, match func(Identity) bool, reason error) int {
	h.mu.Lock()
	r := h.rooms[roomID]
	h.mu.Unlock()
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for p := range r.peers {
		if match(p.who) {
			p.closeLocked(reason)
			n++
		}
	}
	return n
}

// Connected lists the identities of the connections to a room, in no
// particular order; a room that is not loaded has none.
func (h *Hub) Connected(roomID string) []Identity {
	r := h.loadedRoom(roomID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Identity, 0, len(r.peers))
	for p := range r.peers {
		out = append(out, p.who)
	}
	return out
}

// Limits returns the limits in force, defaults included.
func (h *Hub) Limits() Limits { return h.cfg.Limits }

// Loaded returns how many rooms are in memory.
func (h *Hub) Loaded() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.rooms {
		if !r.closed.Load() {
			n++
		}
	}
	return n
}

// Sweep unloads rooms that have been empty for IdleAfter and compacts the logs
// that have more than one row and have not been compacted for CompactEvery. It
// is a function of the clock, so tests drive it with a fake one.
func (h *Hub) Sweep(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.cfg.Now()
	for id, r := range h.rooms {
		r.mu.Lock()
		if !r.closed.Load() {
			r.expireAgentsLocked(now)
		}
		switch {
		case r.closed.Load():
			delete(h.rooms, id)
		case len(r.peers) == 0 && r.presences == 0 && now.Sub(r.activeAt) >= h.cfg.IdleAfter:
			r.unloadLocked(ctx, ErrShutdown, true)
			delete(h.rooms, id)
		case r.rows > 1 && now.Sub(r.compactedAt) >= h.cfg.CompactEvery:
			r.compactLocked(ctx)
		}
		r.mu.Unlock()
	}
}

// Run calls Sweep every interval until ctx ends, then closes the hub.
func (h *Hub) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			h.Close(context.WithoutCancel(ctx))
			return
		case <-t.C:
			h.Sweep(ctx)
		}
	}
}

// Close disconnects everyone and unloads every room, compacting the logs that
// need it. The hub refuses everything afterwards.
func (h *Hub) Close(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for id, r := range h.rooms {
		r.mu.Lock()
		if !r.closed.Load() {
			r.unloadLocked(ctx, ErrShutdown, true)
		}
		r.mu.Unlock()
		delete(h.rooms, id)
	}
}
