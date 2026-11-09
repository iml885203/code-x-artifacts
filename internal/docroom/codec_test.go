package docroom_test

import (
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

func TestSeedingARoomAndReadingItBackGivesTheContentOfEveryCase(t *testing.T) {
	t.Parallel()
	var codec docroom.Codec
	for name, d := range cases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			content, err := docfmt.Canonical(d)
			if err != nil {
				t.Fatal(err)
			}
			state, err := codec.Seed(content)
			if err != nil {
				t.Fatal(err)
			}
			doc := crdt.New()
			if err := crdt.ApplyUpdateV1(doc, state, nil); err != nil {
				t.Fatal(err)
			}
			got, err := codec.Content(doc)
			if err != nil || got != content {
				t.Errorf("content of the seeded room:\n got: %s\nwant: %s\nerr: %v", got, content, err)
			}
			if !codec.Same(content, got) {
				t.Error("the content of a room and the content it was seeded from are the same document")
			}
		})
	}
}

func TestTwoContentsAreTheSameDocumentWhenTheRoomCannotTellThemApart(t *testing.T) {
	t.Parallel()
	var codec docroom.Codec
	base := `{"tabs":[{"id":"t1","title":"T","blocks":[{"id":"b1","type":"paragraph","content":[{"text":"a","marks":["bold","italic"]},{"text":"b","marks":["italic","bold"]}]}]}]}`
	merged := `{"tabs":[{"id":"t1","title":"T","blocks":[{"id":"b1","type":"paragraph","content":[{"text":"ab","marks":["bold","italic"]}]}]}]}`
	changed := `{"tabs":[{"id":"t1","title":"T","blocks":[{"id":"b1","type":"paragraph","content":[{"text":"ac","marks":["bold","italic"]}]}]}]}`
	otherID := `{"tabs":[{"id":"t1","title":"T","blocks":[{"id":"b2","type":"paragraph","content":[{"text":"ab","marks":["bold","italic"]}]}]}]}`
	if !codec.Same(base, merged) {
		t.Error("marks are a set and runs with the same marks are one run")
	}
	for name, other := range map[string]string{"text": changed, "id": otherID, "not a doc": "{", "empty": ""} {
		if codec.Same(base, other) {
			t.Errorf("a change of %s is a different document", name)
		}
	}
}

func TestWhatPeopleTypedBecomesContentOnlyIfADocMayHoldIt(t *testing.T) {
	t.Parallel()
	b := newRoomBuilder()
	b.tab().child("paragraph", "b1").link("click", "javascript:alert(1)")
	if content, err := (docroom.Codec{}).Content(b.doc); err == nil {
		t.Errorf("a link that runs script became content: %s", content)
	}
}

func TestWritingContentIntoARoomReplacesWhatItHeld(t *testing.T) {
	t.Parallel()
	doc := roomHolding(t, cases(t)["everything"], 1)
	prose, err := docfmt.Canonical(cases(t)["prose"])
	if err != nil {
		t.Fatal(err)
	}
	write, err := (docroom.Codec{}).Write(doc, prose)
	if err != nil {
		t.Fatal(err)
	}
	doc.Transact(write)
	if got := jsonOf(t, read(t, doc)); got != jsonOf(t, cases(t)["prose"]) {
		t.Errorf("the room holds %s", got)
	}
	if _, err := (docroom.Codec{}).Write(doc, "{"); err == nil {
		t.Error("content that is not a doc was written into a room")
	}
}
