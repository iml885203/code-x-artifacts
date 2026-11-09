package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUsageCountsOlderThanTheRetentionAreSweptAndNewerOnesStay(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	for _, daysAgo := range []int{UsageKeptDays + 30, UsageKeptDays + 1, UsageKeptDays, 1, 0} {
		for _, who := range []string{"bob", ""} {
			if err := st.CountOpen(ctx, who, "html", now.AddDate(0, 0, -daysAgo)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := st.SweepUsage(ctx, now); err != nil {
		t.Fatal(err)
	}
	entries, err := st.Usage(ctx, "0000-01-01", "9999-12-31")
	if err != nil {
		t.Fatal(err)
	}
	var days []string
	for _, e := range entries {
		days = append(days, e.Day)
	}
	slices.Sort(days)
	want := []string{"2025-09-30", "2025-09-30", "2026-11-03", "2026-11-03", "2026-11-04", "2026-11-04"}
	if !slices.Equal(days, want) {
		t.Errorf("days kept = %v, want %v", days, want)
	}
}

func TestUsageCountsRepeatOpensOfADayInOneRowThatOutlivesTheArtifact(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openStore(ctx, t, filepath.Join(t.TempDir(), "pictor.db"))
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreateArtifact(ctx, "t", "html", "", "<p>", "alice", VisibilityOrganisation)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUser(ctx, User{Username: "bob", DisplayName: "Bob", Department: "QA"}); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	for range 3 {
		if err := st.CountOpen(ctx, "bob", "html", at); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	entries, err := st.Usage(ctx, "0000-01-01", "9999-12-31")
	if err != nil {
		t.Fatal(err)
	}
	want := []UsageEntry{
		{Username: "alice", Kind: "html", Action: UsagePublish, Via: ViaWeb, Count: 1},
		{Username: "bob", Department: "QA", Kind: "html", Action: UsageOpen, Count: 3},
	}
	for i := range entries {
		entries[i].Day = ""
	}
	slices.SortFunc(entries, func(x, y UsageEntry) int { return strings.Compare(x.Username, y.Username) })
	if !slices.Equal(entries, want) {
		t.Errorf("usage = %+v, want %+v", entries, want)
	}
}
