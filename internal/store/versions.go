package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The saved versions of an artifact: what a version is, how one is saved, and
// how the history is read.

// Version is one saved state of an artifact. The kind never differs between
// versions of the same artifact, but is kept so a version stands on its own.
type Version struct {
	// Number counts from 1 within one artifact.
	Number  int    `json:"version"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Format  string `json:"format,omitempty"`
	Content string `json:"-"`
	// Author is who saved the version: the one who published or restored it, or
	// for a version cut from a live room the first of the people who edited it.
	Author string `json:"author"`
	// Authors are everyone who edited the version, Author first; never null.
	Authors []string `json:"authors"`
	// Agents are the agents that made it, each as the person whose account it
	// worked for; never null.
	Agents []VersionAgent `json:"agents"`
	// RestoredFrom is the number of the version this one copies, or 0 when it
	// was published rather than restored.
	RestoredFrom int       `json:"restoredFrom,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	// FileCount is how many files the version has besides its page; only the
	// history list fills it. Files lists them when one version is read.
	FileCount int       `json:"fileCount,omitempty"`
	Files     []FileRef `json:"files,omitempty"`
}

// VersionAgent is an agent that made a version: the person whose account it
// acted for and what the client called itself ("agent" when it did not say).
type VersionAgent struct {
	By     string `json:"by"`
	Client string `json:"client"`
}

// FileRef is one file of a version: where it is, what it is served as and
// which blob holds its bytes (content-addressed, see internal/assets). Path is
// relative to the page.
type FileRef struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// Revision is what a save changes about an artifact.
type Revision struct {
	Title, Format, Content, Author string
	// Authors are the people who edited what this version holds, when more than
	// the one who saved it did (a version cut from a live room). Empty means
	// the Author alone.
	Authors []string
	// Agents are the agents that made it.
	Agents []VersionAgent
	// RestoredFrom is set when the revision copies an earlier version.
	RestoredFrom int
	// Files is the complete set of files of the new version, saved with it.
	Files []FileRef
	// FilesFrom is the version Files was worked out from, 0 when the revision
	// neither carries nor changes files. The save is refused with ErrStale
	// when it is no longer the newest, so files are never carried over from a
	// version someone else has already moved past, even when the publisher
	// named no base version.
	FilesFrom int
}

// UpdateArtifact saves a new version of a page: its title, content and format
// version change, its kind never does, because content is only valid for the
// kind it was written for. The caller passes the format to keep or the one to
// upgrade to. base is the version the caller started from: when it is not 0
// and the page has moved on, nothing is saved and the error is ErrStale; the
// check and the save are one transaction, so two saves cannot both win. An
// empty visibility leaves the current sharing untouched, so republishing never
// widens or narrows who can see the page by accident.
func (s *Store) UpdateArtifact(ctx context.Context, id string, base int, rev Revision, visibility Visibility) error {
	if visibility != "" && !visibility.Valid() {
		return fmt.Errorf("unknown visibility %q", visibility)
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var current int
		var kind string
		err := tx.QueryRowContext(ctx, `SELECT version, kind FROM artifacts WHERE id = ?`, id).Scan(&current, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if (base != 0 && base != current) || (rev.FilesFrom != 0 && rev.FilesFrom != current) {
			return ErrStale
		}
		now := time.Now().UTC().Truncate(time.Second)
		next := current + 1
		if visibility == "" {
			_, err = tx.ExecContext(ctx, `UPDATE artifacts SET title = ?, format = ?, html = ?, version = ?, updated_at = ? WHERE id = ?`,
				rev.Title, rev.Format, rev.Content, next, now.Unix(), id)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE artifacts SET title = ?, format = ?, html = ?, visibility = ?, organisation_edit = organisation_edit AND ? != 'private', version = ?, updated_at = ? WHERE id = ?`,
				rev.Title, rev.Format, rev.Content, visibility, visibility, next, now.Unix(), id)
		}
		if err != nil {
			return err
		}
		activityCtx, err := revisionActivity(ctx, tx, id, current, rev)
		if err != nil {
			return err
		}
		return insertVersion(activityCtx, tx, id, next, rev, kind, now)
	})
}

// insertVersion saves a version and counts it as a publish.
func insertVersion(ctx context.Context, tx *sql.Tx, id string, number int, rev Revision, kind string, at time.Time) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_versions (artifact_id, number, title, kind, format, content, author, restored_from, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, number, rev.Title, kind, rev.Format, rev.Content, rev.Author, rev.RestoredFrom, at.Unix()); err != nil {
		return err
	}
	authors := rev.Authors
	if len(authors) == 0 {
		authors = []string{rev.Author}
	}
	for i, who := range authors {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO version_authors (artifact_id, version, position, username) VALUES (?, ?, ?, ?)`, id, number, i, who); err != nil {
			return fmt.Errorf("save author %s: %w", who, err)
		}
	}
	for i, ag := range rev.Agents {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO version_agents (artifact_id, version, position, username, client) VALUES (?, ?, ?, ?, ?)`, id, number, i, ag.By, ag.Client); err != nil {
			return fmt.Errorf("save agent %s of %s: %w", ag.Client, ag.By, err)
		}
	}
	for _, f := range rev.Files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO version_files (artifact_id, version, path, sha256, content_type, size) VALUES (?, ?, ?, ?, ?, ?)`,
			id, number, f.Path, f.SHA256, f.ContentType, f.Size); err != nil {
			return fmt.Errorf("save file %s: %w", f.Path, err)
		}
	}
	if err := countUsage(ctx, tx, at, rev.Author, kind, UsagePublish, publishVia(ctx, rev)); err != nil {
		return fmt.Errorf("count the publish: %w", err)
	}
	return recordActivity(ctx, tx, id, number)
}

// VersionFiles lists the files of one version by path; none for a version
// that has no files besides its page.
func (s *Store) VersionFiles(ctx context.Context, id string, version int) ([]FileRef, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, sha256, content_type, size FROM version_files WHERE artifact_id = ? AND version = ? ORDER BY path`, id, version)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []FileRef{}
	for rows.Next() {
		var f FileRef
		if err := rows.Scan(&f.Path, &f.SHA256, &f.ContentType, &f.Size); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// Versions lists a page's saved versions without content, newest first.
func (s *Store) Versions(ctx context.Context, id string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT number, title, kind, format, author, restored_from, created_at,
			(SELECT COUNT(*) FROM version_files f WHERE f.artifact_id = v.artifact_id AND f.version = v.number)
		FROM artifact_versions v WHERE artifact_id = ? ORDER BY number DESC`, id)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Version{}
	for rows.Next() {
		var v Version
		var created int64
		if err := rows.Scan(&v.Number, &v.Title, &v.Kind, &v.Format, &v.Author, &v.RestoredFrom, &created, &v.FileCount); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(created, 0).UTC()
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	authors, err := s.versionAuthors(ctx, id)
	if err != nil {
		return nil, err
	}
	agents, err := s.versionAgents(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Authors = authorsOrNone(authors[list[i].Number], list[i].Author)
		list[i].Agents = agentsOrNone(agents[list[i].Number])
	}
	return list, nil
}

// versionAgents lists the agents that made each version of an artifact, by
// version, in the order they were recorded.
func (s *Store) versionAgents(ctx context.Context, id string) (map[int][]VersionAgent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, username, client FROM version_agents WHERE artifact_id = ? ORDER BY version, position`, id)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[int][]VersionAgent{}
	for rows.Next() {
		var n int
		var ag VersionAgent
		if err := rows.Scan(&n, &ag.By, &ag.Client); err != nil {
			return nil, err
		}
		out[n] = append(out[n], ag)
	}
	return out, rows.Err()
}

// agentsOrNone is the recorded agents, or an empty list, so the field is never null.
func agentsOrNone(recorded []VersionAgent) []VersionAgent {
	if recorded == nil {
		return []VersionAgent{}
	}
	return recorded
}

// versionAuthors lists who edited each version of an artifact, by version,
// in the order they were recorded.
func (s *Store) versionAuthors(ctx context.Context, id string) (map[int][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, username FROM version_authors WHERE artifact_id = ? ORDER BY version, position`, id)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[int][]string{}
	for rows.Next() {
		var n int
		var who string
		if err := rows.Scan(&n, &who); err != nil {
			return nil, err
		}
		out[n] = append(out[n], who)
	}
	return out, rows.Err()
}

// authorsOrNone is the recorded authors, or the version's own author when none
// were recorded, so the list is never empty.
func authorsOrNone(recorded []string, author string) []string {
	if len(recorded) == 0 {
		return []string{author}
	}
	return recorded
}

// Version returns one saved version of a page with its content.
func (s *Store) Version(ctx context.Context, id string, number int) (Version, error) {
	v := Version{Number: number}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT title, kind, format, content, author, restored_from, created_at FROM artifact_versions WHERE artifact_id = ? AND number = ?`, id, number).
		Scan(&v.Title, &v.Kind, &v.Format, &v.Content, &v.Author, &v.RestoredFrom, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	v.CreatedAt = time.Unix(created, 0).UTC()
	authors, err := s.versionAuthors(ctx, id)
	if err != nil {
		return Version{}, err
	}
	v.Authors = authorsOrNone(authors[number], v.Author)
	agents, err := s.versionAgents(ctx, id)
	if err != nil {
		return Version{}, err
	}
	v.Agents = agentsOrNone(agents[number])
	return v, nil
}
