package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reearth/ygo/awareness"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"
)

// y-websocket's outer message types: what a client's provider sends and expects.
const (
	msgSync           = 0
	msgAwareness      = 1
	msgQueryAwareness = 3
)

// errRoomGone is internal: the room was dropped from memory between a lookup
// and the lock, so the caller looks it up again.
var errRoomGone = errors.New("the room was dropped from memory")

// source is the origin handed to the document with every change, so that the
// room's observer knows whose it is and does not echo it back to its sender.
// It is a pointer to a non-empty struct on purpose: ygo compares origins with
// ==, and pointers to zero-size values may compare equal.
type source struct {
	who  Identity
	peer *Peer
}

type room struct {
	hub   *Hub
	id    string
	epoch string

	// closed is set once, under mu, when the room leaves memory; it is atomic
	// so the hub can look at it without taking mu.
	closed atomic.Bool

	mu          sync.Mutex
	ctx         context.Context //nolint:containedctx // the request that holds mu, for the log calls made under it
	doc         *crdt.Doc
	aw          *awareness.Awareness
	peers       map[*Peer]struct{}
	owner       map[uint64]any // awareness client id -> the *Peer, or serverPresence, that published it
	presences   int
	agents      map[string]*agentPresence // announced agents by key (Hub.Announce)
	rows        int                       // rows in the log since the last compaction
	size        int                       // encoded state at the last compaction plus the updates accepted since
	activeAt    time.Time
	compactedAt time.Time
	events      []Update
	// queued holds the updates the document made during this operation until
	// settleLocked has had them checked: they are not on disk, and no one has
	// been told of them.
	queued     []queuedUpdate
	queuedSize int
	docBytes   int
	// failure is set by the observer when an update could not be kept; the
	// operation that caused it reads it when the change has returned.
	failure error
}

type queuedUpdate struct {
	update []byte
	origin any
}

// isEmptyUpdate reports whether a V1 update carries no structs and no deletions.
func isEmptyUpdate(update []byte) bool {
	return len(update) == 0 || (len(update) == 2 && update[0] == 0 && update[1] == 0)
}

// serverPresence owns the awareness entries the server publishes itself.
type serverPresence struct{ _ byte }

var presenceOwner = &serverPresence{}

func newRoom(h *Hub, id string, snap Snapshot, docBytes int) (*room, error) {
	if err := CheckDocumentSize(snap.Updates, docBytes); err != nil {
		return nil, err
	}
	now := h.cfg.Now()
	doc := crdt.New(crdt.WithClientID(h.cfg.NewClientID()))
	size := 0
	for i, u := range snap.Updates {
		if err := crdt.ApplyUpdateV1(doc, u, nil); err != nil {
			return nil, fmt.Errorf("room %s: stored update %d of %d does not apply: %w", id, i+1, len(snap.Updates), err)
		}
		size += len(u)
	}
	r := &room{
		hub:         h,
		id:          id,
		epoch:       snap.Epoch,
		doc:         doc,
		aw:          awareness.New(uint64(doc.ClientID())),
		peers:       map[*Peer]struct{}{},
		owner:       map[uint64]any{},
		rows:        len(snap.Updates),
		size:        size,
		docBytes:    docBytes,
		activeAt:    now,
		compactedAt: now,
	}
	// Registered after the log is replayed: what is already on disk is not saved again.
	doc.OnUpdate(r.committed)
	return r, nil
}

// run runs fn with the room locked and then settles what fn caused: a change
// that could not be kept drops the room, a long log is compacted, and the
// accepted updates are reported once the lock is released.
func (r *room) run(ctx context.Context, fn func() error) error {
	r.mu.Lock()
	if r.closed.Load() {
		r.mu.Unlock()
		return errRoomGone
	}
	r.ctx = ctx
	err := fn()
	r.settleLocked()
	if r.failure != nil {
		err = errors.Join(r.failure, err)
		r.failure = nil
		r.unloadLocked(ctx, ErrRoomReset, false)
	} else if r.rows >= r.hub.cfg.CompactAfter {
		r.compactLocked(ctx)
	}
	events := r.events
	r.events = nil
	r.mu.Unlock()
	if report := r.hub.cfg.OnUpdate; report != nil {
		for _, e := range events {
			report(e)
		}
	}
	return err
}

// committed runs inside every change to the document, under the lock of the
// operation that made it. It only takes note: the document cannot be read from
// here (a read waits for the lock this holds), so whether the change may be
// kept is decided by settleLocked when the operation's change has returned.
func (r *room) committed(update []byte, origin any) {
	if r.failure != nil || isEmptyUpdate(update) {
		return
	}
	if total := r.size + r.queuedSize + len(update); total > r.docBytes {
		r.failure = &LimitError{Name: "document size (bytes)", Value: total, Max: r.docBytes}
		return
	}
	r.queued = append(r.queued, queuedUpdate{update, origin})
	r.queuedSize += len(update)
}

// settleLocked keeps what the operation changed, or refuses it. The document
// is checked first, when the hub was given a check (Config.Validate): a change
// it refuses is neither saved nor sent, and the room is dropped from memory and
// loaded again from the log, so the document goes back to what it was. An
// update is appended to the log before anyone is told of it, so a change that
// was acknowledged or shown to another peer is on disk. When it cannot be kept
// the room is dropped by the operation (see run) and the change is neither
// saved nor sent.
func (r *room) settleLocked() {
	queued := r.queued
	r.queued, r.queuedSize = nil, 0
	if len(queued) == 0 || r.failure != nil {
		return
	}
	if check := r.hub.cfg.Validate; check != nil {
		if err := check(r.id, r.doc); err != nil {
			r.failure = &RefusedError{Err: err}
			return
		}
	}
	for _, q := range queued {
		r.keepLocked(q.update, q.origin)
		if r.failure != nil {
			return
		}
	}
}

func (r *room) keepLocked(update []byte, origin any) {
	if err := r.hub.cfg.Log.Append(r.ctx, r.id, r.epoch, update); err != nil {
		r.failure = fmt.Errorf("keep the update of room %s: %w", r.id, err)
		return
	}
	r.rows++
	r.size += len(update)
	r.activeAt = r.hub.cfg.Now()

	src, _ := origin.(*source)
	var author Identity
	var from *Peer
	if src != nil {
		author, from = src.who, src.peer
	}
	r.events = append(r.events, Update{Room: r.id, Epoch: r.epoch, Author: author, Bytes: update})
	msg := frame(msgSync, ygsync.EncodeUpdate(update))
	for p := range r.peers {
		if p != from {
			p.enqueueLocked(msg)
		}
	}
}

// compactLocked folds the log into one row holding the document's state. It
// keeps the epoch: the history is the same, only stored smaller. A failure
// leaves the log as it was and is logged; the next trigger tries again.
func (r *room) compactLocked(ctx context.Context) {
	state := crdt.EncodeStateAsUpdateV1(r.doc, nil)
	if err := r.hub.cfg.Log.Replace(ctx, r.id, r.epoch, state); err != nil {
		r.hub.cfg.Logger.Error("compact the update log", "room", r.id, "err", err)
		return
	}
	r.rows, r.size, r.compactedAt = 1, len(state), r.hub.cfg.Now()
}

// unloadLocked removes the room from memory: it compacts the log when asked
// to, closes every connection with reason, and releases the document. The
// caller removes the room from the hub's map.
func (r *room) unloadLocked(ctx context.Context, reason error, compact bool) {
	if compact && r.rows > 1 {
		r.compactLocked(ctx)
	}
	r.closed.Store(true)
	for p := range r.peers {
		p.closeLocked(reason)
	}
	r.doc.Destroy()
}

func frame(kind uint64, payload []byte) []byte {
	enc := encoding.NewEncoder()
	enc.WriteVarUint(kind)
	if kind == msgAwareness {
		enc.WriteVarBytes(payload)
		return enc.Bytes()
	}
	return append(enc.Bytes(), payload...)
}

func (r *room) addPeerLocked(who Identity) *Peer {
	p := &Peer{
		room:     r,
		who:      who,
		out:      make(chan []byte, r.hub.cfg.PeerQueue),
		done:     make(chan struct{}),
		ids:      map[uint64]struct{}{},
		joinedAt: r.hub.cfg.Now(),
	}
	p.src = &source{who: who, peer: p}
	r.peers[p] = struct{}{}
	r.activeAt = p.joinedAt
	p.enqueueLocked(frame(msgSync, ygsync.EncodeSyncStep1(r.doc)))
	p.enqueueLocked(frame(msgSync, syncStep2Of(r.doc)))
	p.enqueueLocked(frame(msgAwareness, r.aw.EncodeUpdate(nil)))
	return p
}

// syncStep2Of is the whole state of doc as a sync-step-2 message.
func syncStep2Of(doc *crdt.Doc) []byte {
	enc := encoding.NewEncoder()
	enc.WriteVarUint(ygsync.MsgSyncStep2)
	enc.WriteVarBytes(crdt.EncodeStateAsUpdateV1(doc, nil))
	return enc.Bytes()
}

// handleLocked reads one message from a peer.
func (r *room) handleLocked(p *Peer, msg []byte) error {
	dec := encoding.NewDecoder(msg)
	kind, err := dec.ReadVarUint()
	if err != nil {
		return fmt.Errorf("message has no type: %w", err)
	}
	switch kind {
	case msgSync:
		return r.syncLocked(p, dec.RemainingBytes())
	case msgAwareness:
		payload, err := dec.ReadVarBytes()
		if err != nil {
			return fmt.Errorf("awareness message: %w", err)
		}
		return r.awarenessLocked(p, payload)
	case msgQueryAwareness:
		p.enqueueLocked(frame(msgAwareness, r.aw.EncodeUpdate(nil)))
		return nil
	default:
		// y-websocket's auth message and anything newer: not for the server.
		return nil
	}
}

func (r *room) syncLocked(p *Peer, inner []byte) error {
	kind, payload, err := ygsync.ReadSyncMessage(inner)
	if err != nil {
		return fmt.Errorf("sync message: %w", err)
	}
	if kind == ygsync.MsgSyncStep1 {
		reply, err := ygsync.EncodeSyncStep2(r.doc, inner)
		if err != nil {
			return fmt.Errorf("sync step 1: %w", err)
		}
		p.enqueueLocked(frame(msgSync, reply))
		return nil
	}
	// A provider answers our step 1 with an update even when it has nothing to
	// add. Nothing is nothing: it is not a write, so it is neither dropped,
	// counted nor stored.
	if isEmptyUpdate(payload) {
		return nil
	}
	// Step 2 and plain updates both write.
	if p.who.ReadOnly {
		// A reader's provider still answers our step 1 with its own diff, so this is
		// not an error: it is dropped, and counted so a transport can see it.
		p.dropped++
		return nil
	}
	limits := r.hub.cfg.Limits
	if len(payload) > limits.UpdateBytes {
		return &LimitError{Name: "update size (bytes)", Value: len(payload), Max: limits.UpdateBytes}
	}
	if total := r.size + len(payload); total > r.docBytes {
		return &LimitError{Name: "document size (bytes)", Value: total, Max: r.docBytes}
	}
	if err := crdt.ApplyUpdateV1(r.doc, payload, p.src); err != nil {
		return fmt.Errorf("update does not apply: %w", err)
	}
	return nil
}

func (r *room) awarenessLocked(p *Peer, payload []byte) error {
	if limit := r.hub.cfg.Limits.AwarenessBytes; len(payload) > limit {
		return &LimitError{Name: "awareness message size (bytes)", Value: len(payload), Max: limit}
	}
	entries, err := decodeAwareness(payload)
	if err != nil {
		return err
	}
	for i, e := range entries {
		if owner, taken := r.owner[e.clientID]; taken && owner != any(p) {
			return fmt.Errorf("awareness client %d belongs to another connection", e.clientID)
		}
		if _, mine := p.ids[e.clientID]; !mine && len(p.ids) >= maxClientIDsPerPeer {
			return &LimitError{Name: "awareness identities per connection", Value: len(p.ids) + 1, Max: maxClientIDsPerPeer}
		}
		if e.isLeaving() {
			continue
		}
		state, err := withAuthenticatedUser(e.state, p.who.Username, p.who, false)
		if err != nil {
			return err
		}
		entries[i].state = state
	}
	for _, e := range entries {
		r.owner[e.clientID] = p
		p.ids[e.clientID] = struct{}{}
	}
	update := encodeAwareness(entries)
	if err := r.aw.ApplyUpdate(update, p); err != nil {
		return fmt.Errorf("awareness update: %w", err)
	}
	// The sender hears its own awareness back, as in the y-websocket reference
	// server: the provider reconnects a connection that has received nothing for
	// 30 seconds, and a quiet client alone in a room would otherwise be cut
	// between its own 15 second heartbeats.
	r.broadcastAwarenessLocked(update)
	return nil
}

func (r *room) broadcastAwarenessLocked(update []byte) {
	msg := frame(msgAwareness, update)
	for q := range r.peers {
		q.enqueueLocked(msg)
	}
}

// leaveLocked announces that the awareness entries of owner are gone.
func (r *room) leaveLocked(ids map[uint64]struct{}, owner any) {
	var gone []awarenessEntry
	for id := range ids {
		if r.owner[id] != owner {
			continue
		}
		delete(r.owner, id)
		if meta, ok := r.aw.Meta(id); ok {
			gone = append(gone, awarenessEntry{clientID: id, clock: meta.Clock + 1, state: []byte("null")})
		}
	}
	if len(gone) == 0 {
		return
	}
	update := encodeAwareness(gone)
	if err := r.aw.ApplyUpdate(update, owner); err != nil {
		r.hub.cfg.Logger.Warn("remove awareness of a connection that left", "room", r.id, "err", err)
	}
	r.broadcastAwarenessLocked(update)
}

// Present shows an actor the server speaks for, such as an agent, in the
// room's roster under label, until the returned function is called. It is
// the server publishing awareness of its own: an agent is not a connection,
// so nothing else would show that it is there.
func (h *Hub) Present(ctx context.Context, roomID, label string) (leave func(), err error) {
	var id uint64
	err = h.run(ctx, roomID, func(r *room) error {
		id = uint64(h.cfg.NewClientID())
		for _, taken := r.owner[id]; taken; _, taken = r.owner[id] {
			id++
		}
		state, err := withAuthenticatedUser([]byte("{}"), label, Identity{Label: label}, true)
		if err != nil {
			return err
		}
		update := encodeAwareness([]awarenessEntry{{clientID: id, clock: 1, state: state}})
		if err := r.aw.ApplyUpdate(update, presenceOwner); err != nil {
			return fmt.Errorf("show %q in the room: %w", label, err)
		}
		r.owner[id] = presenceOwner
		r.presences++
		r.broadcastAwarenessLocked(update)
		return nil
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			r := h.loadedRoom(roomID)
			if r == nil {
				return
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.closed.Load() {
				return
			}
			r.leaveLocked(map[uint64]struct{}{id: {}}, presenceOwner)
			r.presences--
			r.activeAt = h.cfg.Now()
		})
	}, nil
}

func (h *Hub) loadedRoom(id string) *room {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rooms[id]
}
