package docedit

import (
	"encoding/json"
	"errors"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// newBlocks reads the blocks an operation brings, from its Markdown or its
// blocks. keep is the id the first block takes when it has none (a replace
// keeps the id of what it replaces). Ids left out get one.
func newBlocks(op Op, i int, keep string, newID func(string) string) ([]docfmt.Block, error) {
	hasMD, hasBlocks := op.Markdown != "", len(op.Blocks) > 0 && string(op.Blocks) != "null"
	switch {
	case hasMD && hasBlocks:
		return nil, refuse(i, "send markdown or blocks, not both")
	case hasMD:
		return fromMarkdown(op.Markdown, i, keep, newID)
	case hasBlocks:
		return fromBlocks(op.Blocks, i, keep, newID)
	}
	return nil, refuse(i, "%s needs the new content: markdown, or blocks (a list of blocks)", op.Op)
}

func fromMarkdown(markdown string, i int, keep string, newID func(string) string) ([]docfmt.Block, error) {
	d, err := docfmt.FromMarkdown(markdown, nil, newID)
	if err != nil {
		return nil, refuse(i, "the markdown is refused: %s", err)
	}
	if len(d.Tabs) != 1 || len(d.Tabs[0].Blocks) == 0 {
		return nil, refuse(i, "the markdown has to make blocks in one tab (no tab markers) and not be empty")
	}
	blocks := d.Tabs[0].Blocks
	if keep != "" {
		blocks[0].ID = keep
	}
	return blocks, nil
}

func fromBlocks(raw json.RawMessage, i int, keep string, newID func(string) string) ([]docfmt.Block, error) {
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
		return nil, refuse(i, "blocks is a non-empty list of blocks, as in docs/doc-format.md")
	}
	if _, has := list[0]["id"]; keep != "" && !has {
		list[0]["id"] = keep
	}
	withIDs, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	content, err := docfmt.Normalize(withIDs, newID)
	if err != nil {
		return nil, refuse(i, "the blocks are refused: %s", err)
	}
	d, err := docfmt.Parse(content)
	if err != nil || len(d.Tabs) != 1 {
		return nil, errors.Join(refuse(i, "the blocks are refused"), err)
	}
	return d.Tabs[0].Blocks, nil
}
