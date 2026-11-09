package roomkind_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// roomBlocks is the blocks of the room's first tab.
func (e *env) roomBlocks(id string) []docfmt.Block {
	e.t.Helper()
	var out []docfmt.Block
	err := e.hub.Read(e.t.Context(), id, func(doc *crdt.Doc) error {
		d, err := docroom.Open(doc).Read()
		if err == nil {
			out = d.Tabs[0].Blocks
		}
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

func ids(blocks []docfmt.Block) []string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.ID)
	}
	return out
}

func TestAWholeDocPublishWithARoomOpenChangesTheRoomKeepsTheIdsOfUnchangedBlocksAndShowsLive(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	watcher := e.connect(id, bob)
	before := ids(e.roomBlocks(id))

	p, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nfirst paragraph changed\n\nsecond\n", 1))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if p.Version != 2 {
		t.Errorf("version = %d, want 2", p.Version)
	}
	got := e.roomBlocks(id)
	if len(got) != 3 || got[0].ID != before[0] || got[1].ID != before[1] {
		t.Errorf("block ids %v after the publish, before %v: the blocks that stayed keep their id", ids(got), before)
	}
	if want := []string{"first paragraph changed", "second"}; !slices.Equal(watcher.texts(), want) {
		t.Errorf("a client in the room sees %v, want %v", watcher.texts(), want)
	}

	// The version is what the room holds: being quiet adds nothing.
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 2 {
		t.Errorf("%d versions after the room went quiet, want 2", n)
	}
}

func TestAWholeDocPublishWithAStaleBaseIsRefusedWithTheLatestWhileARoomIsOpen(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	_ = e.roomText(id)
	alice := e.svc.Viewer("alice")
	if _, err := e.svc.Update(t.Context(), alice, id, publishInput("# Plan\n\nsecond version\n", 1)); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Update(t.Context(), alice, id, publishInput("# Plan\n\nfrom the old base\n", 1))
	var refusal *service.Error
	if !errors.As(err, &refusal) || refusal.Code != service.Conflict || refusal.Latest == nil || refusal.Latest.Version != 2 {
		t.Fatalf("publish on a stale base = %v, want a conflict that carries version 2", err)
	}
	if got := e.roomText(id); !slices.Equal(got, []string{"second version"}) {
		t.Errorf("the refused publish changed the room: %v", got)
	}
}

func TestAWholeDocPublishThatRoomHoldsUnseenWorkAgainstIsRefusedAndTheWorkIsSaved(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, bob, "typed by bob")

	_, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nfirst paragraph\n\nmine\n", 1))
	if service.CodeOf(err) != service.Conflict {
		t.Fatalf("publish = %v, want a conflict: the room held work the caller had not seen", err)
	}
	if got := e.versionText(id, 2); !slices.Equal(got, []string{"first paragraph", "typed by bob"}) {
		t.Errorf("version 2 = %v: the unseen work is saved, not lost", got)
	}
	// Told about it, the caller publishes on top of it.
	if _, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nfirst paragraph\n\ntyped by bob\n\nmine\n", 2)); err != nil {
		t.Fatalf("publish on the new base: %v", err)
	}
}

func TestAWholeDocPublishWithoutABaseSavesWhatTheRoomHeldAndThenReplacesIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, bob, "typed by bob")
	if _, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nreplaced\n", 0)); err != nil {
		t.Fatal(err)
	}
	if n := len(e.versions(id)); n != 3 {
		t.Fatalf("%d versions, want the first, bob's work and the publish", n)
	}
	if got := e.versionText(id, 2); !slices.Equal(got, []string{"first paragraph", "typed by bob"}) {
		t.Errorf("bob's work was not kept: %v", got)
	}
	if got := e.roomText(id); !slices.Equal(got, []string{"replaced"}) {
		t.Errorf("the room holds %v", got)
	}
}

func TestAPublishToADocNobodyHasOpenedLiveIsUnchangedAndOpensNoRoom(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	if _, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nsecond\n", 1)); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.st.RealtimeRows(t.Context(), id); rows != 0 {
		t.Errorf("a publish opened a room: %d rows", rows)
	}
}
