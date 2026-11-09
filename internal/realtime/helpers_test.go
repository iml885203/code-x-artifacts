package realtime_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reearth/ygo/awareness"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

// clock is a clock the test moves by hand.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// env is a hub over a real SQLite file.
type env struct {
	t       *testing.T
	st      *store.Store
	clock   *clock
	hub     *realtime.Hub
	cfg     realtime.Config
	mu      sync.Mutex
	updates []realtime.Update
}

func newEnv(t *testing.T, tune func(*realtime.Config)) *env {
	t.Helper()
	st := storetest.Open(t)
	e := &env{t: t, st: st, clock: &clock{t: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}}
	e.cfg = realtime.Config{
		Log: st.RealtimeLog(),
		Now: e.clock.Now,
		OnUpdate: func(u realtime.Update) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.updates = append(e.updates, u)
		},
	}
	if tune != nil {
		tune(&e.cfg)
	}
	e.start()
	return e
}

// start makes a hub over the store's log, as a restart does: whatever the
// previous hub held in memory is gone, whatever it had acknowledged is on disk.
func (e *env) start() {
	e.t.Helper()
	hub, err := realtime.New(e.cfg)
	if err != nil {
		e.t.Fatal(err)
	}
	e.hub = hub
}

func (e *env) room() string {
	e.t.Helper()
	a, err := e.st.CreateArtifact(e.t.Context(), "A document", "doc", "", "# x", "alice", store.VisibilityOrganisation)
	if err != nil {
		e.t.Fatal(err)
	}
	return a.ID
}

func (e *env) seen() []realtime.Update {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]realtime.Update(nil), e.updates...)
}

func (e *env) rows(room string) int {
	e.t.Helper()
	n, err := e.st.RealtimeRows(e.t.Context(), room)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// text reads the shared text "t" of a room.
func (e *env) text(room string) string {
	e.t.Helper()
	var s string
	err := e.hub.Read(e.t.Context(), room, func(doc *crdt.Doc) error {
		s = doc.GetText("t").ToString()
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

var (
	alice  = realtime.Identity{Username: "alice", Label: "Alice"}
	bob    = realtime.Identity{Username: "bob", Label: "Bob"}
	reader = realtime.Identity{Username: "carol", Label: "Carol", ReadOnly: true}
)

// client is an in-process Yjs client: a document that speaks y-websocket
// through a Peer, standing in for a browser.
type client struct {
	t    *testing.T
	doc  *crdt.Doc
	aw   *awareness.Awareness
	peer *realtime.Peer
	errs []error
}

func (e *env) connect(room string, who realtime.Identity) *client {
	e.t.Helper()
	return e.connectAt(room, who, "")
}

func (e *env) connectAt(room string, who realtime.Identity, epoch string) *client {
	e.t.Helper()
	peer, err := e.hub.Join(e.t.Context(), room, who, epoch)
	if err != nil {
		e.t.Fatalf("join as %s: %v", who.Username, err)
	}
	c := &client{t: e.t, doc: crdt.New(), peer: peer}
	c.aw = awareness.New(uint64(c.doc.ClientID()))
	c.doc.OnUpdate(func(update []byte, origin any) {
		if origin == "remote" {
			return
		}
		_ = c.send(frame(0, ygsync.EncodeUpdate(update))) // a refusal is kept in c.errs for the test to read
	})
	c.pump()
	return c
}

func frame(kind uint64, payload []byte) []byte {
	enc := encoding.NewEncoder()
	enc.WriteVarUint(kind)
	if kind == 1 {
		enc.WriteVarBytes(payload)
		return enc.Bytes()
	}
	return append(enc.Bytes(), payload...)
}

// send hands a message to the room and remembers what it answered.
func (c *client) send(msg []byte) error {
	err := c.peer.Receive(context.Background(), msg)
	if err != nil {
		c.errs = append(c.errs, err)
	}
	return err
}

// pump applies everything the room has queued for this client, answering the
// way a provider does, and returns how many messages there were.
func (c *client) pump() int {
	n := 0
	for {
		select {
		case msg, open := <-c.peer.Out():
			if !open {
				return n
			}
			n++
			c.handle(msg)
		default:
			return n
		}
	}
}

func (c *client) handle(msg []byte) {
	dec := encoding.NewDecoder(msg)
	kind, err := dec.ReadVarUint()
	if err != nil {
		c.t.Fatalf("a message from the room has no type: %v", err)
	}
	switch kind {
	case 0:
		reply, err := ygsync.ApplySyncMessage(c.doc, dec.RemainingBytes(), "remote")
		if err != nil {
			c.t.Fatalf("apply a sync message from the room: %v", err)
		}
		if reply != nil {
			_ = c.send(frame(0, reply)) // a refusal is kept in c.errs for the test to read
		}
	case 1:
		payload, err := dec.ReadVarBytes()
		if err != nil {
			c.t.Fatalf("awareness message from the room: %v", err)
		}
		if err := c.aw.ApplyUpdate(payload, "remote"); err != nil {
			c.t.Fatalf("apply awareness from the room: %v", err)
		}
	default:
		c.t.Fatalf("unexpected message type %d from the room", kind)
	}
}

// text is the client's own copy of the shared text.
func (c *client) text() string { return c.doc.GetText("t").ToString() }

// typeAt edits the client's local copy, which sends the change to the room.
func (c *client) typeAt(index int, s string) {
	c.t.Helper()
	txt := c.doc.GetText("t")
	c.doc.Transact(func(txn *crdt.Transaction) { txt.Insert(txn, index, s, nil) })
}

// settle delivers messages between the clients and the room until none are left.
func settle(t *testing.T, cs ...*client) {
	t.Helper()
	for range 100 {
		moved := 0
		for _, c := range cs {
			moved += c.pump()
		}
		if moved == 0 {
			return
		}
	}
	t.Fatal("the clients and the room kept exchanging messages")
}

// names lists the names the client sees in the room's roster.
func (c *client) names() map[string]bool {
	out := map[string]bool{}
	for _, st := range c.aw.GetStates() {
		if st.State == nil {
			continue
		}
		if user, ok := st.State["user"].(map[string]any); ok {
			if name, ok := user["name"].(string); ok {
				out[name] = true
			}
		}
	}
	return out
}

// limitOf unwraps the limit an error names.
func limitOf(t *testing.T, err error) *realtime.LimitError {
	t.Helper()
	var le *realtime.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("error = %v, want a *LimitError", err)
	}
	return le
}

// flakyLog fails every append while fail is set.
type flakyLog struct {
	realtime.Log
	fail atomic.Bool
}

func (l *flakyLog) Append(ctx context.Context, room, epoch string, update []byte) error {
	if l.fail.Load() {
		return errors.New("disk is full")
	}
	return l.Log.Append(ctx, room, epoch, update)
}
