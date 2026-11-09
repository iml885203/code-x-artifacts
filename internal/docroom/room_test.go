package docroom_test

import (
	"errors"
	"testing"
	"time"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
	"github.com/iml885203/code-x-artifacts/internal/store/storetest"
)

var alice = realtime.Identity{Username: "alice", Label: "Alice"}

// hubOver is a hub over a real SQLite file.
func hubOver(t *testing.T) (*realtime.Hub, *store.Store) {
	t.Helper()
	st := storetest.Open(t)
	hub, err := realtime.New(realtime.Config{Log: st.RealtimeLog()})
	if err != nil {
		t.Fatal(err)
	}
	return hub, st
}

func publish(t *testing.T, st *store.Store, kind, format, content string) string {
	t.Helper()
	a, err := st.CreateArtifact(t.Context(), "A document", kind, format, content, "alice", store.VisibilityOrganisation)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func readRoom(t *testing.T, hub *realtime.Hub, room string) docfmt.Doc {
	t.Helper()
	var got docfmt.Doc
	err := hub.Read(t.Context(), room, func(doc *crdt.Doc) error {
		var err error
		got, err = docroom.Open(doc).Read()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAnEditThroughTheHubIsReadBackAndSurvivesARestart(t *testing.T) {
	t.Parallel()
	hub, st := hubOver(t)
	room := publish(t, st, "doc", "2", `{"tabs":[{"id":"t1","title":"T","blocks":[]}]}`)
	next := cases(t)["prose"]
	err := hub.Edit(t.Context(), room, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return docroom.Open(doc).Write(next), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	hub.Close(t.Context())
	restarted, err := realtime.New(realtime.Config{Log: st.RealtimeLog()})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jsonOf(t, readRoom(t, restarted, room)), jsonOf(t, next); got != want {
		t.Errorf("after a restart:\n got: %s\nwant: %s", got, want)
	}
}

func TestAMentionWrittenToTheRoomReachesAnotherEditorAndSurvivesARestart(t *testing.T) {
	t.Parallel()
	hub, st := hubOver(t)
	room := publish(t, st, "doc", "2", `{"tabs":[{"id":"t1","title":"T","blocks":[]}]}`)
	next := cases(t)["mentions"]
	err := hub.Edit(t.Context(), room, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return docroom.Open(doc).Write(next), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Another editor in the room is sent what the room holds.
	var state []byte
	if err := hub.Read(t.Context(), room, func(doc *crdt.Doc) error {
		state = crdt.EncodeStateAsUpdateV1(doc, nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other := crdt.New()
	if err := crdt.ApplyUpdateV1(other, state, nil); err != nil {
		t.Fatal(err)
	}
	want := jsonOf(t, next)
	if got := jsonOf(t, read(t, other)); got != want {
		t.Errorf("another editor sees:\n got: %s\nwant: %s", got, want)
	}
	hub.Close(t.Context())
	restarted, err := realtime.New(realtime.Config{Log: st.RealtimeLog()})
	if err != nil {
		t.Fatal(err)
	}
	if got := jsonOf(t, readRoom(t, restarted, room)); got != want {
		t.Errorf("after a restart:\n got: %s\nwant: %s", got, want)
	}
}

// Reading inside a transaction never returns in ygo. Through Content it comes
// back at once with an error, so a change that reads while it writes fails in
// a test instead of hanging a room.
func TestReadingWhileWritingFailsFastInsteadOfHanging(t *testing.T) {
	t.Parallel()
	doc := crdt.New()
	c := docroom.Open(doc)
	got := make(chan error, 1)
	go func() {
		doc.Transact(c.Change(func(*docroom.Writer) {
			_, err := c.Read()
			got <- err
		}))
	}()
	select {
	case err := <-got:
		if !errors.Is(err, docroom.ErrReadWhileWriting) {
			t.Errorf("got %v, want ErrReadWhileWriting", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the read inside a write hung")
	}
	if _, err := c.Read(); err != nil {
		t.Errorf("reading after the write is fine: %v", err)
	}
}

func TestOpeningTheDocumentInsideATransactionIsTheOneThingTheWrapperCannotStop(t *testing.T) {
	t.Parallel()
	// ygo never returns from doc.GetXmlFragment inside a transaction (see
	// realtime/gotchas_test.go); Open belongs to the plan, which Hub.Edit runs
	// before the transaction. If ygo ever returns here, Open can be relaxed.
	doc := crdt.New()
	returned := make(chan struct{})
	go func() {
		// Left blocked on purpose, on a document nothing else uses.
		doc.Transact(func(*crdt.Transaction) { docroom.Open(doc) })
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("ygo no longer blocks a lookup inside a transaction; docroom.Content can open the fragment lazily")
	case <-time.After(250 * time.Millisecond):
	}
}
