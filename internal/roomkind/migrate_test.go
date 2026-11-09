package roomkind_test

import (
	"slices"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

// oldFormRoom is the state of a room as the first editor kept it: every tab in
// the one fragment, here a single tab holding one paragraph.
func oldFormRoom(tab, block, text string) []byte {
	doc := crdt.New()
	defer doc.Destroy()
	frag := doc.GetXmlFragment(docroom.Fragment)
	t, p, run := crdt.NewYXmlElement("tab"), crdt.NewYXmlElement("paragraph"), crdt.NewYXmlText()
	doc.Transact(func(txn *crdt.Transaction) {
		frag.InsertElement(txn, 0, t)
		t.SetAttribute(txn, "id", tab)
		t.SetAttribute(txn, "title", "Doc")
		t.InsertElement(txn, 0, p)
		p.SetAttribute(txn, "id", block)
		p.InsertText(txn, 0, run)
		run.Insert(txn, 0, text, nil)
	})
	return crdt.EncodeStateAsUpdateV1(doc, nil)
}

func TestARoomWhoseLogIsInTheOldFormIsMigratedWhenItLoadsAndStaysSoAfterARestart(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	// The room's log is what the first editor left: the doc in one fragment.
	log := e.st.RealtimeLog()
	snap, err := log.Load(t.Context(), id, "old-epoch")
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Replace(t.Context(), id, snap.Epoch, oldFormRoom("t_old", "b_old", "typed in the old form")); err != nil {
		t.Fatal(err)
	}

	for range 2 { // as loaded now, and after a restart (the migration was kept in the log)
		if got := e.roomText(id); !slices.Equal(got, []string{"typed in the old form"}) {
			t.Fatalf("the room holds %v", got)
		}
		e.restart()
	}
}
