package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/boardedit"
	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// BoardEdited is what a board_edit did.
type BoardEdited struct {
	// Version is the newest saved version after the call.
	Version int `json:"version"`
	// Saved says a version was written now. It is false when the board has a
	// live room: the change is in the room, where people see it, and a version
	// follows when the room has been quiet.
	Saved bool `json:"saved"`
	boardedit.Result
}

// BoardEdit changes some elements of a board the viewer may edit, all of the
// operations or none, on the board as it is now (its live room's, when it has
// one). In a live room only the fields the operations change are written
// (Merge), so what people change elsewhere at the same time stays, and the
// agent shows in the room; without one the result is a new version.
func (s *Service) BoardEdit(ctx context.Context, v Viewer, id string, ops []boardedit.Op) (BoardEdited, error) {
	defer s.admission.Shared(id)()
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return BoardEdited{}, err
	}
	if a.Kind != (kind.Board{}).Name() {
		return BoardEdited{}, refuse(Invalid, "board_edit changes boards; this artifact is a %s: read it, and publish the whole change", a.Kind)
	}
	a, err = s.Current(ctx, a)
	if err != nil {
		return BoardEdited{}, err
	}
	base, err := boardfmt.Parse(a.Content)
	if err != nil {
		return BoardEdited{}, fmt.Errorf("artifact %s holds a board that does not parse: %w", a.ID, err)
	}
	next, res, err := boardedit.Apply(base, ops)
	if refusal, ok := errors.AsType[*boardedit.Refusal](err); ok {
		return BoardEdited{}, refuse(Invalid, "%s", refusal.Msg)
	}
	if stale, ok := errors.AsType[*boardedit.Stale](err); ok {
		return BoardEdited{}, &Error{Code: Conflict, Msg: stale.Msg, Current: a.Version, ArtifactID: a.ID, StaleElements: stale.Current}
	}
	if err != nil {
		return BoardEdited{}, err
	}
	before, after := boardfmt.Encode(boardfmt.Canonical(base)), boardfmt.Encode(next)
	if err := s.checkBoardFiles(ctx, a.ID, after); err != nil {
		return BoardEdited{}, err
	}
	out := BoardEdited{Version: a.Version, Result: res}
	if before == after {
		return out, nil
	}
	if live, err := s.live(ctx, id); err != nil {
		return BoardEdited{}, err
	} else if live {
		if _, err := s.rooms.Merge(s.mutationContext(ctx, "board_edit"), id, a.Kind, kind.Bundle{Content: before}, kind.Bundle{Content: after}, v.Username); err != nil {
			return BoardEdited{}, fmt.Errorf("write into the live copy of artifact %s: %w", id, err)
		}
		s.showAgent(ctx, v, id, "")
		return out, nil
	}
	if err := s.SaveFromRoom(s.mutationContext(ctx, "board_edit"), id, a.Version, kind.Bundle{Content: after}, []string{v.Username}); err != nil {
		if errors.Is(err, store.ErrStale) {
			return BoardEdited{}, s.conflict(ctx, id, a.Version)
		}
		return BoardEdited{}, err
	}
	out.Version, out.Saved = a.Version+1, true
	return out, nil
}
