package roomkind_test

import (
	"slices"
	"testing"
	"time"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/roomkind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

const justUnderQuiet = roomkind.Quiet - time.Second

func TestAVersionIsCutTwoMinutesAfterTheLastEditAndAnEditInBetweenRestartsTheWait(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, alice, "one")

	e.clock.Advance(time.Minute)
	e.edit(id, alice, "two") // restarts the wait
	e.clock.Advance(justUnderQuiet)
	e.sweep()
	if n := len(e.versions(id)); n != 1 {
		t.Fatalf("%v after the second edit there are %d versions, want 1", justUnderQuiet, n)
	}
	e.clock.Advance(time.Second)
	e.sweep()
	list := e.versions(id)
	if len(list) != 2 {
		t.Fatalf("2 minutes after the last edit there are %d versions, want 2", len(list))
	}
	if got := e.versionText(id, 2); !slices.Equal(got, []string{"first paragraph", "one", "two"}) {
		t.Errorf("the version holds %v, the two edits made in between", got)
	}
}

func TestAnUneditedRoomNeverBecomesAVersionHoweverLongItIsOpen(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	c := e.connect(id, alice)
	_ = c.texts()
	e.clock.Advance(time.Hour)
	e.sweep()
	if n := len(e.versions(id)); n != 1 {
		t.Errorf("%d versions after an hour of nobody typing", n)
	}
}

func TestASnapshotHoldsWhatTheRoomHoldsAndAnIdenticalOneWritesNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.edit(id, alice, "one")
	e.clock.Advance(roomkind.Quiet)
	e.sweep()

	latest, err := e.st.Artifact(t.Context(), id)
	if err != nil || latest.Version != 2 {
		t.Fatalf("latest = %d, %v", latest.Version, err)
	}
	var inRoom string
	if err := e.hub.Read(t.Context(), id, func(doc *crdt.Doc) (err error) {
		inRoom, err = docroom.Codec{}.Content(doc)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if latest.Content != inRoom {
		t.Errorf("the version differs from the room:\n version: %s\n    room: %s", latest.Content, inRoom)
	}

	// The room is written with what it already holds: an edit, and no change.
	err = e.hub.Edit(t.Context(), id, bob, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return docroom.Codec{}.Write(doc, latest.Content)
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 2 {
		t.Errorf("%d versions after an edit that changed nothing, want 2", n)
	}
	pending, _ := e.st.PendingRooms(t.Context())
	if len(pending) != 0 {
		t.Errorf("the room is still waiting for a version: %v", pending)
	}
}

func TestTheAuthorsOfAVersionAreTheIdentitiesThatEditedAndNotTheNamesTheyGaveThemselves(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	a := e.connect(id, alice)
	b := e.connect(id, bob)
	reader := e.connect(id, dave)

	a.typeParagraph("from alice")
	b.callThemselves("mallory")
	b.typeParagraph("from bob")
	a.typeParagraph("alice again")
	reader.callThemselves("alice")
	reader.typeParagraph("from a reader") // dropped: a reader's writes never reach the room
	e.edit(id, carol, "from the agent")

	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	v, err := e.st.Version(t.Context(), id, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alice", "bob", "carol"}; !slices.Equal(v.Authors, want) {
		t.Errorf("authors = %v, want %v: who edited, in the order they first did, once each", v.Authors, want)
	}
	if v.Author != "alice" {
		t.Errorf("author = %q, want the first to edit", v.Author)
	}
	if got := e.versionText(id, 2); slices.Contains(got, "from a reader") || len(got) != 5 {
		t.Errorf("the version holds %v", got)
	}

	// The next version starts again from nobody: only who edited since.
	e.edit(id, bob, "later")
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if v, err := e.st.Version(t.Context(), id, 3); err != nil || !slices.Equal(v.Authors, []string{"bob"}) {
		t.Errorf("version 3 authors = %v, %v, want only bob", v.Authors, err)
	}
}

func TestEditsWaitingWhenTheLastEditorLeavesBecomeAVersionWhenTheRoomIsQuiet(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	c := e.connect(id, alice)
	c.typeParagraph("typed")
	c.peer.Close()

	e.clock.Advance(justUnderQuiet)
	e.sweep()
	if n := len(e.versions(id)); n != 1 {
		t.Fatalf("%d versions before the room is quiet", n)
	}
	e.clock.Advance(time.Second)
	e.sweep()
	if got := e.versionText(id, 2); !slices.Equal(got, []string{"first paragraph", "typed"}) {
		t.Errorf("version 2 holds %v", got)
	}
}

func TestEditsWaitingAtShutdownSurviveARestartAndBecomeAVersionAfterTheNextPause(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	c := e.connect(id, alice)
	c.typeParagraph("before the restart")
	e.edit(id, carol, "agent before the restart")

	e.restart()
	if got := e.roomText(id); !slices.Equal(got, []string{"first paragraph", "before the restart", "agent before the restart"}) {
		t.Fatalf("after the restart the room holds %v: an accepted edit is not lost", got)
	}
	e.sweep() // the snapshotter sees the waiting room for the first time
	if n := len(e.versions(id)); n != 1 {
		t.Fatalf("%d versions right after the restart", n)
	}
	e.clock.Advance(justUnderQuiet)
	e.sweep()
	if n := len(e.versions(id)); n != 1 {
		t.Fatalf("%d versions before the pause is over", n)
	}
	e.clock.Advance(time.Second)
	e.sweep()
	v, err := e.st.Version(t.Context(), id, 2)
	if err != nil || !slices.Equal(v.Authors, []string{"alice", "carol"}) {
		t.Fatalf("version 2 = %+v, %v; want the two who edited before the restart", v, err)
	}
	if got := e.versionText(id, 2); len(got) != 3 {
		t.Errorf("version 2 holds %v", got)
	}
}

func TestARoomOfAKindThatHasNoCodecIsNeverTurnedIntoAVersion(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	p, err := e.svc.Create(t.Context(), e.svc.Viewer("alice"), service.PublishInput{Title: "Page", Content: "<p>x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.MarkPending(t.Context(), p.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	e.sweep()
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(p.ID)); n != 1 {
		t.Errorf("a page has %d versions", n)
	}
	if pending, _ := e.st.PendingRooms(t.Context()); len(pending) != 0 {
		t.Errorf("the page is still waiting: %v", pending)
	}
}

func TestARoomHoldingSomethingADocMayNotHoldIsNotKeptAndTriesAgainLater(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.writeLink(id, alice, "javascript:alert(1)")

	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 1 {
		t.Fatalf("a link that runs script became version %d", n)
	}
	if pending, _ := e.st.PendingRooms(t.Context()); len(pending) != 1 {
		t.Errorf("the room should still be waiting: %v", pending)
	}
	// Fixed by the next edit, it is kept.
	e.writeLink(id, alice, "https://example.com")
	e.clock.Advance(roomkind.Quiet)
	e.sweep()
	if n := len(e.versions(id)); n != 2 {
		t.Errorf("%d versions after the link was fixed, want 2", n)
	}
}

// versionText is the paragraph text of a version.
func (e *env) versionText(id string, number int) []string {
	e.t.Helper()
	v, err := e.st.Version(e.t.Context(), id, number)
	if err != nil {
		e.t.Fatal(err)
	}
	d, err := docfmt.Parse(v.Content)
	if err != nil {
		e.t.Fatal(err)
	}
	return paragraphs(d)
}

func TestARoomThatCannotBeSavedIsTriedLessAndLessOftenAndLogsAnErrorOnlyOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.doc()
	e.writeLink(id, alice, "javascript:alert(1)")

	// Quiet, then 2x, 4x, 8x Quiet between tries, checked a minute at a time.
	var triedAt []time.Duration
	var elapsed time.Duration
	e.clock.Advance(roomkind.Quiet)
	elapsed += roomkind.Quiet
	last := 0
	for elapsed < 40*time.Minute {
		e.sweep()
		if tries, _ := e.logs.counts(); tries != last {
			triedAt = append(triedAt, elapsed)
			last = tries
		}
		e.clock.Advance(time.Minute)
		elapsed += time.Minute
	}
	want := []time.Duration{2 * time.Minute, 6 * time.Minute, 14 * time.Minute, 30 * time.Minute}
	if !slices.Equal(triedAt, want) {
		t.Errorf("tried at %v, want %v: each wait twice the one before", triedAt, want)
	}
	if _, errs := e.logs.counts(); errs != 1 {
		t.Errorf("%d error logs, want 1", errs)
	}
}
