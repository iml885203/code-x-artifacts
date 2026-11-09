package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/boardfmt"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// checkBoardFiles refuses a board whose images show files the artifact does not
// have: an image is a file uploaded to the board's own artifact (upload_asset).
// id is empty for a board not created yet, which has no files.
func (s *Service) checkBoardFiles(ctx context.Context, id, content string) error {
	b, err := boardfmt.Parse(content)
	if err != nil {
		return nil //nolint:nilerr // what is not a board is refused by its own validation, in its own words
	}
	wanted := boardfmt.FileIDs(b)
	if len(wanted) == 0 {
		return nil
	}
	if id == "" {
		return refuse(Invalid, "board: an image shows a file uploaded to the board, and a new board has none: publish it without images, upload the picture with upload_asset, then add the image with board_edit")
	}
	assets, err := s.store.Assets(ctx, id)
	if err != nil {
		return fmt.Errorf("list the files of artifact %s: %w", id, err)
	}
	have := map[string]bool{}
	var names []string
	for _, a := range assets {
		have[a.Name] = true
		names = append(names, a.Name)
	}
	for _, el := range boardfmt.Canonical(b).Elements {
		if f, _ := el["fileId"].(string); el["type"] == "image" && !have[f] {
			return refuse(Invalid, "board: image %s shows %q, which is not a file of this board; upload it with upload_asset first (the board's files: %s)", el.ID(), f, list(names))
		}
	}
	return nil
}

func list(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// validateWithFiles is validate, and for a board the check that its images show
// files of the artifact (a, empty for a new one).
func (s *Service) validateWithFiles(ctx context.Context, in PublishInput, a store.Artifact) (PublishInput, error) {
	in, err := s.validate(in, a)
	if err != nil || in.Kind != (kind.Board{}).Name() {
		return in, err
	}
	return in, s.checkBoardFiles(ctx, a.ID, in.Content)
}
