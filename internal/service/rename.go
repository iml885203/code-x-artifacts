package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// MaxTitleRunes is the longest title a rename takes: the header holds one line.
const MaxTitleRunes = 200

// Rename gives an artifact the viewer may edit a new title. It is a new version
// with the content and the files the newest saved version has, so history stays
// append-only and the content is not touched (a doc's H1 is its own text). Work
// in a live room is not part of it: the version a room cuts later carries the
// title the artifact has by then. The same title again changes nothing.
func (s *Service) Rename(ctx context.Context, v Viewer, id, title string) (store.Artifact, error) {
	defer s.admission.Shared(id)()
	return s.rename(ctx, v, id, title)
}

func (s *Service) rename(ctx context.Context, v Viewer, id, title string) (store.Artifact, error) {
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return store.Artifact{}, refuse(Invalid, "a title is required")
	case strings.ContainsAny(title, "\r\n"):
		return store.Artifact{}, refuse(Invalid, "a title is one line")
	case utf8.RuneCountInString(title) > MaxTitleRunes:
		return store.Artifact{}, refuse(Invalid, "a title is at most %d characters; this one has %d", MaxTitleRunes, utf8.RuneCountInString(title))
	}
	if title == a.Title {
		return a, nil
	}
	k, err := kind.Lookup(a.Kind)
	if err != nil {
		return store.Artifact{}, refuse(Invalid, "%s", err)
	}
	plan, err := s.planFiles(ctx, k, a, nil)
	if err != nil {
		return store.Artifact{}, err
	}
	rev := store.Revision{Title: title, Format: a.Format, Content: a.Content, Author: v.Username, Files: plan.manifest, FilesFrom: plan.from}
	return s.save(ctx, v, a, a.Version, rev, "")
}
