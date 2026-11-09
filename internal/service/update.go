package service

import (
	"context"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// checkAudienceChange refuses a publish that names another audience from someone who may not change it.
func checkAudienceChange(v Viewer, a store.Artifact, in PublishInput) error {
	if in.Visibility != "" && in.Visibility != a.Visibility && !v.CanChange(a) {
		return refuse(Forbidden, "only the author can change who can see this artifact; publish without visibility to keep it as it is")
	}
	return nil
}

// Update saves a new version of an artifact the viewer may edit. When
// in.BaseVersion is given and is no longer the newest, nothing is saved and the
// refusal is a Conflict that names the newest version. Only the author may name
// another audience; an editor who repeats the current one changes nothing.
func (s *Service) Update(ctx context.Context, v Viewer, id string, in PublishInput) (Published, error) {
	defer s.admitPublish(id, in)()
	return s.update(ctx, v, id, in)
}

// update is Update for a caller that has already been admitted.
func (s *Service) update(ctx context.Context, v Viewer, id string, in PublishInput) (Published, error) {
	a, err := s.Edit(ctx, v, id)
	if err != nil {
		return Published{}, err
	}
	if err := checkAudienceChange(v, a, in); err != nil {
		return Published{}, err
	}
	if err := s.checkBase(ctx, a, in.BaseVersion); err != nil {
		return Published{}, err
	}
	in, err = s.validateWithFiles(ctx, in, a)
	if err != nil {
		return Published{}, err
	}
	k, err := kind.Lookup(in.Kind)
	if err != nil {
		return Published{}, refuse(Invalid, "%s", err)
	}
	mutationCtx := s.mutationContext(ctx, "publish")
	// While the artifact has a live room its content is written only through it.
	if p, done, err := s.publishLive(mutationCtx, v, a, k, in); done || err != nil {
		return p, err
	}
	if len(in.SourceBases) > 0 {
		return Published{}, nonliveSourceBaseRefusal(a)
	}
	if a, in.BaseVersion, err = s.publishThroughRoom(mutationCtx, v, a, k, in); err != nil {
		return Published{}, err
	}
	plan, err := s.planFiles(ctx, k, a, in.Files)
	if err != nil {
		return Published{}, err
	}
	if err := validateContent(k, in.Content, in.Format, plan.set); err != nil {
		return Published{}, contentRefusal(err, in)
	}
	rewritten, err := plan.normalizeFiles(k, in.Format, in.Files)
	if err != nil {
		return Published{}, err
	}
	if err := s.storeBlobs(a.ID, plan); err != nil {
		return Published{}, err
	}
	rev := store.Revision{Title: in.Title, Format: in.Format, Content: in.Content, Author: v.Username, Agents: agents(ctx, v), Files: plan.manifest, FilesFrom: plan.from}
	saved, err := s.save(mutationCtx, v, a, in.BaseVersion, rev, in.Visibility)
	if err != nil {
		return Published{}, err
	}
	return Published{Artifact: saved, Review: review(saved, a.Content, plan.set, sentPaths(in.Files)), Rewritten: rewritten}, nil
}

func (s *Service) admitPublish(id string, in PublishInput) func() {
	if in.Visibility != "" {
		return s.admission.Exclusive(id)
	}
	return s.admission.Shared(id)
}
