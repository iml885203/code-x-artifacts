package service

import (
	"context"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Pin pins or unpins an artifact for the viewer. A pin is the person's own and
// only exists on an artifact they can see: one they cannot see answers like an
// id that does not exist, so a pin never says whether an artifact is there.
// Both directions are idempotent. The artifact comes back as the viewer sees it.
func (s *Service) Pin(ctx context.Context, v Viewer, id string, pinned bool) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if v.Anonymous() {
		return store.Artifact{}, refuse(Forbidden, "sign in to pin artifacts")
	}
	if err := s.store.SetPin(ctx, v.Username, a.ID, pinned); err != nil {
		return store.Artifact{}, fmt.Errorf("set the pin of artifact %s: %w", a.ID, err)
	}
	a.Pinned = pinned
	return a, nil
}

// withViewerState fills in what only this viewer's own state says about the
// artifacts: their comments, their pins and when they opened them.
func (s *Service) withViewerState(ctx context.Context, v Viewer, id string, list ...*store.Artifact) error {
	if err := s.withComments(ctx, v, id, list...); err != nil {
		return err
	}
	if v.Anonymous() {
		return nil
	}
	pins, err := s.store.Pinned(ctx, v.Username, id)
	if err != nil {
		return fmt.Errorf("read pins: %w", err)
	}
	for _, a := range list {
		a.Pinned = pins[a.ID]
	}
	return s.withOpens(ctx, v, id, list...)
}
