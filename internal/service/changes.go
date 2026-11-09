package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// ChangedVersion is one version in the changes: who saved it and when, never
// what it said (reading old versions is for the people who may edit).
type ChangedVersion struct {
	Version      int       `json:"version"`
	Author       string    `json:"author,omitempty"`
	RestoredFrom int       `json:"restoredFrom,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Changes is what happened to an artifact since a version or a time.
type Changes struct {
	ID string `json:"id"`
	// Version, Title and UpdatedAt are where the artifact is now.
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Versions are the versions saved since, oldest first.
	Versions []ChangedVersion `json:"versions"`
	// Comments are the comments written since, oldest first. Empty for a viewer
	// who may not read comments.
	Comments []store.Comment `json:"comments"`
	// Access lists the changes of audience or editors since, oldest first, each
	// as what the audience and editors became. Empty for a viewer who may not
	// read the editors.
	Access []store.AccessChange `json:"access"`
}

// ChangesSince tells what happened to an artifact the viewer can see, since a
// version (sinceVersion, from 1: everything after that version, and what
// happened from the moment it was saved) or since a time (since, RFC 3339).
// Exactly one is given. Items stamped in the same second as the starting point
// are included, so nothing is missed; a caller matches repeats by version
// number or comment id. Whoever cannot see the artifact gets the answer for one
// that does not exist.
func (s *Service) ChangesSince(ctx context.Context, v Viewer, id string, sinceVersion int, since string) (Changes, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return Changes{}, err
	}
	after, from, err := s.startOfChanges(ctx, a, sinceVersion, since)
	if err != nil {
		return Changes{}, err
	}
	versions, err := s.store.VersionsSince(ctx, a.ID, after, from)
	if err != nil {
		return Changes{}, fmt.Errorf("list the versions of artifact %s since then: %w", a.ID, err)
	}
	out := Changes{ID: a.ID, Version: a.Version, Title: a.Title, UpdatedAt: a.UpdatedAt, Versions: make([]ChangedVersion, len(versions)), Comments: []store.Comment{}, Access: []store.AccessChange{}}
	for i, ver := range versions {
		out.Versions[i] = ChangedVersion{Version: ver.Number, RestoredFrom: ver.RestoredFrom, CreatedAt: ver.CreatedAt}
		if !v.Anonymous() {
			out.Versions[i].Author = ver.Author
		}
	}
	if v.Anonymous() {
		return out, nil
	}
	if out.Comments, err = s.store.CommentsSince(ctx, a.ID, from); err != nil {
		return Changes{}, fmt.Errorf("list the comments of artifact %s since then: %w", a.ID, err)
	}
	for i := range out.Comments {
		out.Comments[i] = commentForViewer(v, a, out.Comments[i])
	}
	if out.Access, err = s.store.AccessChangesSince(ctx, a.ID, from); err != nil {
		return Changes{}, fmt.Errorf("list the access changes of artifact %s since then: %w", a.ID, err)
	}
	return out, nil
}

// startOfChanges turns the question into where to start: the version after
// which versions count (0 for a question by time) and the time from which
// everything counts.
func (s *Service) startOfChanges(ctx context.Context, a store.Artifact, sinceVersion int, since string) (after int, from time.Time, err error) {
	switch {
	case sinceVersion != 0 && since != "":
		return 0, time.Time{}, refuse(Invalid, "send since_version or since, not both")
	case sinceVersion == 0 && since == "":
		return 0, time.Time{}, refuse(Invalid, "say where to start: since_version (the newest version you have seen) or since (a time, RFC 3339 such as 2026-10-20T09:30:00Z)")
	case sinceVersion == 0:
		if from, err = time.Parse(time.RFC3339, since); err != nil {
			return 0, time.Time{}, refuse(Invalid, "since %q is not a time in RFC 3339, such as 2026-10-20T09:30:00Z", since)
		}
		return 0, from, nil
	case sinceVersion < 1 || sinceVersion > a.Version:
		return 0, time.Time{}, refuse(Invalid, "this artifact has no version %d; its newest is %d", sinceVersion, a.Version)
	}
	if from, err = s.store.VersionTime(ctx, a.ID, sinceVersion); err != nil {
		return 0, time.Time{}, fmt.Errorf("read when version %d of artifact %s was saved: %w", sinceVersion, a.ID, err)
	}
	// Versions after this one, and what happened from the moment it was saved.
	return sinceVersion, from, nil
}

// recordAccess notes who changed an artifact's audience or editors when the
// artifact is not what it was before. A change nobody can tell from the
// previous state (the same audience again, an editor already named) leaves no
// trace. The change itself is already saved, so a failure here is logged and
// not reported as the change failing.
func (s *Service) recordAccess(ctx context.Context, before store.Artifact, by string) {
	after, err := s.store.Artifact(ctx, before.ID)
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		report(ctx, Failure, "could not read an artifact to record who changed its access", "id", before.ID, "err", err)
		s.accessChanged(before.ID) // not knowing whether it changed, the watchers re-check
		return
	}
	if sameAccess(before, after) {
		return
	}
	s.accessChanged(after.ID)
	change := store.AccessChange{By: by, Visibility: after.Visibility, Editors: after.Editors, OrganisationEdit: after.OrganisationEdit, At: nowUTC()}
	if err := s.store.RecordAccessChange(ctx, after.ID, change); err != nil {
		report(ctx, Failure, "could not record who changed the access to an artifact", "id", after.ID, "err", err)
	}
}

func sameAccess(a, b store.Artifact) bool {
	if a.Visibility != b.Visibility || a.OrganisationEdit != b.OrganisationEdit || len(a.Editors) != len(b.Editors) {
		return false
	}
	x, y := slices.Clone(a.Editors), slices.Clone(b.Editors)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}
