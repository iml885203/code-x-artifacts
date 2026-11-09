package service

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// publishLive does a publish into an artifact whose room is open when the
// artifact is of a kind that is edited live and not written as one piece of
// content (docs are, and take the whole-content path); done says it did. The
// room is the working copy of such an artifact, so its content is written only
// through it: a version made beside it would be overwritten by the room's next
// snapshot.
func (s *Service) publishLive(ctx context.Context, v Viewer, a store.Artifact, k kind.Kind, in PublishInput) (p Published, done bool, err error) {
	if s.rooms == nil || !s.rooms.Holds(a.Kind) {
		return p, false, nil
	}
	if _, composed := k.(kind.Composer); composed {
		return p, false, nil
	}
	if live, err := s.rooms.Live(ctx, a.ID); err != nil || !live {
		return p, false, err
	}
	p, err = s.publishIntoRoom(ctx, v, a, k, in)
	return p, true, err
}

// publishIntoRoom is a publish with files into an artifact whose room is open
// (a design): what the agent changed from its explicit per-file source reads is merged
// into the room, so
// whatever people changed in it since stays. Nothing is saved as a version; the
// room's next snapshot is the version, with the agent among its makers.
func (s *Service) publishIntoRoom(ctx context.Context, v Viewer, a store.Artifact, k kind.Kind, in PublishInput) (Published, error) {
	plan, err := s.planFiles(ctx, k, a, in.Files)
	if err != nil {
		return Published{}, err
	}
	if err := validateContent(k, in.Content, in.Format, plan.set); err != nil {
		return Published{}, contentRefusal(err, in)
	}
	if _, err := plan.normalizeFiles(k, in.Format, in.Files); err != nil {
		return Published{}, err
	}
	have, err := s.FileTexts(ctx, a.ID, a.Version)
	if err != nil {
		return Published{}, err
	}
	next := maps.Clone(have)
	for path, text := range in.Files {
		if text == nil {
			delete(next, path)
		} else {
			next[path] = *text
		}
	}
	base, expect, err := s.sourcePublishBase(ctx, v, a, have, in)
	if err != nil {
		return Published{}, err
	}
	touched, err := s.rooms.MergeExpect(ctx, a.ID, a.Kind, base, kind.Bundle{Content: in.Content, Files: next}, v.Username, expect)
	if err != nil {
		return Published{}, mergeRefusal(a, err)
	}
	s.showAgentTouched(ctx, v, a.ID, in.Content, touched)
	if err := s.applyAlongside(ctx, v, &a, in); err != nil {
		return Published{}, err
	}
	rewritten, err := s.roomFiles(ctx, a, in.Files)
	if err != nil {
		return Published{}, err
	}
	return Published{Artifact: a, Review: review(a, a.Content, plan.set, sentPaths(in.Files)), Rewritten: rewritten, Live: true}, nil
}

// applyAlongside does what a publish says besides the content, which the room
// does not hold: the title and who can see it.
func (s *Service) applyAlongside(ctx context.Context, v Viewer, a *store.Artifact, in PublishInput) error {
	if in.Visibility != "" && in.Visibility != a.Visibility {
		saved, err := s.setVisibility(ctx, v, a.ID, in.Visibility)
		if err != nil {
			return err
		}
		*a = saved
	}
	if in.Title != a.Title {
		renamed, err := s.rename(ctx, v, a.ID, in.Title)
		if err != nil {
			return err
		}
		*a = renamed
	}
	return nil
}

// roomFiles is the text of the files a publish sent, as the room holds them
// now: with the ids it gave the elements that had none, which is what the
// publisher is told its frames became.
func (s *Service) roomFiles(ctx context.Context, a store.Artifact, sent map[string]*string) (map[string]string, error) {
	held, err := s.rooms.Content(ctx, a.ID, a.Kind)
	if err != nil {
		return nil, fmt.Errorf("read the live copy of artifact %s: %w", a.ID, err)
	}
	out := map[string]string{}
	for path, text := range sent {
		if text != nil {
			if now, ok := held.Files[path]; ok {
				out[path] = now
			}
		}
	}
	return out, nil
}

// mergeRefusal is what a merge that did not go through says to the agent.
func mergeRefusal(a store.Artifact, err error) error {
	if limit, ok := errors.AsType[*realtime.LimitError](err); ok {
		return roomCapacityRefusal(limit)
	}
	if _, ok := errors.AsType[*realtime.EpochError](err); ok {
		return sourceBaseRefusal(a, "design.json")
	}
	if merge, ok := errors.AsType[*designroom.MergeError](err); ok {
		if merge.Conflict {
			return &Error{Code: Conflict, Msg: merge.Msg, Current: a.Version, ArtifactID: a.ID}
		}
		return refuse(Invalid, "%s", merge.Msg)
	}
	return fmt.Errorf("write into the live copy of artifact %s: %w", a.ID, err)
}

func roomCapacityRefusal(limit *realtime.LimitError) error {
	return refuse(Invalid, "nothing was published: %s; retain your draft; reduce the proposed change if the current room fits, otherwise ask the operator to recover the retained room log; do not retry unchanged", limit)
}

// showAgentTouched puts the agent of the request in the room of the artifact
// with the elements it changed, by frame id, for a few seconds.
func (s *Service) showAgentTouched(ctx context.Context, v Viewer, id, manifest string, touched map[string][]string) {
	key, label, ok := agentLabel(ctx, v)
	if !ok {
		return
	}
	byFrame := map[string][]string{}
	if m, problems := designfmt.Parse(manifest); len(problems) == 0 {
		for _, placed := range m.Frames() {
			if pids := touched[placed.Frame.File]; len(pids) > 0 {
				byFrame[placed.Frame.ID] = pids
			}
		}
	}
	if err := s.rooms.PresentTouched(ctx, id, key, label, byFrame); err != nil {
		report(ctx, Warning, "show an agent in a room", "artifact", id, "label", label, "err", err)
	}
	if ag, ok := versionAgent(ctx, v); ok {
		if err := s.store.MarkPendingAgent(ctx, id, ag.By, ag.Client); err != nil {
			report(ctx, Warning, "remember an agent that wrote into a room", "artifact", id, "label", label, "err", err)
		}
	}
}

// liveFile is a file of the artifact as its room holds it, for a read of the
// newest version, or false when the artifact has no room or the room does not
// hold that file. The ref says the size and type of the text it came with.
func (s *Service) liveFile(ctx context.Context, a store.Artifact, path string) (store.FileRef, string, bool, error) {
	if s.rooms == nil || !s.rooms.Holds(a.Kind) {
		return store.FileRef{}, "", false, nil
	}
	if k, err := kind.Lookup(a.Kind); err != nil {
		return store.FileRef{}, "", false, nil //nolint:nilerr // a kind that is not known has no files to read
	} else if _, composed := k.(kind.Composer); composed {
		return store.FileRef{}, "", false, nil
	}
	if live, err := s.rooms.Live(ctx, a.ID); err != nil || !live {
		return store.FileRef{}, "", false, err
	}
	held, err := s.rooms.Content(ctx, a.ID, a.Kind)
	if err != nil {
		return store.FileRef{}, "", false, nil //nolint:nilerr // a room holding something no version could hold is not what the saved version says either; the saved one is the answer
	}
	text, ok := held.Files[path]
	if !ok {
		return store.FileRef{}, "", false, nil
	}
	return store.FileRef{Path: path, ContentType: files.ContentTypeOf(path), Size: int64(len(text))}, text, true, nil
}
