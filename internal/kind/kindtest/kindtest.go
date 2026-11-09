// Package kindtest provides a kind with versioned content, so the layers
// above internal/kind can be tested against format versions before a shipped
// kind has any.
package kindtest

import (
	"errors"
	"testing"

	"github.com/iml885203/code-x-artifacts/internal/kind"
)

// Name is the registered name of the test kind.
const Name = "versioned"

// RejectedInV2 is content that format version v1 accepts and v2 refuses, so a
// test can tell which version validated a publish.
const RejectedInV2 = "legacy-only"

// Versioned is a kind whose rendering names the format version it used.
type Versioned struct{ Versions []string }

// Name implements kind.Kind.
func (Versioned) Name() string { return Name }

// FormatVersions implements kind.Kind.
func (v Versioned) FormatVersions() []string { return v.Versions }

// Validate implements kind.Kind.
func (Versioned) Validate(content, format string) error {
	if content == "" {
		return errors.New("a versioned artifact needs content")
	}
	if format == "v2" && content == RejectedInV2 {
		return errors.New("v2 no longer accepts this content")
	}
	return nil
}

// Render implements kind.Kind: the page says which format version drew it.
func (Versioned) Render(content, format string) (string, error) {
	return "<!doctype html><title>t</title><p data-runtime=\"" + format + "\">" + content + "</p>", nil
}

// Guide implements kind.Kind.
func (Versioned) Guide() string { return "Write any text." }

// Register adds the test kind with the given format versions for the rest of
// the test and returns a function that removes it early, which is how a test
// plays "the binary was upgraded": remove, then Register with more versions.
func Register(t testing.TB, versions ...string) (remove func()) {
	t.Helper()
	remove = kind.Add(&Versioned{Versions: versions})
	t.Cleanup(remove)
	return remove
}
