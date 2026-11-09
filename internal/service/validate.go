package service

import (
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// What makes a publish valid: the checks every way of publishing shares, so an
// inline publish and a publish session cannot disagree.

// validate checks a publish against the kind it is for. current is the
// artifact being updated, or the zero value for a new one.
func (s *Service) validate(in PublishInput, current store.Artifact) (PublishInput, error) {
	in, _, err := s.validateHeader(in, current)
	if err != nil {
		return in, err
	}
	if !in.stored {
		k, err := kind.Lookup(in.Kind)
		if err != nil {
			return in, refuse(Invalid, "%s", err)
		}
		if in, err = compose(in, current, k); err != nil {
			return in, err
		}
	}
	return in, nil
}

// validateHeader checks everything about a publish that does not need its
// content: the title, the audience, and that the kind and its format version
// exist and agree with the artifact being updated. It returns the publish with
// the kind and format filled in.
func (s *Service) validateHeader(in PublishInput, current store.Artifact) (PublishInput, kind.Kind, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return in, nil, refuse(Invalid, "title is required (when updating, send the current title to keep it)")
	}
	if in.Visibility != "" {
		if err := s.checkVisibility(in.Visibility); err != nil {
			return in, nil, err
		}
	}
	in.Kind = kindName(in, current)
	k, err := kind.Lookup(in.Kind)
	if err != nil {
		return in, nil, refuse(Invalid, "%s", err)
	}
	if current.Kind != "" && in.Kind != current.Kind {
		return in, nil, refuse(Invalid, "this artifact is kind %s and its kind cannot change; publish a new artifact for kind %s", current.Kind, in.Kind)
	}
	in.Format = formatFor(in, current, k)
	if !kind.HasFormat(k, in.Format) {
		return in, nil, refuse(Invalid, "kind %s has no format version %q; %s", in.Kind, in.Format, formatChoices(k))
	}
	return in, k, nil
}

// kindName is the kind a publish is for: the one named, else the artifact's
// own, else doc when blocks are sent, else the default.
func kindName(in PublishInput, current store.Artifact) string {
	switch {
	case in.Kind != "":
		return in.Kind
	case current.Kind != "":
		return current.Kind
	case len(in.Blocks) > 0:
		return "doc"
	}
	return kind.Default
}

// formatFor is the content format version a publish is written in: the one
// named, the kind's current one when blocks are sent, the artifact's own, or
// for a new artifact the kind's current one.
func formatFor(in PublishInput, current store.Artifact, k kind.Kind) string {
	switch {
	case in.Format != "":
		return in.Format
	case len(in.Blocks) > 0:
		return kind.CurrentFormat(k)
	case current.Format != "":
		return current.Format
	case current.Kind != "":
		return ""
	}
	return kind.CurrentFormat(k)
}

// compose turns what the publisher sent into the content to store, for a kind
// that does so (a doc's blocks get their ids), and refuses blocks for one that
// does not.
func compose(in PublishInput, current store.Artifact, k kind.Kind) (PublishInput, error) {
	c, ok := k.(kind.Composer)
	if !ok {
		if len(in.Blocks) > 0 {
			return in, refuse(Invalid, "kind %s takes content, not blocks", in.Kind)
		}
		return in, nil
	}
	previous := ""
	if current.Format == in.Format {
		previous = current.Content
	}
	content, err := c.Compose(in.Format, in.Content, in.Blocks, previous)
	if err != nil {
		return in, contentRefusal(err, in)
	}
	in.Content, in.Blocks = content, nil
	return in, nil
}

// validateContent checks the content a publish carries. A kind that judges it
// together with its files (react compiles them into the page, design checks that
// every frame has its file) has to see the files of the same version; any other
// judges the content alone.
func validateContent(k kind.Kind, content, format string, set files.Set) error {
	if v, ok := k.(kind.FileValidator); ok && len(set) > 0 {
		return v.ValidateFiles(content, format, set)
	}
	return k.Validate(content, format)
}

// contentRefusal is the refusal for content a kind did not accept. One that is
// only over the size limit points at the way to send a large file.
func contentRefusal(err error, in PublishInput) error {
	if kind.IsTooLarge(err) && !in.fromFile {
		return refuse(Invalid, "%s. A page this large goes up from a file instead of being typed out: begin_publish and commit_publish over MCP, POST /api/publish-sessions over HTTP", err)
	}
	return refuse(Invalid, "%s", err)
}

func formatChoices(k kind.Kind) string {
	if len(k.FormatVersions()) == 0 {
		return "this kind has no versions, omit format"
	}
	return "the versions are: " + strings.Join(k.FormatVersions(), ", ")
}

// VisibilityHint is the sentence that lists the audiences.
const VisibilityHint = `visibility must be "private", "organisation" or "public"`

// checkVisibility validates a visibility an author asks for. Public is refused
// while the deployment has public links off, so nobody is led to believe a
// link is open to anyone.
func (s *Service) checkVisibility(v store.Visibility) error {
	switch {
	case !v.Valid():
		return refuse(Invalid, "%s", VisibilityHint)
	case v == store.VisibilityPublic && !s.publicLinks:
		return refuse(Invalid, "public links are turned off on this server; use \"organisation\" to share with everyone who can sign in")
	default:
		return nil
	}
}
