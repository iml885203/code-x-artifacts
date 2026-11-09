package service

import (
	"sort"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// review asks the artifact's kind for its notes on content that was accepted.
// previous is the content of the version it replaces, empty for a new artifact;
// set holds the files of the new version and sent the paths this publish sent.
func review(a store.Artifact, previous string, set files.Set, sent []string) []string {
	k, err := kind.Lookup(a.Kind)
	if err != nil {
		return nil
	}
	var notes []string
	if r, ok := k.(kind.Reviewer); ok {
		notes = r.Review(a.Content, a.Format, previous)
	}
	if r, ok := k.(kind.FileReviewer); ok {
		notes = append(notes, r.ReviewFiles(a.Content, a.Format, set, sent)...)
	}
	return notes
}

// sentPaths lists the files a publish put (not removed), sorted.
func sentPaths(patch map[string]*string) []string {
	var paths []string
	for p, text := range patch {
		if text != nil {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}
