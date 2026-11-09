package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DirectoryPerson is lookup metadata, never an authenticated profile or grant.
type DirectoryPerson struct {
	Username    string
	DisplayName string
}

// DirectorySnapshot describes one bounded read, including unsuccessful reads.
// StartedAt orders concurrent sign-ins; only Complete reads replace membership.
type DirectorySnapshot struct {
	Scope     string
	StartedAt int64
	Complete  bool
	People    []DirectoryPerson
}

// DirectoryStatus tells a picker whether its cached roster was refreshed fully.
type DirectoryStatus struct {
	Complete  bool  `json:"complete"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}

var directoryAccount = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// ValidDirectoryPerson bounds the metadata that can become a stored mention.
func ValidDirectoryPerson(p DirectoryPerson) bool {
	if !directoryAccount.MatchString(p.Username) {
		return false
	}
	if strings.TrimSpace(p.DisplayName) == "" || !utf8.ValidString(p.DisplayName) || utf8.RuneCountInString(p.DisplayName) > 100 {
		return false
	}
	for _, r := range p.DisplayName {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// RecordDirectory atomically replaces only a complete, validated generation.
// An incomplete read changes freshness, never names or membership. Older reads
// cannot overwrite a later-started read, even when their network request ends last.
func (s *Store) RecordDirectory(ctx context.Context, d DirectorySnapshot) error {
	if err := validateDirectorySnapshot(d); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin directory generation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var previous int64
	err = tx.QueryRowContext(ctx, "SELECT attempted_at FROM directory_status WHERE id=1").Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read directory generation: %w", err)
	}
	if previous >= d.StartedAt {
		return nil
	}
	if d.Complete {
		if _, err = tx.ExecContext(ctx, "UPDATE directory_people SET active=0"); err != nil {
			return fmt.Errorf("retain directory names: %w", err)
		}
		for _, p := range d.People {
			if _, err = tx.ExecContext(ctx, `INSERT INTO directory_people(username,display_name,active) VALUES(?,?,1) ON CONFLICT(username) DO UPDATE SET display_name=excluded.display_name,active=1`, p.Username, p.DisplayName); err != nil {
				return fmt.Errorf("record directory person: %w", err)
			}
		}
	}
	updatedAt := int64(0)
	if d.Complete {
		updatedAt = d.StartedAt / 1_000_000_000
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO directory_status(id,scope,snapshot_scope,attempted_at,updated_at,complete) VALUES(1,?,?,?, ?,?)
 ON CONFLICT(id) DO UPDATE SET scope=excluded.scope,attempted_at=excluded.attempted_at,complete=excluded.complete,
 snapshot_scope=CASE WHEN excluded.complete THEN excluded.scope ELSE directory_status.snapshot_scope END,
 updated_at=CASE WHEN excluded.complete THEN excluded.updated_at ELSE directory_status.updated_at END`, d.Scope, d.Scope, d.StartedAt, updatedAt, d.Complete)
	if err != nil {
		return fmt.Errorf("record directory freshness: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit directory generation: %w", err)
	}
	return nil
}

// DirectoryState reports incomplete until a full read, and after a failed read.
func (s *Store) DirectoryState(ctx context.Context) (DirectoryStatus, error) {
	var d DirectoryStatus
	err := s.db.QueryRowContext(ctx, "SELECT complete,updated_at FROM directory_status WHERE id=1").Scan(&d.Complete, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return d, fmt.Errorf("read directory freshness: %w", err)
	}
	return d, nil
}

func validateDirectorySnapshot(d DirectorySnapshot) error {
	if d.Scope == "" || len(d.Scope) > 128 || d.StartedAt <= 0 {
		return errors.New("invalid directory generation")
	}
	if d.Complete {
		if len(d.People) == 0 || len(d.People) > 10000 {
			return errors.New("invalid directory size")
		}
		seen := map[string]bool{}
		for _, p := range d.People {
			if !ValidDirectoryPerson(p) || seen[p.Username] {
				return errors.New("invalid or duplicate directory person")
			}
			seen[p.Username] = true
		}
	}
	return nil
}
