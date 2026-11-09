package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/service"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

const commentIntro = `Every artifact has one conversation under it: people who can open the artifact and are signed in leave comments,
the author and the author's agent read and answer them. Comments are plain text. A comment may point at a place (anchor).
list and read show how many an artifact has (comments.count) and, on your own artifacts, how many you have not seen
yet (comments.unread). `

const commentsDoc = commentIntro + `comments lists the comments of an artifact you can open, oldest first. Each has an id, author, body,
createdAt and forAgent (true when it is meant for the author's agent to act on; false is a note between people). A comment on a place has an
anchor, a JSON object whose meaning belongs to the kind of artifact (a doc block and a quote, a design frame and element); read it to
know what the comment is about. The anchor is kept even when its target has since been removed. A reply has parentId (the comment that
started its thread). On your own artifact, a comment by someone
else that you have not seen has unread: true. Use it when the person asks what people said about a page, or to find
what to answer.
mark_read: optional. true records that you (as the author) have seen every comment this call returned, so the author's
unread count goes down; leave it out to only look. Only do it when the person would agree they have seen them or when
you are about to answer them.`

type commentsIn struct {
	ID       string `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
	MarkRead bool   `json:"mark_read,omitempty" jsonschema:"true to record the returned comments as seen by you; only matters on your own artifact"`
}

type commentsOut struct {
	Comments []store.Comment `json:"comments"`
}

func (t *tools) comments(ctx context.Context, _ *mcp.CallToolRequest, in commentsIn) (*mcp.CallToolResult, commentsOut, error) {
	list, err := t.e.svc.Comments(ctx, t.viewer(ctx), in.ID)
	if err != nil {
		return nil, commentsOut{}, t.fail("comments", err)
	}
	if in.MarkRead && len(list) > 0 {
		if err := t.e.svc.MarkCommentsSeen(ctx, t.viewer(ctx), in.ID, list[len(list)-1].ID); err != nil {
			return nil, commentsOut{}, t.fail("comments", err)
		}
	}
	return nil, commentsOut{Comments: list}, nil
}

const commentDoc = commentIntro + `comment adds a comment to an artifact you can open (your own, or one someone shared with you). Use it to answer
what people wrote, or to leave feedback on someone else's page. Write plain text (no HTML or Markdown is rendered),
at most 4000 characters, and say what you did or what you need, as you would to a colleague.
body: the comment, required.
reply_to: optional id of the comment you answer (from comments). The reply joins that comment's thread; leave it out
to start a new one. Answering a person is a reply_to, a new remark about the page is not.
anchor: optional JSON object saying where the comment points; copy the shape of the anchors comments returns, at most 2000
bytes (doc: tab, block and quote; design: type=design, frameId, optional pid, x/y in frame pixels and optional name; see each kind guide from kinds). A reply takes its thread's place, so do not send anchor with reply_to.
for_agent: optional, true unless you say otherwise; false marks a remark between people that no agent should act on.
Comments cannot be edited through this tool. Use delete_comment only when the person asks to remove their own content.`

type commentIn struct {
	ID       string         `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
	Body     string         `json:"body" jsonschema:"the comment, plain text, at most 4000 characters"`
	ReplyTo  string         `json:"reply_to,omitempty" jsonschema:"id of the comment you are answering, from the comments tool; omit to start a new thread"`
	Anchor   map[string]any `json:"anchor,omitempty" jsonschema:"a JSON object saying where the comment points, in the shape the comments tool returns; omit for a comment on the whole page"`
	ForAgent *bool          `json:"for_agent,omitempty" jsonschema:"false marks a remark between people that no agent should act on; default true"`
}

func (t *tools) comment(ctx context.Context, _ *mcp.CallToolRequest, in commentIn) (*mcp.CallToolResult, store.Comment, error) {
	var anchor json.RawMessage
	if in.Anchor != nil {
		var err error
		if anchor, err = json.Marshal(in.Anchor); err != nil {
			return nil, store.Comment{}, t.fail("comment", fmt.Errorf("anchor is not JSON: %w", err))
		}
	}
	c, err := t.e.svc.AddComment(ctx, t.viewer(ctx), in.ID, service.NewComment{Body: in.Body, ReplyTo: in.ReplyTo, Anchor: anchor, ForAgent: in.ForAgent})
	if err != nil {
		return nil, store.Comment{}, t.fail("comment", err)
	}
	return nil, c, nil
}

const deleteCommentDoc = commentIntro + `delete_comment removes only your own authored comment content on an artifact you can still open.
id: the artifact id. comment_id: the comment id from comments.
Other people's replies are never deleted. A deleted root with remaining replies keeps its id and anchor as a clearly marked,
empty tombstone; deleted replies and empty deleted threads disappear from comments. The deleted body is not retained in reads.
This cannot restore the removed words. Use it only when the person asked. Repeating an owned deletion succeeds without changing
anyone else's content; loss of artifact access or a different comment author still refuses.`

type deleteCommentIn struct {
	ID        string `json:"id" jsonschema:"the artifact id"`
	CommentID string `json:"comment_id" jsonschema:"your own comment id, as comments returned it"`
}

func (t *tools) deleteComment(ctx context.Context, _ *mcp.CallToolRequest, in deleteCommentIn) (*mcp.CallToolResult, deletedOut, error) {
	if err := t.e.svc.DeleteComment(ctx, t.viewer(ctx), in.ID, in.CommentID); err != nil {
		return nil, deletedOut{}, t.fail("delete_comment", err)
	}
	return nil, deletedOut{Deleted: true}, nil
}

// registerComments adds the tools of this feature to the server.
func (t *tools) registerComments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "comments", Description: commentsDoc, Annotations: readOnlyExceptMark("Read the comments on an artifact")}, t.comments)
	mcp.AddTool(s, &mcp.Tool{Name: "comment", Description: commentDoc, Annotations: &mcp.ToolAnnotations{Title: "Comment on an artifact", DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, t.comment)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_comment", Description: deleteCommentDoc, Annotations: &mcp.ToolAnnotations{Title: "Delete your own comment", DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.deleteComment)
}
