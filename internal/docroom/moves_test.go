package docroom_test

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

func tabIDs(d docfmt.Doc) []string {
	var out []string
	for _, t := range d.Tabs {
		out = append(out, t.ID)
	}
	return out
}

func textOfTab(d docfmt.Doc, id string) string {
	for _, t := range d.Tabs {
		if t.ID == id && len(t.Blocks) > 0 {
			var sb strings.Builder
			for _, r := range t.Blocks[0].Content {
				sb.WriteString(r.Text)
			}
			return sb.String()
		}
	}
	return ""
}

// sync gives each document the changes the other has.
func sync(t *testing.T, a, b *crdt.Doc) {
	t.Helper()
	ua, ub := crdt.EncodeStateAsUpdateV1(a, b.StateVector()), crdt.EncodeStateAsUpdateV1(b, a.StateVector())
	if err := crdt.ApplyUpdateV1(b, ua, nil); err != nil {
		t.Fatal(err)
	}
	if err := crdt.ApplyUpdateV1(a, ub, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAMoveWhileSomeoneTypesInTheMovingTabKeepsTheOrderAndEveryCharacter(t *testing.T) {
	t.Parallel()
	for name, to := range map[string][]int{
		"to the front": {2, 0, 1},
		"to the back":  {1, 2, 0},
		"one down":     {1, 0, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mover := roomHolding(t, threeTabs(), 11)
			typist := overTheWire(t, mover)

			// The mover reorders the tabs the way an agent's tab_move does (Apply); the typist types in the first tab at the same time.
			moved := threeTabs()
			moved.Tabs = []docfmt.Tab{threeTabs().Tabs[to[0]], threeTabs().Tabs[to[1]], threeTabs().Tabs[to[2]]}
			write, err := docroom.Open(mover).Apply(moved)
			if err != nil {
				t.Fatal(err)
			}
			mover.Transact(write)
			el, _ := docroom.Open(typist).TabElement("t1")
			run := el.Children()[0].(*crdt.YXmlElement).Children()[0].(*crdt.YXmlText)
			typeAt(typist, run, run.Len(), " + typed")

			sync(t, mover, typist)
			for who, doc := range map[string]*crdt.Doc{"mover": mover, "typist": typist} {
				got := read(t, doc)
				want := []string{"t1", "t2", "t3"}
				want = []string{want[to[0]], want[to[1]], want[to[2]]}
				if !slices.Equal(tabIDs(got), want) {
					t.Errorf("%s: order %v, want %v", who, tabIDs(got), want)
				}
				if text := textOfTab(got, "t1"); text != "text of t1 + typed" {
					t.Errorf("%s: the text typed while the tab moved is %q", who, text)
				}
			}
		})
	}
}

func TestTwoPeopleWhoMoveTabsAtOnceEndWithEveryTabOnceInTheSameOrder(t *testing.T) {
	t.Parallel()
	a := roomHolding(t, threeTabs(), 21)
	b := overTheWire(t, a)
	for doc, order := range map[*crdt.Doc][]int{a: {2, 0, 1}, b: {1, 2, 0}} {
		next := threeTabs()
		next.Tabs = []docfmt.Tab{next.Tabs[order[0]], next.Tabs[order[1]], next.Tabs[order[2]]}
		write, err := docroom.Open(doc).Apply(next)
		if err != nil {
			t.Fatal(err)
		}
		doc.Transact(write)
	}
	sync(t, a, b)
	ga, gb := read(t, a), read(t, b)
	if !slices.Equal(tabIDs(ga), tabIDs(gb)) {
		t.Fatalf("the two copies differ: %v and %v", tabIDs(ga), tabIDs(gb))
	}
	got := slices.Clone(tabIDs(ga))
	slices.Sort(got)
	if !slices.Equal(got, []string{"t1", "t2", "t3"}) {
		t.Errorf("tabs after two moves at once: %v", tabIDs(ga))
	}
	for _, tab := range ga.Tabs {
		if want := "text of " + tab.ID; textOfTab(ga, tab.ID) != want {
			t.Errorf("tab %s holds %q", tab.ID, textOfTab(ga, tab.ID))
		}
	}
}

// What the editor wrote before tabs had a fragment each: one fragment of every tab.
func legacyRoom(t *testing.T) *crdt.Doc {
	t.Helper()
	doc := crdt.New()
	frag := doc.GetXmlFragment(docroom.Fragment)
	for _, tab := range threeTabs().Tabs {
		e := crdt.NewYXmlElement("tab")
		p := crdt.NewYXmlElement("paragraph")
		text := crdt.NewYXmlText()
		doc.Transact(func(txn *crdt.Transaction) {
			frag.InsertElement(txn, frag.Len(), e)
			e.SetAttribute(txn, "id", tab.ID)
			e.SetAttribute(txn, "title", tab.Title)
			e.InsertElement(txn, 0, p)
			p.SetAttribute(txn, "id", tab.Blocks[0].ID)
			p.InsertText(txn, 0, text)
			text.Insert(txn, 0, tab.Blocks[0].Content[0].Text, nil)
		})
	}
	return doc
}

func TestARoomInTheOldOneFragmentFormIsReadAndMigratedWithoutChangingTheDocument(t *testing.T) {
	t.Parallel()
	doc := legacyRoom(t)
	if got, want := jsonOf(t, read(t, doc)), jsonOf(t, docroom.Canon(threeTabs())); got != want {
		t.Fatalf("read the old form:\n got: %s\nwant: %s", got, want)
	}
	c := docroom.Open(doc)
	migrate, err := c.Migrate()
	if err != nil || migrate == nil {
		t.Fatalf("Migrate: %v, %v", migrate != nil, err)
	}
	doc.Transact(migrate)
	if got, want := jsonOf(t, read(t, doc)), jsonOf(t, docroom.Canon(threeTabs())); got != want {
		t.Errorf("after the migration:\n got: %s\nwant: %s", got, want)
	}
	if again, err := docroom.Open(doc).Migrate(); err != nil || again != nil {
		t.Errorf("a migrated room needs no migration: %v, %v", again != nil, err)
	}
	// It is now moved by its ids alone, and a copy of the room over the wire reads the same.
	if got := jsonOf(t, read(t, overTheWire(t, doc))); got != jsonOf(t, docroom.Canon(threeTabs())) {
		t.Errorf("a peer reads %s", got)
	}
}

func TestAMoveFromTheEditorMergedWithTypingFromAnotherEditorIsReadByGo(t *testing.T) {
	t.Parallel()
	fx := struct{ Base, Move, Typing string }{}
	raw := readMoveFixture(t)
	fx.Base, fx.Move, fx.Typing = raw.Base, raw.Move, raw.Typing
	decode := func(h string) []byte {
		b, err := hex.DecodeString(h)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for name, order := range map[string][]string{"move first": {fx.Move, fx.Typing}, "typing first": {fx.Typing, fx.Move}} {
		doc := crdt.New()
		for _, u := range append([]string{fx.Base}, order...) {
			if err := crdt.ApplyUpdateV1(doc, decode(u), nil); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		got := read(t, doc)
		if !slices.Equal(tabIDs(got), []string{"t3", "t1", "t2"}) || textOfTab(got, "t3") != "text of t3 + typed while it moved" {
			t.Errorf("%s: tabs %v, t3 reads %q", name, tabIDs(got), textOfTab(got, "t3"))
		}
	}
}
