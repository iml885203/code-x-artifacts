package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// MaxCommentChars is the longest comment, counted in characters.
const MaxCommentChars = 4000

// MaxAnchorBytes is the longest anchor, counted in bytes of compact JSON.
const MaxAnchorBytes = 2000

// NewComment is what a person or an agent says when adding a comment.
type NewComment struct {
	Body    string
	ReplyTo string
	// Anchor points at a place on the artifact: a JSON object whose meaning
	// belongs to the artifact's kind. Empty for a comment on the whole page; a
	// reply takes its thread's place and carries none.
	Anchor json.RawMessage
	// ForAgent says whether the author's agent should act on the comment; nil
	// means yes.
	ForAgent *bool
}

// Comments lists an artifact's comments, oldest first. Whoever may comment may
// read them. For the artifact's author, comments by others they have not seen
// are marked Unread.
func (s *Service) Comments(ctx context.Context, v Viewer, id string) ([]store.Comment, error) {
	a, err := s.commenting(ctx, v, id)
	if err != nil {
		return nil, err
	}
	list, err := s.store.Comments(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("list comments of artifact %s: %w", a.ID, err)
	}
	for i := range list {
		list[i] = commentForViewer(v, a, list[i])
	}
	if a.Author != v.Username {
		return list, nil
	}
	seen, err := s.store.SeenSeq(ctx, a.ID, v.Username)
	if err != nil {
		return nil, fmt.Errorf("read what %s has seen of the comments of artifact %s: %w", v.Username, a.ID, err)
	}
	for i := range list {
		list[i].Unread = !list[i].Deleted && list[i].Author != v.Username && list[i].Seq > seen
	}
	return list, nil
}

// AddComment adds a comment to an artifact the viewer may comment on. A reply
// names the comment it answers; replying to a reply joins that reply's thread,
// so threads stay one level deep.
func (s *Service) AddComment(ctx context.Context, v Viewer, id string, in NewComment) (store.Comment, error) {
	a, err := s.commenting(ctx, v, id)
	if err != nil {
		return store.Comment{}, err
	}
	body, replyTo := strings.TrimSpace(in.Body), in.ReplyTo
	switch {
	case body == "":
		return store.Comment{}, refuse(Invalid, "body is empty: write something to say")
	case !utf8.ValidString(body) || strings.ContainsRune(body, 0):
		return store.Comment{}, refuse(Invalid, "a comment is text: send valid UTF-8 without NUL characters")
	case utf8.RuneCountInString(body) > MaxCommentChars:
		return store.Comment{}, refuse(Invalid, "this comment is %d characters; a comment can be at most %d. Say it shorter, or split it into two comments", utf8.RuneCountInString(body), MaxCommentChars)
	}
	anchor, err := cleanAnchor(in.Anchor)
	if err != nil {
		return store.Comment{}, err
	}
	if anchor != nil && replyTo != "" {
		return store.Comment{}, refuse(Invalid, "a reply has no anchor of its own: it belongs to the place of the thread it answers; leave anchor out, or leave reply_to out to start a new thread there")
	}
	parent := ""
	if replyTo != "" {
		target, err := s.store.Comment(ctx, a.ID, replyTo)
		if errors.Is(err, store.ErrNotFound) {
			return store.Comment{}, refuse(Invalid, "reply_to names no comment on this artifact; use comments to see the ids, or leave it out to start a new thread")
		}
		if err != nil {
			return store.Comment{}, fmt.Errorf("read the comment being replied to on artifact %s: %w", a.ID, err)
		}
		parent = target.ID
		if target.ParentID != "" {
			parent = target.ParentID
		}
	}
	c, err := s.store.AddComment(ctx, store.Comment{ArtifactID: a.ID, Author: v.Username, Body: body, ParentID: parent, Anchor: anchor, ForAgent: in.ForAgent == nil || *in.ForAgent, CreatedAt: nowUTC()})
	if err != nil {
		return store.Comment{}, fmt.Errorf("add a comment to artifact %s: %w", a.ID, err)
	}
	report(ctx, Information, "comment added", "id", a.ID, "comment", c.ID, "user", v.Username, "reply", parent != "")
	return commentForViewer(v, a, c), nil
}

func commentForViewer(v Viewer, a store.Artifact, c store.Comment) store.Comment {
	c.CanDelete = !c.Deleted && v.CanDeleteComment(a, c)
	if c.Deleted {
		c.Body, c.ForAgent, c.Unread = "", false, false
	}
	return c
}

// DeleteComment removes only the caller's content while retaining any other person's replies.
// An owned repeat succeeds; losing artifact access or naming another author's comment still refuses.
func (s *Service) DeleteComment(ctx context.Context, v Viewer, id, commentID string) error {
	a, err := s.commenting(ctx, v, id)
	if err != nil {
		return err
	}
	c, err := s.store.Comment(ctx, a.ID, commentID)
	if errors.Is(err, store.ErrNotFound) {
		return refuse(NotFound, "no comment with that id on this artifact")
	}
	if err != nil {
		return fmt.Errorf("read the comment to delete: %w", err)
	}
	if !v.CanDeleteComment(a, c) {
		return refuse(Forbidden, "you may delete only your own comment")
	}
	if err := s.store.DeleteComment(ctx, a.ID, c.ID, v.Username); err != nil {
		return fmt.Errorf("delete comment %s on artifact %s: %w", c.ID, a.ID, err)
	}
	report(ctx, Information, "comment deleted", "id", a.ID, "comment", c.ID, "user", v.Username)
	return nil
}

// cleanAnchor checks an anchor. The service only knows it is a bounded, non-empty
// JSON object: what it points at belongs to the artifact's kind, and a target
// that is gone later does not make the comment invalid, so nothing here looks
// the target up.
func cleanAnchor(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	var anchor map[string]any
	if err := json.Unmarshal(trimmed, &anchor); err != nil || len(anchor) == 0 {
		return nil, refuse(Invalid, "anchor must be a non-empty JSON object that says where the comment points, for example {\"block\":\"b3\",\"quote\":\"the words\"}; leave it out for a comment on the whole page")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, refuse(Invalid, "anchor is not valid JSON")
	}
	if compact.Len() > MaxAnchorBytes {
		return nil, refuse(Invalid, "this anchor is %d bytes; an anchor can be at most %d. Keep the quote short: name the place, not the page", compact.Len(), MaxAnchorBytes)
	}
	return anchor, nil
}

// MarkCommentsSeen records that the viewer has seen the artifact's comments up
// to and including the one named. It is for the artifact's author, the only
// person told about new comments; for anyone else it does nothing. Naming the
// last comment that was shown, rather than "all", keeps a comment that arrived
// meanwhile unread.
func (s *Service) MarkCommentsSeen(ctx context.Context, v Viewer, id, upTo string) error {
	a, err := s.commenting(ctx, v, id)
	if err != nil {
		return err
	}
	c, err := s.store.Comment(ctx, a.ID, upTo)
	if errors.Is(err, store.ErrNotFound) {
		return refuse(Invalid, "upTo names no comment on this artifact; send the id of the newest comment you showed")
	}
	if err != nil {
		return fmt.Errorf("read the comment marked as seen on artifact %s: %w", a.ID, err)
	}
	if a.Author != v.Username {
		return nil
	}
	if err := s.store.MarkSeen(ctx, a.ID, v.Username, c.Seq); err != nil {
		return fmt.Errorf("mark comments of artifact %s as seen by %s: %w", a.ID, v.Username, err)
	}
	return nil
}

// commenting loads an artifact the viewer may comment on: one they can see,
// while signed in. Whoever cannot see it gets the answer for an artifact that
// does not exist.
func (s *Service) commenting(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if !v.CanComment(a) {
		return store.Artifact{}, refuse(Forbidden, "sign in to read or write comments")
	}
	return a, nil
}

// withComments fills in what the artifacts' comments look like to the viewer.
func (s *Service) withComments(ctx context.Context, v Viewer, id string, list ...*store.Artifact) error {
	if v.Anonymous() {
		return nil
	}
	sums, err := s.store.CommentSummaries(ctx, v.Username, id)
	if err != nil {
		return fmt.Errorf("count comments: %w", err)
	}
	for _, a := range list {
		a.Comments = sums[a.ID]
	}
	return nil
}
