package roomkind

import (
	"context"
	"errors"
	"fmt"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/realtime"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// ArtifactReader is what seeding and snapshots need of the store.
type ArtifactReader interface {
	Artifact(ctx context.Context, id string) (store.Artifact, error)
}

// FileReader is what seeding and snapshots need of the files of a version.
type FileReader interface {
	// FileTexts is the text of every file of a version of an artifact.
	FileTexts(ctx context.Context, id string, version int) (files.Set, error)
}

// latestBundle is the latest version of an artifact as a bundle, with its files
// when the room holds files (held.Files is not nil).
func latestBundle(ctx context.Context, fr FileReader, a store.Artifact, held kind.Bundle) (kind.Bundle, error) {
	b := kind.Bundle{Content: a.Content}
	if held.Files == nil || fr == nil {
		return b, nil
	}
	var err error
	if b.Files, err = fr.FileTexts(ctx, a.ID, a.Version); err != nil {
		return b, fmt.Errorf("read the files of %s version %d: %w", a.ID, a.Version, err)
	}
	return b, nil
}

// SeededLog is a room log that starts a room from the latest version of its
// artifact. A room that has no log yet is an artifact nobody has opened live:
// its first load turns the latest version into the room's state and stores it
// as the log's first row (a doc keeps its block ids), so the room and the
// version agree from the start. An artifact of a kind with no codec starts with
// an empty room. A room that has a log is never seeded again: the log is the
// working copy.
func SeededLog(inner realtime.Log, artifacts ArtifactReader, fr FileReader, codecs Codecs, docBytes int) realtime.Log {
	return seededLog{Log: inner, artifacts: artifacts, files: fr, codecs: codecs, docBytes: docBytes}
}

type seededLog struct {
	realtime.Log
	artifacts ArtifactReader
	files     FileReader
	codecs    Codecs
	docBytes  int
}

// DesignDocumentBytes is a working-state budget, independent of source-file
// validity. Design holds every frame's editable tree in one document; its
// encoded history shares this budget. Other kinds keep the generic limit.
const DesignDocumentBytes = 8 << 20

func (l seededLog) DocumentLimit(ctx context.Context, room string) (int, error) {
	if l.docBytes > 0 {
		return l.docBytes, nil
	}
	a, err := l.artifacts.Artifact(ctx, room)
	if errors.Is(err, store.ErrNotFound) {
		return 0, realtime.ErrNoRoom
	}
	if err != nil {
		return 0, err
	}
	if a.Kind == "design" {
		return DesignDocumentBytes, nil
	}
	return realtime.DefaultLimits.DocBytes, nil
}

func (l seededLog) Load(ctx context.Context, room, newEpoch string) (realtime.Snapshot, error) {
	limit, err := l.DocumentLimit(ctx, room)
	if err != nil {
		return realtime.Snapshot{}, err
	}
	snap, err := l.Log.Load(ctx, room, newEpoch)
	if err != nil {
		return snap, err
	}
	if len(snap.Updates) > 0 {
		if err := realtime.CheckDocumentSize(snap.Updates, limit); err != nil {
			return realtime.Snapshot{}, err
		}
		return l.migrated(ctx, room, snap, limit)
	}
	a, err := l.artifacts.Artifact(ctx, room)
	if err != nil {
		return realtime.Snapshot{}, fmt.Errorf("read the latest version of %s: %w", room, err)
	}
	codec, ok := l.codecs[a.Kind]
	if !ok {
		return snap, nil
	}
	version := kind.Bundle{Content: a.Content}
	if l.files != nil && hasFiles(a) {
		if version.Files, err = l.files.FileTexts(ctx, a.ID, a.Version); err != nil {
			return realtime.Snapshot{}, fmt.Errorf("read the files of version %d of %s: %w", a.Version, room, err)
		}
	}
	state, err := codec.Seed(version)
	if err != nil {
		return realtime.Snapshot{}, fmt.Errorf("start room %s from version %d: %w", room, a.Version, err)
	}
	if err := realtime.CheckDocumentSize([][]byte{state}, limit); err != nil {
		return realtime.Snapshot{}, err
	}
	if err := l.Replace(ctx, room, snap.Epoch, state); err != nil {
		return realtime.Snapshot{}, fmt.Errorf("keep the start of room %s: %w", room, err)
	}
	snap.Updates = [][]byte{state}
	return snap, nil
}

// hasFiles says the artifact is of a kind whose room holds files.
func hasFiles(a store.Artifact) bool {
	k, err := kind.Lookup(a.Kind)
	if err != nil {
		return false
	}
	_, ok := k.(kind.FileKind)
	return ok
}

// migrated brings a room whose log was written in an older form of its kind up
// to date: the change is applied to a copy of the room, and what it changed is
// appended to the log, before anyone has the room.
func (l seededLog) migrated(ctx context.Context, room string, snap realtime.Snapshot, limit int) (realtime.Snapshot, error) {
	a, err := l.artifacts.Artifact(ctx, room)
	if err != nil {
		return realtime.Snapshot{}, fmt.Errorf("read the artifact of room %s: %w", room, err)
	}
	m, ok := l.codecs[a.Kind].(Migrator)
	if !ok {
		return snap, nil
	}
	doc := crdt.New()
	defer doc.Destroy()
	for i, u := range snap.Updates {
		if err := crdt.ApplyUpdateV1(doc, u, nil); err != nil {
			return realtime.Snapshot{}, fmt.Errorf("room %s: stored update %d of %d does not apply: %w", room, i+1, len(snap.Updates), err)
		}
	}
	update, err := m.Migrate(doc)
	if err != nil {
		return realtime.Snapshot{}, fmt.Errorf("migrate room %s: %w", room, err)
	}
	if update == nil {
		return snap, nil
	}
	if err := realtime.CheckDocumentSize(append(snap.Updates, update), limit); err != nil {
		return realtime.Snapshot{}, err
	}
	if err := l.Append(ctx, room, snap.Epoch, update); err != nil {
		return realtime.Snapshot{}, fmt.Errorf("keep the migration of room %s: %w", room, err)
	}
	snap.Updates = append(snap.Updates, update)
	return snap, nil
}
