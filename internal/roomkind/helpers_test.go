package roomkind_test

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"

	"github.com/reearth/ygo/awareness"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

// clock is a clock the test moves by hand; the hub and the snapshotter share it.
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

var (
	alice = realtime.Identity{Username: "alice", Label: "Alice"}
	bob   = realtime.Identity{Username: "bob", Label: "Bob"}
	carol = realtime.Identity{Username: "carol", Label: "Carol's agent"}
	dave  = realtime.Identity{Username: "dave", Label: "Dave", ReadOnly: true}
)

// env is the server's live side over a real SQLite file: a hub whose rooms start
// from the latest version, a snapshotter, and the service that saves versions.
type env struct {
	t     *testing.T
	st    *store.Store
	svc   *service.Service
	clock *clock
	hub   *realtime.Hub
	snap  *roomkind.Snapshotter
	logs  *logRecorder
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st := storetest.Open(t)
	disk, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		t:     t,
		st:    st,
		svc:   service.New(st, disk, false),
		clock: &clock{t: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)},
		logs:  &logRecorder{},
	}
	e.start()
	return e
}

// start makes the live side as a restart does: whatever the previous hub and
// snapshotter held in memory is gone, whatever was on disk is still there.
func (e *env) start() {
	e.t.Helper()
	codecs := roomkind.DefaultCodecs()
	e.snap = roomkind.NewSnapshotter(roomkind.Config{Store: e.st, Versions: e.svc, Files: e.svc, Codecs: codecs, Now: e.clock.Now, Logger: slog.New(e.logs)})
	hub, err := realtime.New(realtime.Config{
		ReceiveGuard: e.svc.ArtifactGuard().Shared,
		Log:          roomkind.SeededLog(e.st.RealtimeLog(), e.st, e.svc, codecs, 0),
		Now:          e.clock.Now,
		OnUpdate:     e.snap.Updated,
		Validate:     codecs.Check,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.snap.Use(hub)
	e.svc.SetLiveRooms(e.snap)
	e.hub = hub
}

// restart is a server restart: the hub closes, writing what it holds, and a new
// one starts over the same database.
func (e *env) restart() {
	e.t.Helper()
	e.hub.Close(e.t.Context())
	e.start()
}

// doc publishes a doc, as alice, and returns its id.
func (e *env) doc() string {
	e.t.Helper()
	p, err := e.svc.Create(e.t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Doc", Kind: "doc", Content: "# Plan\n\nfirst paragraph\n"})
	if err != nil {
		e.t.Fatal(err)
	}
	return p.ID
}

// sweep runs the snapshotter's sweep.
func (e *env) sweep() { e.snap.Sweep(e.t.Context()) }

func (e *env) versions(id string) []store.Version {
	e.t.Helper()
	list, err := e.st.Versions(e.t.Context(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return list
}

// roomText is the text of every paragraph in the room, in order.
func (e *env) roomText(id string) []string {
	e.t.Helper()
	var out []string
	err := e.hub.Read(e.t.Context(), id, func(doc *crdt.Doc) error {
		d, err := docroom.Open(doc).Read()
		if err != nil {
			return err
		}
		out = paragraphs(d)
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

// edit appends a paragraph to the room on behalf of who, the way the server
// writes for an agent (Hub.Edit).
func (e *env) edit(id string, who realtime.Identity, text string) {
	e.t.Helper()
	err := e.hub.Edit(e.t.Context(), id, who, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return appendParagraph(doc, nextID(), text), nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

var paragraphCount atomic.Int64

// nextID is an id for a paragraph the test adds. It must not look like an id the
// fixtures use ("p1", "p2"): a duplicate id makes the room's content invalid, and a
// read then quietly falls back to the saved version.
func nextID() string { return "added-" + strconv.FormatInt(paragraphCount.Add(1), 10) }

// appendParagraph is the write of a paragraph at the end of the first tab. It
// looks nothing up inside the transaction.
func appendParagraph(doc *crdt.Doc, id, text string) func(*crdt.Transaction) {
	tab := firstTab(doc)
	para := crdt.NewYXmlElement("paragraph")
	run := crdt.NewYXmlText()
	return func(txn *crdt.Transaction) {
		tab.InsertElement(txn, tab.Len(), para)
		para.SetAttribute(txn, "id", id)
		para.InsertText(txn, 0, run)
		run.Insert(txn, 0, text, nil)
	}
}

// client is an in-process editor: a document that speaks y-websocket through a
// Peer, standing in for a browser.
type client struct {
	t    *testing.T
	doc  *crdt.Doc
	aw   *awareness.Awareness
	peer *realtime.Peer
}

func (e *env) connect(id string, who realtime.Identity) *client {
	e.t.Helper()
	peer, err := e.hub.Join(e.t.Context(), id, who, "")
	if err != nil {
		e.t.Fatalf("join as %s: %v", who.Username, err)
	}
	c := &client{t: e.t, doc: crdt.New(), peer: peer}
	c.aw = awareness.New(uint64(c.doc.ClientID()))
	c.doc.OnUpdate(func(update []byte, origin any) {
		if origin == "remote" {
			return
		}
		_ = c.peer.Receive(context.Background(), frame(0, ygsync.EncodeUpdate(update))) // a dropped write of a reader is not an error
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

// pump applies what the room has queued for the client.
func (c *client) pump() {
	for {
		select {
		case msg, open := <-c.peer.Out():
			if !open {
				return
			}
			c.handle(msg)
		default:
			return
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
			c.t.Fatalf("apply a sync message: %v", err)
		}
		if reply != nil {
			_ = c.peer.Receive(context.Background(), frame(0, reply)) // a dropped write of a reader is not an error
		}
	case 1:
		payload, err := dec.ReadVarBytes()
		if err != nil {
			c.t.Fatal(err)
		}
		if err := c.aw.ApplyUpdate(payload, "remote"); err != nil {
			c.t.Fatal(err)
		}
	}
}

// type adds a paragraph to the client's copy, which sends it to the room.
func (c *client) typeParagraph(text string) {
	c.t.Helper()
	c.pump()
	c.doc.Transact(appendParagraph(c.doc, nextID(), text))
	c.pump()
}

// callThemselves announces a name in the roster that is not the connection's.
func (c *client) callThemselves(name string) {
	c.t.Helper()
	c.aw.SetLocalState(map[string]any{"user": map[string]any{"name": name}})
	if err := c.peer.Receive(context.Background(), frame(1, c.aw.EncodeUpdate(nil))); err != nil {
		c.t.Fatal(err)
	}
}

// texts is the paragraph text of the client's copy.
func (c *client) texts() []string {
	c.t.Helper()
	c.pump()
	d, err := docroom.Open(c.doc).Read()
	if err != nil {
		c.t.Fatal(err)
	}
	return paragraphs(d)
}

// writeLink replaces the room's content with a doc holding one paragraph that is
// a link to href, on behalf of who.
func (e *env) writeLink(id string, who realtime.Identity, href string) {
	e.t.Helper()
	d := docfmt.Doc{Tabs: []docfmt.Tab{{ID: "t1", Title: "T", Blocks: []docfmt.Block{
		{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "link", Marks: []string{docfmt.MarkLink}, Href: href}}},
	}}}}
	err := e.hub.Edit(e.t.Context(), id, who, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return docroom.Open(doc).Write(d), nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// publishInput is a Markdown update of the doc on top of the given version.
func publishInput(markdown string, base int) service.PublishInput {
	return service.PublishInput{Title: "Doc", Content: markdown, BaseVersion: base}
}

// logRecorder counts what the snapshotter logs, by level.
type logRecorder struct {
	mu            sync.Mutex
	errors, tries int
}

func (l *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (l *logRecorder) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *logRecorder) WithGroup(string) slog.Handler            { return l }
func (l *logRecorder) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.Message == "save a version of a room" {
		l.tries++
		if r.Level >= slog.LevelError {
			l.errors++
		}
	}
	return nil
}

func (l *logRecorder) counts() (tries, errors int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tries, l.errors
}

// agents lists the agents the client sees in the roster: name to block.
func (c *client) agents() map[string]string {
	c.t.Helper()
	c.pump()
	out := map[string]string{}
	for _, st := range c.aw.GetStates() {
		user, _ := st.State["user"].(map[string]any)
		if user["agent"] == true {
			block, _ := st.State["block"].(string)
			out[user["name"].(string)] = block
		}
	}
	return out
}

// firstTab is the element of the first tab of a document, for a test that types into it as a person would.
func firstTab(doc *crdt.Doc) *crdt.YXmlElement {
	tab, ok := docroom.Open(doc).FirstTabElement()
	if !ok {
		panic("the document has no tab")
	}
	return tab
}
