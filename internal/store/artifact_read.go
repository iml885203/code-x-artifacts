package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Artifact returns one page with its content.
func (s *Store) Artifact(ctx context.Context, id string) (Artifact, error) {
	a := Artifact{ID: id}
	var created, updated int64
	var source Source
	err := s.db.QueryRowContext(ctx, `SELECT a.title, a.kind, a.format, a.html, a.author, COALESCE(u.display_name, ''), a.visibility, a.organisation_edit, a.version, a.copied_from, a.copied_from_title, a.created_at, a.updated_at FROM artifacts a LEFT JOIN users u ON u.username = a.author WHERE a.id = ?`, id).
		Scan(&a.Title, &a.Kind, &a.Format, &a.Content, &a.Author, &a.AuthorDisplayName, &a.Visibility, &a.OrganisationEdit, &a.Version, &source.ID, &source.Title, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, err
	}
	editors, err := s.editors(ctx, id)
	if err != nil {
		return Artifact{}, err
	}
	a.AuthorDisplayName = authorLabel(a.Author, a.AuthorDisplayName)
	a.Editors = append([]string{}, editors[id]...)
	if source.ID != "" {
		a.CopiedFrom = &source
	}
	a.CreatedAt, a.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	return a, nil
}

// Artifacts lists every page without content, most recently updated first,
// It does not filter by viewer: the web layer's access rules decide who sees what.
func (s *Store) Artifacts(ctx context.Context) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.title, a.kind, a.format, a.author, COALESCE(u.display_name, ''), a.visibility, a.organisation_edit, a.version, a.created_at, a.updated_at FROM artifacts a LEFT JOIN users u ON u.username = a.author ORDER BY a.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Artifact{}
	for rows.Next() {
		var a Artifact
		var created, updated int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Kind, &a.Format, &a.Author, &a.AuthorDisplayName, &a.Visibility, &a.OrganisationEdit, &a.Version, &created, &updated); err != nil {
			return nil, err
		}
		a.AuthorDisplayName = authorLabel(a.Author, a.AuthorDisplayName)
		a.CreatedAt, a.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
		list = append(list, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	editors, err := s.editors(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Editors = append([]string{}, editors[list[i].ID]...)
	}
	return list, nil
}

func authorLabel(account, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return account
}
