package store

import (
	"errors"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/realtime"
)

func TestARoomsLogHasOneEpochAndOnlyForAnArtifactThatExists(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	log := st.RealtimeLog()
	a := artifactBy(t, st, "alice")

	if _, err := log.Load(ctx, "no-such-artifact", "e0"); !errors.Is(err, realtime.ErrNoRoom) {
		t.Fatalf("load of a room without an artifact = %v, want ErrNoRoom", err)
	}
	snap, err := log.Load(ctx, a.ID, "e1")
	if err != nil || snap.Epoch != "e1" || len(snap.Updates) != 0 {
		t.Fatalf("first load = %+v, %v", snap, err)
	}
	// The epoch is the one the log was created with, whatever a later load proposes.
	if snap, err = log.Load(ctx, a.ID, "e2"); err != nil || snap.Epoch != "e1" {
		t.Fatalf("second load = %+v, %v", snap, err)
	}
}

func TestAppendsKeepTheirOrderAndNeedTheEpochOfTheLog(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	log := st.RealtimeLog()
	a := artifactBy(t, st, "alice")
	if _, err := log.Load(ctx, a.ID, "e1"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"one", "two", "three"} {
		if err := log.Append(ctx, a.ID, "e1", []byte(u)); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.Append(ctx, a.ID, "other", []byte("x")); !errors.Is(err, realtime.ErrEpochMismatch) {
		t.Errorf("append under another epoch = %v, want ErrEpochMismatch", err)
	}
	snap, err := log.Load(ctx, a.ID, "e1")
	if err != nil || len(snap.Updates) != 3 || string(snap.Updates[0]) != "one" || string(snap.Updates[2]) != "three" {
		t.Errorf("rows in append order = %q, %v", snap.Updates, err)
	}

	// Replacing the log is how it is compacted and rebuilt: one row, the epoch it names.
	if err := log.Replace(ctx, a.ID, "e3", []byte("state")); err != nil {
		t.Fatal(err)
	}
	if snap, _ = log.Load(ctx, a.ID, "e9"); snap.Epoch != "e3" || len(snap.Updates) != 1 || string(snap.Updates[0]) != "state" {
		t.Errorf("after replace = %+v", snap)
	}
	if err := log.Append(ctx, a.ID, "e1", []byte("late")); !errors.Is(err, realtime.ErrEpochMismatch) {
		t.Errorf("append under the epoch before the replace = %v, want ErrEpochMismatch", err)
	}
}

func TestARoomsLogGoesWithItsArtifact(t *testing.T) {
	ctx := t.Context()
	st := openForComments(t)
	log := st.RealtimeLog()
	a := artifactBy(t, st, "alice")
	if err := log.Replace(ctx, a.ID, "e1", []byte("state")); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.RealtimeRows(ctx, a.ID); n != 1 {
		t.Fatalf("rows = %d", n)
	}
	if err := st.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := st.RealtimeRows(ctx, a.ID); err != nil || n != 0 {
		t.Errorf("rows of a deleted artifact's room = %d, %v", n, err)
	}
}
