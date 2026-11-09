package roomkind

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Quiet is how long a room goes without an edit before what it holds becomes a
// version. Versions are cut only this way, and just before a restore: there is
// no "save now", and a version is never thinned.
const Quiet = 2 * time.Minute

// Pending is the store's record of rooms with edits no version holds yet.
type Pending interface {
	ArtifactReader
	MarkPending(ctx context.Context, room, username string) error
	PendingRooms(ctx context.Context) ([]string, error)
	PendingAuthors(ctx context.Context, room string) ([]string, error)
	ClearPending(ctx context.Context, room string) error
	RealtimeRows(ctx context.Context, room string) (int, error)
}

// Versions saves a version from a room (service.Service.SaveFromRoom).
type Versions interface {
	SaveFromRoom(ctx context.Context, id string, base int, v kind.Bundle, authors []string) error
}

// Rooms is the hub the snapshotter reads and writes through.
type Rooms interface {
	EditExpect(context.Context, string, realtime.Identity, string, func(*crdt.Doc) (func(*crdt.Transaction), error)) error
	Read(ctx context.Context, room string, fn func(doc *crdt.Doc) error) error
	Edit(ctx context.Context, room string, author realtime.Identity, plan func(doc *crdt.Doc) (func(*crdt.Transaction), error)) error
	Announce(ctx context.Context, room, key, label, block string) error
	AnnounceTouched(ctx context.Context, room, key, label string, touched map[string][]string) error
}

// Config wires a Snapshotter. Everything but Now, Quiet and Logger is required.
type Config struct {
	Store Pending
	// Files reads the text of a version's files; a kind whose room holds files
	// is compared with the latest version through it.
	Files    FileReader
	Versions Versions
	Codecs   Codecs
	// Now is the clock the quiet period is measured on; tests replace it.
	Now    func() time.Time
	Quiet  time.Duration
	Logger *slog.Logger
}

// Snapshotter cuts versions from rooms. Whoever edits a room is told to it by
// Updated (the hub's OnUpdate); Sweep, called on a timer, turns each room that
// has been quiet into a version. What is waiting for a version is in the store,
// so a restart does not lose it; how long a room has been quiet is in memory,
// and a room found waiting after a restart is quiet from the moment the
// snapshotter first sees it.
type Snapshotter struct {
	cfg   Config
	rooms Rooms

	// mu is held while a room is saved, so an edit reported meanwhile is
	// recorded after the version that may or may not hold it, never lost
	// between the two.
	mu     sync.Mutex
	lastAt map[string]time.Time
	// failures counts the saves of a room that failed in a row; each one makes
	// the wait before the next try twice as long, up to maxBackoff.
	failures map[string]int
}

// maxBackoff is the longest wait between two tries to save a room that cannot be saved.
const maxBackoff = time.Hour

// NewSnapshotter returns a snapshotter. Give it the hub with Use before it is run.
func NewSnapshotter(cfg Config) *Snapshotter {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Quiet <= 0 {
		cfg.Quiet = Quiet
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Snapshotter{cfg: cfg, lastAt: map[string]time.Time{}, failures: map[string]int{}}
}

// Use sets the hub the snapshotter reads and writes rooms through. The hub is
// made with the snapshotter's Updated as its OnUpdate, so the two meet here.
func (s *Snapshotter) Use(rooms Rooms) { s.rooms = rooms }

// Updated records that an accepted update is waiting for a version, and whose
// it is. The author is the identity the connection or the server-side edit was
// made for, never a name the client claims. It is the hub's OnUpdate.
func (s *Snapshotter) Updated(u realtime.Update) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.cfg.Store.MarkPending(ctx, u.Room, u.Author.Username); err != nil {
		s.cfg.Logger.Error("note that a room has edits no version holds", "room", u.Room, "err", err)
	}
	s.lastAt[u.Room] = s.cfg.Now()
	delete(s.failures, u.Room) // someone is editing: it may be fixed now
}

// Sweep saves a version of every room that has been quiet for Quiet. A room
// that cannot be saved stays waiting and is tried again after another quiet
// period.
func (s *Snapshotter) Sweep(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.cfg.Store.PendingRooms(ctx)
	if err != nil {
		s.cfg.Logger.Error("list the rooms waiting for a version", "err", err)
		return
	}
	now := s.cfg.Now()
	waiting := map[string]bool{}
	for _, room := range pending {
		waiting[room] = true
		last, seen := s.lastAt[room]
		if !seen {
			s.lastAt[room] = now
			continue
		}
		if now.Sub(last) < s.cfg.Quiet {
			continue
		}
		if err := s.saveLocked(ctx, room); err != nil {
			s.failedLocked(ctx, room, now, err)
		}
	}
	for room := range s.lastAt {
		if !waiting[room] {
			delete(s.lastAt, room)
		}
	}
}

// failedLocked notes that a room could not be saved. It is logged as an error
// the first time and once more each time the wait doubles past an hour of
// trying; in between it is a debug line, so a room that stays unsavable does
// not fill the log. The next try waits twice as long as the last, so the room
// is tried again after Quiet, then 2x, 4x... up to an hour.
func (s *Snapshotter) failedLocked(ctx context.Context, room string, now time.Time, err error) {
	n := min(s.failures[room]+1, 20) // 20 doublings are far past the cap, and a shift must not overflow
	s.failures[room] = n
	wait := min(s.cfg.Quiet<<n, maxBackoff)
	s.lastAt[room] = now.Add(wait - s.cfg.Quiet)
	level := slog.LevelDebug
	if n == 1 || wait == maxBackoff && s.cfg.Quiet<<(n-1) < maxBackoff {
		level = slog.LevelError
	}
	s.cfg.Logger.Log(ctx, level, "save a version of a room", "room", room, "failures", n, "retryIn", wait, "err", err)
}

// Run calls Sweep every interval until ctx ends.
func (s *Snapshotter) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Sweep(ctx)
		}
	}
}

// saveLocked cuts a version from the room if it holds anything the latest
// version does not, and forgets what was waiting.
func (s *Snapshotter) saveLocked(ctx context.Context, room string) error {
	a, err := s.cfg.Store.Artifact(ctx, room)
	if errors.Is(err, store.ErrNotFound) {
		return s.forgetLocked(ctx, room)
	}
	if err != nil {
		return err
	}
	codec, ok := s.cfg.Codecs[a.Kind]
	if !ok {
		return s.forgetLocked(ctx, room)
	}
	var held kind.Bundle
	if err := s.rooms.Read(ctx, room, func(doc *crdt.Doc) (err error) {
		held, err = codec.Content(doc)
		return err
	}); err != nil {
		return &unsavable{err}
	}
	latest, err := latestBundle(ctx, s.cfg.Files, a, held)
	if err != nil {
		return err
	}
	if codec.Same(latest, held) {
		return s.forgetLocked(ctx, room)
	}
	authors, err := s.cfg.Store.PendingAuthors(ctx, room)
	if err != nil {
		return err
	}
	if err := s.cfg.Versions.SaveFromRoom(ctx, room, a.Version, held, authors); err != nil {
		return err
	}
	return s.forgetLocked(ctx, room)
}

func (s *Snapshotter) forgetLocked(ctx context.Context, room string) error {
	delete(s.lastAt, room)
	delete(s.failures, room)
	return s.cfg.Store.ClearPending(ctx, room)
}

// unsavable is a room whose content the kind would not accept as a version:
// what people typed there is not something to keep.
type unsavable struct{ err error }

func (e *unsavable) Error() string {
	return "what the room holds cannot be a version: " + e.err.Error()
}
func (e *unsavable) Unwrap() error { return e.err }

// Flush cuts a version from the artifact's room now, if the room holds
// anything the latest version does not: the protective version before a
// restore. An artifact whose room was never opened has nothing to flush.
func (s *Snapshotter) Flush(ctx context.Context, id string) error {
	rows, err := s.cfg.Store.RealtimeRows(ctx, id)
	if err != nil || rows == 0 {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.saveLocked(ctx, id)
	if u, ok := errors.AsType[*unsavable](err); ok {
		// A restore is the way out of a room that holds something invalid, so it
		// must not be refused for it; the restore replaces that content.
		s.cfg.Logger.Warn("a room holds something that cannot be a version; restoring over it", "room", id, "err", u)
		return nil
	}
	return err
}

// Apply writes content into the artifact's room as an edit by who, if the room
// was ever opened. A room that was not will be started from the latest version
// when it is, which the restore is about to make.
func (s *Snapshotter) Apply(ctx context.Context, id, kindName string, v kind.Bundle, who string) error {
	codec, ok := s.cfg.Codecs[kindName]
	if !ok {
		return nil
	}
	rows, err := s.cfg.Store.RealtimeRows(ctx, id)
	if err != nil || rows == 0 {
		return err
	}
	return s.rooms.Edit(ctx, id, realtime.Identity{Username: who}, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		return codec.Write(doc, v)
	})
}

// Merge changes the artifact's room by what an agent made of base, as an edit by
// who, leaving what people changed since in place (Merger). It says which
// elements of which files it touched.
func (s *Snapshotter) Merge(ctx context.Context, id, kindName string, base, next kind.Bundle, who string) (map[string][]string, error) {
	return s.MergeExpect(ctx, id, kindName, base, next, who, kind.SourceExpectation{})
}

// MergeExpect validates whole-file and epoch expectations before the actual
// three-way merge, under one Hub mutation lock. It never acquires admission.
func (s *Snapshotter) MergeExpect(ctx context.Context, id, kindName string, base, next kind.Bundle, who string, expect kind.SourceExpectation) (map[string][]string, error) {
	codec, ok := s.cfg.Codecs[kindName]
	if !ok {
		return nil, fmt.Errorf("a %s has no live copy", kindName)
	}
	merger, ok := codec.(Merger)
	if !ok {
		return nil, fmt.Errorf("the live copy of a %s does not take a change made from a version", kindName)
	}
	var touched map[string][]string
	err := s.rooms.EditExpect(ctx, id, realtime.Identity{Username: who}, expect.Epoch, func(doc *crdt.Doc) (func(*crdt.Transaction), error) {
		if len(expect.Frames) > 0 {
			held, err := codec.Content(doc)
			if err != nil {
				return nil, err
			}
			for path, want := range expect.Frames {
				source, present := held.Files[path]
				if (want.Absent && present) || (!want.Absent && (!present || source != want.Source)) {
					return nil, &designroom.MergeError{Conflict: true, Msg: "source changed before the room mutation; retain your draft, read current source and reconcile"}
				}
			}
		}
		write, t, err := merger.Merge(doc, base, next)
		touched = t
		return write, err
	})
	return touched, err
}

// Live reports whether the artifact has a room that was ever opened: one that
// has a log, which is the working copy.
func (s *Snapshotter) Live(ctx context.Context, id string) (bool, error) {
	rows, err := s.cfg.Store.RealtimeRows(ctx, id)
	return rows > 0, err
}

// Content is what the artifact's room holds, as the content of a version of the
// given kind. It is an error for a room that holds something the kind would
// not accept.
func (s *Snapshotter) Content(ctx context.Context, id, kindName string) (kind.Bundle, error) {
	codec, ok := s.cfg.Codecs[kindName]
	if !ok {
		return kind.Bundle{}, fmt.Errorf("a %s has no live copy", kindName)
	}
	var v kind.Bundle
	err := s.rooms.Read(ctx, id, func(doc *crdt.Doc) (err error) {
		v, err = codec.Content(doc)
		return err
	})
	return v, err
}

// Holds reports whether artifacts of the kind are edited in a room.
func (s *Snapshotter) Holds(kindName string) bool {
	_, ok := s.cfg.Codecs[kindName]
	return ok
}

// PresentTouched shows an agent in the artifact's room with the elements it
// changed, for a few seconds: touched is {frame: [pid…]}.
func (s *Snapshotter) PresentTouched(ctx context.Context, id, key, label string, touched map[string][]string) error {
	return s.rooms.AnnounceTouched(ctx, id, key, label, touched)
}

// Present shows an agent in the artifact's room at a block, for a few seconds.
func (s *Snapshotter) Present(ctx context.Context, id, key, label, block string) error {
	return s.rooms.Announce(ctx, id, key, label, block)
}
