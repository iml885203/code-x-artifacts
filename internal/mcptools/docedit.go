package mcptools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/docfmt"
	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const docEditDoc = `Change some blocks of a doc without rewriting it: use this for a small change; publish is for a new document.
Every block has an id and a hash: read with view "blocks" (or "markdown", which lists the ids of the top-level blocks) first.
ops is a list, done in order, all or none: if one cannot be done, or the result would not be a valid doc, nothing is changed.
Inline date fields in blocks are runs {"date":"2026-10-24"} in content, wherever text runs are accepted. A date is a valid
calendar day YYYY-MM-DD (years 0001 through 9999) with no text, marks, href, user or name. Read view blocks to retain the field;
Markdown and exports contain only its visible date text. To change a date, replace its containing block with blocks and ifHash.
Each op has op and:
 replace: id (the block), and the new content as markdown (any Markdown, one or more blocks) or blocks (a list of blocks, see the
   doc guide in kinds). The first new block keeps the id of the block it replaces unless it brings its own.
 insert: where, with exactly one of before (a block id), after (a block id), or tab (a tab id) with at "start" or "end"; and the
   new content as markdown or blocks.
 delete: id.
 tab_add: title (one line), and where: at "start" or "end" (the default is the end), or before or after (a tab id); optionally the
   first content of the tab as markdown or blocks. Without content the new tab starts with one empty heading 1 for the person to type
   into. The result lists the tabs with the id of the new one.
 tab_rename: tab (the tab id), title.
 tab_delete: tab. Deletes the tab with its blocks; the last tab of a doc cannot be deleted.
 tab_move: tab, and exactly one of before or after (another tab id) or at "start" or "end".
A tab op names its tab with tab (never id) and takes no ifHash. read lists the tabs with their ids and titles in every view.
ifHash (optional on any op): the hash read gave for the block the op names (for an insert, the block it is next to). When that
block is not what you read, because a person edited it meanwhile, the whole call is refused and the current state of the stale
blocks comes back: merge your change into it and send the call again with the new hashes. Hashes are checked against the doc as it
was before the call. Always send ifHash for a block you are rewriting from what you read.
baseVersion (optional): the newest saved version you know; a newer one refuses the call. Not needed when you send ifHash.
When people have the doc open, the change goes into the live copy and they see it at once, and what they are typing elsewhere is
kept; the result says saved false, and a version follows a couple of minutes after the doc goes quiet. Otherwise it is saved as a
new version now (saved true). The result lists the blocks you wrote with their new hashes, the ids you deleted, and review: things to
fix or mention (for example an image without alt text).
When the call deletes a block that people commented on (a delete, a replace by blocks with ids of their own, a deleted tab or
quote), the edit is not blocked: it is done, and commentsOnDeletedBlocks lists each such thread (id, tab, block, quote). The
threads stay in comments with their quote but no longer point at text in the doc: tell the person, or reply on the thread.`

// editOp is one operation as an agent sends it; its blocks are the value
// itself, not a string.
type editOp struct {
	Op       string `json:"op" jsonschema:"replace, insert, delete, tab_add, tab_rename, tab_delete or tab_move"`
	ID       string `json:"id,omitempty" jsonschema:"replace and delete: the id of the block"`
	IfHash   string `json:"ifHash,omitempty" jsonschema:"the hash read gave for the block this op names (for an insert: the block it is next to); a different block refuses the whole call"`
	Before   string `json:"before,omitempty" jsonschema:"insert: put the new blocks before this block id; tab_add and tab_move: before this tab id"`
	After    string `json:"after,omitempty" jsonschema:"insert: put the new blocks after this block id; tab_add and tab_move: after this tab id"`
	Tab      string `json:"tab,omitempty" jsonschema:"insert: a tab id, with at; tab_rename, tab_delete and tab_move: the tab"`
	At       string `json:"at,omitempty" jsonschema:"insert with tab, tab_add and tab_move: start or end"`
	Markdown string `json:"markdown,omitempty" jsonschema:"replace and insert: the new content as Markdown (use this or blocks)"`
	Blocks   any    `json:"blocks,omitempty" jsonschema:"replace and insert: the new content as a list of blocks (use this or markdown); send the value itself, not a string"`
	Title    string `json:"title,omitempty" jsonschema:"tab_add and tab_rename: the title of the tab, one line"`
}

type docEditIn struct {
	ID          string   `json:"id" jsonschema:"the id of the doc"`
	Ops         []editOp `json:"ops" jsonschema:"the operations, done in order, all or none"`
	BaseVersion int      `json:"baseVersion,omitempty" jsonschema:"optional: the newest saved version you know; a newer one refuses the call"`
}

type docEditOut struct {
	service.DocEdited
	Next string `json:"next"`
}

func (t *tools) docEdit(ctx context.Context, _ *mcp.CallToolRequest, in docEditIn) (*mcp.CallToolResult, docEditOut, error) {
	ops := make([]docedit.Op, len(in.Ops))
	for i, o := range in.Ops {
		blocks, err := jsonArg(o.Blocks)
		if err != nil {
			return nil, docEditOut{}, fmt.Errorf("ops[%d]: blocks must be JSON: a list of blocks", i)
		}
		ops[i] = docedit.Op{Op: o.Op, ID: o.ID, IfHash: o.IfHash, Before: o.Before, After: o.After, Tab: o.Tab, At: o.At, Markdown: o.Markdown, Blocks: blocks, Title: o.Title}
	}
	res, err := t.e.svc.DocEdit(t.asAgent(ctx), t.viewer(ctx), in.ID, service.DocEditInput{BaseVersion: in.BaseVersion, Ops: ops})
	if err != nil {
		return nil, docEditOut{}, t.fail("doc_edit", err)
	}
	next := fmt.Sprintf("Saved as version %d.", res.Version)
	if !res.Saved {
		next = "Done in the live copy: people who have the doc open see it now, and a version follows when it has been quiet for a couple of minutes."
	}
	if len(res.Review) > 0 {
		next += " Review before you tell the person it is done: see review."
	}
	return nil, docEditOut{DocEdited: res, Next: next}, nil
}

// withBlocks adds to a read of a doc the ids of its blocks: an outline beside
// the Markdown, or with view blocks every block in full instead of the Markdown.
func (t *tools) withBlocks(out *readOut, a store.Artifact, view string) error {
	if view != "" && view != "markdown" && view != "blocks" {
		return errors.New(`view is "markdown" or "blocks"`)
	}
	if a.Kind != "doc" {
		if view == "blocks" {
			return fmt.Errorf("this artifact is a %s, not a doc: it has no blocks; read it without view", a.Kind)
		}
		return nil
	}
	d, err := docfmt.Parse(a.Content)
	if err != nil {
		return fmt.Errorf("this doc does not parse: %w", err)
	}
	if view == "blocks" {
		out.Content, out.Tabs = "", docedit.View(d)
		return nil
	}
	out.Outline = docedit.Outlines(d)
	out.Tabs = docedit.Tabs(d)
	return nil
}

// registerDocEdit adds the tools of this feature to the server.
func (t *tools) registerDocEdit(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "doc_edit", Description: docEditDoc, Annotations: &mcp.ToolAnnotations{Title: "Change some blocks of a doc", DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}}, t.docEdit)
}
