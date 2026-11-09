package store

import (
	"path/filepath"
	"testing"
	"time"
)

// openedThree is a store with three artifacts that bob opened in order, all in
// the same instant, keeping at most two, and the first of them opened by carol.
func openedThree(t *testing.T) (*Store, []string) {
	t.Helper()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	t.Cleanup(func() { _ = st.Close() })
	var ids []string
	for range 3 {
		a, err := st.CreateArtifact(ctx, "t", "html", "", "<p>", "alice", VisibilityOrganisation)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.ID)
	}
	at := time.Unix(1_800_000_000, 0)
	for _, id := range ids {
		if err := st.RecordOpen(ctx, "bob", id, at, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.RecordOpen(ctx, "carol", ids[0], at, 2); err != nil {
		t.Fatal(err)
	}
	return st, ids
}

func TestAPersonKeepsOnlyTheirNewestOpensAndALaterOpenAlwaysSortsAfter(t *testing.T) {
	t.Parallel()
	st, ids := openedThree(t)
	opened, err := st.Opened(t.Context(), "bob", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != 2 {
		t.Fatalf("bob keeps %d opens, want the newest 2: %v", len(opened), opened)
	}
	if _, kept := opened[ids[0]]; kept {
		t.Error("bob's oldest open was kept past the bound")
	}
	if !opened[ids[2]].After(opened[ids[1]]) {
		t.Errorf("opens made in the same instant do not keep their order: %v", opened)
	}
	if theirs, err := st.Opened(t.Context(), "carol", ""); err != nil || len(theirs) != 1 {
		t.Errorf("bob's opens pushed out carol's: %v %v", theirs, err)
	}
	if one, err := st.Opened(t.Context(), "bob", ids[2]); err != nil || len(one) != 1 {
		t.Errorf("narrowing to one artifact: %v %v", one, err)
	}
}

func TestADeletedArtifactTakesItsOpensWithIt(t *testing.T) {
	t.Parallel()
	st, ids := openedThree(t)
	if err := st.DeleteArtifact(t.Context(), ids[2]); err != nil {
		t.Fatal(err)
	}
	if opened, err := st.Opened(t.Context(), "bob", ""); err != nil || len(opened) != 1 {
		t.Errorf("a deleted artifact keeps its opens: %v %v", opened, err)
	}
}
