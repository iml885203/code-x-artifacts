package store

import (
	"context"
	"database/sql"
	"time"
)

// UsageKeptDays is how many days of usage counts are kept; the housekeeping
// sweep deletes older ones.
const UsageKeptDays = 400

// What a usage count counts, and through what a publish came.
const (
	UsageOpen    = "open"
	UsagePublish = "publish"
	ViaWeb       = "web"
	ViaAgent     = "agent"
)

// UsageDay is the day a usage count at that time belongs to.
func UsageDay(at time.Time) string { return at.UTC().Format(time.DateOnly) }

// UsageEntry is one person's count of one action on one kind on one day, with
// the department AD reported at that person's latest sign-in ("" when it
// reported none, or for the anonymous viewer, whose Username is "").
type UsageEntry struct {
	Day, Username, Department, Kind, Action, Via string
	Count                                        int
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func countUsage(ctx context.Context, ex execer, at time.Time, username, kind, action, via string) error {
	_, err := ex.ExecContext(ctx, `
		INSERT INTO usage_daily (day, username, kind, action, via, count) VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT (day, username, kind, action, via) DO UPDATE SET count = count + 1`,
		UsageDay(at), username, kind, action, via)
	return err
}

// CountOpen counts one open of an artifact of the kind by the person, or by
// the anonymous viewer when username is "".
func (s *Store) CountOpen(ctx context.Context, username, kind string, at time.Time) error {
	return countUsage(ctx, s.db, at, username, kind, UsageOpen, "")
}

type throughAgentKey struct{}

// ThroughAgent marks what is saved under ctx as made with a personal token,
// for the usage counts to tell an agent's publish from one made in the browser.
func ThroughAgent(ctx context.Context) context.Context {
	return context.WithValue(ctx, throughAgentKey{}, true)
}

// publishVia is how a new version came: through an agent when a personal
// token made the request or an agent wrote what a room's version holds.
func publishVia(ctx context.Context, rev Revision) string {
	if agent, _ := ctx.Value(throughAgentKey{}).(bool); agent || len(rev.Agents) > 0 {
		return ViaAgent
	}
	return ViaWeb
}

// Usage lists every count from the first day to the last, both included
// (YYYY-MM-DD).
func (s *Store) Usage(ctx context.Context, first, last string) ([]UsageEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.day, u.username, COALESCE(p.department, ''), u.kind, u.action, u.via, u.count
		FROM usage_daily u LEFT JOIN users p ON p.username = u.username AND u.username != ''
		WHERE u.day >= ? AND u.day <= ?`, first, last)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := []UsageEntry{}
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.Day, &e.Username, &e.Department, &e.Kind, &e.Action, &e.Via, &e.Count); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SweepUsage deletes the counts of days more than UsageKeptDays before at.
func (s *Store) SweepUsage(ctx context.Context, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM usage_daily WHERE day < ?`, UsageDay(at.AddDate(0, 0, -UsageKeptDays)))
	return err
}
