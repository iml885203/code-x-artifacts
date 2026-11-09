package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// DetachedThread is a comment thread whose block a doc_edit deleted: where it
// pointed, from its anchor, so the caller can find it in comments.
type DetachedThread struct {
	ID    string `json:"id"`
	Tab   string `json:"tab,omitempty"`
	Block string `json:"block"`
	Quote string `json:"quote,omitempty"`
}

// withDetachedThreads adds to an edit's result the threads anchored in the
// blocks it removed, when the viewer may read the comments. It runs before the
// edit is written, so a failure to read the comments leaves the doc as it was.
func (s *Service) withDetachedThreads(ctx context.Context, v Viewer, a store.Artifact, out DocEdited, removed []string) (DocEdited, error) {
	if len(removed) == 0 || !v.CanComment(a) {
		return out, nil
	}
	list, err := s.store.Comments(ctx, a.ID)
	if err != nil {
		return DocEdited{}, fmt.Errorf("list the comments of artifact %s to check the blocks an edit deletes: %w", a.ID, err)
	}
	detached := detachedThreads(list, removed)
	if len(detached) == 0 {
		return out, nil
	}
	out.CommentsOnDeletedBlocks = detached
	out.Review = append(slices.Clone(out.Review), fmt.Sprintf("%d comment thread(s) were anchored in blocks this edit deleted (see commentsOnDeletedBlocks); they keep their quote but no longer point at text in the doc: tell the person, or reply on the thread", len(detached)))
	return out, nil
}

// detachedThreads are the threads of the list whose anchor names one of the
// removed blocks. Replies carry no anchor: they belong to their thread.
func detachedThreads(list []store.Comment, removed []string) []DetachedThread {
	gone := make(map[string]bool, len(removed))
	for _, id := range removed {
		gone[id] = true
	}
	var out []DetachedThread
	for _, c := range list {
		block, _ := c.Anchor["block"].(string)
		if c.ParentID != "" || !gone[block] {
			continue
		}
		tab, _ := c.Anchor["tab"].(string)
		quote, _ := c.Anchor["quote"].(string)
		out = append(out, DetachedThread{ID: c.ID, Tab: tab, Block: block, Quote: quote})
	}
	return out
}
