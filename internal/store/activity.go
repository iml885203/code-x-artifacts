package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// TokenProvenance contains authenticated nonsecret identity and optional self-reported client metadata.
// TokenID is independent of the credential's hash; Client is never proof of a process identity.
type TokenProvenance struct {
	Owner         string
	TokenID       string
	TokenLabel    string
	Transport     string
	Client        string
	ClientVersion string
}

// AuthenticateToken resolves a credential once, without returning it or its hash.
func (s *Store) AuthenticateToken(ctx context.Context, plain string) (TokenProvenance, error) {
	var p TokenProvenance
	err := s.db.QueryRowContext(ctx, `SELECT username,id,name FROM tokens WHERE hash=?`, hashToken(plain)).Scan(&p.Owner, &p.TokenID, &p.TokenLabel)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

type (
	activityKey   struct{}
	activityWrite struct {
		Provenance    TokenProvenance
		ID, Operation string
		At            time.Time
	}
)

// ActivityContext admits one logical content mutation. Nested persistence shares its event ID,
// so a multi-update room mutation and its saved version cannot create duplicate history rows.
func ActivityContext(ctx context.Context, p TokenProvenance, operation string, at time.Time) context.Context {
	return context.WithValue(ctx, activityKey{}, activityWrite{Provenance: p, ID: newID(), Operation: operation, At: at.UTC()})
}

// ActivityEntry stores references only. Titles are resolved under current access at the service boundary.
type ActivityEntry struct {
	ID            string    `json:"id"`
	TokenID       string    `json:"tokenID"`
	TokenLabel    string    `json:"tokenLabel"`
	Revoked       bool      `json:"revoked"`
	Transport     string    `json:"transport"`
	Client        string    `json:"client"`
	ClientVersion string    `json:"clientVersion,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
	Operation     string    `json:"operation"`
	ArtifactID    string    `json:"-"`
	Version       int       `json:"-"`
}

// ActivityFilter uses indexed owner/token/time keys; callers validate its bounded limit.
type ActivityFilter struct {
	TokenID    string
	Limit      int
	BeforeID   string
	BeforeTime time.Time
}

// Activity lists an owner's bounded history. The service validates limits and hides inaccessible targets.
func (s *Store) Activity(ctx context.Context, owner string, f ActivityFilter, now time.Time) ([]ActivityEntry, error) {
	query := `SELECT a.id,a.token_id,a.token_label,NOT EXISTS(SELECT 1 FROM tokens t WHERE t.id=a.token_id AND t.username=a.owner),a.transport,a.client,a.client_version,a.occurred_at,a.operation,a.artifact_id,a.version FROM agent_activity a WHERE a.owner=? AND a.occurred_at>=?`
	args := []any{owner, now.Add(-30 * 24 * time.Hour).UnixMilli()}
	if f.TokenID != "" {
		query += ` AND a.token_id=?`
		args = append(args, f.TokenID)
	}
	if f.BeforeID != "" {
		query += ` AND (a.occurred_at < ? OR (a.occurred_at = ? AND a.id < ?))`
		args = append(args, f.BeforeTime.UnixMilli(), f.BeforeTime.UnixMilli(), f.BeforeID)
	}
	query += ` ORDER BY a.occurred_at DESC,a.id DESC LIMIT ?`
	args = append(args, f.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		var at int64
		if err = rows.Scan(&e.ID, &e.TokenID, &e.TokenLabel, &e.Revoked, &e.Transport, &e.Client, &e.ClientVersion, &at, &e.Operation, &e.ArtifactID, &e.Version); err != nil {
			return nil, err
		}
		e.OccurredAt = time.UnixMilli(at).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// ActivityToken is a live or recently used revoked token, without authentication material.
type ActivityToken struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Revoked       bool       `json:"revoked"`
	LastUsed      *time.Time `json:"lastUsed,omitempty"`
	LastConnected *time.Time `json:"lastConnected,omitempty"`
}

// ActivityTokens includes only owned live or retained revoked credential references.
func (s *Store) ActivityTokens(ctx context.Context, owner string, now time.Time) ([]ActivityToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.name,0,u.last_used,u.last_connected FROM tokens t LEFT JOIN agent_token_usage u ON u.owner=t.username AND u.token_id=t.id AND u.last_used>=? WHERE t.username=?
 UNION ALL SELECT u.token_id,u.token_label,1,u.last_used,u.last_connected FROM agent_token_usage u WHERE u.owner=? AND u.last_used>=? AND NOT EXISTS(SELECT 1 FROM tokens t WHERE t.id=u.token_id AND t.username=u.owner) ORDER BY 2,1 LIMIT 1000`, now.Add(-30*24*time.Hour).UnixMilli(), owner, owner, now.Add(-30*24*time.Hour).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := []ActivityToken{}
	for rows.Next() {
		var t ActivityToken
		var used, connected sql.NullInt64
		if err = rows.Scan(&t.ID, &t.Name, &t.Revoked, &used, &connected); err != nil {
			return nil, err
		}
		if used.Valid {
			v := time.UnixMilli(used.Int64).UTC()
			t.LastUsed = &v
		}
		if connected.Valid && connected.Int64 >= now.Add(-30*24*time.Hour).UnixMilli() {
			v := time.UnixMilli(connected.Int64).UTC()
			t.LastConnected = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// BoundedLabel permits only short printable metadata, never unbounded protocol strings.
func BoundedLabel(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	chars := []rune(value)
	if len(chars) > limit {
		chars = chars[:limit]
	}
	return string(chars)
}

func recordActivity(ctx context.Context, tx *sql.Tx, artifactID string, version int) error {
	w, ok := ctx.Value(activityKey{}).(activityWrite)
	if !ok || w.Provenance.TokenID == "" {
		return nil
	}
	switch w.Operation {
	case "publish", "restore", "doc_edit", "board_edit", "connect":
	default:
		return fmt.Errorf("unsupported activity operation")
	}
	p := w.Provenance
	if p.Owner == "" || p.Transport != "http" && p.Transport != "mcp" {
		return fmt.Errorf("invalid activity provenance")
	}
	var bucket any
	if w.Operation == "connect" {
		bucket = w.At.Unix() / 60
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_activity(id,owner,token_id,token_label,transport,client,client_version,occurred_at,operation,artifact_id,version,connection_bucket) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, w.ID, p.Owner, p.TokenID, BoundedLabel(p.TokenLabel, 60), p.Transport, BoundedLabel(p.Client, 60), BoundedLabel(p.ClientVersion, 40), w.At.UnixMilli(), w.Operation, artifactID, version, bucket)
	if err != nil {
		return err
	}
	if err = touchUsage(ctx, tx, p, w.At, w.Operation == "connect"); err != nil {
		return err
	}
	return pruneOwnerActivity(ctx, tx, p.Owner, w.At)
}

func touchUsage(ctx context.Context, tx *sql.Tx, p TokenProvenance, at time.Time, connected bool) error {
	var last any
	if connected {
		last = at.UnixMilli()
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_token_usage(owner,token_id,token_label,last_used,last_connected) VALUES(?,?,?,?,?) ON CONFLICT(owner,token_id) DO UPDATE SET token_label=excluded.token_label,last_used=MAX(last_used,excluded.last_used),last_connected=CASE WHEN excluded.last_connected IS NULL THEN last_connected ELSE MAX(COALESCE(last_connected,0),excluded.last_connected) END WHERE excluded.last_used-last_used>=60000 OR excluded.last_connected IS NOT NULL`, p.Owner, p.TokenID, BoundedLabel(p.TokenLabel, 60), at.UnixMilli(), last)
	return err
}

func pruneOwnerActivity(ctx context.Context, tx *sql.Tx, owner string, at time.Time) error {
	cutoff := at.Add(-30 * 24 * time.Hour).UnixMilli()
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_activity WHERE owner=? AND (occurred_at<? OR id IN(SELECT id FROM agent_activity WHERE owner=? ORDER BY occurred_at DESC,id DESC LIMIT -1 OFFSET 1000))`, owner, cutoff, owner); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM agent_token_usage WHERE owner=? AND (last_used<? OR token_id IN(SELECT token_id FROM agent_token_usage WHERE owner=? ORDER BY last_used DESC,token_id DESC LIMIT -1 OFFSET 1000))`, owner, cutoff, owner)
	return err
}

// RecordConnection coalesces a successful initialize in its one-minute client bucket.
func (s *Store) RecordConnection(ctx context.Context, p TokenProvenance, at time.Time) error {
	ctx = ActivityContext(ctx, p, "connect", at)
	return s.inTx(ctx, func(tx *sql.Tx) error { return recordActivity(ctx, tx, "", 0) })
}

// TouchTokenUse coalesces successful read use; it never creates a history row.
func (s *Store) TouchTokenUse(ctx context.Context, p TokenProvenance, at time.Time) error {
	// Most authenticated reads fall inside the persisted throttle window. Keep that path read-only;
	// the conditional upsert below still arbitrates concurrent callers when the window expires.
	var last int64
	err := s.db.QueryRowContext(ctx, `SELECT last_used FROM agent_token_usage WHERE owner=? AND token_id=?`, p.Owner, p.TokenID).Scan(&last)
	if err == nil && at.UnixMilli()-last < 60000 {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := touchUsage(ctx, tx, p, at, false); err != nil {
			return err
		}
		return pruneOwnerActivity(ctx, tx, p.Owner, at)
	})
}

// SweepActivity removes expired metadata through the server's existing housekeeping loop.
func (s *Store) SweepActivity(ctx context.Context, at time.Time) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_activity WHERE occurred_at<?`, at.Add(-30*24*time.Hour).UnixMilli()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM agent_token_usage WHERE last_used<?`, at.Add(-30*24*time.Hour).UnixMilli())
		return err
	})
}

// WithoutActivity prevents protective/background snapshots from taking credit for the caller's next edit.
func WithoutActivity(ctx context.Context) context.Context {
	return context.WithValue(ctx, activityKey{}, nil)
}

// HasActivityToken does not reveal foreign or expired revoked references, including when the summary list is capped.
func (s *Store) HasActivityToken(ctx context.Context, owner, id string, at time.Time) (bool, error) {
	var found bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tokens WHERE username=? AND id=?) OR EXISTS(SELECT 1 FROM agent_activity WHERE owner=? AND token_id=? AND occurred_at>=?) OR EXISTS(SELECT 1 FROM agent_token_usage WHERE owner=? AND token_id=? AND last_used>=?)`, owner, id, owner, id, at.Add(-30*24*time.Hour).UnixMilli(), owner, id, at.Add(-30*24*time.Hour).UnixMilli()).Scan(&found)
	return found, err
}
