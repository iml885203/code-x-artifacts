package roomkind

import (
	"context"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/reearth/ygo/crdt"
)

// SourceSnapshot captures a complete source bundle and its live epoch atomically.
func (s *Snapshotter) SourceSnapshot(ctx context.Context, id, kindName string) (kind.Bundle, string, error) {
	reader, ok := s.rooms.(interface {
		ReadEpoch(context.Context, string, func(*crdt.Doc, string) error) error
	})
	if !ok {
		return kind.Bundle{}, "", fmt.Errorf("source snapshot requires coherent epoch reads")
	}
	codec, ok := s.cfg.Codecs[kindName]
	if !ok {
		return kind.Bundle{}, "", fmt.Errorf("a %s has no source snapshot", kindName)
	}
	var bundle kind.Bundle
	var epoch string
	err := reader.ReadEpoch(ctx, id, func(doc *crdt.Doc, generation string) (err error) {
		bundle, err = codec.Content(doc)
		epoch = generation
		return err
	})
	return bundle, epoch, err
}

// SourceFile reads one file of a room without building the rest of it.
func (s *Snapshotter) SourceFile(ctx context.Context, id, kindName, path string) (kind.SourceFile, error) {
	reader, ok := s.rooms.(interface {
		ReadEpoch(context.Context, string, func(*crdt.Doc, string) error) error
	})
	if !ok {
		return kind.SourceFile{}, fmt.Errorf("source file requires coherent epoch reads")
	}
	codec, ok := s.cfg.Codecs[kindName].(OneFileCodec)
	if !ok {
		return kind.SourceFile{}, fmt.Errorf("a %s has no one-file read", kindName)
	}
	var out kind.SourceFile
	err := reader.ReadEpoch(ctx, id, func(doc *crdt.Doc, generation string) (err error) {
		out.Content, out.Text, out.Present, err = codec.File(doc, path)
		out.Epoch = generation
		return err
	})
	return out, err
}
