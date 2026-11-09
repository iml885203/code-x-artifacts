package service

import (
	"context"
	"fmt"
	"maps"

	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

func sourceBaseRefusal(a store.Artifact, path string) error {
	return &Error{
		Code: Conflict, Current: a.Version, ArtifactID: a.ID,
		Msg: fmt.Sprintf("nothing was published: sourceBase for %q is missing, stale, retired or mismatched; retain your draft, read current source, reconcile your changes and send its sourceBases", path),
	}
}

func nonliveSourceBaseRefusal(a store.Artifact) error {
	return &Error{
		Code: Conflict, Current: a.Version, ArtifactID: a.ID,
		Msg: "nothing was published: this artifact has no live Design source; retain your draft, read it again and reconcile, then publish without sourceBases using the returned version as baseVersion",
	}
}

// sourcePublishBase resolves all supplied proofs before any room or metadata
// mutation. Unsent files are identical in base/next so the merge leaves them.
func (s *Service) sourcePublishBase(ctx context.Context, v Viewer, a store.Artifact, saved map[string]string, in PublishInput) (kind.Bundle, kind.SourceExpectation, error) {
	held, epoch, live, err := s.sourceSnapshot(ctx, a)
	if err != nil {
		return kind.Bundle{}, kind.SourceExpectation{}, err
	}
	if !live {
		return kind.Bundle{}, kind.SourceExpectation{}, nonliveSourceBaseRefusal(a)
	}
	loaded, err := s.loadSourceBases(v, a, epoch, in)
	if err != nil {
		return kind.Bundle{}, kind.SourceExpectation{}, err
	}
	base, err := resolveSourceBundle(a, saved, in, held, loaded)
	if err != nil {
		return kind.Bundle{}, kind.SourceExpectation{}, err
	}
	expect := kind.SourceExpectation{Epoch: epoch, Frames: map[string]kind.FrameExpectation{}}
	for path, text := range in.Files {
		read, ok := loaded[path]
		if ok && (read.Absent || text == nil) {
			expect.Frames[path] = kind.FrameExpectation{Absent: read.Absent, Source: read.Source}
		}
		if !ok && text == nil {
			expect.Frames[path] = kind.FrameExpectation{Absent: true}
		}
	}
	return base, expect, nil
}

func resolveSourceBundle(a store.Artifact, saved map[string]string, in PublishInput, held kind.Bundle, loaded map[string]designsourceedit.ReadBase) (kind.Bundle, error) {
	base := kind.Bundle{Content: a.Content, Files: maps.Clone(saved)}
	if manifest, ok := loaded["design.json"]; ok {
		if manifest.Absent {
			return kind.Bundle{}, sourceBaseRefusal(a, "design.json")
		}
		base.Content = manifest.Source
	} else if in.Content == held.Content {
		base.Content = held.Content
	} else if in.Content != a.Content {
		return kind.Bundle{}, sourceBaseRefusal(a, "design.json")
	}
	for path, text := range in.Files {
		read, hasBase := loaded[path]
		source, err := sourceFileBaseline(a, path, text, held.Files, read, hasBase)
		if err != nil {
			return kind.Bundle{}, err
		}
		if source == nil {
			delete(base.Files, path)
		} else {
			base.Files[path] = *source
		}
	}

	return base, nil
}

func (s *Service) loadSourceBases(v Viewer, a store.Artifact, epoch string, in PublishInput) (map[string]designsourceedit.ReadBase, error) {
	bases, err := s.readBases()
	if err != nil {
		return nil, err
	}
	loaded := map[string]designsourceedit.ReadBase{}
	for path, token := range in.SourceBases {
		base, err := bases.Load(token, designsourceedit.ScopeOf(sourceBasis(v, a, path, epoch)))
		if err != nil {
			return nil, sourceBaseRefusal(a, path)
		}
		if path != "design.json" {
			if _, sent := in.Files[path]; !sent {
				return nil, refuse(Invalid, "sourceBases contains unsent file %q", path)
			}
		}
		loaded[path] = base
	}
	return loaded, nil
}

func sourceFileBaseline(a store.Artifact, path string, text *string, held map[string]string, read designsourceedit.ReadBase, hasBase bool) (*string, error) {
	current, present := held[path]
	if !hasBase {
		if text == nil && !present {
			return nil, nil
		}
		if text != nil && present && *text == current {
			return &current, nil
		}
		return nil, sourceBaseRefusal(a, path)
	}
	if read.Absent {
		if present {
			return nil, sourceBaseRefusal(a, path)
		}
		return nil, nil
	}
	// Whole-frame removal cannot erase source changed since the explicit read.
	if !present || (text == nil && current != read.Source) {
		return nil, sourceBaseRefusal(a, path)
	}
	return &read.Source, nil
}
