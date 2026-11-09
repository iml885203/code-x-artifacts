package realtime_test

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

func TestAnIdleRoomIsUnloadedAfterItsTimeAndComesBackFromDisk(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.IdleAfter = 5 * time.Minute })
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "still here")
	a.typeAt(10, " after")

	// With someone in it the room stays, however long it is quiet.
	e.clock.Advance(time.Hour)
	e.hub.Sweep(t.Context())
	if got := e.hub.Loaded(); got != 1 {
		t.Fatalf("a room with a connection was unloaded: %d loaded", got)
	}

	a.peer.Close()
	e.clock.Advance(4*time.Minute + 59*time.Second)
	e.hub.Sweep(t.Context())
	if got := e.hub.Loaded(); got != 1 {
		t.Fatalf("the room was unloaded %s after the last connection left, before its time", 4*time.Minute+59*time.Second)
	}
	e.clock.Advance(time.Second)
	e.hub.Sweep(t.Context())
	if got := e.hub.Loaded(); got != 0 {
		t.Fatalf("an idle room is still loaded: %d", got)
	}
	if got := e.rows(room); got != 1 {
		t.Errorf("unloading left %d rows in the log, want it compacted to 1", got)
	}

	b := e.connect(room, bob)
	settle(t, b)
	if b.text() != "still here after" {
		t.Errorf("the room came back as %q", b.text())
	}
}

func TestTheLogOfARoomThatStaysOpenIsCompactedOnASchedule(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) {
		c.CompactAfter = 1000
		c.CompactEvery = 10 * time.Minute
	})
	room := e.room()
	a := e.connect(room, alice)
	for i := range 6 {
		a.typeAt(i, "x")
	}
	e.clock.Advance(9 * time.Minute)
	e.hub.Sweep(t.Context())
	if got := e.rows(room); got != 6 {
		t.Fatalf("compacted before its time: %d rows", got)
	}
	e.clock.Advance(time.Minute)
	e.hub.Sweep(t.Context())
	if got := e.rows(room); got != 1 {
		t.Fatalf("the log has %d rows after the scheduled compaction, want 1", got)
	}
	// Edits go on from the compacted log.
	a.typeAt(6, "y")
	e.start()
	if got := e.text(room); got != "xxxxxxy" {
		t.Errorf("room = %q", got)
	}
}

func TestClosingTheHubDisconnectsEveryoneAndCompacts(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil)
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "a")
	a.typeAt(1, "b")
	e.hub.Close(t.Context())
	if !errors.Is(a.peer.Err(), realtime.ErrShutdown) {
		t.Errorf("connection ended with %v, want ErrShutdown", a.peer.Err())
	}
	if got := e.rows(room); got != 1 {
		t.Errorf("closing left %d rows, want 1", got)
	}
	if _, err := e.hub.Join(t.Context(), room, alice, ""); !errors.Is(err, realtime.ErrShutdown) {
		t.Errorf("join after close = %v, want ErrShutdown", err)
	}
}

// An unloaded room must be unreachable, not just unlisted: with weak pointers
// the garbage collector decides, so this proves nothing holds the room.
func TestAnUnloadedRoomsMemoryIsReleased(t *testing.T) {
	t.Parallel()
	e := newEnv(t, func(c *realtime.Config) { c.IdleAfter = time.Minute })
	room := e.room()
	a := e.connect(room, alice)
	a.typeAt(0, "text that lives in the room's document")
	a.peer.Close()

	ref := realtime.LoadedRoomForTest(e.hub, room)
	if ref.Value() == nil {
		t.Fatal("the room is not loaded")
	}
	e.clock.Advance(time.Minute)
	e.hub.Sweep(t.Context())
	if e.hub.Loaded() != 0 {
		t.Fatal("the room was not unloaded")
	}
	// The client and its peer were the last things that could point at it.
	a.peer, a.doc = nil, nil
	for range 10 {
		runtime.GC()
		if ref.Value() == nil {
			return
		}
	}
	t.Error("the room is unloaded but still in memory")
}
