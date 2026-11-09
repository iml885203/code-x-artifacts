package service

import (
	"slices"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

func TestUsageAddsUpPerPeriodAndGroupCountingEachPersonOnce(t *testing.T) {
	entries := []store.UsageEntry{
		// Wednesday and Thursday of the week of Monday 2026-11-02.
		{Day: "2026-11-04", Username: "bob", Kind: "html", Action: store.UsageOpen, Count: 3},
		{Day: "2026-11-05", Username: "bob", Kind: "doc", Action: store.UsageOpen, Count: 1},
		{Day: "2026-11-05", Username: "alice", Department: "QA", Kind: "html", Action: store.UsagePublish, Via: store.ViaAgent, Count: 2},
		{Day: "2026-11-05", Username: "alice", Department: "QA", Kind: "html", Action: store.UsagePublish, Via: store.ViaWeb, Count: 1},
		{Day: "2026-11-05", Kind: "html", Action: store.UsageOpen, Count: 4},
		// Sunday, the last day of that week.
		{Day: "2026-11-08", Username: "alice", Department: "QA", Kind: "html", Action: store.UsageOpen, Count: 1},
		// Monday of the next week.
		{Day: "2026-11-09", Username: "alice", Department: "QA", Kind: "html", Action: store.UsageOpen, Count: 1},
	}
	cases := []struct {
		name   string
		period string
		by     []string
		want   []UsageRow
	}{
		{"one total per week", UsagePeriodWeek, nil, []UsageRow{
			{Period: "2026-11-02", Opens: 5, AnonymousOpens: 4, Publishes: 3, WebPublishes: 1, AgentPublishes: 2, ActivePeople: 2, PublishingPeople: 1},
			{Period: "2026-11-09", Opens: 1, ActivePeople: 1},
		}},
		{"per week and department", UsagePeriodWeek, []string{UsageByDepartment}, []UsageRow{
			{Period: "2026-11-02", Department: "QA", Opens: 1, Publishes: 3, WebPublishes: 1, AgentPublishes: 2, ActivePeople: 1, PublishingPeople: 1},
			{Period: "2026-11-02", Department: UsageAnonymous, AnonymousOpens: 4},
			{Period: "2026-11-02", Department: UsageUnknownDepartment, Opens: 4, ActivePeople: 1},
			{Period: "2026-11-09", Department: "QA", Opens: 1, ActivePeople: 1},
		}},
		{"per day and kind", UsagePeriodDay, []string{UsageByKind}, []UsageRow{
			{Period: "2026-11-04", Kind: "html", Opens: 3, ActivePeople: 1},
			{Period: "2026-11-05", Kind: "doc", Opens: 1, ActivePeople: 1},
			{Period: "2026-11-05", Kind: "html", AnonymousOpens: 4, Publishes: 3, WebPublishes: 1, AgentPublishes: 2, ActivePeople: 1, PublishingPeople: 1},
			{Period: "2026-11-08", Kind: "html", Opens: 1, ActivePeople: 1},
			{Period: "2026-11-09", Kind: "html", Opens: 1, ActivePeople: 1},
		}},
		{"nothing counted", UsagePeriodWeek, nil, []UsageRow{}},
	}
	for _, c := range cases {
		in := entries
		if len(c.want) == 0 {
			in = nil
		}
		if got := summarizeUsage(in, c.period, c.by); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}
