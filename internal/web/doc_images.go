package web

import (
	"context"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// withoutMissingPictures puts the placeholder in a doc page where it shows a
// file of the artifact's own that is no longer uploaded. Uploads belong to the
// artifact and not to a version, so an older version can name one that was
// deleted. A failure to list the files leaves the page as rendered: the
// pictures are then drawn and the browser shows the ones it cannot load.
func (s *Server) withoutMissingPictures(ctx context.Context, artifactID, page string) string {
	if !strings.Contains(page, assets.PathPrefix(artifactID)) {
		return page
	}
	list, err := s.store.Assets(ctx, artifactID)
	if err != nil {
		s.log.Error("list files for the placeholders of a doc", "id", artifactID, "err", err)
		return page
	}
	have := make(map[string]bool, len(list))
	for _, a := range list {
		have[a.Name] = true
	}
	return docfmt.MissingPictures(page, artifactID, func(name string) bool { return have[name] })
}
