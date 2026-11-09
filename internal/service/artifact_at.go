package service

import (
	"context"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// ArtifactAt is the artifact as one saved version left it: its title, kind,
// format, content and files. Version 0, or the newest version's number, is the
// artifact itself, for anyone who can see it; an older version follows the
// history rule of Version, so a reader is refused what the author took out.
func (s *Service) ArtifactAt(ctx context.Context, v Viewer, id string, version int) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil || version == 0 || version == a.Version {
		return a, err
	}
	ver, err := s.Version(ctx, v, id, version)
	if err != nil {
		return store.Artifact{}, err
	}
	a.Title, a.Kind, a.Format, a.Content, a.Version, a.Files = ver.Title, ver.Kind, ver.Format, ver.Content, ver.Number, ver.Files
	return a, nil
}
