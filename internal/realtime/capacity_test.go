package realtime_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/reearth/ygo/crdt"
)

func TestAnOversizedReplacementKeepsTheLogEpochContentAndConnectedWriters(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.Limits.DocBytes = 300 })
	id := e.room()
	a := e.connect(id, alice)
	b := e.connect(id, bob)
	a.typeAt(0, "small")
	settle(t, a, b)
	before, err := e.st.RealtimeLog().Load(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	err = e.hub.Replace(t.Context(), id, func(doc *crdt.Doc) error {
		text := doc.GetText("t")
		doc.Transact(func(txn *crdt.Transaction) { text.Insert(txn, 0, strings.Repeat("x", 500), nil) })
		return nil
	})
	if limitOf(t, err).Max != 300 {
		t.Fatal("wrong bound")
	}
	after, err := e.st.RealtimeLog().Load(t.Context(), id, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("refused replacement changed log or epoch")
	}
	if e.text(id) != "small" || a.peer.Err() != nil || b.peer.Err() != nil {
		t.Fatal("refused replacement changed content or disconnected writers")
	}
	b.typeAt(5, " kept")
	settle(t, a, b)
	if e.text(id) != "small kept" {
		t.Fatal("writer could not continue")
	}
}
