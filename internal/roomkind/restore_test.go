package roomkind_test

import (
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/roomkind"
)

func TestRestoreSavesWhatTheRoomHeldFirstThenShowsTheRestoredVersionLiveAndAddsOneVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, alice, "one")
	e.clock.Advance(roomkind.Quiet)
	e.sweep() // version 2
	watcher := e.connect(id, bob)
	e.edit(id, carol, "unsaved")

	before := e.contents(id)

	if _, err := e.svc.Restore(t.Context(), e.svc.Viewer("alice"), id, 1, 0); err != nil {
		t.Fatalf("restore: %v", err)
	}

	list := e.versions(id)
	if len(list) != 4 {
		t.Fatalf("%d versions after the restore, want the 2 before, the protective one and the restore", len(list))
	}
	for number, content := range e.contents(id) {
		if was, existed := before[number]; existed && was != content {
			t.Errorf("version %d changed", number)
		}
	}
	if got := e.versionText(id, 3); !slices.Equal(got, []string{"first paragraph", "one", "unsaved"}) {
		t.Errorf("the protective version holds %v: the work nobody had saved", got)
	}
	if v, _ := e.st.Version(t.Context(), id, 3); !slices.Equal(v.Authors, []string{"carol"}) {
		t.Errorf("the protective version is by %v, who edited", v.Authors)
	}
	restored, _ := e.st.Version(t.Context(), id, 4)
	if restored.RestoredFrom != 1 || restored.Author != "alice" || restored.Content != before[1] {
		t.Errorf("version 4 = restoredFrom %d by %s, want a copy of version 1 by alice", restored.RestoredFrom, restored.Author)
	}

	want := []string{"first paragraph"}
	if got := e.roomText(id); !slices.Equal(got, want) {
		t.Errorf("the room holds %v after the restore", got)
	}
	if got := watcher.texts(); !slices.Equal(got, want) {
		t.Errorf("someone who was in the room sees %v: the restore was not live", got)
	}

	// The room and the newest version agree, so being quiet adds nothing.
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 4 {
		t.Errorf("%d versions after the room went quiet, want 4", n)
	}
}

func TestRestoringWhenTheRoomHoldsNothingNewAddsExactlyOneVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, alice, "one")
	e.clock.Advance(roomkind.Quiet)
	e.sweep() // version 2; the room holds nothing newer

	if _, err := e.svc.Restore(t.Context(), e.svc.Viewer("alice"), id, 1, 2); err != nil {
		t.Fatalf("restore with the base the caller saw: %v", err)
	}
	if n := len(e.versions(id)); n != 3 {
		t.Errorf("%d versions, want 3", n)
	}
	if got := e.roomText(id); !slices.Equal(got, []string{"first paragraph"}) {
		t.Errorf("the room holds %v", got)
	}
}

func TestRestoringADocNobodyHasOpenedLiveNeedsNoRoom(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	if _, err := e.svc.Update(t.Context(), e.svc.Viewer("alice"), id, publishInput("# Plan\n\nsecond\n", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Restore(t.Context(), e.svc.Viewer("alice"), id, 1, 0); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.st.RealtimeRows(t.Context(), id); rows != 0 {
		t.Errorf("a restore opened a room: %d rows", rows)
	}
	// When someone does open it, it starts from the restored version.
	if got := e.roomText(id); !slices.Equal(got, []string{"first paragraph"}) {
		t.Errorf("the room started with %v", got)
	}
}

func TestARestoreIsNotRefusedBecauseTheRoomHoldsSomethingInvalid(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.writeLink(id, alice, "javascript:alert(1)")
	if _, err := e.svc.Restore(t.Context(), e.svc.Viewer("alice"), id, 1, 0); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n := len(e.versions(id)); n != 2 {
		t.Errorf("%d versions: the invalid content is not kept, only the restore is added", n)
	}
	if got := e.roomText(id); !slices.Equal(got, []string{"first paragraph"}) {
		t.Errorf("the room holds %v", got)
	}
}

// contents is the content of every version of an artifact, by number.
func (e *env) contents(id string) map[int]string {
	e.t.Helper()
	out := map[int]string{}
	for _, v := range e.versions(id) {
		full, err := e.st.Version(e.t.Context(), id, v.Number)
		if err != nil {
			e.t.Fatal(err)
		}
		out[v.Number] = full.Content
	}
	return out
}
