// Package service is what pictor does with artifacts and their shared data:
// who may see or change what, what a valid publish looks like, which kinds
// exist. The HTTP API and the MCP tools are two front ends over it, so a rule
// changes in one place and the two cannot drift apart. It knows nothing about
// transport authentication or secret credentials: callers supply the Viewer and nonsecret provenance.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/artifactguard"
	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/designsourceedit"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// MaxDocBytes is the largest shared document.
const MaxDocBytes = 1 << 20

// Service holds the artifact rules over a store.
type Service struct {
	admission   artifactguard.Admission
	store       *store.Store
	files       *assets.Disk
	publicLinks bool
	// usageReaders are the accounts the deployment named to read usage (Viewer.CanReadUsage).
	usageReaders []string
	fileLimits   files.Limits
	// watchers are told when who may see or edit an artifact changed (OnAccessChange).
	watchers []func(artifactID string)
	// rooms is the live copy of the artifacts that have one, nil while there is none.
	rooms LiveRooms
	// now is the clock for the expiry of publish sessions.
	now func() time.Time
	// viewers is who has an artifact open now, and how long they stay (viewers.go).
	viewers                 viewerBook
	personStays, agentStays time.Duration
	sourceOnce              sync.Once
	sourceBases             *designsourceedit.ReadBases
	sourceErr               error
	// maxGuests and maxGuestsPerArtifact bound the guest rows anonymous traffic can add (guests.go).
	maxGuests, maxGuestsPerArtifact int
}

// New builds a Service. disk holds the bytes of the artifacts' assets. publicLinks is whether the deployment lets an artifact
// be public; see Viewer.CanView.
func New(st *store.Store, disk *assets.Disk, publicLinks bool) *Service {
	return &Service{admission: &artifactguard.Registry{}, store: st, files: disk, publicLinks: publicLinks, fileLimits: files.DefaultLimits, now: time.Now, personStays: PersonStays, agentStays: AgentStays, maxGuests: MaxGuests, maxGuestsPerArtifact: MaxGuestsPerArtifact}
}

// ArtifactGuard is the process admission boundary shared with sockets and peers.
func (s *Service) ArtifactGuard() artifactguard.Admission { return s.admission }

// SetArtifactGuard registers the same process registry before the service is used.
func (s *Service) SetArtifactGuard(g artifactguard.Admission) {
	if g != nil {
		s.admission = g
	}
}

// OnAccessChange registers fn to be called with an artifact's id whenever who
// may see it or edit it has changed or the artifact is gone: its audience, its
// editors, the organisation switch, or deletion. It is called after the change
// is saved, on the goroutine that made it, so a connection that outlives a
// request (the realtime socket) can be re-checked before the change returns.
// Register before use. Callbacks run inside exclusive admission and must not
// reenter guarded reads or mutators; unguarded Edit and Hub disconnect are safe.
func (s *Service) OnAccessChange(fn func(artifactID string)) {
	s.watchers = append(s.watchers, fn)
}

func (s *Service) accessChanged(artifactID string) {
	s.retireSourceBases(artifactID)
	for _, fn := range s.watchers {
		fn(artifactID)
	}
}

// PublicLinks reports whether this deployment allows public artifacts.
func (s *Service) PublicLinks() bool { return s.publicLinks }

// Viewer is the signed-in person with this account name.
func (s *Service) Viewer(username string) Viewer {
	return Viewer{Username: username, PublicLinks: s.publicLinks, UsageReaders: s.usageReaders}
}

// Anonymous is the viewer of a request that carries no credentials.
func (s *Service) Anonymous() Viewer { return Viewer{PublicLinks: s.publicLinks} }

// Artifact loads an artifact the viewer may see. Whoever may not gets the same
// answer as for an id that does not exist, so a refusal never says whether an
// artifact is there (TestWhoCannotSeeAnArtifactCannotTellItExists).
func (s *Service) Artifact(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.store.Artifact(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !v.CanView(a)) {
		return store.Artifact{}, errNotFound
	}
	if err != nil {
		return store.Artifact{}, fmt.Errorf("load artifact %s: %w", id, err)
	}
	if err := s.withViewerState(ctx, v, a.ID, &a); err != nil {
		return store.Artifact{}, err
	}
	if err := s.withFiles(ctx, &a); err != nil {
		return store.Artifact{}, err
	}
	s.withSource(ctx, v, &a)
	return v.RedactArtifact(a), nil
}

// withSource keeps the id of the artifact a copy came from only while the
// viewer may open that artifact, so the link is never one that leads nowhere
// or tells a viewer something exists that they may not see. The title stays: it
// is what the copy recorded.
func (s *Service) withSource(ctx context.Context, v Viewer, a *store.Artifact) {
	if a.CopiedFrom == nil {
		return
	}
	src := *a.CopiedFrom
	if from, err := s.store.Artifact(ctx, src.ID); err != nil || !v.CanView(from) {
		src.ID = ""
	}
	a.CopiedFrom = &src
}

// Edit is Artifact for publishing, restoring and managing files: someone who
// can see the artifact but may not edit it is refused with Forbidden.
func (s *Service) Edit(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if !v.CanEdit(a) {
		return store.Artifact{}, refuse(Forbidden, "only the author, or someone the author made an editor, can edit this artifact")
	}
	return a, nil
}

// Own is Artifact for what only the author does (change the audience or the
// editors, delete): someone who can see the artifact but did not author it is
// refused with Forbidden.
func (s *Service) Own(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if !v.CanChange(a) {
		return store.Artifact{}, refuse(Forbidden, "only the author can change this artifact")
	}
	return a, nil
}

// PublishInput is what a publisher sends.
type PublishInput struct {
	Title string
	// Kind is optional: a new artifact is html and an update keeps the kind
	// the artifact already has.
	Kind string
	// Format is optional: a new artifact gets the kind's current content
	// format version, and an update keeps the version the artifact already
	// has. Naming a version on an update upgrades (or pins) the artifact to it;
	// the content is validated against that version.
	Format  string
	Content string
	// Blocks is structured content for a kind that takes it (a doc): JSON that
	// names the content in the kind's own terms. Blocks and Content are
	// alternatives; Blocks implies the kind's structured format.
	Blocks json.RawMessage
	// stored says Content is already in the form the store keeps, as when a
	// copy takes the content of its source, so the kind does not compose it.
	stored bool
	// fromFile says the content came from a publish session, where a size
	// refusal must not send the publisher to the flow it just used.
	fromFile bool
	// BaseVersion is the version the publisher started from. On an update it
	// must still be the newest, otherwise the update is refused with Conflict
	// and nothing is saved. Zero means the publisher did not say, and the
	// update is applied on top of whatever is newest (so a publisher that
	// predates versions keeps working, at the risk it always had).
	BaseVersion int
	// Visibility is optional: a new artifact defaults to private and an
	// update keeps whatever the artifact already has.
	Visibility store.Visibility
	// Files changes the files that come with the page, as a difference on the
	// version being built on: a text adds or replaces the file at that path,
	// nil removes it, and a path that is not named is carried over. Without
	// Files the version keeps exactly the files the one before it had.
	Files map[string]*string
	// SourceBases names the exact per-file live reads this change was built on.
	SourceBases map[string]string
}

// Published is the result of a publish: the artifact as saved, and what its
// kind has to say about the content the publisher cannot see for itself (a
// slide that will not fit, a slide that went missing). Review never means the
// publish failed; it is advice the publisher should act on.
type Published struct {
	store.Artifact
	Review []string
	// Rewritten is the text of each file as it was stored, for the files whose
	// kind stores them in a form of its own and so changed what the publisher
	// sent (a design frame gets a data-pid on every element). Nil when nothing was.
	Rewritten map[string]string
	// Live says the publish went into the artifact's live room, where people are
	// editing it, and no version was saved: the room's next snapshot is the
	// version, and Artifact is still the newest saved one.
	Live bool
}

// Create publishes a new artifact authored by the viewer.
func (s *Service) Create(ctx context.Context, v Viewer, in PublishInput) (Published, error) {
	if len(in.SourceBases) > 0 {
		return Published{}, refuse(Invalid, "sourceBases only apply to an existing live design")
	}
	if v.Anonymous() {
		return Published{}, refuse(Forbidden, "sign in to publish")
	}
	in, err := s.validateWithFiles(ctx, in, store.Artifact{})
	if err != nil {
		return Published{}, err
	}
	if in.Visibility == "" {
		in.Visibility = store.VisibilityPrivate
	}
	k, err := kind.Lookup(in.Kind)
	if err != nil {
		return Published{}, refuse(Invalid, "%s", err)
	}
	plan, err := s.planFiles(ctx, k, store.Artifact{}, in.Files)
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
	id := store.NewArtifactID()
	if err := s.storeBlobs(id, plan); err != nil {
		return Published{}, err
	}
	a, err := s.store.CreateArtifactFrom(s.mutationContext(ctx, "publish"), store.NewArtifact{
		ID: id, Title: in.Title, Kind: in.Kind, Format: in.Format, Content: in.Content, Author: v.Username,
		Visibility: in.Visibility, Files: plan.manifest,
	})
	if err != nil {
		return Published{}, fmt.Errorf("create artifact: %w", err)
	}
	a.Files = plan.manifest
	report(ctx, Information, "artifact created", "id", a.ID, "user", a.Author)
	s.logFiles(ctx, "artifact files saved", a.ID, plan.manifest)
	return Published{Artifact: a, Review: review(a, "", plan.set, sentPaths(in.Files)), Rewritten: rewritten}, nil
}

// checkBase refuses a base version that is not the artifact's newest.
func (s *Service) checkBase(ctx context.Context, a store.Artifact, base int) error {
	switch {
	case base < 0:
		return refuse(Invalid, "baseVersion must be a version number from 1; read the artifact to see its current version")
	case base > a.Version:
		return refuse(Invalid, "this artifact has no version %d; its newest is %d", base, a.Version)
	case base != 0 && base != a.Version:
		return s.conflict(ctx, a.ID, base)
	}
	return nil
}

// conflict builds the refusal for a stale base from the artifact as it is now,
// so the message names the version that won.
func (s *Service) conflict(ctx context.Context, id string, base int) error {
	now, err := s.store.Artifact(ctx, id)
	if err != nil {
		return fmt.Errorf("reload artifact %s after a stale update: %w", id, err)
	}
	// The newest version is handed to a publisher that will merge into it and
	// send it back, so it carries the content as a publisher writes it.
	if written, err := Written(now.Kind, now.Format, now.Content); err == nil {
		now.Content = written
	}
	return &Error{
		Code: Conflict, Current: now.Version, Latest: &now, ArtifactID: id,
		Msg: fmt.Sprintf("nothing was saved: this artifact is now at version %d, but you started from version %d, so saving would overwrite the newer version. "+
			"Read version %d, merge your changes into it, and publish again with baseVersion %d", now.Version, base, now.Version, now.Version),
	}
}

// save writes a revision and turns a lost race into the same Conflict as a
// stale base that was seen up front.
func (s *Service) save(ctx context.Context, v Viewer, a store.Artifact, base int, rev store.Revision, visibility store.Visibility) (store.Artifact, error) {
	err := s.store.UpdateArtifact(ctx, a.ID, base, rev, visibility)
	if errors.Is(err, store.ErrStale) {
		if base == 0 {
			// The publisher named no base, but the files it built on are not
			// the newest any more: say which version they were.
			base = rev.FilesFrom
		}
		return store.Artifact{}, s.conflict(ctx, a.ID, base)
	}
	if err != nil {
		return store.Artifact{}, fmt.Errorf("update artifact %s: %w", a.ID, err)
	}
	report(ctx, Information, "artifact saved", "id", a.ID, "user", rev.Author, "restoredFrom", rev.RestoredFrom)
	s.logFiles(ctx, "artifact files saved", a.ID, rev.Files)
	s.recordAccess(ctx, a, rev.Author)
	return s.reload(ctx, v, a.ID)
}

// history is Edit for reading old versions, with a refusal that says what the
// viewer asked for.
func (s *Service) history(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if !v.CanEdit(a) {
		return store.Artifact{}, refuse(Forbidden, "only the author and editors can read this artifact's older versions")
	}
	return a, nil
}

// Versions lists the saved versions of an artifact the viewer may edit, newest
// first. Only the author and editors read history: an old version can hold
// what the author took out of the page on purpose, which the people who only
// read it must not get back.
func (s *Service) Versions(ctx context.Context, v Viewer, id string) ([]store.Version, error) {
	a, err := s.history(ctx, v, id)
	if err != nil {
		return nil, err
	}
	list, err := s.store.Versions(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions of artifact %s: %w", a.ID, err)
	}
	return list, nil
}

// Version reads one saved version of an artifact the viewer may edit, with content.
func (s *Service) Version(ctx context.Context, v Viewer, id string, number int) (store.Version, error) {
	a, err := s.history(ctx, v, id)
	if err != nil {
		return store.Version{}, err
	}
	ver, err := s.store.Version(ctx, a.ID, number)
	if errors.Is(err, store.ErrNotFound) {
		return store.Version{}, refuse(Invalid, "this artifact has no version %d; its versions are 1 to %d", number, a.Version)
	}
	if err != nil {
		return store.Version{}, fmt.Errorf("read version %d of artifact %s: %w", number, a.ID, err)
	}
	if err := s.withVersionFiles(ctx, a.ID, &ver); err != nil {
		return store.Version{}, err
	}
	return ver, nil
}

// SetVisibility changes who can see the viewer's own artifact.
func (s *Service) SetVisibility(ctx context.Context, v Viewer, id string, visibility store.Visibility) (store.Artifact, error) {
	defer s.admission.Exclusive(id)()
	return s.setVisibility(ctx, v, id, visibility)
}

func (s *Service) setVisibility(ctx context.Context, v Viewer, id string, visibility store.Visibility) (store.Artifact, error) {
	a, err := s.Own(ctx, v, id)
	if err != nil {
		return store.Artifact{}, err
	}
	if err := s.checkVisibility(visibility); err != nil {
		return store.Artifact{}, err
	}
	if err := s.store.SetSharing(ctx, a.ID, visibility); err != nil {
		return store.Artifact{}, fmt.Errorf("set visibility of artifact %s: %w", a.ID, err)
	}
	report(ctx, Information, "artifact sharing changed", "id", a.ID, "user", v.Username, "visibility", visibility)
	s.recordAccess(ctx, a, v.Username)
	return s.reload(ctx, v, a.ID)
}

func (s *Service) reload(ctx context.Context, v Viewer, id string) (store.Artifact, error) {
	a, err := s.store.Artifact(ctx, id)
	if err != nil {
		return store.Artifact{}, fmt.Errorf("reload artifact %s: %w", id, err)
	}
	if err := s.withViewerState(ctx, v, a.ID, &a); err != nil {
		return store.Artifact{}, err
	}
	if err := s.withFiles(ctx, &a); err != nil {
		return store.Artifact{}, err
	}
	s.withSource(ctx, v, &a)
	return v.RedactArtifact(a), nil
}

// Delete removes the viewer's own artifact, its shared data and its assets.
func (s *Service) Delete(ctx context.Context, v Viewer, id string) error {
	defer s.admission.Exclusive(id)()
	a, err := s.Own(ctx, v, id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteArtifact(ctx, a.ID); err != nil {
		return fmt.Errorf("delete artifact %s: %w", a.ID, err)
	}
	// Version files and their manifest went with the artifact (the cascade);
	// the blobs are in the artifact's directory, removed below.
	// The records went with the artifact; the files are removed after, so a
	// failure here leaves unreachable bytes, never a record without its file.
	if err := s.files.RemoveArtifact(a.ID); err != nil {
		report(ctx, Failure, "could not delete the asset files of a deleted artifact", "id", a.ID, "err", err)
	}
	report(ctx, Information, "artifact deleted", "id", a.ID, "user", v.Username)
	s.accessChanged(a.ID)
	return nil
}

// DocCount is how many shared documents a viewer's own artifact holds; it is
// what a delete would lose besides the page.
func (s *Service) DocCount(ctx context.Context, v Viewer, id string) (int, error) {
	a, err := s.Own(ctx, v, id)
	if err != nil {
		return 0, err
	}
	n, err := s.store.DocCount(ctx, a.ID)
	if err != nil {
		return 0, fmt.Errorf("count documents of artifact %s: %w", a.ID, err)
	}
	return n, nil
}

// AssetCount is how many files a viewer's own artifact holds; like DocCount it
// is part of what a delete would lose.
func (s *Service) AssetCount(ctx context.Context, v Viewer, id string) (int, error) {
	a, err := s.Own(ctx, v, id)
	if err != nil {
		return 0, err
	}
	list, err := s.store.Assets(ctx, a.ID)
	if err != nil {
		return 0, fmt.Errorf("count assets of artifact %s: %w", a.ID, err)
	}
	return len(list), nil
}

// Profile is what whoami returns. The anonymous viewer is an empty person
// with Anonymous set.
type Profile struct {
	store.User
	Anonymous bool `json:"anonymous"`
}

// Me describes the viewer. Someone who has never signed in on this server
// still gets an answer built from their account name.
func (s *Service) Me(ctx context.Context, v Viewer) (Profile, error) {
	if v.Anonymous() {
		return Profile{User: store.User{Groups: []string{}}, Anonymous: true}, nil
	}
	u, err := s.store.User(ctx, v.Username)
	if errors.Is(err, store.ErrNotFound) {
		return Profile{User: store.User{Username: v.Username, DisplayName: v.Username, Groups: []string{}}}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("load user %s: %w", v.Username, err)
	}
	return Profile{User: u}, nil
}

// Docs lists one collection of an artifact's shared data.
func (s *Service) Docs(ctx context.Context, v Viewer, id, collection string) ([]store.Doc, error) {
	if _, err := s.Artifact(ctx, v, id); err != nil {
		return nil, err
	}
	docs, err := s.store.Docs(ctx, id, collection)
	if err != nil {
		return nil, fmt.Errorf("list documents of artifact %s: %w", id, err)
	}
	for i := range docs {
		docs[i] = v.Redact(docs[i])
	}
	return docs, nil
}

// Doc reads one shared document.
func (s *Service) Doc(ctx context.Context, v Viewer, id, collection, doc string) (store.Doc, error) {
	if _, err := s.Artifact(ctx, v, id); err != nil {
		return store.Doc{}, err
	}
	d, err := s.store.Doc(ctx, id, collection, doc)
	if errors.Is(err, store.ErrNotFound) {
		return store.Doc{}, errNoDocument
	}
	if err != nil {
		return store.Doc{}, fmt.Errorf("read document of artifact %s: %w", id, err)
	}
	return v.Redact(d), nil
}

// writable is Artifact for writes to the shared db. The front ends keep the
// anonymous viewer out before they get here; this is the second lock, so wiring
// a route wrongly cannot open the db.
func (s *Service) writable(ctx context.Context, v Viewer, id string) error {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return err
	}
	if !v.CanWrite(a) {
		return refuse(Forbidden, "sign in to write")
	}
	return nil
}

// Writable reports whether the viewer may write to the artifact's shared db.
// Front ends call it before reading a request body, so a body error never
// tells someone who may not see the artifact that it exists.
func (s *Service) Writable(ctx context.Context, v Viewer, id string) error {
	return s.writable(ctx, v, id)
}

// Written is the content of an artifact as its publisher writes it, which is
// what the publish tool takes back: a doc stored as blocks is Markdown.
func Written(kindName, format, content string) (string, error) {
	k, err := kind.Lookup(kindName)
	if err != nil {
		return "", err
	}
	if c, ok := k.(kind.Composer); ok {
		return c.Written(content, format)
	}
	return content, nil
}

// SetClock replaces the clock that publish sessions expire by, for tests that
// must not wait ten minutes.
func (s *Service) SetClock(now func() time.Time) { s.now = now }
