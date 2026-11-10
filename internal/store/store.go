// Package store keeps pages, viewer profiles, personal tokens and each page's
// shared documents in one SQLite file.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver, so the image needs no cgo
)

// ErrNotFound reports that the addressed row does not exist.
var ErrNotFound = errors.New("not found")

// ErrStale reports that an update named a base version that is no longer the
// artifact's newest, so applying it would overwrite someone else's save.
var ErrStale = errors.New("base version is not the newest")

// Store is the SQLite-backed persistence for the service.
type Store struct {
	db *sql.DB
}

// User is a viewer's directory profile as of their last sign-in.
type User struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
	Groups      []string `json:"groups"`
	// Department is what AD reported at sign-in, "" when it reported none; only
	// the usage counts read it.
	Department string `json:"-"`
}

// Visibility says how far beyond its author an artifact is shared: nobody,
// the organisation, or anyone with the link. The web layer's access rules are
// the only place that interprets it.
type Visibility string

const (
	// VisibilityPrivate shows the artifact to its author only.
	VisibilityPrivate Visibility = "private"
	// VisibilityOrganisation shows it to everyone who can sign in.
	VisibilityOrganisation Visibility = "organisation"
	// VisibilityPublic shows it to anyone who has the link, signed in or not,
	// while the deployment allows public links.
	VisibilityPublic Visibility = "public"
)

// Valid reports whether v is a visibility this version understands.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPrivate, VisibilityOrganisation, VisibilityPublic:
		return true
	default:
		return false
	}
}

// Artifact is one published page.
type Artifact struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Kind names how Content is validated and rendered. The store keeps it as
	// an opaque name; the kind registry owns what the names mean.
	Kind string `json:"kind"`
	// Format is the version of the kind's content format (and runtime) the
	// content was published against, fixed until the author republishes with
	// another one. Empty for kinds without versioned content, html among them.
	Format  string `json:"format,omitempty"`
	Content string `json:"-"`
	// Version is the number of the newest saved version, counted from 1; every
	// publish and every restore adds one. Title, Format and Content are those
	// of that version.
	Version    int        `json:"version"`
	Author     string     `json:"author"`
	Visibility Visibility `json:"visibility"`
	// Editors are the accounts the author named to publish new versions; the
	// author is not in the list. Never null in JSON.
	Editors []string `json:"editors"`
	// Files is the files of the newest version, besides its page; absent when
	// it has none. The service fills it for one artifact, never in a listing.
	Files []FileRef `json:"files,omitempty"`
	// OrganisationEdit is the author's switch "everyone who can sign in may
	// edit". It only counts while the audience is the organisation or public;
	// the access rules (service.Viewer.CanEdit) are the only place that says so.
	OrganisationEdit bool      `json:"organisationEdit"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	// CopiedFrom is the artifact this one was copied from, nil for one that
	// was published fresh. The store records it; the service decides whether
	// the viewer may be given the source's id (see Source.ID).
	CopiedFrom *Source `json:"copiedFrom,omitempty"`
	// Comments is what the artifact's comments look like to the person asking.
	// The store never fills it: the service does, per viewer.
	Comments CommentSummary `json:"comments"`
	// Pinned says whether the person asking has pinned it. Like Comments the
	// store never fills it: the service does, per viewer.
	Pinned bool `json:"pinned"`
	// OpenedAt is when the person asking last opened it in the viewer, nil when
	// they never did. The service fills it, per viewer, like Pinned.
	OpenedAt *time.Time `json:"openedAt,omitempty"`
}

// Source says which artifact a copy was made from. The title is kept as it was
// when the copy was made, so the copy can still say where it came from after the
// source is renamed, hidden or deleted. ID is empty when the viewer may no
// longer open the source, so there is nothing to link to.
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Doc is one JSON document in a page's shared state.
type Doc struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data"`
	UpdatedBy string          `json:"updatedBy"`
	UpdatedAt time.Time       `json:"updatedAt"`
	// Version counts the writes of this document, from 1; a write that names
	// the version it was built on (ifVersion) is refused when it is not this one.
	Version int `json:"version"`
}

// Token describes a personal API token; the secret itself is never stored.
type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
	username     TEXT PRIMARY KEY,
	display_name TEXT NOT NULL,
	email        TEXT NOT NULL,
	groups_json  TEXT NOT NULL,
	last_login   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS artifacts (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	html       TEXT NOT NULL,
	author     TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS tokens (
	id         TEXT PRIMARY KEY,
	hash       TEXT NOT NULL UNIQUE,
	username   TEXT NOT NULL,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS docs (
	artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
	collection  TEXT NOT NULL,
	doc_id      TEXT NOT NULL,
	data        TEXT NOT NULL,
	updated_by  TEXT NOT NULL,
	updated_at  INTEGER NOT NULL,
	PRIMARY KEY (artifact_id, collection, doc_id)
);
`

// Open opens or creates the database at path and applies the schema.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema+oauthSchema); err != nil {
		return nil, errors.Join(fmt.Errorf("apply schema: %w", err), db.Close())
	}
	if err := migrate(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate: %w", err), db.Close())
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SessionKey returns the key that signs session cookies, creating it on first
// boot so the deployment carries no secret of its own.
func (s *Store) SessionKey(ctx context.Context) ([]byte, error) {
	var key []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'session_key'`).Scan(&key)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	key = make([]byte, 32)
	rand.Read(key)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('session_key', ?)`, key); err != nil {
		return nil, err
	}
	return key, nil
}

// SaveUser records a viewer's profile at sign-in.
func (s *Store) SaveUser(ctx context.Context, u User) error {
	groups, _ := json.Marshal(u.Groups)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, display_name, email, groups_json, department, last_login) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (username) DO UPDATE SET
			display_name = excluded.display_name, email = excluded.email, groups_json = excluded.groups_json,
			department = excluded.department, last_login = excluded.last_login`,
		u.Username, u.DisplayName, u.Email, string(groups), u.Department, time.Now().Unix())
	return err
}

// User returns the profile saved at the viewer's last sign-in.
func (s *Store) User(ctx context.Context, username string) (User, error) {
	u := User{Username: username}
	var groups string
	err := s.db.QueryRowContext(ctx, `SELECT display_name, email, groups_json FROM users WHERE username = ?`, username).
		Scan(&u.DisplayName, &u.Email, &groups)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if err := json.Unmarshal([]byte(groups), &u.Groups); err != nil {
		return User{}, fmt.Errorf("groups of %s: %w", username, err)
	}
	return u, nil
}

// NewArtifact is a page about to be created, as its version 1.
type NewArtifact struct {
	// ID is optional; a fresh one is made when it is empty. A caller that has
	// to store the page's files before the page exists names it first (see
	// NewArtifactID).
	ID                           string
	Title, Kind, Format, Content string
	Author                       string
	Visibility                   Visibility
	// Files is the files of version 1, whose blobs are already stored.
	Files []FileRef
}

// CreateArtifact stores a new page under a fresh id, as its version 1.
func (s *Store) CreateArtifact(ctx context.Context, title, kind, format, content, author string, visibility Visibility) (Artifact, error) {
	return s.CreateArtifactFrom(ctx, NewArtifact{Title: title, Kind: kind, Format: format, Content: content, Author: author, Visibility: visibility})
}

// CreateArtifactFrom is CreateArtifact for a page that may name its own id and
// come with files: the page, version 1 and its manifest are saved together.
func (s *Store) CreateArtifactFrom(ctx context.Context, in NewArtifact) (Artifact, error) {
	if !in.Visibility.Valid() {
		return Artifact{}, fmt.Errorf("unknown visibility %q", in.Visibility)
	}
	if in.ID == "" {
		in.ID = newID()
	}
	now := time.Now().UTC().Truncate(time.Second)
	a := Artifact{ID: in.ID, Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content, Author: in.Author, Visibility: in.Visibility, Editors: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts (id, title, kind, format, html, author, visibility, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Title, a.Kind, a.Format, a.Content, a.Author, a.Visibility, a.Version, now.Unix(), now.Unix()); err != nil {
			return err
		}
		return insertVersion(ctx, tx, a.ID, a.Version, Revision{Title: in.Title, Format: in.Format, Content: in.Content, Author: in.Author, Files: in.Files}, in.Kind, now)
	})
	return a, err
}

// NewArtifactID returns an id for an artifact that is about to be created, for
// callers that need it before the artifact exists (a copy rewrites the file
// URLs in its content to its own id).
func NewArtifactID() string { return newID() }

// CopyInput is a new artifact made from another one.
type CopyInput struct {
	// ID is the id of the copy, from NewArtifactID.
	ID                           string
	Title, Kind, Format, Content string
	Author                       string
	Source                       Source
	// Files is the copy's own files; their blobs are already stored under
	// the copy's id.
	Files []FileRef
}

// CreateCopy stores a copy as a private artifact, version 1, that records the
// artifact it came from.
func (s *Store) CreateCopy(ctx context.Context, in CopyInput) (Artifact, error) {
	now := time.Now().UTC().Truncate(time.Second)
	a := Artifact{ID: in.ID, Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content, Author: in.Author, Visibility: VisibilityPrivate, Editors: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now, CopiedFrom: &in.Source}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts (id, title, kind, format, html, author, visibility, version, copied_from, copied_from_title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Title, a.Kind, a.Format, a.Content, a.Author, a.Visibility, a.Version, in.Source.ID, in.Source.Title, now.Unix(), now.Unix()); err != nil {
			return err
		}
		return insertVersion(ctx, tx, a.ID, a.Version, Revision{Title: a.Title, Format: a.Format, Content: a.Content, Author: a.Author, Files: in.Files}, a.Kind, now)
	})
	return a, err
}

// BlobRefs maps each artifact to the blobs (by sha256) that some version still
// names. A blob written for a publish that did not finish is in no manifest,
// which is how the sweep finds it.
func (s *Store) BlobRefs(ctx context.Context) (map[string]map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT artifact_id, sha256 FROM version_files`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	refs := map[string]map[string]struct{}{}
	for rows.Next() {
		var id, sha string
		if err := rows.Scan(&id, &sha); err != nil {
			return nil, err
		}
		if refs[id] == nil {
			refs[id] = map[string]struct{}{}
		}
		refs[id][sha] = struct{}{}
	}
	return refs, rows.Err()
}

// inTx runs fn in a transaction that commits when fn returns nil.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, rollback(tx))
	}
	return tx.Commit()
}

// rollback reports a failed rollback only; sql.ErrTxDone means fn already ended it.
func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}

// SetSharing changes who can see a page without touching its content or its
// update time. Making a page private also switches "everyone in the
// organisation can edit" off, so turning the audience back up later does not
// hand out editing nobody asked for again.
func (s *Store) SetSharing(ctx context.Context, id string, visibility Visibility) error {
	if !visibility.Valid() {
		return fmt.Errorf("unknown visibility %q", visibility)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE artifacts SET visibility = ?, organisation_edit = organisation_edit AND ? != 'private' WHERE id = ?`, visibility, visibility, id)
	return requireRow(res, err)
}

// SetEditors changes who besides the author may edit a page, in one
// transaction: the accounts to add, the accounts to remove and, when
// organisationEdit is not nil, the organisation switch. Adding an account that
// is already an editor and removing one that is not are no-ops.
func (s *Store) SetEditors(ctx context.Context, id string, add, remove []string, organisationEdit *bool) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM artifacts WHERE id = ?`, id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		now := time.Now().Unix()
		for _, u := range add {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO artifact_editors (artifact_id, username, added_at) VALUES (?, ?, ?)`, id, u, now); err != nil {
				return err
			}
		}
		for _, u := range remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM artifact_editors WHERE artifact_id = ? AND username = ?`, id, u); err != nil {
				return err
			}
		}
		if organisationEdit != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE artifacts SET organisation_edit = ? WHERE id = ?`, *organisationEdit, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// editors lists the editors of one page, or of every page when id is empty,
// by page id. Each list is sorted by account name.
func (s *Store) editors(ctx context.Context, id string) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, username FROM artifact_editors WHERE ? = '' OR artifact_id = ? ORDER BY username`, id, id)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	byArtifact := map[string][]string{}
	for rows.Next() {
		var aid, user string
		if err := rows.Scan(&aid, &user); err != nil {
			return nil, err
		}
		byArtifact[aid] = append(byArtifact[aid], user)
	}
	return byArtifact, rows.Err()
}

// DeleteArtifact removes a page and, by cascade, its shared documents, versions
// and asset records (the asset files are the caller's to delete).
func (s *Store) DeleteArtifact(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM artifacts WHERE id = ?`, id)
	return requireRow(res, err)
}

// Artifact returns one page with its content.
func (s *Store) Artifact(ctx context.Context, id string) (Artifact, error) {
	a := Artifact{ID: id}
	var created, updated int64
	var source Source
	err := s.db.QueryRowContext(ctx, `SELECT title, kind, format, html, author, visibility, organisation_edit, version, copied_from, copied_from_title, created_at, updated_at FROM artifacts WHERE id = ?`, id).
		Scan(&a.Title, &a.Kind, &a.Format, &a.Content, &a.Author, &a.Visibility, &a.OrganisationEdit, &a.Version, &source.ID, &source.Title, &created, &updated)
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, kind, format, author, visibility, organisation_edit, version, created_at, updated_at FROM artifacts ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Artifact{}
	for rows.Next() {
		var a Artifact
		var created, updated int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Kind, &a.Format, &a.Author, &a.Visibility, &a.OrganisationEdit, &a.Version, &created, &updated); err != nil {
			return nil, err
		}
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

// CreateToken returns the plaintext token once; only its hash is kept.
func (s *Store) CreateToken(ctx context.Context, username, name string) (string, error) {
	raw := make([]byte, 24)
	rand.Read(raw)
	plain := "pct_" + hex.EncodeToString(raw)
	_, err := s.db.ExecContext(ctx, `INSERT INTO tokens (id, hash, username, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		newID(), hashToken(plain), username, name, time.Now().Unix())
	return plain, err
}

// TokenUser resolves a plaintext token to the username that owns it.
func (s *Store) TokenUser(ctx context.Context, plain string) (string, error) {
	p, err := s.AuthenticateToken(ctx, plain)
	return p.Owner, err
}

// Tokens lists a user's tokens, newest first.
func (s *Store) Tokens(ctx context.Context, username string) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM tokens WHERE username = ? ORDER BY created_at DESC`, username)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Token{}
	for rows.Next() {
		var t Token
		var created int64
		if err := rows.Scan(&t.ID, &t.Name, &created); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0).UTC()
		list = append(list, t)
	}
	return list, rows.Err()
}

// DeleteToken revokes one of the user's own tokens.
func (s *Store) DeleteToken(ctx context.Context, username, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE id = ? AND username = ?`, id, username)
	return requireRow(res, err)
}

// Doc returns one shared document of a page.
func (s *Store) Doc(ctx context.Context, artifactID, collection, docID string) (Doc, error) {
	d := Doc{ID: docID}
	var data string
	var updated int64
	err := s.db.QueryRowContext(ctx, `SELECT data, updated_by, updated_at, version FROM docs WHERE artifact_id = ? AND collection = ? AND doc_id = ?`,
		artifactID, collection, docID).Scan(&data, &d.UpdatedBy, &updated, &d.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Doc{}, ErrNotFound
	}
	d.Data, d.UpdatedAt = json.RawMessage(data), time.Unix(updated, 0).UTC()
	return d, err
}

// Docs lists a collection of a page's shared documents, oldest write first.
func (s *Store) Docs(ctx context.Context, artifactID, collection string) ([]Doc, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT doc_id, data, updated_by, updated_at, version FROM docs WHERE artifact_id = ? AND collection = ? ORDER BY updated_at, doc_id`,
		artifactID, collection)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	list := []Doc{}
	for rows.Next() {
		var d Doc
		var data string
		var updated int64
		if err := rows.Scan(&d.ID, &data, &d.UpdatedBy, &updated, &d.Version); err != nil {
			return nil, err
		}
		d.Data, d.UpdatedAt = json.RawMessage(data), time.Unix(updated, 0).UTC()
		list = append(list, d)
	}
	return list, rows.Err()
}

// DocCount is how many shared documents the artifact holds, in every collection.
func (s *Store) DocCount(ctx context.Context, artifactID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs WHERE artifact_id = ?`, artifactID).Scan(&n)
	return n, err
}

// closeRows releases a result set whose errors were already reported by rows.Err.
func closeRows(rows *sql.Rows) { _ = rows.Close() }

func requireRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func newID() string {
	b := make([]byte, 10)
	rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
