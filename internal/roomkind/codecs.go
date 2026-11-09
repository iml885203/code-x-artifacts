package roomkind

import (
	"reflect"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
	"github.com/iml885203/code-x-artifacts/internal/boardroom"
	"github.com/iml885203/code-x-artifacts/internal/designfmt"
	"github.com/iml885203/code-x-artifacts/internal/designroom"
	"github.com/iml885203/code-x-artifacts/internal/docroom"
	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
)

// docCodec gives docroom's Codec the shape of a version: a doc is its content.
type docCodec struct{ docroom.Codec }

func (c docCodec) Seed(v kind.Bundle) ([]byte, error) { return c.Codec.Seed(v.Content) }

func (c docCodec) Content(doc *crdt.Doc) (kind.Bundle, error) {
	content, err := c.Codec.Content(doc)
	return kind.Bundle{Content: content}, err
}

func (c docCodec) Same(a, b kind.Bundle) bool { return c.Codec.Same(a.Content, b.Content) }

func (c docCodec) Write(doc *crdt.Doc, v kind.Bundle) (func(*crdt.Transaction), error) {
	return c.Codec.Write(doc, v.Content)
}

// Migrate moves a room that holds its tabs in the one fragment the first editor
// wrote into the form where each tab has a fragment of its own.
func (c docCodec) Migrate(doc *crdt.Doc) ([]byte, error) {
	write, err := docroom.Open(doc).Migrate()
	if err != nil || write == nil {
		return nil, err
	}
	before := doc.StateVector()
	doc.Transact(write)
	return crdt.EncodeStateAsUpdateV1(doc, before), nil
}

// designCodec is a design's version in its room: design.json and the frame files.
type designCodec struct{}

func (designCodec) Seed(v kind.Bundle) ([]byte, error) {
	return designroom.State(designroom.Design{Manifest: v.Content, Frames: v.Files})
}

// Content is what the room holds as a version. It has to pass the same
// validation as any publish: a room is written by people, and what they made
// becomes a version only if a design may hold it.
func (designCodec) Content(doc *crdt.Doc) (kind.Bundle, error) {
	d, err := designroom.Open(doc).Read()
	if err != nil {
		return kind.Bundle{}, err
	}
	frames := files.Set{}
	for path, text := range d.Frames {
		normal, err := designfmt.Normalize(text)
		if err != nil {
			return kind.Bundle{}, err
		}
		frames[path] = normal
	}
	if err := designfmt.Validate(d.Manifest, frames); err != nil {
		return kind.Bundle{}, err
	}
	return kind.Bundle{Content: d.Manifest, Files: frames}, nil
}

// File is one frame as Content would give it, built alone.
func (designCodec) File(doc *crdt.Doc, path string) (string, string, bool, error) {
	one, err := designroom.Open(doc).FrameNow(path)
	if err != nil || !one.Present {
		return one.Manifest, "", false, err
	}
	normal, err := designfmt.Normalize(one.Text)
	if err != nil {
		return "", "", false, err
	}
	return one.Manifest, normal, true, nil
}

// Same compares a version and a room's content frame by frame, in the form the
// room gives back.
func (designCodec) Same(a, b kind.Bundle) bool {
	if !sameManifest(a.Content, b.Content) || len(a.Files) != len(b.Files) {
		return false
	}
	for path, text := range a.Files {
		other, ok := b.Files[path]
		if !ok {
			return false
		}
		x, errX := designfmt.Normalize(text)
		y, errY := designfmt.Normalize(other)
		if errX != nil || errY != nil || x != y {
			return false
		}
	}
	return true
}

func (designCodec) Write(doc *crdt.Doc, v kind.Bundle) (func(*crdt.Transaction), error) {
	return designroom.Open(doc).Write(designroom.Design{Manifest: v.Content, Frames: v.Files})
}

// Merge is the change an agent's version makes to the room, by difference from
// the version the agent started from (designroom.Merge).
func (designCodec) Merge(doc *crdt.Doc, base, next kind.Bundle) (func(*crdt.Transaction), map[string][]string, error) {
	write, touched, err := designroom.Open(doc).Merge(
		designroom.Design{Manifest: base.Content, Frames: base.Files},
		designroom.Design{Manifest: next.Content, Frames: next.Files},
	)
	return write, touched, err
}

// Check refuses a tree with a cycle.
func (designCodec) Check(doc *crdt.Doc) error { return designroom.Check(doc) }

// sameManifest is whether two design.json texts are the same design: the room
// writes its own spelling of what a publisher wrote, so the texts are compared
// as the pages and frames they name.
func sameManifest(a, b string) bool {
	if a == b {
		return true
	}
	x, problemsX := designfmt.Parse(a)
	y, problemsY := designfmt.Parse(b)
	return len(problemsX) == 0 && len(problemsY) == 0 && reflect.DeepEqual(x, y)
}

// boardCodec is a board's version in its room: its elements, one entry per
// field (internal/boardroom). The version is the room in the stored form
// (boardfmt.Canonical), so labels sit on their shapes whoever moved what.
type boardCodec struct{}

func (boardCodec) Seed(v kind.Bundle) ([]byte, error) {
	b, err := boardfmt.Parse(v.Content)
	if err != nil {
		return nil, err
	}
	return boardroom.State(boardfmt.Canonical(b))
}

func (boardCodec) Content(doc *crdt.Doc) (kind.Bundle, error) {
	b, err := boardroom.Read(doc)
	if err != nil {
		return kind.Bundle{}, err
	}
	content := boardfmt.Encode(boardfmt.Canonical(b))
	if err := boardfmt.Validate(content, boardfmt.Format); err != nil {
		return kind.Bundle{}, err
	}
	return kind.Bundle{Content: content}, nil
}

func (boardCodec) Same(a, b kind.Bundle) bool {
	x, errX := boardfmt.Parse(a.Content)
	y, errY := boardfmt.Parse(b.Content)
	return errX == nil && errY == nil && boardfmt.Encode(boardfmt.Canonical(x)) == boardfmt.Encode(boardfmt.Canonical(y))
}

func (boardCodec) Write(doc *crdt.Doc, v kind.Bundle) (func(*crdt.Transaction), error) {
	b, err := boardfmt.Parse(v.Content)
	if err != nil {
		return nil, err
	}
	return boardroom.Write(doc, boardfmt.Canonical(b))
}

// Check refuses what a board may not hold (boardroom.Check).
func (boardCodec) Check(doc *crdt.Doc) error { return boardroom.Check(doc) }

// Merge is the change an agent's board makes to the board it started from,
// written field by field into what the room holds now (boardroom.Merge).
func (boardCodec) Merge(doc *crdt.Doc, base, next kind.Bundle) (func(*crdt.Transaction), map[string][]string, error) {
	b, err := boardfmt.Parse(base.Content)
	if err != nil {
		return nil, nil, err
	}
	n, err := boardfmt.Parse(next.Content)
	if err != nil {
		return nil, nil, err
	}
	write, touched, err := boardroom.Merge(doc, boardfmt.Canonical(b), boardfmt.Canonical(n))
	return write, map[string][]string{boardroom.Map: touched}, err
}
