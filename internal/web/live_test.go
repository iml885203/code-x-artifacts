package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"
	ygsync "github.com/reearth/ygo/sync"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

// fakeClock is the server's time, moved by the test. Timers fire when the
// clock is advanced past them; waitTimers blocks until the server has armed
// that many, so a test never advances the clock before the server is waiting on it.
type fakeClock struct {
	mu     sync.Mutex
	cond   *sync.Cond
	now    time.Time
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	c := &fakeClock{now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, fakeTimer{at: c.now.Add(d), ch: ch})
	c.cond.Broadcast()
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.timers[:0]
	for _, t := range c.timers {
		if t.at.After(c.now) {
			kept = append(kept, t)
			continue
		}
		t.ch <- c.now
	}
	c.timers = kept
	c.cond.Broadcast()
}

// waitTimers returns once at least n timers are waiting to fire.
func (c *fakeClock) waitTimers(t *testing.T, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.cond.Broadcast()
	})
	defer stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.timers) < n {
		if ctx.Err() != nil {
			t.Fatalf("the server armed %d timer(s), the test waited for %d", len(c.timers), n)
		}
		c.cond.Wait()
	}
}

// liveHarness is a server whose clock the test holds, with a handle on the
// Server for what only it can say (the room's content, the heartbeat count).
type liveHarness struct {
	*harness
	app   *Server
	clock *fakeClock
}

func newLiveHarness(t *testing.T, options ...func(*Config)) *liveHarness {
	t.Helper()
	st := storetest.Open(t)
	files, err := assets.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := newFakeDirectory()
	clock := newFakeClock()
	cfg := Config{
		Store: st, Assets: files, Authenticator: dir, SessionKey: []byte("test-key"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Clock: clock,
	}
	for _, option := range options {
		option(&cfg)
	}
	app := New(cfg)
	srv := httptest.NewTLSServer(app.Handler())
	t.Cleanup(srv.Close)
	return &liveHarness{harness: &harness{t: t, srv: srv, st: st, dir: dir}, app: app, clock: clock}
}

type ticketReply struct {
	Ticket           string `json:"ticket"`
	Epoch            string `json:"epoch"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
	Path             string `json:"path"`
}

// ticket asks for a socket ticket the way the page does: the viewer's cookie and the bridge header.
func (h *liveHarness) ticket(c *http.Client, id string) (response, string) {
	h.t.Helper()
	return h.do(c, "POST", "/api/artifacts/"+id+"/live", "", bridgeHeaders(h.harness))
}

func (h *liveHarness) mustTicket(c *http.Client, id string) ticketReply {
	h.t.Helper()
	res, body := h.ticket(c, id)
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("ticket for %s: %d %s", id, res.StatusCode, body)
	}
	return decode[ticketReply](h.t, body)
}

// dial opens the socket with the headers a page's own socket would carry,
// unless the test replaces them.
func (h *liveHarness) dial(c *http.Client, room, ticket, query string, header http.Header) (conn *websocket.Conn, status int, err error) {
	h.t.Helper()
	if header == nil {
		header = http.Header{"Origin": {h.srv.URL}}
	}
	url := h.srv.URL + "/ws/" + room + "?ticket=" + ticket + query
	conn, res, err := websocket.Dial(h.t.Context(), url, &websocket.DialOptions{HTTPClient: c, HTTPHeader: header})
	if res != nil {
		status = res.StatusCode
		if res.Body != nil {
			_ = res.Body.Close()
		}
	}
	return conn, status, err
}

// liveClient is a Yjs client over a real WebSocket: a document that speaks
// y-websocket, with its own reader.
type liveClient struct {
	t     *testing.T
	conn  *websocket.Conn
	mu    sync.Mutex
	doc   *crdt.Doc
	step2 int // sync step 2 messages received
	// closed is closed when the connection ended; code and reason say how.
	closed chan struct{}
	code   websocket.StatusCode
	reason string
	// changed is signalled for every message and for the close.
	changed chan struct{}
}

func (h *liveHarness) connect(c *http.Client, room string) *liveClient {
	h.t.Helper()
	return h.connectWith(c, room, "")
}

func (h *liveHarness) connectWith(c *http.Client, room, query string) *liveClient {
	h.t.Helper()
	tk := h.mustTicket(c, room)
	conn, status, err := h.dial(c, room, tk.Ticket, query, nil)
	if err != nil {
		h.t.Fatalf("open the socket of %s: %v (status %d)", room, err, status)
	}
	return h.attach(conn)
}

func (h *liveHarness) attach(conn *websocket.Conn) *liveClient {
	h.t.Helper()
	lc := &liveClient{t: h.t, conn: conn, doc: crdt.New(), closed: make(chan struct{}), changed: make(chan struct{}, 1)}
	lc.doc.OnUpdate(func(update []byte, origin any) {
		if origin == "remote" {
			return
		}
		lc.write(append([]byte{0}, ygsync.EncodeUpdate(update)...))
	})
	go lc.read()
	h.t.Cleanup(func() { _ = conn.CloseNow() })
	return lc
}

func (lc *liveClient) write(msg []byte) {
	// The connection may be closed by the server already: that is what some tests check.
	_ = lc.conn.Write(lc.t.Context(), websocket.MessageBinary, msg)
}

func (lc *liveClient) read() {
	defer close(lc.closed)
	for {
		_, msg, err := lc.conn.Read(lc.t.Context())
		if err != nil {
			lc.mu.Lock()
			lc.code = websocket.CloseStatus(err)
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				lc.reason = ce.Reason
			}
			lc.mu.Unlock()
			lc.signal()
			return
		}
		dec := encoding.NewDecoder(msg)
		kind, err := dec.ReadVarUint()
		if err != nil || kind != 0 {
			continue // awareness: not what these tests look at
		}
		inner := dec.RemainingBytes()
		lc.mu.Lock()
		if t, _ := dec.ReadVarUint(); t == ygsync.MsgSyncStep2 {
			lc.step2++
		}
		reply, err := ygsync.ApplySyncMessage(lc.doc, inner, "remote")
		lc.mu.Unlock()
		if err != nil {
			lc.t.Errorf("apply a message from the server: %v", err)
		}
		if reply != nil {
			lc.write(append([]byte{0}, reply...))
		}
		lc.signal()
	}
}

func (lc *liveClient) signal() {
	select {
	case lc.changed <- struct{}{}:
	default:
	}
}

// waitFor blocks until cond holds (checked on every message and on close).
func (lc *liveClient) waitFor(what string, cond func() bool) {
	lc.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		lc.mu.Lock()
		ok := cond()
		lc.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-lc.changed:
		case <-lc.closed:
			lc.mu.Lock()
			ok = cond()
			lc.mu.Unlock()
			if ok {
				return
			}
			lc.t.Fatalf("the connection ended (code %d %q) while waiting for %s", lc.code, lc.reason, what)
		case <-deadline:
			lc.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// synced waits for the room's first full state: the handshake is complete.
func (lc *liveClient) synced() {
	lc.t.Helper()
	lc.waitFor("the first state from the room", func() bool { return lc.step2 > 0 })
}

func (lc *liveClient) text() string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.doc.GetText("t").ToString()
}

func (lc *liveClient) typeText(s string) {
	txt := lc.doc.GetText("t")
	lc.doc.Transact(func(txn *crdt.Transaction) { txt.Insert(txn, txt.Len(), s, nil) })
}

// barrier returns once the server has handled everything this client sent
// before it: messages on one connection are handled in order, and the answer
// to a state-vector request comes after them.
func (lc *liveClient) barrier() {
	lc.t.Helper()
	lc.mu.Lock()
	before := lc.step2
	lc.mu.Unlock()
	lc.write(append([]byte{0}, ygsync.EncodeSyncStep1(crdt.New())...))
	lc.waitFor("the server's answer", func() bool { return lc.step2 > before })
}

// ended waits for the server to close the connection and says how.
func (lc *liveClient) ended() (websocket.StatusCode, string) {
	lc.t.Helper()
	select {
	case <-lc.closed:
	case <-time.After(10 * time.Second):
		lc.t.Fatal("the connection did not end")
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.code, lc.reason
}

func (lc *liveClient) open() bool {
	select {
	case <-lc.closed:
		return false
	default:
		return true
	}
}

func (h *liveHarness) roomText(id string) string {
	h.t.Helper()
	var s string
	err := h.app.live.hub.Read(h.t.Context(), id, func(doc *crdt.Doc) error {
		s = doc.GetText("t").ToString()
		return nil
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

// liveWorld is the people and artifacts the live tests share: alice authors,
// bob is named an editor, carol is only in the organisation.
type liveWorld struct {
	h                                *liveHarness
	alice, bob, carol                *http.Client
	aliceTok                         string
	private, organisation, orgEditor artifactView
}

func newLiveWorld(t *testing.T, options ...func(*Config)) *liveWorld {
	t.Helper()
	h := newLiveHarness(t, options...)
	w := &liveWorld{h: h, alice: h.browser("alice", "pw-a"), bob: h.browser("bob", "pw-b"), carol: h.browser("carol", "pw-c"), aliceTok: h.token("alice")}
	w.private = h.publishAs(w.aliceTok, "Private", "<p>x", store.VisibilityPrivate)
	w.organisation = h.publishAs(w.aliceTok, "Organisation", "<p>x", store.VisibilityOrganisation)
	w.orgEditor = h.publishAs(w.aliceTok, "Everyone edits", "<p>x", store.VisibilityOrganisation)
	for id, body := range map[string]string{
		w.private.ID:      `{"add":["bob"]}`,
		w.organisation.ID: `{"add":["bob"]}`,
		w.orgEditor.ID:    `{"organisationEdit":true}`,
	} {
		if res, out := patchEditors(h.harness, w.aliceTok, id, body); res.StatusCode != http.StatusOK {
			t.Fatalf("share %s: %d %s", id, res.StatusCode, out)
		}
	}
	return w
}

func TestTheSocketRefusesCrossOriginNullAndMissingOriginsAndThatIsNotWhatSpendsTheTicket(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	tk := h.mustTicket(w.alice, w.organisation.ID)

	for name, header := range map[string]http.Header{
		"another site":              {"Origin": {"https://evil.example"}},
		"a sandboxed page (null)":   {"Origin": {"null"}},
		"the same host, wrong port": {"Origin": {"https://127.0.0.1:1"}},
		"no Origin at all":          {},
	} {
		_, status, err := h.dial(w.alice, w.organisation.ID, tk.Ticket, "", header)
		if err == nil || status != http.StatusForbidden {
			t.Errorf("%s: err %v, status %d; want the upgrade refused with 403", name, err, status)
		}
	}
	// The ticket was never presented to the point of being spent: the same one opens the socket from the site's own page.
	conn, _, err := h.dial(w.alice, w.organisation.ID, tk.Ticket, "", nil)
	if err != nil {
		t.Fatalf("the same origin, with the ticket the refused attempts left unspent: %v", err)
	}
	h.attach(conn).synced()
}

func TestATicketWorksOnceOnlyForItsArtifactAndNotAfterThirtySeconds(t *testing.T) {
	t.Parallel()
	// Each case has a world of its own: one of them moves the clock.
	dialStatus := func(w *liveWorld, room, ticket string) int {
		t.Helper()
		conn, status, err := w.h.dial(w.alice, room, ticket, "", nil)
		if err == nil {
			w.h.attach(conn)
			return http.StatusSwitchingProtocols
		}
		return status
	}

	t.Run("once", func(t *testing.T) {
		t.Parallel()
		w := newLiveWorld(t)
		tk := w.h.mustTicket(w.alice, w.organisation.ID)
		if tk.ExpiresInSeconds != 30 || tk.Epoch == "" || tk.Path != "/ws/"+w.organisation.ID {
			t.Errorf("reply = %+v", tk)
		}
		if got := dialStatus(w, w.organisation.ID, tk.Ticket); got != http.StatusSwitchingProtocols {
			t.Fatalf("first use: %d", got)
		}
		if got := dialStatus(w, w.organisation.ID, tk.Ticket); got != http.StatusUnauthorized {
			t.Errorf("second use: %d, want 401", got)
		}
	})
	t.Run("only for its artifact", func(t *testing.T) {
		t.Parallel()
		w := newLiveWorld(t)
		tk := w.h.mustTicket(w.alice, w.organisation.ID)
		if got := dialStatus(w, w.orgEditor.ID, tk.Ticket); got != http.StatusUnauthorized {
			t.Errorf("for another artifact: %d, want 401", got)
		}
		if got := dialStatus(w, w.organisation.ID, tk.Ticket); got != http.StatusUnauthorized {
			t.Errorf("a ticket presented for the wrong artifact is spent: %d, want 401", got)
		}
	})
	t.Run("not after thirty seconds", func(t *testing.T) {
		t.Parallel()
		w := newLiveWorld(t)
		early, late := w.h.mustTicket(w.alice, w.organisation.ID), w.h.mustTicket(w.alice, w.organisation.ID)
		w.h.clock.Advance(29 * time.Second)
		if got := dialStatus(w, w.organisation.ID, early.Ticket); got != http.StatusSwitchingProtocols {
			t.Errorf("after 29 seconds: %d, want it to open", got)
		}
		w.h.clock.Advance(time.Second)
		if got := dialStatus(w, w.organisation.ID, late.Ticket); got != http.StatusUnauthorized {
			t.Errorf("after 30 seconds: %d, want 401", got)
		}
	})
	t.Run("a made-up ticket or none", func(t *testing.T) {
		t.Parallel()
		w := newLiveWorld(t)
		for _, tk := range []string{"", "made-up"} {
			if got := dialStatus(w, w.organisation.ID, tk); got != http.StatusUnauthorized {
				t.Errorf("ticket %q: %d, want 401", tk, got)
			}
		}
	})
}

// probe tries to write through a client and says whether the room took it.
func probe(h *liveHarness, lc *liveClient, room string) bool {
	h.t.Helper()
	lc.synced()
	before := h.roomText(room)
	lc.typeText("!")
	lc.barrier()
	return h.roomText(room) != before
}

func TestEachPersonGetsTheRightToWriteToReceiveOnlyOrTheAnswerOfAMissingArtifact(t *testing.T) {
	t.Parallel()
	type want int
	const (
		writes want = iota
		receives
		missing
		signIn
	)
	// who picks the person and the artifact out of a world.
	cases := []struct {
		name string
		who  func(w *liveWorld) (*http.Client, artifactView)
		want want
	}{
		{"the author", func(w *liveWorld) (*http.Client, artifactView) { return w.alice, w.organisation }, writes},
		{"a named editor", func(w *liveWorld) (*http.Client, artifactView) { return w.bob, w.organisation }, writes},
		{"someone the organisation lets edit", func(w *liveWorld) (*http.Client, artifactView) { return w.carol, w.orgEditor }, writes},
		{"a reader", func(w *liveWorld) (*http.Client, artifactView) { return w.carol, w.organisation }, receives},
		{"a stranger to a private artifact", func(w *liveWorld) (*http.Client, artifactView) { return w.carol, w.private }, missing},
		{"an anonymous visitor", func(w *liveWorld) (*http.Client, artifactView) { return w.h.client(nil), w.organisation }, signIn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newLiveWorld(t)
			h := w.h
			client, artifact := c.who(w)
			// Someone is in the room already, so that a reader's receiving can be seen.
			watcher := h.connect(w.alice, artifact.ID)
			watcher.synced()
			seen := watcher.text()
			res, body := h.ticket(client, artifact.ID)
			switch c.want {
			case signIn, missing:
				status := map[want]int{signIn: http.StatusUnauthorized, missing: http.StatusNotFound}[c.want]
				res2, body2 := h.ticket(client, "no-such-artifact")
				if res.StatusCode != status || res2.StatusCode != res.StatusCode || body != body2 {
					t.Fatalf("%s got %d %q, a missing artifact gets %d %q; want %d and matching bodies", c.name, res.StatusCode, body, res2.StatusCode, body2, status)
				}
				return
			case writes, receives:
			}
			if res.StatusCode != http.StatusOK {
				t.Fatalf("ticket: %d %s", res.StatusCode, body)
			}
			conn, _, err := h.dial(client, artifact.ID, decode[ticketReply](t, body).Ticket, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			participant := h.attach(conn)
			took := probe(h, participant, artifact.ID)
			if (c.want == writes) != took {
				t.Errorf("%s: the room took the write = %v", c.name, took)
			}
			// What the room queued for the watcher before this round trip has reached it by the end of it.
			watcher.barrier()
			if got := watcher.text(); (c.want == writes) != (got != seen) {
				t.Errorf("the watcher's text went from %q to %q after %s tried to write", seen, got, c.name)
			}
			if c.want == receives {
				const update = "editor's subsequent update"
				watcher.typeText(update)
				watcher.barrier()
				// A reader-initiated sync could hide a missing broadcast.
				participant.waitFor("the editor's subsequent update", func() bool {
					return strings.Contains(participant.doc.GetText("t").ToString(), update)
				})
			}
		})
	}
}

func TestAStrangerIsRefusedAtTheSocketToo(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	// Alice's ticket for the private artifact, spent by carol: the ticket names a person, not whoever holds it.
	tk := h.mustTicket(w.alice, w.private.ID)
	conn, _, err := h.dial(w.carol, w.private.ID, tk.Ticket, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// It opens, as alice: the ticket is alice's. What carol cannot do is get one.
	h.attach(conn).synced()
	if res, _ := h.ticket(w.carol, w.private.ID); res.StatusCode != http.StatusNotFound {
		t.Errorf("carol's own ticket for a private artifact: %d, want 404", res.StatusCode)
	}

	// After the audience changed, a ticket made earlier does not let its person in.
	tk = h.mustTicket(w.bob, w.private.ID)
	if res, out := api(h.harness, "PATCH", "/api/artifacts/"+w.private.ID+"/editors", `{"remove":["bob"]}`, w.aliceTok); res.StatusCode != http.StatusOK {
		t.Fatalf("remove bob: %d %s", res.StatusCode, out)
	}
	if _, status, err := h.dial(w.bob, w.private.ID, tk.Ticket, "", nil); err == nil || status != http.StatusNotFound {
		t.Errorf("a ticket of a person who lost access: err %v, status %d, want 404", err, status)
	}
}

func TestRemovingAnEditorClosesTheirLiveConnectionAndTheirNextWriteIsRefused(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.organisation.ID
	bob, alice := h.connect(w.bob, room), h.connect(w.alice, room)
	if !probe(h, bob, room) {
		t.Fatal("bob, a named editor, could not write")
	}

	if res, out := patchEditors(h.harness, w.aliceTok, room, `{"remove":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("remove bob: %d %s", res.StatusCode, out)
	}
	// The connection is closed before the request that removed him returned.
	if code, reason := bob.ended(); code != 4403 || !strings.Contains(reason, "reconnect") {
		t.Errorf("bob's connection ended with %d %q, want 4403", code, reason)
	}
	if !alice.open() {
		t.Error("alice's connection was closed by a change that is not about her")
	}
	// Whatever he still sends on the closed connection goes nowhere.
	was := h.roomText(room)
	bob.typeText("too late")
	alice.barrier()
	if got := h.roomText(room); got != was {
		t.Errorf("a write after the removal reached the room: %q", got)
	}

	// He can come back, but as a reader: the artifact is still the organisation's to read.
	before := h.roomText(room)
	again := h.connect(w.bob, room)
	if probe(h, again, room) {
		t.Error("after losing the right to edit, bob's write reached the room")
	}
	if got := h.roomText(room); got != before {
		t.Errorf("the room changed from %q to %q", before, got)
	}
}

func TestSwitchingOrganisationEditingOffClosesTheWritersItNoLongerCoversAndLeavesTheNamedOnes(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.orgEditor.ID
	if res, out := patchEditors(h.harness, w.aliceTok, room, `{"add":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("name bob: %d %s", res.StatusCode, out)
	}
	carol, bob := h.connect(w.carol, room), h.connect(w.bob, room)
	if !probe(h, carol, room) || !probe(h, bob, room) {
		t.Fatal("both should be able to write while the organisation edits")
	}
	if res, out := patchEditors(h.harness, w.aliceTok, room, `{"organisationEdit":false}`); res.StatusCode != http.StatusOK {
		t.Fatalf("switch off: %d %s", res.StatusCode, out)
	}
	if code, _ := carol.ended(); code != 4403 {
		t.Errorf("carol, who edited as part of the organisation, was closed with %d", code)
	}
	if !bob.open() {
		t.Error("bob is a named editor and keeps writing")
	}
}

func TestChangingTheAudienceClosesTheConnectionsOfPeopleWhoCanNoLongerSeeTheArtifact(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.organisation.ID
	alice, bob, carol := h.connect(w.alice, room), h.connect(w.bob, room), h.connect(w.carol, room)
	for _, c := range []*liveClient{alice, bob, carol} {
		c.synced()
	}

	if res, out := api(h.harness, "PATCH", "/api/artifacts/"+room, `{"visibility":"private"}`, w.aliceTok); res.StatusCode != http.StatusOK {
		t.Fatalf("make it private: %d %s", res.StatusCode, out)
	}
	if code, _ := carol.ended(); code != 4403 {
		t.Errorf("carol, who can no longer see it: %d", code)
	}
	// Bob is a named editor: a named editor always sees the artifact, so he stays.
	if !bob.open() || !alice.open() {
		t.Error("people who still see the artifact were disconnected")
	}
	if res, _ := h.ticket(w.carol, room); res.StatusCode != http.StatusNotFound {
		t.Errorf("carol's ticket after it went private: %d, want 404", res.StatusCode)
	}
}

func TestDeletingTheArtifactClosesEveryConnectionToIt(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.organisation.ID
	bob, carol := h.connect(w.bob, room), h.connect(w.carol, room)
	bob.synced()
	carol.synced()
	if res, out := api(h.harness, "DELETE", "/api/artifacts/"+room, "", w.aliceTok); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", res.StatusCode, out)
	}
	for who, c := range map[string]*liveClient{"bob": bob, "carol": carol} {
		if code, _ := c.ended(); code != 4403 {
			t.Errorf("%s: closed with %d", who, code)
		}
	}
	if res, _ := h.ticket(w.bob, room); res.StatusCode != http.StatusNotFound {
		t.Errorf("ticket for a deleted artifact: %d", res.StatusCode)
	}
}

func TestAConnectionPastItsLifetimeIsClosedAndMustReauthorise(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.organisation.ID
	lc := h.connect(w.bob, room)
	lc.synced()
	h.clock.waitTimers(t, 2) // the lifetime and the heartbeat

	h.clock.Advance(59 * time.Minute)
	if !lc.open() {
		t.Fatal("closed before its lifetime")
	}
	h.clock.Advance(time.Minute)
	if code, reason := lc.ended(); code != 4408 || !strings.Contains(reason, "lifetime") {
		t.Errorf("ended with %d %q, want 4408", code, reason)
	}

	// Reconnecting needs a new ticket, and the rights are decided again.
	if _, status, err := h.dial(w.bob, room, "", "", nil); err == nil || status != http.StatusUnauthorized {
		t.Errorf("reconnecting without a ticket: %v (status %d)", err, status)
	}
	if res, out := patchEditors(h.harness, w.aliceTok, room, `{"remove":["bob"]}`); res.StatusCode != http.StatusOK {
		t.Fatalf("remove bob: %d %s", res.StatusCode, out)
	}
	again := h.connect(w.bob, room)
	if probe(h, again, room) {
		t.Error("the reconnection kept the right to write that was removed")
	}
}

func TestAQuietConnectionIsPingedSoTheIngressDoesNotCutIt(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	lc := h.connect(w.carol, w.organisation.ID)
	lc.synced()
	h.clock.waitTimers(t, 2)
	for i := range 3 {
		h.clock.Advance(25 * time.Second)
		// The heartbeat is re-armed once the ping was answered.
		h.clock.waitTimers(t, 2)
		if got := h.app.live.pings.Load(); got != int64(i+1) {
			t.Fatalf("after %d heartbeat(s) the server has pinged %d time(s)", i+1, got)
		}
	}
	if !lc.open() {
		t.Error("a connection that answers its pings was closed")
	}
}

func TestAClientFromBeforeARebuildIsToldTheNewEpochAndStartsOver(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	room := w.organisation.ID
	tk := h.mustTicket(w.bob, room)
	lc := h.connectWith(w.bob, room, "&epoch="+tk.Epoch)
	lc.synced()

	err := h.app.live.hub.Replace(t.Context(), room, func(doc *crdt.Doc) error {
		txt := doc.GetText("t")
		doc.Transact(func(txn *crdt.Transaction) { txt.Insert(txn, 0, "rebuilt", nil) })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	code, current := lc.ended()
	if code != 4001 || current == "" || current == tk.Epoch {
		t.Fatalf("the open connection ended with %d %q, want 4001 and the new epoch", code, current)
	}
	// Coming back with the old epoch is told the same; the new epoch gets the new text.
	stale := h.connectWith(w.bob, room, "&epoch="+tk.Epoch)
	if code, reason := stale.ended(); code != 4001 || reason != current {
		t.Errorf("a client with the old epoch: %d %q, want 4001 %q", code, reason, current)
	}
	fresh := h.connectWith(w.bob, room, "&epoch="+current)
	fresh.waitFor("the rebuilt text", func() bool { return fresh.doc.GetText("t").ToString() == "rebuilt" })
}

func TestAnUpdateOverTheLimitClosesTheSocketNamingTheLimit(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t, func(c *Config) { c.Realtime.UpdateBytes = 100 })
	h := w.h
	room := w.organisation.ID
	lc := h.connect(w.bob, room)
	lc.synced()
	lc.typeText(strings.Repeat("x", 300))
	code, reason := lc.ended()
	if code != 4002 || !strings.Contains(reason, "update size (bytes)") || !strings.Contains(reason, "100") {
		t.Errorf("ended with %d %q, want 4002 naming the limit", code, reason)
	}
	if got := h.roomText(room); got != "" {
		t.Errorf("the refused update reached the room: %q", got)
	}
}

func TestTheServerClosesEveryConnectionWhenItStops(t *testing.T) {
	t.Parallel()
	w := newLiveWorld(t)
	h := w.h
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		h.app.Run(ctx)
		close(done)
	}()
	lc := h.connect(w.bob, w.organisation.ID)
	lc.synced()
	stop()
	if code, _ := lc.ended(); code != websocket.StatusGoingAway {
		t.Errorf("ended with %d, want going away", code)
	}
	<-done
}
