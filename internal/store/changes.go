package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// AccessChange is what an artifact's audience and editors became when somebody
// changed either: the state after the change, and who made it.
type AccessChange struct {
	By               string     `json:"by"`
	Visibility       Visibility `json:"visibility"`
	Editors          []string   `json:"editors"`
	OrganisationEdit bool       `json:"organisationEdit"`
	At               time.Time  `json:"at"`
}

// RecordAccessChange stores the state an artifact's audience and editors are in
// after by changed them.
func (s *Store) RecordAccessChange(ctx context.Context, artifactID string, c AccessChange) error {
	editors, err := json.Marshal(append([]string{}, c.Editors...))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO access_changes (artifact_id, changed_by, visibility, editors_json, organisation_edit, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		artifactID, c.By, c.Visibility, string(editors), c.OrganisationEdit, c.At.Unix())
	return err
}

// AccessChangesSince lists the changes made at or after at, oldest first.
func (s *Store) AccessChangesSince(ctx context.Context, artifactID string, at time.Time) ([]AccessChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT changed_by, visibility, editors_json, organisation_edit, created_at FROM access_changes WHERE artifact_id = ? AND created_at >= ? ORDER BY seq`, artifactID, at.Unix())
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []AccessChange{}
	for rows.Next() {
		var c AccessChange
		var editors string
		var created int64
		if err := rows.Scan(&c.By, &c.Visibility, &editors, &c.OrganisationEdit, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(editors), &c.Editors); err != nil {
			return nil, err
		}
		c.At = time.Unix(created, 0).UTC()
		list = append(list, c)
	}
	return list, rows.Err()
}

// CommentsSince lists the comments written at or after at, oldest first.
func (s *Store) CommentsSince(ctx context.Context, artifactID string, at time.Time) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE artifact_id = ? AND created_at >= ? ORDER BY seq`, artifactID, at.Unix())
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows, artifactID)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// VersionsSince lists the versions numbered above number and saved at or
// after at, without content, oldest first. Zero values leave a bound out.
func (s *Store) VersionsSince(ctx context.Context, artifactID string, number int, at time.Time) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number, title, kind, format, author, restored_from, created_at FROM artifact_versions WHERE artifact_id = ? AND number > ? AND created_at >= ? ORDER BY number`, artifactID, number, at.Unix())
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Version{}
	for rows.Next() {
		var v Version
		var created int64
		if err := rows.Scan(&v.Number, &v.Title, &v.Kind, &v.Format, &v.Author, &v.RestoredFrom, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(created, 0).UTC()
		list = append(list, v)
	}
	return list, rows.Err()
}

// VersionTime is when a version was saved.
func (s *Store) VersionTime(ctx context.Context, artifactID string, number int) (time.Time, error) {
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM artifact_versions WHERE artifact_id = ? AND number = ?`, artifactID, number).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(created, 0).UTC(), nil
}
