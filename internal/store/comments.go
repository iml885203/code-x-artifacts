package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Comment is one note in an artifact's thread. A reply names the top-level
// comment it answers in ParentID; a top-level comment has none.
type Comment struct {
	ID         string `json:"id"`
	ArtifactID string `json:"-"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	// Deleted marks removed content; its body is empty, but a root may still hold other people's replies.
	Deleted bool `json:"deleted,omitempty"`
	// CanDelete is filled by the service for the current viewer, never stored.
	CanDelete bool   `json:"canDelete"`
	ParentID  string `json:"parentId,omitempty"`
	// Anchor says where on the artifact the comment points: a JSON object whose
	// meaning belongs to the artifact's kind (a doc block and a quote, a design
	// frame and element). The store keeps its keys and values and never looks
	// inside; nothing checks that its target still exists. Empty for a comment on the
	// whole page.
	Anchor map[string]any `json:"anchor,omitempty"`
	// ForAgent is true when the comment is meant for the author's agent to act on.
	ForAgent bool `json:"forAgent"`
	// Unread is set by the service for the one person it is relevant to, the
	// artifact's author, and only on comments of other people they have not seen.
	Unread    bool      `json:"unread,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// Seq is the comment's place in the order of all comments; unread-ness is
	// measured in it, and it is never shown.
	Seq int64 `json:"-"`
}

// CommentSummary is what a list entry says about an artifact's comments.
type CommentSummary struct {
	Count  int `json:"count"`
	Unread int `json:"unread"`
}

// commentColumns is what every comment query selects, in the order scanComment reads it.
const commentColumns = `seq, id, author, body, parent_id, anchor, for_agent, created_at, deleted`

type rowScanner interface{ Scan(dest ...any) error }

func scanComment(row rowScanner, artifactID string) (Comment, error) {
	c := Comment{ArtifactID: artifactID}
	var anchor string
	var created int64
	if err := row.Scan(&c.Seq, &c.ID, &c.Author, &c.Body, &c.ParentID, &anchor, &c.ForAgent, &created, &c.Deleted); err != nil {
		return Comment{}, err
	}
	if anchor != "" {
		if err := json.Unmarshal([]byte(anchor), &c.Anchor); err != nil {
			return Comment{}, fmt.Errorf("comment %s has an anchor that is not a JSON object: %w", c.ID, err)
		}
	}
	c.CreatedAt = time.Unix(created, 0).UTC()
	return c, nil
}

// AddComment stores a comment and returns it with its place in the order.
func (s *Store) AddComment(ctx context.Context, c Comment) (Comment, error) {
	c.ID = newID()
	anchor := []byte{}
	if len(c.Anchor) > 0 {
		var err error
		if anchor, err = json.Marshal(c.Anchor); err != nil {
			return Comment{}, fmt.Errorf("encode the anchor of a comment: %w", err)
		}
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO comments (id, artifact_id, author, body, parent_id, anchor, for_agent, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ArtifactID, c.Author, c.Body, c.ParentID, string(anchor), c.ForAgent, c.CreatedAt.Unix())
	if err != nil {
		return Comment{}, err
	}
	if c.Seq, err = res.LastInsertId(); err != nil {
		return Comment{}, err
	}
	return c, nil
}

// Comment returns one comment of an artifact.
func (s *Store) Comment(ctx context.Context, artifactID, id string) (Comment, error) {
	c, err := scanComment(s.db.QueryRowContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE artifact_id = ? AND id = ?`, artifactID, id), artifactID)
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	return c, err
}

// Comments lists an artifact's comments, oldest first.
func (s *Store) Comments(ctx context.Context, artifactID string) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE artifact_id = ?1 AND
		(deleted = 0 OR (parent_id = '' AND id IN (SELECT parent_id FROM comments WHERE artifact_id = ?1 AND deleted = 0 AND parent_id <> '')))
		ORDER BY seq`, artifactID)
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

// DeleteComment removes only the named author's content. Identity survives for replies and idempotent repeats.
func (s *Store) DeleteComment(ctx context.Context, artifactID, id, author string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE comments SET body = '', deleted = 1, for_agent = 0 WHERE artifact_id = ? AND id = ? AND author = ?`, artifactID, id, author)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SeenSeq is the place of the newest comment the person has seen on the
// artifact; zero when they have seen none.
func (s *Store) SeenSeq(ctx context.Context, artifactID, username string) (int64, error) {
	var seen int64
	err := s.db.QueryRowContext(ctx, `SELECT seen_seq FROM comment_reads WHERE artifact_id = ? AND username = ?`, artifactID, username).Scan(&seen)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seen, err
}

// MarkSeen records that the person has seen the artifact's comments up to the
// one in place seq. It only moves forward, so a stale call cannot make a
// comment that was seen unread again.
func (s *Store) MarkSeen(ctx context.Context, artifactID, username string, seq int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO comment_reads (artifact_id, username, seen_seq) VALUES (?, ?, ?)
		ON CONFLICT (artifact_id, username) DO UPDATE SET seen_seq = MAX(seen_seq, excluded.seen_seq)`, artifactID, username, seq)
	return err
}

// CommentSummaries counts comments per artifact, for the artifacts with any.
// Unread counts, on artifacts authored by reader, the comments by other people
// that reader has not seen; it is zero on everyone else's artifacts, because
// only an author is told about new comments. A non-empty artifactID narrows the
// answer to that artifact.
func (s *Store) CommentSummaries(ctx context.Context, reader, artifactID string) (map[string]CommentSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.artifact_id, COUNT(*),
		       COALESCE(SUM(CASE WHEN a.author = ?1 AND c.author <> ?1 AND c.seq > COALESCE(r.seen_seq, 0) THEN 1 ELSE 0 END), 0)
		FROM comments c
		JOIN artifacts a ON a.id = c.artifact_id
		LEFT JOIN comment_reads r ON r.artifact_id = c.artifact_id AND r.username = ?1
		WHERE c.deleted = 0 AND (?2 = '' OR c.artifact_id = ?2)
		GROUP BY c.artifact_id`, reader, artifactID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[string]CommentSummary{}
	for rows.Next() {
		var id string
		var sum CommentSummary
		if err := rows.Scan(&id, &sum.Count, &sum.Unread); err != nil {
			return nil, err
		}
		out[id] = sum
	}
	return out, rows.Err()
}
