package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// DocEditInput is a change to some blocks of a doc.
type DocEditInput struct {
	// BaseVersion, when given, is the newest saved version the caller knows; a
	// newer one refuses the call like any stale publish. The guards on the
	// blocks (ifHash) are what protect what people type in a live room, which
	// moves faster than versions do.
	BaseVersion int
	Ops         []docedit.Op
}

// DocEdited is what a doc_edit did.
type DocEdited struct {
	// Version is the newest saved version after the call.
	Version int `json:"version"`
	// Saved says a version was written now. It is false when the doc has a live
	// room: the edit is in the room, where people see it, and a version follows
	// when the room has been quiet.
	Saved bool `json:"saved"`
	docedit.Result
	// CommentsOnDeletedBlocks are the comment threads anchored in a block the
	// call deleted. The edit is not held back for them: the threads keep their
	// quote, and the caller is told so that someone looks at them.
	CommentsOnDeletedBlocks []DetachedThread `json:"commentsOnDeletedBlocks,omitempty"`
}

// DocRead is a doc as an agent reads it to edit it.
type DocRead struct {
	Artifact store.Artifact
	Doc      docfmt.Doc
}

// ReadDoc is the doc a viewer may read, as it is now: with the content of its
// live room when it has one.
func (s *Service) ReadDoc(ctx context.Context, v Viewer, id string) (DocRead, error) {
	defer s.admission.Shared(id)()
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return DocRead{}, err
	}
	d, a, err := s.currentDoc(ctx, a)
	return DocRead{Artifact: a, Doc: d}, err
}

func (s *Service) currentDoc(ctx context.Context, a store.Artifact) (docfmt.Doc, store.Artifact, error) {
	if a.Kind != (kind.Doc{}).Name() {
		return docfmt.Doc{}, a, refuse(Invalid, "this artifact is a %s, not a doc, so it has no blocks; read it without view, and publish a change", a.Kind)
	}
	a, err := s.Current(ctx, a)
	if err != nil {
		return docfmt.Doc{}, a, err
	}
	d, err := docfmt.Parse(a.Content)
	if err != nil {
		return docfmt.Doc{}, a, fmt.Errorf("artifact %s holds a doc that does not parse: %w", a.ID, err)
	}
	return d, a, nil
}

// DocEdit changes some blocks of a doc the viewer may edit. The operations are
// done on the doc as it is now (the live room's, when it has one), and all of
// them or none. With a live room the change goes into the room, as an edit by
// the viewer, touching only what differs, so what people are typing stays;
// without one it is saved as a new version.
func (s *Service) DocEdit(ctx context.Context, v Viewer, id string, in DocEditInput) (DocEdited, error) {
	defer s.admission.Shared(id)()
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return DocEdited{}, err
	}
	if a.Kind != (kind.Doc{}).Name() {
		return DocEdited{}, refuse(Invalid, "doc_edit changes docs; this artifact is a %s: read it, and publish the whole change", a.Kind)
	}
	if in.BaseVersion != 0 {
		if err := s.checkBase(ctx, a, in.BaseVersion); err != nil {
			return DocEdited{}, err
		}
	}
	doc, a, err := s.currentDoc(ctx, a)
	if err != nil {
		return DocEdited{}, err
	}
	next, content, res, err := s.editedContent(a, doc, in)
	if err != nil {
		return DocEdited{}, err
	}
	out := DocEdited{Version: a.Version, Result: res}
	if before, err := docfmt.Canonical(doc); err == nil && before == content {
		return out, nil
	}
	if out, err = s.withDetachedThreads(ctx, v, a, out, docedit.Removed(doc, next)); err != nil {
		return DocEdited{}, err
	}
	if live, err := s.live(ctx, id); err != nil {
		return DocEdited{}, err
	} else if live {
		if err := s.rooms.Apply(s.mutationContext(ctx, "doc_edit"), id, a.Kind, kind.Bundle{Content: content}, v.Username); err != nil {
			return DocEdited{}, fmt.Errorf("write into the live copy of artifact %s: %w", id, err)
		}
		s.showAgent(ctx, v, id, firstWritten(res))
		return out, nil
	}
	if err := s.SaveFromRoom(s.mutationContext(ctx, "doc_edit"), id, a.Version, kind.Bundle{Content: content}, []string{v.Username}); err != nil {
		if errors.Is(err, store.ErrStale) {
			return DocEdited{}, s.conflict(ctx, id, a.Version)
		}
		return DocEdited{}, err
	}
	out.Version, out.Saved = a.Version+1, true
	return out, nil
}

// live says whether the artifact has a live room.
func (s *Service) live(ctx context.Context, id string) (bool, error) {
	if s.rooms == nil {
		return false, nil
	}
	return s.rooms.Live(ctx, id)
}

// editedContent does the operations on the doc and gives the content they
// make, or the refusal that says why they cannot.
func (s *Service) editedContent(a store.Artifact, doc docfmt.Doc, in DocEditInput) (docfmt.Doc, string, docedit.Result, error) {
	next, res, err := docedit.Apply(doc, in.Ops, docfmt.NewID)
	if refusal, ok := errors.AsType[*docedit.Refusal](err); ok {
		return doc, "", res, refuse(Invalid, "%s", refusal.Msg)
	}
	if stale, ok := errors.AsType[*docedit.Stale](err); ok {
		return doc, "", res, &Error{Code: Conflict, Msg: stale.Msg, Current: a.Version, ArtifactID: a.ID, Stale: stale.Blocks}
	}
	if err != nil {
		return doc, "", res, err
	}
	content, err := docfmt.Canonical(next)
	return next, content, res, err
}

// firstWritten is the block an edit's caret goes to: the first block it wrote,
// or none when it only deleted.
func firstWritten(res docedit.Result) string {
	if len(res.Blocks) > 0 {
		return res.Blocks[0].ID
	}
	return ""
}
