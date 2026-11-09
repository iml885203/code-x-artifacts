package realtime_test

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/reearth/ygo/crdt"
	"github.com/reearth/ygo/encoding"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

func TestTwoClientsConvergeAfterConcurrentEdits(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "start. ")
	settle(t, a, b)

	// Both type at the same place before either has seen the other's edit.
	a.typeAt(0, "A says hi. ")
	b.typeAt(0, "B says hi. ")
	b.typeAt(b.doc.GetText("t").Len(), "end B.")
	settle(t, a, b)

	if a.text() != b.text() {
		t.Fatalf("clients diverged:\n a: %q\n b: %q", a.text(), b.text())
	}
	for _, want := range []string{"A says hi. ", "B says hi. ", "start. ", "end B."} {
		if !strings.Contains(a.text(), want) {
			t.Errorf("text %q lost %q", a.text(), want)
		}
	}
	if got := e.text(room); got != a.text() {
		t.Errorf("the room holds %q, the clients hold %q", got, a.text())
	}
}

func TestAnAcknowledgedUpdateIsStillThereWhenTheRoomIsReopened(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "kept")
	a.typeAt(4, " and kept")
	if len(a.errs) != 0 {
		t.Fatalf("writes were refused: %v", a.errs)
	}

	// The hub is replaced without being closed: no compaction, no goodbye, only
	// what was acknowledged before. This is a crash and a restart.
	e.start()
	if got := e.text(room); got != "kept and kept" {
		t.Errorf("after a restart the room holds %q", got)
	}

	// A client that opens the reloaded room gets the same text.
	b := e.connect(room, bob)
	settle(t, b)
	if b.text() != "kept and kept" {
		t.Errorf("a new client got %q", b.text())
	}
}

func TestEveryAcceptedUpdateIsOnDiskBeforeItIsAcknowledged(t *testing.T) {
	t.Parallel()
	// ygo's own websocket provider coalesces writes for two seconds, which is a
	// two second window in which an acknowledged edit is lost to a crash. The
	// hub does not use it: the row is appended before Receive returns.
	e := newEnv(t, func(c *realtime.Config) { c.CompactAfter = 1000 })
	room := e.room()
	a := e.connect(room, alice)
	before := e.rows(room)
	for i := range 5 {
		a.typeAt(0, "x")
		if got := e.rows(room); got != before+i+1 {
			t.Fatalf("after acknowledged update %d the log has %d rows, want %d", i+1, got, before+i+1)
		}
	}
}

func TestAHandshakeThatChangesNothingAddsNoRow(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "one")
	rows := e.rows(room)
	b := e.connect(room, bob)
	settle(t, a, b)
	if got := e.rows(room); got != rows {
		t.Errorf("a client that joined and changed nothing added %d row(s)", got-rows)
	}
}

func TestTheUpdateLogIsCompactedAfterTheThresholdAndKeepsTheContent(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.CompactAfter = 5 })
	room := e.room()
	a := e.connect(room, alice)
	epoch, err := e.hub.Epoch(t.Context(), room)
	if err != nil {
		t.Fatal(err)
	}

	for i := range 4 {
		a.typeAt(i, "x")
	}
	if got := e.rows(room); got != 4 {
		t.Fatalf("below the threshold the log has %d rows, want 4", got)
	}
	a.typeAt(4, "x")
	if got := e.rows(room); got != 1 {
		t.Fatalf("at the threshold the log has %d rows, want 1", got)
	}
	if got, _ := e.hub.Epoch(t.Context(), room); got != epoch {
		t.Errorf("compacting changed the epoch from %s to %s: clients would start over for nothing", epoch, got)
	}
	e.start()
	if got := e.text(room); got != "xxxxx" {
		t.Errorf("after compaction and a restart the room holds %q", got)
	}
}

func TestAClientFromAnOlderEpochIsToldToStartOver(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "old history")
	oldEpoch, err := e.hub.Epoch(t.Context(), room)
	if err != nil {
		t.Fatal(err)
	}

	err = e.hub.Replace(t.Context(), room, func(doc *crdt.Doc) error {
		txt := doc.GetText("t")
		doc.Transact(func(txn *crdt.Transaction) { txt.Insert(txn, 0, "rebuilt", nil) })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The connection that was open is closed and told why.
	var ee *realtime.EpochError
	select {
	case <-a.peer.Done():
		if !errors.As(a.peer.Err(), &ee) {
			t.Fatalf("the open connection ended with %v, want an *EpochError", a.peer.Err())
		}
	default:
		t.Fatal("the connection to the rebuilt room is still open")
	}
	if got := e.rows(room); got != 1 {
		t.Errorf("the rebuilt log has %d rows, want 1", got)
	}

	// A client that comes back with what it holds from the old epoch is refused,
	// and told the epoch to start over from.
	_, err = e.hub.Join(t.Context(), room, alice, oldEpoch)
	if !errors.As(err, &ee) || ee.Current == oldEpoch || ee.Current == "" {
		t.Fatalf("join with the old epoch = %v, want an *EpochError naming the new epoch", err)
	}
	// Starting over (no local state, or the new epoch) works and gets the new content.
	for _, epoch := range []string{"", ee.Current} {
		c := e.connectAt(room, bob, epoch)
		settle(t, c)
		if c.text() != "rebuilt" {
			t.Errorf("a client that joined with epoch %q got %q", epoch, c.text())
		}
	}
}

// limitedEnv is a room with small limits and two people in it, one of whom has typed.
func limitedEnv(t *testing.T) (e *env, room string, a, b *client) {
	t.Helper()
	e = newEnv(t, func(c *realtime.Config) {
		c.Limits = realtime.Limits{UpdateBytes: 200, PeersPerRoom: 2, AwarenessBytes: 80, DocBytes: 400}
	})
	room = e.room()
	a, b = e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "fits")
	settle(t, a, b)
	return e, room, a, b
}

func TestAnOversizedUpdateIsRefusedNamingTheLimitAndTheValueAndChangesNothing(t *testing.T) {
	t.Parallel()
	e, room, a, b := limitedEnv(t)
	rows := e.rows(room)

	a.typeAt(0, strings.Repeat("x", 300))
	if len(a.errs) != 1 {
		t.Fatalf("errors = %v, want the one refusal", a.errs)
	}
	le := limitOf(t, a.errs[0])
	if le.Name != "update size (bytes)" || le.Max != 200 || le.Value < 300 {
		t.Errorf("limit = %+v", le)
	}
	for _, want := range []string{"update size (bytes)", "200"} {
		if !strings.Contains(a.errs[0].Error(), want) {
			t.Errorf("message %q does not say %q", a.errs[0], want)
		}
	}
	settle(t, b)
	if e.text(room) != "fits" || b.text() != "fits" || e.rows(room) != rows {
		t.Errorf("the refused update changed the room (%q), the other peer (%q) or the log (%d rows)", e.text(room), b.text(), e.rows(room))
	}
}

func TestAPeerOverTheCapIsRefusedNamingTheLimitAndTheValue(t *testing.T) {
	t.Parallel()
	e, room, _, _ := limitedEnv(t)
	_, err := e.hub.Join(t.Context(), room, realtime.Identity{Username: "dave"}, "")
	le := limitOf(t, err)
	if le.Name != "peers in the room" || le.Max != 2 || le.Value != 3 {
		t.Errorf("limit = %+v, want 3 peers against a cap of 2", le)
	}
	if !strings.Contains(err.Error(), "peers in the room") || !strings.Contains(err.Error(), "3") || !strings.Contains(err.Error(), "2") {
		t.Errorf("message %q does not name the limit and the numbers", err)
	}
}

func TestAnOversizedAwarenessMessageIsRefusedNamingTheLimitAndTheValue(t *testing.T) {
	t.Parallel()
	_, _, a, _ := limitedEnv(t)
	a.aw.SetLocalState(map[string]any{"user": map[string]any{"name": "a"}, "pad": strings.Repeat("p", 200)})
	err := a.send(frame(1, a.aw.EncodeUpdate([]uint64{a.aw.ClientID()})))
	le := limitOf(t, err)
	if le.Name != "awareness message size (bytes)" || le.Max != 80 || le.Value <= 80 {
		t.Errorf("limit = %+v", le)
	}
}

func TestADocumentOverTheCapIsRefusedNamingTheLimitAndTheValueAndKeepsWhatFitted(t *testing.T) {
	t.Parallel()
	e, room, a, b := limitedEnv(t)
	b.peer.Close()

	// A client whose update was refused holds a change the room does not, so what it
	// sends next may depend on it: a transport closes such a connection and the
	// client starts again. This is a client that starts again.
	a.peer.Close()
	c := e.connect(room, alice)
	// Each of these is under the update cap; together they pass the document cap.
	var refused error
	for i := 0; i < 10 && refused == nil; i++ {
		c.typeAt(0, strings.Repeat("y", 150))
		if len(c.errs) > 0 {
			refused = c.errs[0]
		}
	}
	if refused == nil {
		t.Fatal("the document grew past its cap without a refusal")
	}
	le := limitOf(t, refused)
	if le.Name != "document size (bytes)" || le.Max != 400 || le.Value <= 400 {
		t.Errorf("limit = %+v", le)
	}
	if !strings.Contains(refused.Error(), "document size (bytes)") || !strings.Contains(refused.Error(), "400") {
		t.Errorf("message %q does not name the limit and its value", refused)
	}
	if got := e.text(room); !strings.HasSuffix(got, "fits") || len(got) > 400 {
		t.Errorf("the room holds %d bytes of text, %q...", len(got), got[:min(len(got), 20)])
	}
}

func TestAReadOnlyPeersWriteNeverReachesTheRoomTheLogOrOthers(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, r := e.connect(room, alice), e.connect(room, reader)
	a.typeAt(0, "from alice")
	settle(t, a, r)
	if r.text() != "from alice" {
		t.Fatalf("the reader did not receive the edit: %q", r.text())
	}
	rows, updates := e.rows(room), len(e.seen())

	// The reader's copy is edited anyway, as a hostile client would.
	r.typeAt(0, "FORGED ")
	if len(r.errs) != 0 {
		t.Errorf("the dropped write is not an error to the client: %v", r.errs)
	}
	settle(t, a, r)

	if got := e.text(room); got != "from alice" {
		t.Errorf("the room holds %q after a reader's write", got)
	}
	if got := e.rows(room); got != rows {
		t.Errorf("the log grew from %d to %d rows", rows, got)
	}
	if got := len(e.seen()); got != updates {
		t.Errorf("%d update(s) reported, want none for the reader's write", got-updates)
	}
	if a.text() != "from alice" {
		t.Errorf("the editor saw %q", a.text())
	}
	if got := r.peer.Dropped(); got != 1 {
		t.Errorf("dropped writes = %d, want 1", got)
	}

	// A reader still gets the room's answer to its sync step 1.
	late := e.connect(room, reader)
	settle(t, late)
	if late.text() != "from alice" {
		t.Errorf("a second reader got %q", late.text())
	}
}

func TestEmojiAndCJKSurviveAnEditAndAReload(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "你好 😀 world")
	settle(t, a, b)
	// An index after the emoji counts it as two UTF-16 code units.
	b.typeAt(b.doc.GetText("t").Len(), " 𠮷野家")
	b.typeAt(5, "🎉")
	settle(t, a, b)

	const want = "你好 😀🎉 world 𠮷野家"
	if a.text() != want || b.text() != want {
		t.Fatalf("clients hold %q and %q, want %q", a.text(), b.text(), want)
	}
	e.start()
	if got := e.text(room); got != want {
		t.Errorf("after a reload the room holds %q, want %q", got, want)
	}
}

func TestEveryUpdateIsAttributedToTheConnectionThatSentIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "a")
	settle(t, a, b)
	b.typeAt(1, "b")
	settle(t, a, b)

	err := e.hub.Edit(t.Context(), room, realtime.Identity{Username: "alice", Label: "Alice's agent"}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		txt := doc.GetText("t")
		return func(txn *crdt.Transaction) { txt.Insert(txn, 0, "agent ", nil) }, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var authors []string
	for _, u := range e.seen() {
		authors = append(authors, u.Author.Label)
		if u.Room != room || len(u.Bytes) == 0 || u.Epoch == "" {
			t.Errorf("update = %+v", u)
		}
	}
	if got := strings.Join(authors, ","); got != "Alice,Bob,Alice's agent" {
		t.Errorf("authors = %q", got)
	}
}

func TestAnEditFromTheServerIsSavedSentToEveryPeerAndMayReadBeforeItWrites(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, r := e.connect(room, alice), e.connect(room, reader)
	a.typeAt(0, "line")
	settle(t, a, r)
	rows := e.rows(room)

	// The plan reads the document (the length) and decides where to write.
	err := e.hub.Edit(t.Context(), room, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		txt := doc.GetText("t")
		at := txt.Len()
		return func(txn *crdt.Transaction) { txt.Insert(txn, at, " + edit", nil) }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	settle(t, a, r)
	for name, got := range map[string]string{"the editor": a.text(), "the reader": r.text(), "the room": e.text(room)} {
		if got != "line + edit" {
			t.Errorf("%s holds %q", name, got)
		}
	}
	if got := e.rows(room); got != rows+1 {
		t.Errorf("the log has %d rows, want %d", got, rows+1)
	}

	// A plan that changes nothing, or fails, writes nothing.
	boom := errors.New("not today")
	if err := e.hub.Edit(t.Context(), room, alice, func(*crdt.Doc) (func(*crdt.Transaction), error) { return nil, nil }); err != nil {
		t.Errorf("a plan with nothing to write returned %v", err)
	}
	if err := e.hub.Edit(t.Context(), room, alice, func(*crdt.Doc) (func(*crdt.Transaction), error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("a failing plan returned %v", err)
	}
	if got := e.rows(room); got != rows+1 {
		t.Errorf("a plan that wrote nothing added rows: %d", got)
	}
}

func TestAnEditThatBreaksTheDocumentLimitKeepsNothingAndTheRoomIsReloaded(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.Limits = realtime.Limits{DocBytes: 300} })
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "small")
	settle(t, a)
	rows := e.rows(room)

	err := e.hub.Edit(t.Context(), room, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		txt := doc.GetText("t")
		return func(txn *crdt.Transaction) { txt.Insert(txn, 0, strings.Repeat("z", 500), nil) }, nil
	})
	le := limitOf(t, err)
	if le.Name != "document size (bytes)" || le.Max != 300 {
		t.Errorf("limit = %+v", le)
	}
	if got := e.rows(room); got != rows {
		t.Errorf("the refused edit left %d rows, want %d", got, rows)
	}
	if !errors.Is(a.peer.Err(), realtime.ErrRoomReset) {
		t.Errorf("the connection ended with %v, want ErrRoomReset: the room was reloaded from disk", a.peer.Err())
	}
	if got := e.text(room); got != "small" {
		t.Errorf("the room holds %q after the refused edit", got)
	}
}

func TestAnUpdateThatCannotBeSavedIsNotSentAndTheRoomIsReloaded(t *testing.T) {
	t.Parallel()
	failing := &flakyLog{}
	e := newEnv(t, func(c *realtime.Config) {
		failing.Log = c.Log
		c.Log = failing
	})
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "saved")
	settle(t, a, b)

	failing.fail.Store(true)
	a.errs = nil
	a.typeAt(5, " lost")
	if len(a.errs) != 1 || !strings.Contains(a.errs[0].Error(), "keep the update") {
		t.Fatalf("errors = %v, want the write to be refused with the reason", a.errs)
	}
	if b.pump() != 0 || b.text() != "saved" {
		t.Errorf("an update that was not saved reached another peer: %q", b.text())
	}
	for _, c := range []*client{a, b} {
		if !errors.Is(c.peer.Err(), realtime.ErrRoomReset) {
			t.Errorf("connection ended with %v, want ErrRoomReset", c.peer.Err())
		}
	}

	failing.fail.Store(false)
	if got := e.text(room); got != "saved" {
		t.Errorf("the room reloaded as %q, want what was acknowledged", got)
	}
}

func TestDisconnectClosesOnlyTheMatchingConnections(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a, b, c := e.connect(room, alice), e.connect(room, bob), e.connect(room, bob)

	reason := errors.New("no longer an editor")
	n := e.hub.Disconnect(room, func(who realtime.Identity) bool { return who.Username == "bob" }, reason)
	if n != 2 {
		t.Errorf("closed %d connections, want both of bob's", n)
	}
	for _, p := range []*client{b, c} {
		if !errors.Is(p.peer.Err(), reason) {
			t.Errorf("bob's connection ended with %v", p.peer.Err())
		}
		if err := p.send(frame(0, nil)); !errors.Is(err, realtime.ErrPeerClosed) {
			t.Errorf("a message on a closed connection = %v, want ErrPeerClosed", err)
		}
	}
	if a.peer.Err() != nil {
		t.Errorf("alice's connection ended: %v", a.peer.Err())
	}
	if got := e.hub.Disconnect("not a loaded room", func(realtime.Identity) bool { return true }, reason); got != 0 {
		t.Errorf("closed %d connections of a room that is not loaded", got)
	}
}

func TestAConnectionThatDoesNotKeepUpIsDroppedAndTheOthersAreNot(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.PeerQueue = 6 })
	room := e.room()
	a, slow, fast := e.connect(room, alice), e.connect(room, bob), e.connect(room, bob)
	for i := range 10 {
		a.typeAt(i, "x")
		fast.pump()
	}
	if !errors.Is(slow.peer.Err(), realtime.ErrSlowPeer) {
		t.Errorf("the connection that never read ended with %v, want ErrSlowPeer", slow.peer.Err())
	}
	if fast.peer.Err() != nil || a.peer.Err() != nil {
		t.Errorf("connections that kept up were closed: %v, %v", fast.peer.Err(), a.peer.Err())
	}
	if fast.text() != "xxxxxxxxxx" {
		t.Errorf("the connection that kept up holds %q", fast.text())
	}
}

func TestARoomOfADeletedArtifactCannotBeJoined(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "text")
	if err := e.st.DeleteArtifact(t.Context(), room); err != nil {
		t.Fatal(err)
	}
	if got := e.rows(room); got != 0 {
		t.Errorf("the deleted artifact's log still has %d rows", got)
	}
	e.start()
	if _, err := e.hub.Join(t.Context(), room, alice, ""); !errors.Is(err, realtime.ErrNoRoom) {
		t.Errorf("join = %v, want ErrNoRoom", err)
	}
	if _, err := e.hub.Join(t.Context(), "no-such-artifact", alice, ""); !errors.Is(err, realtime.ErrNoRoom) {
		t.Errorf("join of an id that never existed = %v, want ErrNoRoom", err)
	}
}

// The browser's "Saved" indicator (web/src/editor/provider.ts) counts the step 2
// answers to the step 1s it sent, and so has to know that a join brings exactly one
// step 2 nobody asked for: the room's whole state, right after the room's own step 1.
func TestJoiningBringsItsOwnStep1ThenOneUnpromptedStep2ThenAwarenessAndNothingElse(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	peer, err := e.hub.Join(t.Context(), room, alice, "")
	if err != nil {
		t.Fatal(err)
	}
	type message struct{ outer, inner uint64 }
	var got []message
	for {
		select {
		case msg := <-peer.Out():
			dec := encoding.NewDecoder(msg)
			outer, err := dec.ReadVarUint()
			if err != nil {
				t.Fatal(err)
			}
			m := message{outer: outer}
			if outer == 0 {
				if m.inner, err = dec.ReadVarUint(); err != nil {
					t.Fatal(err)
				}
			}
			got = append(got, m)
			continue
		default:
		}
		break
	}
	// outer 0 is sync (inner 0 step 1, inner 1 step 2), outer 1 is awareness.
	want := []message{{0, 0}, {0, 1}, {1, 0}}
	if len(got) != len(want) {
		t.Fatalf("a join queued %d messages %v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d is %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAChangeTheHubsCheckRefusesIsNeitherSavedNorSentAndTheRoomGoesBackToWhatItWas(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) {
		c.Validate = func(_ string, doc *crdt.Doc) error {
			if strings.Contains(doc.GetText("t").ToString(), "forbidden") {
				return errors.New("the text says what it may not")
			}
			return nil
		}
	})
	room := e.room()
	a, b := e.connect(room, alice), e.connect(room, bob)
	a.typeAt(0, "fine")
	settle(t, a, b)
	rows, updates := e.rows(room), len(e.seen())

	// The change that breaks the rule is refused: not on disk, not in anyone's copy of the room.
	a.typeAt(0, "forbidden ")
	if len(a.errs) == 0 {
		t.Fatal("the change was accepted")
	}
	var refused *realtime.RefusedError
	if !errors.As(a.errs[0], &refused) || !strings.Contains(a.errs[0].Error(), "what it may not") {
		t.Errorf("the refusal is %v, want one that says why", a.errs[0])
	}
	if e.rows(room) != rows || len(e.seen()) != updates {
		t.Errorf("a refused change was saved: rows %d then %d, updates %d then %d", rows, e.rows(room), updates, len(e.seen()))
	}
	if got := e.text(room); got != "fine" {
		t.Errorf("the room holds %q, want what it held before", got)
	}
	if b.text() != "fine" {
		t.Errorf("another peer was shown the refused change: %q", b.text())
	}
}

// People join while others are typing, all at once. Whatever the order, once the
// messages are delivered every client and the room hold the same text, with
// every edit in it.
func TestPeopleJoiningAndTypingAtOnceAllEndWithTheSameTextAndEveryEdit(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	const people, edits = 8, 6
	clients := make([]*client, people)
	var wg sync.WaitGroup
	for i := range people {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range i { // different joiners arrive at different moments
				runtime.Gosched()
			}
			c := e.connect(room, realtime.Identity{Username: fmt.Sprintf("u%d", i), Label: fmt.Sprintf("U%d", i)})
			for k := range edits {
				c.typeAt(0, fmt.Sprintf("<%d.%d>", i, k))
				c.pump()
			}
			clients[i] = c
		}()
	}
	wg.Wait()
	settle(t, clients...)
	want := e.text(room)
	for i, c := range clients {
		if c.text() != want {
			t.Errorf("client %d holds %q, the room %q", i, c.text(), want)
		}
	}
	for i := range people {
		for k := range edits {
			if tag := fmt.Sprintf("<%d.%d>", i, k); !strings.Contains(want, tag) {
				t.Errorf("the room lost %s", tag)
			}
		}
	}
}
