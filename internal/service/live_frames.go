package service

import (
	"context"
	"errors"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// LiveGrant is what the frames of a design are served under to a person who
// may join its room: the frames as the room holds them, not as a version saved
// them. It names the account it was made for (internal/web signs it), and every
// read asks again whether that account may still see the room.
type LiveGrant struct {
	ArtifactID string
	Account    string
}

// LiveFrame returns one frame of a design as its room holds it now, and the
// design with the room's manifest as its content, for the account a live grant
// was made for. Without a room it is the frame of the newest version. A grant
// for someone who may not see the room, a frame that is not there and anything
// else it cannot read all get the same not-found.
func (s *Service) LiveFrame(ctx context.Context, g LiveGrant, path string) (store.Artifact, string, error) {
	if designfmt.CheckFramePath(path) != nil {
		return store.Artifact{}, "", errNotFound
	}
	defer s.admission.Shared(g.ArtifactID)()
	v := Viewer{Username: g.Account, PublicLinks: s.publicLinks}
	a, err := s.Artifact(ctx, v, g.ArtifactID)
	if err != nil {
		return store.Artifact{}, "", err
	}
	if a.Kind != "design" || !v.CanSeeLive(a) {
		return store.Artifact{}, "", errNotFound
	}
	now, err := s.designFileNow(ctx, v, a, path)
	if refusal, ok := errors.AsType[*Error](err); ok && refusal.Code == NotFound {
		return store.Artifact{}, "", errNotFound
	}
	if err != nil {
		return store.Artifact{}, "", err
	}
	if !now.Present {
		return store.Artifact{}, "", errNotFound
	}
	if now.Live {
		a.Content = now.Manifest
	}
	return a, now.Text, nil
}

// designFile is one file of a design as people see it now.
type designFile struct {
	Ref     store.FileRef
	Text    string
	Present bool
	// Live says it was read from the room, whose manifest and epoch come with it.
	Live     bool
	Manifest string
	Epoch    string
}

// designFileNow reads one file of a design as people see it now: from the room
// when it is live (a file the room does not have is not present, whatever a
// version holds), else from the newest version. It is the one reading behind
// the live address (LiveFrame) and reading the file over HTTP and MCP
// (ReadSourceFile), so they never disagree.
func (s *Service) designFileNow(ctx context.Context, v Viewer, a store.Artifact, path string) (designFile, error) {
	held, live, err := s.sourceFile(ctx, a, path)
	if err != nil {
		return designFile{}, err
	}
	if !live {
		ref, text, err := s.readFile(ctx, v, a.ID, path, 0)
		return designFile{Ref: ref, Text: text, Present: err == nil}, err
	}
	return designFile{
		Ref:  store.FileRef{Path: path, ContentType: files.ContentTypeOf(path), Size: int64(len(held.Text))},
		Text: held.Text, Present: held.Present, Live: true, Manifest: held.Content, Epoch: held.Epoch,
	}, nil
}
