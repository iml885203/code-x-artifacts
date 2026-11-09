package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// BlobGrace is how long a blob nobody names is kept before the sweep deletes
// it: far longer than any publish takes, so the sweep never removes the blob of
// a publish that has written its bytes and not yet recorded them.
const BlobGrace = 24 * time.Hour

// SetFileLimits replaces the limits on an artifact's files. They are fixed in
// code for a deployment (files.DefaultLimits); this is for tests that need to
// reach a limit without sending 256 MB.
func (s *Service) SetFileLimits(l files.Limits) { s.fileLimits = l }

// FileLimits are the limits in force.
func (s *Service) FileLimits() files.Limits { return s.fileLimits }

// withFiles gives an artifact the files of its newest version.
func (s *Service) withFiles(ctx context.Context, a *store.Artifact) error {
	list, err := s.store.VersionFiles(ctx, a.ID, a.Version)
	if err != nil {
		return fmt.Errorf("list files of artifact %s: %w", a.ID, err)
	}
	if len(list) > 0 {
		a.Files = list
	}
	return nil
}

// plannedFiles is what a publish does with the files of the version it builds on.
type plannedFiles struct {
	// manifest is every file of the new version.
	manifest []store.FileRef
	// from is the version the manifest was worked out from, 0 when it does not
	// depend on one (see store.Revision.FilesFrom).
	from int
	// blobs are the contents to store before the version is saved.
	blobs map[string][]byte
	// set is the text of every file of the new version, for a kind that compiles
	// them into its page; nil for any other kind, whose files are never read
	// to publish a page.
	set files.Set
}

// replace stores text instead of what the publisher sent for the file at path
// (its normal form), keeping the plan's manifest, blobs and set in step.
func (p *plannedFiles) replace(path, text string) {
	data := []byte(text)
	sum := sha256Hex(data)
	for i := range p.manifest {
		if p.manifest[i].Path == path {
			p.manifest[i].Size, p.manifest[i].SHA256 = int64(len(data)), sum
		}
	}
	p.blobs[sum] = data
	if p.set != nil {
		p.set[path] = text
	}
}

// dropUnusedBlobs forgets the blobs that no file of the manifest names any more,
// the bytes a publish sent before they were normalised.
func (p *plannedFiles) dropUnusedBlobs() {
	used := make(map[string]bool, len(p.manifest))
	for _, f := range p.manifest {
		used[f.SHA256] = true
	}
	for sum := range p.blobs {
		if !used[sum] {
			delete(p.blobs, sum)
		}
	}
}

// normalizeFiles gives each file the publish sent the form its kind stores it
// in, for a kind that has one, once the content has been validated. It returns
// the files whose stored text is not what was sent, so the publisher is told.
func (p *plannedFiles) normalizeFiles(k kind.Kind, format string, patch map[string]*string) (map[string]string, error) {
	n, ok := k.(kind.FileNormalizer)
	if !ok {
		return nil, nil
	}
	rewritten := map[string]string{}
	for path, sent := range patch {
		if sent == nil {
			continue
		}
		text, err := n.NormalizeFile(format, path, *sent)
		if err != nil {
			return nil, refuse(Invalid, "file %s: %s", path, err)
		}
		if text != *sent {
			p.replace(path, text)
			rewritten[path] = text
		}
	}
	p.dropUnusedBlobs()
	return rewritten, nil
}

// planFiles works out the files of the version a publish creates: the version
// it builds on (current, the zero value for a new artifact) with the publisher's
// patch applied. Nothing is written; a refusal here saves nothing at all.
func (s *Service) planFiles(ctx context.Context, k kind.Kind, current store.Artifact, patch map[string]*string) (plannedFiles, error) {
	var carried []store.FileRef
	if current.ID != "" {
		var err error
		if carried, err = s.store.VersionFiles(ctx, current.ID, current.Version); err != nil {
			return plannedFiles{}, fmt.Errorf("list files of artifact %s: %w", current.ID, err)
		}
	}
	if len(patch) == 0 {
		plan := plannedFiles{manifest: carried}
		if len(carried) > 0 {
			plan.from = current.Version
			var err error
			if plan.set, err = s.bundleSet(current.ID, k, carried, nil); err != nil {
				return plannedFiles{}, err
			}
		}
		return plan, nil
	}
	fk, ok := k.(kind.FileKind)
	if !ok {
		return plannedFiles{}, refuse(Invalid, "a %s artifact takes no files: it is one piece of content. Only %s artifacts come with files", k.Name(), strings.Join(kind.NamesWithFiles(), ", "))
	}
	sizes := make(map[string]int64, len(carried))
	byPath := make(map[string]store.FileRef, len(carried))
	for _, f := range carried {
		sizes[f.Path], byPath[f.Path] = f.Size, f
	}
	lim := s.fileLimits
	lim.Accept = fk.AcceptedFileTypes()
	change, err := files.Apply(sizes, patch, lim)
	if err != nil {
		return plannedFiles{}, refuse(Invalid, "%s", err)
	}
	plan := plannedFiles{blobs: map[string][]byte{}}
	if current.ID != "" {
		plan.from = current.Version
	}
	for _, p := range change.Paths {
		if ct, put := change.Put[p]; put {
			data := []byte(*patch[p])
			sum := sha256Hex(data)
			plan.blobs[sum] = data
			plan.manifest = append(plan.manifest, store.FileRef{Path: p, ContentType: ct, Size: int64(len(data)), SHA256: sum})
			continue
		}
		plan.manifest = append(plan.manifest, byPath[p])
	}
	if plan.set, err = s.bundleSet(current.ID, k, plan.manifest, patch); err != nil {
		return plannedFiles{}, err
	}
	return plan, nil
}

// bundleSet reads the text of every file in manifest for a kind that judges its
// content together with its files; changed holds the text of the files this
// publish sends, which are not stored yet. Any other kind gets nil: its files are
// loaded by the browser, and a publish should not read hundreds of megabytes to
// change a title.
func (s *Service) bundleSet(artifactID string, k kind.Kind, manifest []store.FileRef, changed map[string]*string) (files.Set, error) {
	if _, ok := k.(kind.FileValidator); !ok || len(manifest) == 0 {
		return nil, nil
	}
	set := make(files.Set, len(manifest))
	for _, f := range manifest {
		if text, ok := changed[f.Path]; ok && text != nil {
			set[f.Path] = *text
			continue
		}
		data, err := s.files.ReadBlob(artifactID, f.SHA256)
		if err != nil {
			return nil, fmt.Errorf("file %s of artifact %s is recorded but its bytes are unreadable: %w", f.Path, artifactID, err)
		}
		set[f.Path] = string(data)
	}
	return set, nil
}

// BundleSet is the text of every file of one version of an artifact, for the
// page of a kind that compiles them in (react); nil for any other kind or for a
// version without files. Callers have already been allowed to see the version.
func (s *Service) BundleSet(ctx context.Context, artifactID string, version int, k kind.Kind) (files.Set, error) {
	if _, ok := k.(kind.Bundler); !ok {
		return nil, nil
	}
	manifest, err := s.store.VersionFiles(ctx, artifactID, version)
	if err != nil {
		return nil, fmt.Errorf("list files of artifact %s: %w", artifactID, err)
	}
	return s.bundleSet(artifactID, k, manifest, nil)
}

// storeBlobs writes the bytes of a plan under the artifact. Writing is
// idempotent and happens before the version is saved: a failure after this
// leaves blobs no manifest names, which the sweep deletes.
func (s *Service) storeBlobs(artifactID string, plan plannedFiles) error {
	for sum, data := range plan.blobs {
		if err := s.files.WriteBlob(artifactID, sum, data); err != nil {
			return fmt.Errorf("store file of artifact %s: %w", artifactID, err)
		}
	}
	return nil
}

func (s *Service) logFiles(ctx context.Context, msg string, artifactID string, manifest []store.FileRef) {
	if len(manifest) == 0 {
		return
	}
	var total int64
	for _, f := range manifest {
		total += f.Size
	}
	report(ctx, Information, msg, "id", artifactID, "files", len(manifest), "fileBytes", total)
}

// errNoFile is what a path that is not in the version gets.
func errNoFile(path string, manifest []store.FileRef) *Error {
	names := make([]string, len(manifest))
	for i, f := range manifest {
		names[i] = f.Path
	}
	list := "this version has no files"
	if len(names) > 0 {
		list = "its files are: " + strings.Join(names, ", ")
	}
	return &Error{Code: NotFound, Msg: fmt.Sprintf("not found: there is no file %q; %s", path, list), What: "file"}
}

// ReadFile reads the text of one file of an artifact the viewer may see: of the
// newest version, or of an older one for the people who may read history.
// version 0 means the newest.
func (s *Service) ReadFile(ctx context.Context, v Viewer, id, path string, version int) (store.FileRef, string, error) {
	defer s.admission.Shared(id)()
	return s.readFile(ctx, v, id, path, version)
}

func (s *Service) readFile(ctx context.Context, v Viewer, id, path string, version int) (store.FileRef, string, error) {
	a, err := s.Artifact(ctx, v, id)
	if err != nil {
		return store.FileRef{}, "", err
	}
	if version == 0 {
		version = a.Version
		// What people are looking at is what the room holds, before any version does.
		if ref, text, ok, err := s.liveFile(ctx, a, path); err != nil || ok {
			return ref, text, err
		}
	} else if version != a.Version {
		if _, err := s.Version(ctx, v, id, version); err != nil {
			return store.FileRef{}, "", err
		}
	}
	manifest, err := s.store.VersionFiles(ctx, a.ID, version)
	if err != nil {
		return store.FileRef{}, "", fmt.Errorf("list files of artifact %s: %w", a.ID, err)
	}
	i := slices.IndexFunc(manifest, func(f store.FileRef) bool { return f.Path == path })
	if i < 0 {
		return store.FileRef{}, "", errNoFile(path, manifest)
	}
	data, err := s.files.ReadBlob(a.ID, manifest[i].SHA256)
	if err != nil {
		return store.FileRef{}, "", fmt.Errorf("file %s of artifact %s version %d is recorded but its bytes are unreadable: %w", path, a.ID, version, err)
	}
	return manifest[i], string(data), nil
}

// VersionHasFiles reports whether a version of an artifact the caller has
// already been allowed to see comes with files, which decides whether /raw
// sends the browser to the address that carries a grant.
func (s *Service) VersionHasFiles(ctx context.Context, a store.Artifact, version int) (bool, error) {
	list, err := s.store.VersionFiles(ctx, a.ID, version)
	if err != nil {
		return false, fmt.Errorf("list files of artifact %s: %w", a.ID, err)
	}
	return len(list) > 0, nil
}

// GrantedPage is the page a grant opens: the artifact, if the grant's audience
// is still its audience, and the version the grant was made for. Whatever is
// wrong (a stale audience, a version that does not exist) is the same refusal
// as an artifact that is not there.
func (s *Service) GrantedPage(ctx context.Context, g Grant) (store.Artifact, store.Version, error) {
	a, err := s.Artifact(ctx, s.Granted(g), g.ArtifactID)
	if err != nil {
		return store.Artifact{}, store.Version{}, err
	}
	ver, err := s.store.Version(ctx, a.ID, g.Version)
	if errors.Is(err, store.ErrNotFound) {
		return store.Artifact{}, store.Version{}, errNotFound
	}
	if err != nil {
		return store.Artifact{}, store.Version{}, fmt.Errorf("read version %d of artifact %s: %w", g.Version, a.ID, err)
	}
	return a, ver, nil
}

// GrantedFile opens one file of the version a grant was made for. A path that
// is not there is the same refusal as everything else a grant cannot read.
func (s *Service) GrantedFile(ctx context.Context, g Grant, path string) (store.Artifact, store.FileRef, assets.Readable, error) {
	a, _, err := s.GrantedPage(ctx, g)
	if err != nil {
		return store.Artifact{}, store.FileRef{}, nil, err
	}
	manifest, err := s.store.VersionFiles(ctx, a.ID, g.Version)
	if err != nil {
		return store.Artifact{}, store.FileRef{}, nil, fmt.Errorf("list files of artifact %s: %w", a.ID, err)
	}
	i := slices.IndexFunc(manifest, func(f store.FileRef) bool { return f.Path == path })
	if i < 0 || !files.Servable(path) {
		// A source that is only ever imported is not something to load.
		return store.Artifact{}, store.FileRef{}, nil, errNotFound
	}
	f, err := s.files.OpenBlob(a.ID, manifest[i].SHA256)
	if err != nil {
		return store.Artifact{}, store.FileRef{}, nil, fmt.Errorf("file %s of artifact %s version %d is recorded but its bytes are unreadable: %w", path, a.ID, g.Version, err)
	}
	return a, manifest[i], f, nil
}

// SweepBlobs deletes the blobs no version names that are older than BlobGrace,
// the leftovers of publishes that stored their bytes and then failed. It
// returns how many it deleted.
func (s *Service) SweepBlobs(ctx context.Context, now time.Time) (int, error) {
	refs, err := s.store.BlobRefs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the files in use: %w", err)
	}
	removed, err := s.files.SweepBlobs(refs, now.Add(-BlobGrace))
	if removed > 0 {
		report(ctx, Information, "unreferenced file blobs deleted", "blobs", removed)
	}
	return removed, err
}

// restorableFiles is the manifest a restore gives the new version: the files
// of the version being brought back. Like its page they are checked again, so
// a restore never brings back a file the rules of today would refuse.
func (s *Service) restorableFiles(ctx context.Context, k kind.Kind, id string, number int) ([]store.FileRef, files.Set, error) {
	manifest, err := s.store.VersionFiles(ctx, id, number)
	if err != nil {
		return nil, nil, fmt.Errorf("list files of artifact %s: %w", id, err)
	}
	if len(manifest) == 0 {
		return nil, nil, nil
	}
	fk, ok := k.(kind.FileKind)
	if !ok {
		return nil, nil, refuse(Invalid, "version %d can no longer be restored as it is: a %s artifact takes no files, and it has %d", number, k.Name(), len(manifest))
	}
	_, judged := k.(kind.FileValidator)
	var set files.Set
	if judged {
		set = make(files.Set, len(manifest))
	}
	for _, f := range manifest {
		data, err := s.files.ReadBlob(id, f.SHA256)
		if err != nil {
			return nil, nil, fmt.Errorf("file %s of artifact %s version %d is recorded but its bytes are unreadable: %w", f.Path, id, number, err)
		}
		if _, err := files.Check(f.Path, string(data), fk.AcceptedFileTypes()); err != nil {
			return nil, nil, refuse(Invalid, "version %d can no longer be restored as it is: %s", number, err)
		}
		if judged {
			set[f.Path] = string(data)
		}
	}
	return manifest, set, nil
}

// withVersionFiles lists the files of a version that is being read in full.
func (s *Service) withVersionFiles(ctx context.Context, id string, ver *store.Version) error {
	list, err := s.store.VersionFiles(ctx, id, ver.Number)
	if err != nil {
		return fmt.Errorf("list files of artifact %s: %w", id, err)
	}
	if len(list) > 0 {
		ver.Files = list
	}
	return nil
}
