package roomkind_test

import (
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"

	"github.com/reearth/ygo/crdt"
)

func TestARoomForADocWithNoLogStartsFromTheLatestVersionAndKeepsItsIds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	latest, err := e.st.Artifact(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	want, err := docfmt.Parse(latest.Content)
	if err != nil {
		t.Fatal(err)
	}

	var got docfmt.Doc
	err = e.hub.Read(t.Context(), id, func(doc *crdt.Doc) (err error) {
		got, err = docroom.Open(doc).Read()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if want.Tabs[0].ID != got.Tabs[0].ID || len(got.Tabs[0].Blocks) != len(want.Tabs[0].Blocks) {
		t.Fatalf("the room starts with %+v, the version holds %+v", got, want)
	}
	for i, b := range want.Tabs[0].Blocks {
		if got.Tabs[0].Blocks[i].ID != b.ID {
			t.Errorf("block %d has id %q in the room and %q in the version", i, got.Tabs[0].Blocks[i].ID, b.ID)
		}
	}
	if rows, err := e.st.RealtimeRows(t.Context(), id); err != nil || rows != 1 {
		t.Errorf("the start is the first row of the log: %d rows, %v", rows, err)
	}
}

func TestARoomWithALogIsNotSeededAgainWhenTheArtifactIsNewer(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	_ = e.roomText(id) // opens the room: its log starts here
	e.restart()

	// A version the room did not produce (written around it, as no code path does).
	latest, err := e.st.Artifact(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	newer := store.Revision{Title: "Doc", Format: latest.Format, Content: `{"tabs":[{"id":"t1","title":"Document","blocks":[]}]}`, Author: "alice"}
	if err := e.st.UpdateArtifact(t.Context(), id, latest.Version, newer, ""); err != nil {
		t.Fatal(err)
	}
	if got := e.roomText(id); !slices.Equal(got, []string{"first paragraph"}) {
		t.Errorf("a room that has a log is its own working copy, it holds %v", got)
	}
}

func TestAPageIsNotADocAndItsRoomStartsEmpty(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	p, err := e.svc.Create(t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Page", Content: "<p>x"})
	if err != nil {
		t.Fatal(err)
	}
	err = e.hub.Read(t.Context(), p.ID, func(doc *crdt.Doc) error {
		if n := doc.GetXmlFragment(docroom.Fragment).Len(); n != 0 {
			t.Errorf("the room of a page holds %d nodes", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
