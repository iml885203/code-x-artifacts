package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// A publish session is a publish whose bytes travel out of band, for a page
// that already exists as a file and is too large for the publisher to type into
// a request (docs/large-publish.md). begin announces every file by path, hash
// and size; the server answers with an upload slot for each blob it does not
// have; the bytes arrive one by one through ReceiveUpload; commit then creates
// the version through the same Create and Update as an inline publish, so
// validation, limits and conflicts are the same by construction.

const (
	// SessionTTL is how long a session, and each of its upload URLs, works.
	SessionTTL = 10 * time.Minute
	// MaxOpenSessions is how many unexpired sessions one person may hold, which
	// bounds the bytes waiting on the disk for people who never commit.
	MaxOpenSessions = 10
	// EntryPath is the path that names the page itself, the content of a
	// publish, among the files of a session. A file path needs an extension, so
	// no file can have this name.
	EntryPath = "content"

	// stagingPrefix starts the directory that holds the bytes of a session; no
	// artifact id starts with it.
	stagingPrefix = "~"
	// stagingGrace is how old a staging directory without a session must be
	// before the sweep removes it.
	stagingGrace = time.Hour
)

func stagingKey(session string) string { return stagingPrefix + session }

// BeginInput is what begin announces.
type BeginInput struct {
	// ArtifactID names the artifact to update; empty creates a new one.
	ArtifactID  string
	Title       string
	Kind        string
	Format      string
	BaseVersion int
	Visibility  store.Visibility
	// Files is every file the publish sends, with the page itself at EntryPath.
	Files []store.SessionFile
	// Remove is the paths the publish takes out of the artifact's files.
	Remove []string
}

// Slot is one blob the server is waiting for.
type Slot struct {
	// Paths are the files that have these bytes.
	Paths  []string
	SHA256 string
	Size   int64
	// Secret is the one-use secret of the upload URL; the front end builds the
	// URL around it, and nothing else may keep it.
	Secret string
}

// Begun is the answer to begin.
type Begun struct {
	Session   string
	ExpiresAt time.Time
	// Send is the blobs to upload, one slot each.
	Send []Slot
	// Stored is the paths whose bytes the server already has.
	Stored []string
	// Limits are what one publish may hold.
	Limits files.Limits
}

// BeginPublish opens a publish session for a new artifact or an update of one
// the viewer may edit. It refuses what an inline publish would refuse without
// seeing the bytes: who may publish, a stale base version, the kind and its
// format, file paths, types and sizes, so an upload is never wasted on a
// publish that cannot succeed.
func (s *Service) BeginPublish(ctx context.Context, v Viewer, in BeginInput) (Begun, error) {
	if v.Anonymous() {
		return Begun{}, refuse(Forbidden, "sign in to publish")
	}
	if _, err := s.SweepPublishSessions(ctx); err != nil {
		report(ctx, Failure, "sweep publish sessions", "err", err)
	}
	current, err := s.beginTarget(ctx, v, in)
	if err != nil {
		return Begun{}, err
	}
	head, k, err := s.validateHeader(PublishInput{Title: in.Title, Kind: in.Kind, Format: in.Format, Visibility: in.Visibility}, current)
	if err != nil {
		return Begun{}, err
	}
	if err := s.checkDeclared(k, in); err != nil {
		return Begun{}, err
	}
	have, err := s.heldBlobs(ctx, current)
	if err != nil {
		return Begun{}, err
	}
	stored, send := splitFiles(in.Files, have)
	sess := store.PublishSession{
		ID: store.NewSessionID(), Username: v.Username, ArtifactID: in.ArtifactID, BaseVersion: in.BaseVersion,
		Title: head.Title, Kind: head.Kind, Format: head.Format, Visibility: head.Visibility,
		Files: in.Files, Removed: in.Remove, CreatedAt: s.now().UTC(),
	}
	sess.ExpiresAt = sess.CreatedAt.Add(SessionTTL)
	blobs := make([]store.SessionFile, len(send))
	for i, sl := range send {
		blobs[i] = store.SessionFile{SHA256: sl.SHA256, Size: sl.Size}
	}
	secrets, err := s.store.CreatePublishSession(ctx, sess, blobs, MaxOpenSessions)
	if errors.Is(err, store.ErrTooManySessions) {
		return Begun{}, refuse(Invalid, "you already have %d publish sessions open; commit them, or wait up to %d minutes for them to expire", MaxOpenSessions, int(SessionTTL/time.Minute))
	}
	if err != nil {
		return Begun{}, fmt.Errorf("begin publish session: %w", err)
	}
	for i := range send {
		send[i].Secret = secrets[send[i].SHA256]
	}
	report(ctx, Information, "publish session begun", "session", sess.ID, "user", v.Username, "artifact", in.ArtifactID, "files", len(in.Files), "toUpload", len(send))
	return Begun{Session: sess.ID, ExpiresAt: sess.ExpiresAt, Send: send, Stored: stored, Limits: s.fileLimits}, nil
}

// beginTarget is the artifact a session updates, checked as Update checks it
// (who may edit, who may change the audience, the base version); the zero value
// for a session that creates one.
func (s *Service) beginTarget(ctx context.Context, v Viewer, in BeginInput) (store.Artifact, error) {
	if in.ArtifactID == "" {
		return store.Artifact{}, nil
	}
	a, err := s.Edit(ctx, v, in.ArtifactID)
	if err != nil {
		return store.Artifact{}, err
	}
	if in.Visibility != "" && in.Visibility != a.Visibility && !v.CanChange(a) {
		return store.Artifact{}, refuse(Forbidden, "only the author can change who can see this artifact; publish without visibility to keep it as it is")
	}
	if err := s.checkBase(ctx, a, in.BaseVersion); err != nil {
		return store.Artifact{}, err
	}
	return a, nil
}

// checkDeclared refuses what the files a session announced could never be.
func (s *Service) checkDeclared(k kind.Kind, in BeginInput) error {
	lim := s.fileLimits
	fk, takesFiles := k.(kind.FileKind)
	var entry bool
	var total int64
	seen := make(map[string]bool, len(in.Files))
	for _, f := range in.Files {
		if err := checkDeclaredFile(f, lim, seen); err != nil {
			return err
		}
		total += f.Size
		if f.Path == EntryPath {
			entry = true
			continue
		}
		if !takesFiles {
			return refuse(Invalid, "a %s artifact takes no files: it is one piece of content. Only %s artifacts come with files", k.Name(), strings.Join(kind.NamesWithFiles(), ", "))
		}
		if err := files.CheckPath(f.Path, fk.AcceptedFileTypes()); err != nil {
			return refuse(Invalid, "%s", err)
		}
	}
	if !entry {
		return refuse(Invalid, "files must include the page itself, with path %q, as the content of every publish; the other paths are the files that come with it", EntryPath)
	}
	if len(in.Files)-1+len(in.Remove) > lim.PerPublish {
		return refuse(Invalid, "this publish changes %d files; at most %d files in one publish, so send the rest in a later publish", len(in.Files)-1+len(in.Remove), lim.PerPublish)
	}
	if total > lim.TotalBytes {
		return refuse(Invalid, "these files add up to %d bytes; a publish carries at most %d bytes in all", total, lim.TotalBytes)
	}
	for _, p := range in.Remove {
		if seen[strings.ToLower(p)] {
			return refuse(Invalid, "path %q is both sent and removed; do one", p)
		}
		if !takesFiles {
			return refuse(Invalid, "a %s artifact takes no files, so there is none to remove", k.Name())
		}
	}
	return nil
}

// checkDeclaredFile checks what one announced file says about itself.
func checkDeclaredFile(f store.SessionFile, lim files.Limits, seen map[string]bool) error {
	switch {
	case !validSHA256(f.SHA256):
		return refuse(Invalid, "file %q: sha256 must be the 64 lower-case hex digits of the file's SHA-256 (shasum -a 256 <file>)", f.Path)
	case f.Size < 0:
		return refuse(Invalid, "file %q: size must be the file's size in bytes", f.Path)
	case f.Size > int64(lim.FileBytes):
		return refuse(Invalid, "file %q is %d bytes; a file can be at most %d bytes", f.Path, f.Size, lim.FileBytes)
	case seen[strings.ToLower(f.Path)]:
		return refuse(Invalid, "file %q is listed twice; list each path once", f.Path)
	case f.Path == EntryPath && f.Size == 0:
		return refuse(Invalid, "the file at %q is empty; it is the page itself", EntryPath)
	}
	seen[strings.ToLower(f.Path)] = true
	return nil
}

func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// heldBlobs is the hashes of the files of the artifact's newest version: bytes
// the server holds for good, which a publish on top of it need not be sent again.
func (s *Service) heldBlobs(ctx context.Context, current store.Artifact) (map[string]bool, error) {
	if current.ID == "" {
		return nil, nil
	}
	manifest, err := s.store.VersionFiles(ctx, current.ID, current.Version)
	if err != nil {
		return nil, fmt.Errorf("list files of artifact %s: %w", current.ID, err)
	}
	have := make(map[string]bool, len(manifest))
	for _, f := range manifest {
		have[f.SHA256] = true
	}
	return have, nil
}

// splitFiles separates the paths whose bytes the server has from the blobs it
// is waiting for, one slot for each distinct content. The page itself is never
// held: it is stored with the version, not as a blob.
func splitFiles(declared []store.SessionFile, have map[string]bool) (stored []string, send []Slot) {
	index := map[string]int{}
	for _, f := range declared {
		if f.Path != EntryPath && have[f.SHA256] {
			stored = append(stored, f.Path)
			continue
		}
		if i, ok := index[f.SHA256]; ok {
			send[i].Paths = append(send[i].Paths, f.Path)
			continue
		}
		index[f.SHA256] = len(send)
		send = append(send, Slot{Paths: []string{f.Path}, SHA256: f.SHA256, Size: f.Size})
	}
	slices.Sort(stored)
	for i := range send {
		slices.Sort(send[i].Paths)
	}
	slices.SortFunc(send, func(a, b Slot) int { return strings.Compare(a.Paths[0], b.Paths[0]) })
	return stored, send
}

var errNoUpload = &Error{Code: NotFound, Msg: "this upload URL does not work: it is unknown, expired (they last 10 minutes), or was already used. Call begin_publish again for new ones", What: "upload"}

var errNoSession = &Error{Code: NotFound, Msg: "no such publish session: it does not exist, belongs to someone else, expired (a session lasts 10 minutes), or was already committed. Call begin_publish again", What: "session"}

// ReceiveUpload stores the bytes sent to an upload URL. The secret in the URL
// is the only credential: it is bound to one session, one hash and one size, so
// whatever it opens, it opens only for those bytes, and only once.
func (s *Service) ReceiveUpload(ctx context.Context, secret string, body io.Reader) error {
	now := s.now()
	slot, err := s.store.OpenUpload(ctx, secret, now)
	if errors.Is(err, store.ErrNotFound) {
		return errNoUpload
	}
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(body, slot.Size+1))
	if err != nil {
		return refuse(Invalid, "could not read the request body: send the file itself as the body (curl -X PUT --data-binary @file URL)")
	}
	switch {
	case int64(len(data)) > slot.Size:
		return refuse(Invalid, "the body is larger than the %d bytes this URL was issued for; send the file whose size and sha256 you gave to begin_publish", slot.Size)
	case int64(len(data)) < slot.Size:
		return refuse(Invalid, "the body is %d bytes but this URL was issued for %d; send the whole file (curl -X PUT --data-binary @file URL), the one whose size and sha256 you gave to begin_publish", len(data), slot.Size)
	case sha256Hex(data) != slot.SHA256:
		return refuse(Invalid, "the body's sha256 is not the one this URL was issued for; send the file whose sha256 you gave to begin_publish, unchanged since")
	}
	if err := s.files.WriteBlob(stagingKey(slot.SessionID), slot.SHA256, data); err != nil {
		return fmt.Errorf("stage upload of session %s: %w", slot.SessionID, err)
	}
	if err := s.store.UseUpload(ctx, secret, now); errors.Is(err, store.ErrNotFound) {
		return errNoUpload
	} else if err != nil {
		return fmt.Errorf("record upload of session %s: %w", slot.SessionID, err)
	}
	return nil
}

// CommitPublish creates the version a session announced, from the bytes that
// were uploaded, and returns what an inline publish returns; created says
// whether it made a new artifact. A session commits once. When the publish is
// refused the session is kept until it expires, so a transient failure can be
// retried; a refusal that says what to change means a new session.
func (s *Service) CommitPublish(ctx context.Context, v Viewer, session string, sourceBases ...map[string]string) (p Published, created bool, err error) {
	if v.Anonymous() {
		return Published{}, false, refuse(Forbidden, "sign in to publish")
	}
	now := s.now()
	if err := s.store.ClaimPublishSession(ctx, session, v.Username, now); errors.Is(err, store.ErrNotFound) {
		return Published{}, false, errNoSession
	} else if err != nil {
		return Published{}, false, fmt.Errorf("claim publish session: %w", err)
	}
	defer func() {
		if err != nil {
			if rerr := s.store.ReleasePublishSession(ctx, session); rerr != nil {
				report(ctx, Failure, "release publish session", "session", session, "err", rerr)
			}
		}
	}()
	sess, err := s.store.PublishSession(ctx, session, v.Username, now)
	if errors.Is(err, store.ErrNotFound) {
		return Published{}, false, errNoSession
	}
	if err != nil {
		return Published{}, false, fmt.Errorf("load publish session %s: %w", session, err)
	}
	in, err := s.sessionPublish(ctx, sess)
	if err != nil {
		return Published{}, false, err
	}
	if len(sourceBases) > 1 {
		return Published{}, false, refuse(Invalid, "send one sourceBases map")
	}
	if len(sourceBases) == 1 {
		in.SourceBases = sourceBases[0]
	}
	created = sess.ArtifactID == ""
	if created {
		p, err = s.Create(ctx, v, in)
	} else {
		p, err = s.Update(ctx, v, sess.ArtifactID, in)
	}
	if err != nil {
		return Published{}, false, err
	}
	if err := s.store.DeletePublishSession(ctx, session); err != nil {
		report(ctx, Failure, "delete committed publish session", "session", session, "err", err)
	}
	if err := s.files.RemoveArtifact(stagingKey(session)); err != nil {
		report(ctx, Failure, "remove staged bytes of a committed session", "session", session, "err", err)
	}
	report(ctx, Information, "publish session committed", "session", session, "user", v.Username, "id", p.ID)
	return p, created, nil
}

// sessionPublish is the inline publish a session stands for, read from the
// bytes that were uploaded and the ones the server already had.
func (s *Service) sessionPublish(ctx context.Context, sess store.PublishSession) (PublishInput, error) {
	arrived, err := s.store.UploadedBlobs(ctx, sess.ID)
	if err != nil {
		return PublishInput{}, fmt.Errorf("list uploads of session %s: %w", sess.ID, err)
	}
	in := PublishInput{
		Title: sess.Title, Kind: sess.Kind, Format: sess.Format, BaseVersion: sess.BaseVersion,
		Visibility: sess.Visibility, fromFile: true,
	}
	patch := map[string]*string{}
	var missing []string
	for _, f := range sess.Files {
		data, err := s.sessionBytes(sess, f, arrived[f.SHA256])
		if err != nil {
			missing = append(missing, f.Path)
			continue
		}
		if !utf8.Valid(data) {
			return PublishInput{}, refuse(Invalid, "%q is not UTF-8 text; a page and its files are text", f.Path)
		}
		text := string(data)
		if f.Path == EntryPath {
			in.Content = text
			continue
		}
		patch[f.Path] = &text
	}
	if len(missing) > 0 {
		return PublishInput{}, refuse(Invalid, "nothing was published: the bytes of %s have not arrived. Upload them to the URLs begin_publish gave (they work for 10 minutes) and commit again; for a file that was listed as stored, or once the URLs have expired, call begin_publish again", strings.Join(missing, ", "))
	}
	for _, p := range sess.Removed {
		patch[p] = nil
	}
	if len(patch) > 0 {
		in.Files = patch
	}
	return in, nil
}

// sessionBytes reads the bytes of one announced file: from the session's own
// uploads, or from the artifact for a blob the server already held.
func (s *Service) sessionBytes(sess store.PublishSession, f store.SessionFile, uploaded bool) ([]byte, error) {
	if uploaded {
		return s.files.ReadBlob(stagingKey(sess.ID), f.SHA256)
	}
	if f.Path == EntryPath || sess.ArtifactID == "" {
		return nil, errors.New("not uploaded")
	}
	return s.files.ReadBlob(sess.ArtifactID, f.SHA256)
}

// SweepPublishSessions deletes the sessions that have expired, with the bytes
// uploaded to them, and any staged bytes that belong to no session. It
// returns how many sessions it deleted.
func (s *Service) SweepPublishSessions(ctx context.Context) (int, error) {
	now := s.now()
	ids, err := s.store.ExpirePublishSessions(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired publish sessions: %w", err)
	}
	var errs []error
	for _, id := range ids {
		errs = append(errs, s.files.RemoveArtifact(stagingKey(id)))
	}
	live, err := s.store.PublishSessionIDs(ctx)
	if err != nil {
		return len(ids), errors.Join(append(errs, fmt.Errorf("list publish sessions: %w", err))...)
	}
	dirs, err := s.files.DirsWithPrefix(stagingPrefix, now.Add(-stagingGrace))
	if err != nil {
		return len(ids), errors.Join(append(errs, err)...)
	}
	for _, d := range dirs {
		if !live[strings.TrimPrefix(d, stagingPrefix)] {
			errs = append(errs, s.files.RemoveArtifact(d))
		}
	}
	if len(ids) > 0 {
		report(ctx, Information, "expired publish sessions deleted", "sessions", len(ids))
	}
	return len(ids), errors.Join(errs...)
}

// UploadPath is where the bytes for a slot go, on the host the publisher reached
// the server at.
func UploadPath(secret string) string { return "/api/uploads/" + secret }

// UploadView is a slot as both front ends show it.
type UploadView struct {
	// Paths are the files that have these bytes; send them once.
	Paths  []string `json:"paths"`
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	// URL takes the bytes as the body of a PUT, once, with no credential. It is a
	// secret for ten minutes: never print or log it.
	URL string `json:"url"`
}

// LimitsView is what one publish may hold, as both front ends show it.
type LimitsView struct {
	FileBytes  int   `json:"fileBytes"`
	Files      int   `json:"files"`
	TotalBytes int64 `json:"totalBytes"`
}

// BegunView is the answer to begin as both front ends send it.
type BegunView struct {
	Session          string    `json:"session"`
	ExpiresAt        time.Time `json:"expiresAt"`
	ExpiresInSeconds int       `json:"expiresInSeconds"`
	// Missing is the blobs to upload, one upload URL for each; an empty list
	// means everything is already there and the session can be committed.
	Missing []UploadView `json:"missing"`
	// Stored is the paths the server already has the bytes of.
	Stored []string   `json:"stored"`
	Limits LimitsView `json:"limits"`
}

// View is the answer with the upload URLs built on baseURL.
func (b Begun) View(baseURL string) BegunView {
	v := BegunView{
		Session: b.Session, ExpiresAt: b.ExpiresAt, ExpiresInSeconds: int(SessionTTL / time.Second),
		Missing: make([]UploadView, len(b.Send)), Stored: b.Stored,
		Limits: LimitsView{FileBytes: b.Limits.FileBytes, Files: b.Limits.PerPublish, TotalBytes: b.Limits.TotalBytes},
	}
	if v.Stored == nil {
		v.Stored = []string{}
	}
	for i, sl := range b.Send {
		v.Missing[i] = UploadView{Paths: sl.Paths, SHA256: sl.SHA256, Size: sl.Size, URL: baseURL + UploadPath(sl.Secret)}
	}
	return v
}
