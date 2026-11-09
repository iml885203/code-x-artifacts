package docroom_test

import (
	"strings"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
)

func choiceDoc(options []string, value string) docfmt.Doc {
	return paragraphDoc(docfmt.Block{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "Pick "}, {Dropdown: &docfmt.Dropdown{Options: options, Value: value}}, {Text: " now"}, {Date: "2026-10-24"}}})
}

func TestChangingDropdownConfigurationRetainsConcurrentTextAndWritesOneAtomicAttribute(t *testing.T) {
	t.Parallel()
	before := choiceDoc([]string{"A", "B"}, "A")
	server := roomHolding(t, before, 1)
	client := overTheWire(t, server)
	next := choiceDoc([]string{"A", "C"}, "C")
	change(t, server, next)
	typeAt(client, textOf(t, client, "b1", -1), 5, "peer ")
	merge(t, server, client)
	got := read(t, server).Tabs[0].Blocks[0]
	if got.ID != "b1" || docfmt.PlainText(got.Content) != "Pick peer C now2026-10-24" {
		t.Fatalf("merged fields and text: %+v", got)
	}
	if got, want := jsonOf(t, read(t, overTheWire(t, server))), jsonOf(t, read(t, server)); got != want {
		t.Fatalf("reloaded %s, want %s", got, want)
	}
}

func TestConcurrentDropdownOptionAndSelectionEditsConvergeOnOneWholeValidConfiguration(t *testing.T) {
	t.Parallel()
	server := roomHolding(t, choiceDoc([]string{"A", "B"}, "A"), 1)
	client := overTheWire(t, server)
	optionEdit := choiceDoc([]string{"A", "C"}, "C")
	selectionEdit := choiceDoc([]string{"A", "B"}, "B")
	change(t, server, optionEdit)
	change(t, client, selectionEdit)
	merge(t, server, client)
	got := jsonOf(t, read(t, server))
	if got != jsonOf(t, optionEdit) && got != jsonOf(t, selectionEdit) {
		t.Fatalf("mixed configuration: %s", got)
	}
	if peer := jsonOf(t, read(t, client)); peer != got {
		t.Fatalf("peers disagree: %s / %s", got, peer)
	}
	if _, err := (docroom.Codec{}).Content(server); err != nil {
		t.Fatal(err)
	}
}

func TestAHostileOrMalformedDropdownRoomAttributeCannotBecomeStoredContent(t *testing.T) {
	t.Parallel()
	for _, config := range []string{"not JSON", `null`, `[]`, `{"options":["A"],"value":"B"}`, `{"options":["A"],"value":"A","html":"<script>"}`} {
		b := newRoomBuilder().tab().child("paragraph", "b1").child("dropdown", "")
		b.doc.Transact(func(txn *crdt.Transaction) { b.last.SetAttribute(txn, "config", config) })
		if _, err := (docroom.Codec{}).Content(b.doc); err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0]") {
			t.Fatalf("accepted %s or lost path: %v", config, err)
		}
	}
}
