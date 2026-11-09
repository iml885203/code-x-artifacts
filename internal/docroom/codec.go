package docroom

import (
	"encoding/json"

	"github.com/reearth/ygo/crdt"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// Codec is how a doc and its room become each other's content: the room of an
// artifact of kind doc (internal/roomkind registers it under that name). The
// content is the stored form of format 2, JSON blocks.
type Codec struct{}

// Seed is the room state that starts a room from content.
func (Codec) Seed(content string) ([]byte, error) {
	d, err := docfmt.Parse(content)
	if err != nil {
		return nil, err
	}
	return State(d), nil
}

// Content is what the room holds as the content of a version. It has to pass
// the same validation as any publish: a room is written by people, and what
// they typed becomes a version only if a doc may hold it.
func (Codec) Content(doc *crdt.Doc) (string, error) {
	d, err := Open(doc).Read()
	if err != nil {
		return "", err
	}
	content, err := docfmt.Canonical(d)
	if err != nil {
		return "", err
	}
	if err := docfmt.Validate(content); err != nil {
		return "", err
	}
	return content, nil
}

// Same reports whether two contents are the same document in a room: equal once
// both are in the form a room gives back (Canon).
func (Codec) Same(a, b string) bool {
	x, errA := docfmt.Parse(a)
	y, errB := docfmt.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	jx, errA := json.Marshal(Canon(x))
	jy, errB := json.Marshal(Canon(y))
	return errA == nil && errB == nil && string(jx) == string(jy)
}

// Write is the change that makes the room hold content, for Hub.Edit, which
// runs the plan (this) before the write: the plan may look the document up. It
// changes only what differs (Content.Apply), so what people are typing elsewhere
// stays.
func (Codec) Write(doc *crdt.Doc, content string) (func(*crdt.Transaction), error) {
	d, err := docfmt.Parse(content)
	if err != nil {
		return nil, err
	}
	return Open(doc).Apply(d)
}
