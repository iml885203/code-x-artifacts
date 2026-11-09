package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

const changesSinceDoc = `Ask what changed on an artifact you can open since you last looked: the new versions (who saved them and when, never
their content), the new comments, and any change of audience or editors. There is no push, so an agent that waits for
other people calls this now and then, at an interval that suits the work (a minute is plenty), and reacts to what comes
back. Say where to start with exactly one of:
since_version: the newest version you have seen (from read, publish or an earlier changes_since). Returns the versions
after it, and the comments and access changes from the moment that version was saved.
since: a time, RFC 3339 such as 2026-10-20T09:30:00Z. Returns what happened at or after it.
The answer names where the artifact is now (version, title, updatedAt): next time pass that version as since_version.
Items stamped in the same second as the starting point come back again; match them by version number or comment id.
versions: each has version, author, createdAt, restoredFrom when it was made by restore. To see what a version says, use
read with its number (author and editors only). comments: id, author, body, createdAt, parentId for a reply. access:
each change as what the audience (visibility) and editors became, with who changed it (by) and when (at). Nothing new
is empty lists, not an error. An artifact you cannot open answers as if it did not exist.`

type changesSinceIn struct {
	ID           string `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
	SinceVersion int    `json:"since_version,omitempty" jsonschema:"the newest version number you have seen; returns what happened after it. Give this or since, not both"`
	Since        string `json:"since,omitempty" jsonschema:"a time in RFC 3339 such as 2026-10-20T09:30:00Z; returns what happened at or after it. Give this or since_version, not both"`
}

func (t *tools) changesSince(ctx context.Context, _ *mcp.CallToolRequest, in changesSinceIn) (*mcp.CallToolResult, service.Changes, error) {
	out, err := t.e.svc.ChangesSince(ctx, t.viewer(ctx), in.ID, in.SinceVersion, in.Since)
	if err != nil {
		return nil, service.Changes{}, t.fail("changes_since", err)
	}
	return nil, out, nil
}

// registerChanges adds the tools of this feature to the server.
func (t *tools) registerChanges(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "changes_since", Description: changesSinceDoc, Annotations: readOnly("What changed on an artifact since a version or time")}, t.changesSince)
}
