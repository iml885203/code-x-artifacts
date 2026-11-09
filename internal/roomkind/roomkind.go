// Package roomkind is what turns a live room into versions and back: the part
// of the realtime layer that knows what an artifact's content is. A kind that
// can be edited live supplies a Codec (Docs, Design and Board do), and this package
// does the rest the same way for every kind: it starts a room from the latest
// version, cuts a version when a room has been quiet, and writes a restored
// version into the room (docs/doc-room.md).
//
// internal/realtime stays ignorant of all of it: it holds documents and logs.
package roomkind

import (
	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/kind"
)

// Codec is how one kind's version and its room become each other. A version is
// its content and its files (kind.Bundle).
type Codec interface {
	// Seed is the room state that starts a room from a version.
	Seed(v kind.Bundle) ([]byte, error)
	// Content is what the room holds, as a version. It refuses a room whose
	// content the kind would not accept from a publisher.
	Content(doc *crdt.Doc) (kind.Bundle, error)
	// Same reports whether two versions are the same thing in a room, so a
	// snapshot that equals the previous version is not saved again.
	Same(a, b kind.Bundle) bool
	// Write is the change that makes the room hold a version, for Hub.Edit: it
	// is the plan, which may look the document up, and returns the write.
	Write(doc *crdt.Doc, v kind.Bundle) (func(*crdt.Transaction), error)
}

// Merger is implemented by a Codec whose room can take a version an agent made
// from an older one without overwriting what people did to the room since: the
// difference between base (the version the agent started from) and next (what it
// sent) is the change, applied to whatever the room holds. It also says which
// elements of which files the change touched. Without it the only way in is
// Write, which makes the room hold the version.
type Merger interface {
	Merge(doc *crdt.Doc, base, next kind.Bundle) (write func(*crdt.Transaction), touched map[string][]string, err error)
}

// OneFileCodec is implemented by a Codec that can read one file of its room
// without building the others: the content (design.json) and that file, in the
// form Content would give it, and whether the room holds the file.
type OneFileCodec interface {
	File(doc *crdt.Doc, path string) (content, text string, present bool, err error)
}

// Checker is implemented by a Codec that can say a room's document is one its
// kind may not become (a cycle in a design's tree). The hub asks it before it
// keeps a change.
type Checker interface {
	Check(doc *crdt.Doc) error
}

// Migrator is implemented by a Codec whose room form has changed: Migrate makes
// the change a room that still has the old form needs, and returns it as an
// update (nil when there is nothing to do). The log keeps it, so a room is
// migrated once.
type Migrator interface {
	Migrate(doc *crdt.Doc) ([]byte, error)
}

// Codecs are the kinds that have a live room, by kind name. An artifact of any
// other kind has no content in its room.
type Codecs map[string]Codec

// DefaultCodecs is every kind that can be edited live today.
func DefaultCodecs() Codecs {
	return Codecs{
		(kind.Doc{}).Name():    docCodec{},
		(kind.Design{}).Name(): designCodec{},
		(kind.Board{}).Name():  boardCodec{},
	}
}

// Check is the hub's Validate: it asks every codec that can say, which is cheap
// for a room of another kind (it holds nothing of theirs).
func (c Codecs) Check(_ string, doc *crdt.Doc) error {
	for _, codec := range c {
		if ch, ok := codec.(Checker); ok {
			if err := ch.Check(doc); err != nil {
				return err
			}
		}
	}
	return nil
}
