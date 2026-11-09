package service

import (
	"context"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// OpensKept is how many of a person's most recent opens are remembered; older
// ones drop off their "Recently opened" list.
const OpensKept = 100

// Open records that the viewer opened the artifact in the viewer: it counts in
// the usage (usage.go), and for someone signed in it goes on their own
// "Recently opened" list (List with OrderOpened). Like a pin it only exists on
// an artifact they can see, and nobody else is told of it. The artifact comes
// back as the viewer sees it, with the new time.
func (s *Service) Open(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if err := s.store.CountOpen(ctx, v.Username, a.Kind, s.now()); err != nil {
		return store.Artifact{}, fmt.Errorf("count the open of artifact %s: %w", a.ID, err)
	}
	if v.Anonymous() {
		return a, nil
	}
	if err := s.store.RecordOpen(ctx, v.Username, a.ID, s.now(), OpensKept); err != nil {
		return store.Artifact{}, fmt.Errorf("record the open of artifact %s: %w", a.ID, err)
	}
	if err := s.withOpens(ctx, v, a.ID, &a); err != nil {
		return store.Artifact{}, err
	}
	return a, nil
}

// withOpens fills in when the viewer last opened each artifact.
func (s *Service) withOpens(ctx context.Context, v Viewer, id string, list ...*store.Artifact) error {
	opened, err := s.store.Opened(ctx, v.Username, id)
	if err != nil {
		return fmt.Errorf("read opens: %w", err)
	}
	for _, a := range list {
		if at, ok := opened[a.ID]; ok {
			a.OpenedAt = &at
		}
	}
	return nil
}
