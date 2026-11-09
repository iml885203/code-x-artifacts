package docroom_test

import (
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/reearth/ygo/crdt"
)

func TestChangingADatePreservesConcurrentTypingInTheSameParagraph(t *testing.T) {
	t.Parallel()
	before := paragraphDoc(docfmt.Block{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "Due "}, {Date: "2026-10-24"}, {Text: " now"}}})
	server := roomHolding(t, before, 1)
	client := overTheWire(t, server)
	next := paragraphDoc(docfmt.Block{ID: "b1", Type: docfmt.Paragraph, Content: []docfmt.Inline{{Text: "Due "}, {Date: "2026-10-25"}, {Text: " now"}}})
	change(t, server, next)
	typeAt(client, textOf(t, client, "b1", -1), 4, "tomorrow ")
	merge(t, server, client)
	got := read(t, server).Tabs[0].Blocks[0]
	if got.ID != "b1" || docfmt.PlainText(got.Content) != "Due tomorrow 2026-10-25 now" {
		t.Fatalf("merged: %+v", got)
	}
	if got, want := jsonOf(t, read(t, overTheWire(t, server))), jsonOf(t, read(t, server)); got != want {
		t.Errorf("reloaded %s, want %s", got, want)
	}
	if _, err := (docroom.Codec{}).Content(server); err != nil {
		t.Fatal(err)
	}
}

func TestFieldsInTheRoomSurviveASQLiteLogRestart(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"z_dates", "z_dropdowns"} {
		t.Run(name, func(t *testing.T) {
			hub, st := hubOver(t)
			id := publish(t, st, "doc", "2", `{"tabs":[{"id":"t1","title":"T","blocks":[]}]}`)
			next := cases(t)[name]
			if err := hub.Edit(t.Context(), id, alice, func(doc *crdt.Doc) (func(*crdt.Transaction), error) { return docroom.Open(doc).Write(next), nil }); err != nil {
				t.Fatal(err)
			}
			hub.Close(t.Context())
			restarted, err := realtime.New(realtime.Config{Log: st.RealtimeLog()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { restarted.Close(t.Context()) })
			if got, want := jsonOf(t, readRoom(t, restarted, id)), jsonOf(t, next); got != want {
				t.Fatalf("reloaded room: %s, want %s", got, want)
			}
		})
	}
}
