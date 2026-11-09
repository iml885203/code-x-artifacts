package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// The operations of a slide edit.
const (
	SlideReplace = "replace"
	SlideInsert  = "insert"
	SlideMove    = "move"
	SlideDelete  = "delete"
)

// SlideEditInput is a change to one slide of a deck.
type SlideEditInput struct {
	// BaseVersion, when given, is the version the caller read the slide
	// numbers from; a newer one refuses the edit like any stale publish, since
	// a slide inserted meanwhile would make the numbers point elsewhere.
	BaseVersion int
	Op          string
	// Number is the slide replaced, moved or deleted.
	Number int
	// After is where an insert goes: after slide After, 0 for the start.
	After int
	// To is the number a moved slide ends up with.
	To int
	// Slide is the slide a replace or insert puts in.
	Slide json.RawMessage
}

// SlidesEdited is what a slide edit did: the version it saved, the deck's
// slides now, and the review a publish of the deck would give.
type SlidesEdited struct {
	Version int               `json:"version"`
	Slides  []kind.SlideEntry `json:"slides"`
	Review  []string          `json:"review,omitempty"`
}

// SlideRead is one slide of a deck and the list it is in.
type SlideRead struct {
	Version int               `json:"version"`
	Number  int               `json:"number"`
	Slide   json.RawMessage   `json:"slide"`
	Slides  []kind.SlideEntry `json:"slides"`
}

// SlideList is the slides of a deck at its newest version.
type SlideList struct {
	Version int               `json:"version"`
	Slides  []kind.SlideEntry `json:"slides"`
}

// Slides lists the slides of a deck the viewer can see.
func (s *Service) Slides(ctx context.Context, v Viewer, id string) (SlideList, error) {
	defer s.admission.Shared(id)()
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return SlideList{}, err
	}
	d, err := deckOf(a)
	if err != nil {
		return SlideList{}, err
	}
	return SlideList{Version: a.Version, Slides: d.List()}, nil
}

// Slide is slide n of a deck the viewer can see.
func (s *Service) Slide(ctx context.Context, v Viewer, id string, n int) (SlideRead, error) {
	defer s.admission.Shared(id)()
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return SlideRead{}, err
	}
	d, err := deckOf(a)
	if err != nil {
		return SlideRead{}, err
	}
	raw, err := d.Slide(n)
	if err != nil {
		return SlideRead{}, refuse(Invalid, "%s", err)
	}
	return SlideRead{Version: a.Version, Number: n, Slide: raw, Slides: d.List()}, nil
}

// EditSlide changes one slide of a deck the viewer may edit and saves the
// deck as a new version. It is a publish of the whole deck built from the
// newest version, so it has the same access rules, validation and review; and
// it is saved on that version, so a publish that lands in between is a
// Conflict rather than lost.
func (s *Service) EditSlide(ctx context.Context, v Viewer, id string, in SlideEditInput) (SlidesEdited, error) {
	defer s.admission.Shared(id)()
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return SlidesEdited{}, err
	}
	d, err := deckOf(a)
	if err != nil {
		return SlidesEdited{}, err
	}
	if err := s.checkBase(ctx, a, in.BaseVersion); err != nil {
		return SlidesEdited{}, err
	}
	next, err := editDeck(d, in)
	if err != nil {
		return SlidesEdited{}, refuse(Invalid, "%s", err)
	}
	if next.Content() == d.Content() {
		return SlidesEdited{Version: a.Version, Slides: d.List()}, nil
	}
	p, err := s.update(ctx, v, id, PublishInput{Title: a.Title, Content: next.Content(), BaseVersion: a.Version})
	if err != nil {
		return SlidesEdited{}, err
	}
	return SlidesEdited{Version: p.Version, Slides: next.List(), Review: p.Review}, nil
}

func editDeck(d kind.Deck, in SlideEditInput) (kind.Deck, error) {
	switch in.Op {
	case SlideReplace:
		return d.Replace(in.Number, in.Slide)
	case SlideInsert:
		return d.InsertAfter(in.After, in.Slide)
	case SlideMove:
		return d.Move(in.Number, in.To)
	case SlideDelete:
		return d.Delete(in.Number)
	}
	return kind.Deck{}, fmt.Errorf("a slide edit is %s, %s, %s or %s, not %q", SlideReplace, SlideInsert, SlideMove, SlideDelete, in.Op)
}

// deckOf takes a saved deck apart, refusing an artifact that is not one.
func deckOf(a store.Artifact) (kind.Deck, error) {
	if a.Kind != (kind.Slides{}).Name() {
		return kind.Deck{}, refuse(Invalid, "this artifact is a %s, not a deck: the slide tools work on slides artifacts; read it, and publish the whole change", a.Kind)
	}
	d, err := kind.ParseDeck(a.Content, a.Format)
	if errors.Is(err, kind.ErrDeckFormat) {
		return kind.Deck{}, refuse(Invalid, "%s", err)
	}
	if err != nil {
		return kind.Deck{}, fmt.Errorf("artifact %s: %w", a.ID, err)
	}
	return d, nil
}
