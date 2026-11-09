package service

import (
	"context"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Restore saves a copy of an earlier version as the newest one. History is
// kept: the versions in between stay readable, and the restore itself is a
// version that says which one it copies. Visibility is not part of a version
// and does not change. base works as in Update.
//
// When the artifact has a live room the restore goes through it: what the room
// holds that no version does is saved first, so the restore cannot take work
// from anyone, and then the restored content is written into the room, where
// everyone in it sees it and can undo it.
func (s *Service) Restore(ctx context.Context, v Viewer, id string, number, base int) (store.Artifact, error) {
	defer s.admission.Exclusive(id)()
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if err := s.checkBase(ctx, a, base); err != nil {
		return store.Artifact{}, err
	}
	old, err := s.Version(ctx, v, id, number)
	if err != nil {
		return store.Artifact{}, err
	}
	k, err := kind.Lookup(old.Kind)
	if err != nil {
		return store.Artifact{}, refuse(Invalid, "this server cannot restore version %d: %s", number, err)
	}
	// The version keeps the format it was written for; it must still be one this
	// build can validate, and its content must still pass that validation.
	if !kind.HasFormat(k, old.Format) {
		return store.Artifact{}, refuse(Invalid, "this server cannot restore version %d: it was written for %s format %q, which this build lacks", number, old.Kind, old.Format)
	}
	manifest, set, err := s.restorableFiles(ctx, k, a.ID, number)
	if err != nil {
		return store.Artifact{}, err
	}
	if err := validateContent(k, old.Content, old.Format, set); err != nil {
		return store.Artifact{}, refuse(Invalid, "version %d can no longer be restored as it is: %s", number, err)
	}
	s.retireSourceBases(id)
	mutationCtx := s.mutationContext(ctx, "restore")
	if applied, now, err := s.toRoom(mutationCtx, a.ID, old.Kind, kind.Bundle{Content: old.Content, Files: set}, v.Username, 0); err != nil {
		return store.Artifact{}, err
	} else if applied {
		base = now.Version
	}
	rev := store.Revision{Title: old.Title, Format: old.Format, Content: old.Content, Author: v.Username, Agents: agents(ctx, v), RestoredFrom: number, Files: manifest}
	return s.save(mutationCtx, v, a, base, rev, "")
}
