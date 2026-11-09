package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const (
	// SourceBaseTTL is the lifetime of an explicit source read.
	SourceBaseTTL = 30 * time.Minute
	// SourceBaseLimit bounds the shared per-service registry.
	SourceBaseLimit = 500
)

// SetSourceReadBases registers the source snapshot collaborator before use.
// The first registration or read wins; a used registry can never be swapped.
func (s *Service) SetSourceReadBases(b *designsourceedit.ReadBases) {
	if b != nil {
		s.sourceOnce.Do(func() { s.sourceBases = b })
	}
}

func (s *Service) readBases() (*designsourceedit.ReadBases, error) {
	s.sourceOnce.Do(func() {
		s.sourceBases, s.sourceErr = designsourceedit.NewReadBases(SourceBaseTTL, SourceBaseLimit, func() time.Time { return s.now() })
	})
	return s.sourceBases, s.sourceErr
}

func (s *Service) retireSourceBases(id string) {
	bases, err := s.readBases()
	if err == nil {
		bases.ForgetArtifact(id)
	}
}

type sourceSnapshotter interface {
	SourceSnapshot(context.Context, string, string) (kind.Bundle, string, error)
}

// liveDesign reports whether the artifact is a design whose room is open, so
// what people see now is what the room holds.
func (s *Service) liveDesign(ctx context.Context, a store.Artifact) (bool, error) {
	if a.Kind != "design" || s.rooms == nil || !s.rooms.Holds(a.Kind) {
		return false, nil
	}
	return s.rooms.Live(ctx, a.ID)
}

func (s *Service) sourceSnapshot(ctx context.Context, a store.Artifact) (kind.Bundle, string, bool, error) {
	if live, err := s.liveDesign(ctx, a); err != nil || !live {
		return kind.Bundle{}, "", false, err
	}
	reader, ok := s.rooms.(sourceSnapshotter)
	if !ok {
		return kind.Bundle{}, "", true, fmt.Errorf("live source requires coherent snapshot support")
	}
	bundle, epoch, err := reader.SourceSnapshot(ctx, a.ID, a.Kind)
	if limit, ok := errors.AsType[*realtime.LimitError](err); ok {
		err = roomCapacityRefusal(limit)
	}
	return bundle, epoch, true, err
}

type sourceFiler interface {
	SourceFile(context.Context, string, string, string) (kind.SourceFile, error)
}

// sourceFile reads one file of a live design with its epoch, building only
// that file: a canvas asks for every frame at once, so building the whole
// design for each would cost the square of its frames.
func (s *Service) sourceFile(ctx context.Context, a store.Artifact, path string) (kind.SourceFile, bool, error) {
	if live, err := s.liveDesign(ctx, a); err != nil || !live {
		return kind.SourceFile{}, false, err
	}
	reader, ok := s.rooms.(sourceFiler)
	if !ok {
		return kind.SourceFile{}, true, fmt.Errorf("live source requires coherent one-file reads")
	}
	held, err := reader.SourceFile(ctx, a.ID, a.Kind, path)
	if limit, ok := errors.AsType[*realtime.LimitError](err); ok {
		err = roomCapacityRefusal(limit)
	}
	return held, true, err
}

func sourceBasis(v Viewer, a store.Artifact, path, epoch string) designsourceedit.Basis {
	return designsourceedit.Basis{ArtifactID: a.ID, File: path, Format: a.Format, SavedVersion: a.Version, LiveEpoch: epoch, Principal: v.Username}
}

// SourceRead is a current content read. Write bases are only issued to editors;
// saved-version reads use Version and never receive them.
type SourceRead struct {
	Artifact    store.Artifact
	SourceBases map[string]string
}

// ReadSource returns current content and the exact live manifest baseline.
func (s *Service) ReadSource(ctx context.Context, v Viewer, id string) (SourceRead, error) {
	defer s.admission.Shared(id)()
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return SourceRead{}, err
	}
	held, epoch, live, err := s.sourceSnapshot(ctx, a)
	if err != nil {
		return SourceRead{}, err
	}
	out := SourceRead{Artifact: a}
	if !live {
		out.Artifact, err = s.Current(ctx, a)
		return out, err
	}
	out.Artifact.Content = held.Content
	if v.CanEdit(a) {
		bases, err := s.readBases()
		if err != nil {
			return SourceRead{}, err
		}
		token, err := bases.Remember(sourceBasis(v, a, "design.json", epoch), held.Content)
		if err != nil {
			return SourceRead{}, err
		}
		out.SourceBases = map[string]string{"design.json": token}
	}
	return out, nil
}

// SourceFileRead carries one exact source read, without repeating its body in
// its proof. An absent current frame is explicit and is not an empty file.
type SourceFileRead struct {
	Ref        store.FileRef
	Text       string
	Present    bool
	SourceBase string
}

// ReadSourceFile issues a file baseline only for current live editor reads.
func (s *Service) ReadSourceFile(ctx context.Context, v Viewer, id, path string, version int) (SourceFileRead, error) {
	defer s.admission.Shared(id)()
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return SourceFileRead{}, err
	}
	if version != 0 || a.Kind != "design" {
		ref, text, err := s.readFile(ctx, v, id, path, version)
		return SourceFileRead{Ref: ref, Text: text, Present: err == nil}, err
	}
	if err := designfmt.CheckFramePath(path); err != nil {
		return SourceFileRead{}, refuse(Invalid, "%s", err)
	}
	now, err := s.designFileNow(ctx, v, a, path)
	if err != nil || !now.Live {
		return SourceFileRead{Ref: now.Ref, Text: now.Text, Present: now.Present}, err
	}
	text, present, epoch := now.Text, now.Present, now.Epoch
	if !present && !v.CanEdit(a) {
		return SourceFileRead{}, errNoFile(path, a.Files)
	}
	out := SourceFileRead{Ref: now.Ref, Text: text, Present: present}
	if v.CanEdit(a) {
		bases, err := s.readBases()
		if err != nil {
			return SourceFileRead{}, err
		}
		basis := sourceBasis(v, a, path, epoch)
		if present {
			out.SourceBase, err = bases.Remember(basis, text)
		} else {
			out.SourceBase, err = bases.RememberAbsent(basis)
		}
		if err != nil {
			return SourceFileRead{}, err
		}
	}
	return out, nil
}
