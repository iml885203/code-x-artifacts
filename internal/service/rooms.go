package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// LiveRooms is the live copy of the artifacts that have one (internal/realtime
// and internal/roomkind). The service does not know what a room is; it asks
// these things where the rules need the room to be the working copy.
type LiveRooms interface {
	MergeExpect(context.Context, string, string, kind.Bundle, kind.Bundle, string, kind.SourceExpectation) (map[string][]string, error)
	// Live reports whether the artifact has a room that was ever opened.
	Live(ctx context.Context, id string) (bool, error)
	// Flush saves what the artifact's room holds that no version does as a new
	// version, for the people who edited it. It does nothing for an artifact
	// without a room, or whose room holds nothing new.
	Flush(ctx context.Context, id string) error
	// Apply makes the artifact's room hold a version of the given kind (its
	// content and files), as an edit by who, changing only what differs.
	// It does nothing for an artifact without a room.
	Apply(ctx context.Context, id, kindName string, v kind.Bundle, who string) error
	// Holds reports whether artifacts of the kind are edited in a room.
	Holds(kindName string) bool
	// Content is what the room holds, as a version: its content and its files.
	Content(ctx context.Context, id, kindName string) (kind.Bundle, error)
	// Present shows an agent in the artifact's room, at a block, for a few
	// seconds (see realtime.Hub.Announce). key tells agents apart.
	Present(ctx context.Context, id, key, label, block string) error
	// Merge changes the artifact's room by what an agent made of base (the
	// version it started from), leaving what people changed in the room since,
	// and says which elements of which files it touched (file -> pids).
	Merge(ctx context.Context, id, kindName string, base, next kind.Bundle, who string) (map[string][]string, error)
	// PresentTouched shows an agent in the artifact's room with the elements it
	// changed (frame id -> pids), for a few seconds.
	PresentTouched(ctx context.Context, id, key, label string, touched map[string][]string) error
}

// SetLiveRooms registers the live copy. Register before the service is used.
func (s *Service) SetLiveRooms(r LiveRooms) { s.rooms = r }

// SaveFromRoom saves what a live room holds (v: the content, and the files when
// the room holds files) as a new version on top of base, with everyone who
// edited it as its authors (the first of them is the version's Author). It is
// not a request from a viewer: each person was admitted to the room by the
// access rules when they joined, and the room is the working copy. A base that
// is no longer the newest is store.ErrStale.
func (s *Service) SaveFromRoom(ctx context.Context, id string, base int, v kind.Bundle, authors []string) error {
	a, err := s.store.Artifact(ctx, id)
	if err != nil {
		return err
	}
	if a.Version != base {
		return store.ErrStale
	}
	k, err := kind.Lookup(a.Kind)
	if err != nil {
		return fmt.Errorf("artifact %s: %w", id, err)
	}
	patch, err := s.roomFilePatch(ctx, a, v)
	if err != nil {
		return err
	}
	plan, err := s.planFiles(ctx, k, a, patch)
	if err != nil {
		return err
	}
	if err := validateContent(k, v.Content, a.Format, plan.set); err != nil {
		return fmt.Errorf("what the room of artifact %s holds is not a valid %s: %w", id, a.Kind, err)
	}
	if _, err := plan.normalizeFiles(k, a.Format, patch); err != nil {
		return fmt.Errorf("what the room of artifact %s holds is not a valid %s: %w", id, a.Kind, err)
	}
	if err := s.storeBlobs(a.ID, plan); err != nil {
		return err
	}
	by := a.Author
	if len(authors) > 0 {
		by = authors[0]
	}
	made, err := s.agentsOfRoom(ctx, id, authors)
	if err != nil {
		return err
	}
	rev := store.Revision{Title: a.Title, Format: a.Format, Content: v.Content, Author: by, Authors: authors, Agents: made, Files: plan.manifest, FilesFrom: plan.from}
	if err := s.store.UpdateArtifact(ctx, id, base, rev, ""); err != nil {
		if errors.Is(err, store.ErrStale) {
			return err
		}
		return fmt.Errorf("update artifact %s: %w", id, err)
	}
	report(ctx, Information, "artifact saved from its room", "id", id, "authors", authors)
	return nil
}

// agentsOfRoom are the agents that wrote into the room since its last version,
// and the agent of this request when it is saving the room itself (a doc edit
// into a doc whose room is not open). The request's agent acts for the one
// author it saves as.
func (s *Service) agentsOfRoom(ctx context.Context, room string, authors []string) ([]store.VersionAgent, error) {
	made, err := s.store.PendingAgents(ctx, room)
	if err != nil {
		return nil, fmt.Errorf("read the agents that wrote into room %s: %w", room, err)
	}
	if len(authors) == 1 {
		for _, ag := range agents(ctx, Viewer{Username: authors[0]}) {
			if !slices.Contains(made, ag) {
				made = append(made, ag)
			}
		}
	}
	return made, nil
}

// roomFilePatch is the publish's files that make the newest version's files the
// ones the room holds: what differs or is new, and null for what the room no
// longer has. Nil when the room holds no files.
func (s *Service) roomFilePatch(ctx context.Context, a store.Artifact, v kind.Bundle) (map[string]*string, error) {
	if v.Files == nil {
		return nil, nil
	}
	have, err := s.FileTexts(ctx, a.ID, a.Version)
	if err != nil {
		return nil, err
	}
	patch := map[string]*string{}
	for path, text := range v.Files {
		if before, ok := have[path]; !ok || before != text {
			patch[path] = &text
		}
	}
	for path := range have {
		if _, ok := v.Files[path]; !ok {
			patch[path] = nil
		}
	}
	return patch, nil
}

// FileTexts is the text of every file of a version of an artifact. Callers have
// already been allowed to see the version.
func (s *Service) FileTexts(ctx context.Context, id string, version int) (files.Set, error) {
	manifest, err := s.store.VersionFiles(ctx, id, version)
	if err != nil {
		return nil, fmt.Errorf("list files of artifact %s: %w", id, err)
	}
	set := make(files.Set, len(manifest))
	for _, f := range manifest {
		data, err := s.files.ReadBlob(id, f.SHA256)
		if err != nil {
			return nil, fmt.Errorf("file %s of artifact %s is recorded but its bytes are unreadable: %w", f.Path, id, err)
		}
		set[f.Path] = string(data)
	}
	return set, nil
}

// toRoom writes whole content into the artifact's live room, if it has one:
// what the room holds that no version does is saved first, then the room is
// changed to hold the content, where everyone in it sees it. applied says
// there was a room; now is the artifact after the protective version, which a
// version saved on top of the change builds on. A caller that named a base
// version (base != 0) and finds that the room held unsaved work is refused, as
// for any stale base: the work is saved as a version, and the caller has not
// seen it.
func (s *Service) toRoom(ctx context.Context, id, kindName string, v kind.Bundle, by string, base int) (applied bool, now store.Artifact, err error) {
	if s.rooms == nil {
		return false, now, nil
	}
	if live, err := s.rooms.Live(ctx, id); err != nil || !live {
		return false, now, err
	}
	before, err := s.store.Artifact(ctx, id)
	if err != nil {
		return false, now, fmt.Errorf("reload artifact %s: %w", id, err)
	}
	if err := s.rooms.Flush(store.WithoutActivity(ctx), id); err != nil {
		return false, now, fmt.Errorf("save the live copy of artifact %s first: %w", id, err)
	}
	if now, err = s.store.Artifact(ctx, id); err != nil {
		return false, now, fmt.Errorf("reload artifact %s: %w", id, err)
	}
	if now.Version != before.Version && base != 0 {
		return false, now, s.conflict(ctx, id, base)
	}
	var was kind.Bundle
	if _, _, agent := agentLabel(ctx, Viewer{Username: by}); agent {
		was, _ = s.rooms.Content(ctx, id, kindName) // for the caret; a room that cannot say has no caret
	}
	if err := s.rooms.Apply(ctx, id, kindName, v, by); err != nil {
		return false, now, fmt.Errorf("write into the live copy of artifact %s: %w", id, err)
	}
	s.showAgent(ctx, Viewer{Username: by}, id, changedBlock(was.Content, v.Content))
	return true, now, nil
}

// publishThroughRoom is the live-room step of an update: when the artifact has
// a room the new content goes into it, and the update goes on from the artifact
// as it is after the protective version.
func (s *Service) publishThroughRoom(ctx context.Context, v Viewer, a store.Artifact, k kind.Kind, in PublishInput) (store.Artifact, int, error) {
	if _, ok := k.(kind.Composer); !ok {
		return a, in.BaseVersion, nil
	}
	if in.Content == "" {
		return a, in.BaseVersion, nil
	}
	if err := k.Validate(in.Content, in.Format); err != nil {
		return a, in.BaseVersion, refuse(Invalid, "%s", err)
	}
	applied, now, err := s.toRoom(ctx, a.ID, in.Kind, kind.Bundle{Content: in.Content}, v.Username, in.BaseVersion)
	if err != nil || !applied {
		return a, in.BaseVersion, err
	}
	return now, now.Version, nil
}

// Current is the artifact with the content of its live room when it has one,
// which is what people are looking at and an edit has to start from; the
// version stays the newest saved one.
func (s *Service) Current(ctx context.Context, a store.Artifact) (store.Artifact, error) {
	if s.rooms == nil {
		return a, nil
	}
	if live, err := s.rooms.Live(ctx, a.ID); err != nil || !live {
		return a, err
	}
	held, err := s.rooms.Content(ctx, a.ID, a.Kind)
	if err != nil {
		return a, nil //nolint:nilerr // a room holding something no version could hold is not what the saved version says either; the saved one is the answer
	}
	a.Content = held.Content
	return a, nil
}
