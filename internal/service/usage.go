package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// The periods and groupings a usage summary can have.
const (
	UsagePeriodWeek   = "week"
	UsagePeriodDay    = "day"
	UsageByDepartment = "department"
	UsageByKind       = "kind"
	// UsageUnknownDepartment is the department of someone AD reported none for;
	// UsageAnonymous is the department the anonymous opens are grouped under.
	UsageUnknownDepartment = "unknown"
	UsageAnonymous         = "anonymous"
)

// usageDefaultDays is how far back a summary reaches when it names no start.
const usageDefaultDays = 12 * 7

// SetUsageReaders names the accounts that may read usage (Viewer.CanReadUsage).
func (s *Service) SetUsageReaders(accounts []string) {
	s.usageReaders = nil
	for _, a := range accounts {
		if a = auth.NormalizeUsername(a); a != "" {
			s.usageReaders = append(s.usageReaders, a)
		}
	}
}

// UsageQuery is what a usage summary covers: the days From to To (YYYY-MM-DD,
// UTC, both included), in periods of a week or a day, grouped by none, one or
// both of department and kind.
type UsageQuery struct {
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
	Period string   `json:"period,omitempty"`
	By     []string `json:"by,omitempty"`
}

// UsageSummary is how often artifacts were opened and published. It holds
// counts only: no account, no artifact.
type UsageSummary struct {
	From   string     `json:"from"`
	To     string     `json:"to"`
	Period string     `json:"period"`
	By     []string   `json:"by"`
	Rows   []UsageRow `json:"rows"`
}

// UsageRow is one period of one group. Period is its first day (a Monday for
// a week). Department and Kind are set only when the summary is grouped by
// them. Opens are by people signed in; AnonymousOpens by public-link readers
// who were not. ActivePeople opened or published, PublishingPeople published.
type UsageRow struct {
	Period           string `json:"period"`
	Department       string `json:"department,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Opens            int    `json:"opens"`
	AnonymousOpens   int    `json:"anonymousOpens"`
	Publishes        int    `json:"publishes"`
	WebPublishes     int    `json:"webPublishes"`
	AgentPublishes   int    `json:"agentPublishes"`
	ActivePeople     int    `json:"activePeople"`
	PublishingPeople int    `json:"publishingPeople"`
}

// Usage summarises how often artifacts were opened and published, for the
// accounts the deployment named to read it.
func (s *Service) Usage(ctx context.Context, v Viewer, q UsageQuery) (UsageSummary, error) {
	if !v.CanReadUsage() {
		return UsageSummary{}, refuse(Forbidden, "only the accounts this deployment names as usage readers (PICTOR_USAGE_READERS) can read usage")
	}
	q, err := s.normalizeUsageQuery(q)
	if err != nil {
		return UsageSummary{}, err
	}
	entries, err := s.store.Usage(ctx, q.From, q.To)
	if err != nil {
		return UsageSummary{}, fmt.Errorf("read usage: %w", err)
	}
	return UsageSummary{From: q.From, To: q.To, Period: q.Period, By: q.By, Rows: summarizeUsage(entries, q.Period, q.By)}, nil
}

func (s *Service) normalizeUsageQuery(q UsageQuery) (UsageQuery, error) {
	to, err := usageDate("to", cmp.Or(q.To, store.UsageDay(s.now())))
	if err != nil {
		return q, err
	}
	from, err := usageDate("from", cmp.Or(q.From, store.UsageDay(to.AddDate(0, 0, 1-usageDefaultDays))))
	if err != nil {
		return q, err
	}
	if from.After(to) {
		return q, refuse(Invalid, "from (%s) is after to (%s)", store.UsageDay(from), store.UsageDay(to))
	}
	if to.Sub(from) >= store.UsageKeptDays*24*time.Hour {
		return q, refuse(Invalid, "usage is kept for %d days; ask for at most that many days at once", store.UsageKeptDays)
	}
	period := cmp.Or(q.Period, UsagePeriodWeek)
	if period != UsagePeriodWeek && period != UsagePeriodDay {
		return q, refuse(Invalid, "period must be %q or %q, got %q", UsagePeriodWeek, UsagePeriodDay, q.Period)
	}
	by := []string{}
	for _, b := range []string{UsageByDepartment, UsageByKind} {
		if slices.Contains(q.By, b) {
			by = append(by, b)
		}
	}
	for _, b := range q.By {
		if !slices.Contains(by, b) {
			return q, refuse(Invalid, "by takes %q and %q, got %q", UsageByDepartment, UsageByKind, b)
		}
	}
	return UsageQuery{From: store.UsageDay(from), To: store.UsageDay(to), Period: period, By: by}, nil
}

func usageDate(name, value string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, refuse(Invalid, "%s must be a date as YYYY-MM-DD, got %q", name, value)
	}
	return d, nil
}

type usageKey struct{ period, department, kind string }

// summarizeUsage adds the counts of each person and day up into rows of the
// period and the groups asked for, ordered by period, then department, then
// kind. Accounts are only used to count distinct people and never leave it.
func summarizeUsage(entries []store.UsageEntry, period string, by []string) []UsageRow {
	rows := map[usageKey]*UsageRow{}
	active, publishing := map[usageKey]map[string]bool{}, map[usageKey]map[string]bool{}
	for _, e := range entries {
		k := usageKey{period: usagePeriodStart(e.Day, period)}
		if slices.Contains(by, UsageByDepartment) {
			k.department = usageDepartment(e)
		}
		if slices.Contains(by, UsageByKind) {
			k.kind = e.Kind
		}
		r := rows[k]
		if r == nil {
			r = &UsageRow{Period: k.period, Department: k.department, Kind: k.kind}
			rows[k] = r
			active[k], publishing[k] = map[string]bool{}, map[string]bool{}
		}
		switch {
		case e.Action == store.UsageOpen && e.Username == "":
			r.AnonymousOpens += e.Count
		case e.Action == store.UsageOpen:
			r.Opens += e.Count
		case e.Action == store.UsagePublish:
			r.Publishes += e.Count
			if e.Via == store.ViaAgent {
				r.AgentPublishes += e.Count
			} else {
				r.WebPublishes += e.Count
			}
			publishing[k][e.Username] = true
		}
		if e.Username != "" {
			active[k][e.Username] = true
		}
	}
	out := make([]UsageRow, 0, len(rows))
	for k, r := range rows {
		r.ActivePeople, r.PublishingPeople = len(active[k]), len(publishing[k])
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b UsageRow) int {
		return cmp.Or(cmp.Compare(a.Period, b.Period), cmp.Compare(a.Department, b.Department), cmp.Compare(a.Kind, b.Kind))
	})
	return out
}

func usageDepartment(e store.UsageEntry) string {
	switch {
	case e.Username == "":
		return UsageAnonymous
	case e.Department == "":
		return UsageUnknownDepartment
	default:
		return e.Department
	}
}

// usagePeriodStart is the first day of the period the day is in: the day
// itself, or the Monday of its week.
func usagePeriodStart(day, period string) string {
	if period != UsagePeriodWeek {
		return day
	}
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	back := (int(d.Weekday()) + 6) % 7
	return store.UsageDay(d.AddDate(0, 0, -back))
}
