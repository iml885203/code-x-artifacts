package mcptools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

const versionsDoc = `List the saved versions of an artifact you may edit (author or editor), newest first, without content. Every publish and every
restore saves a version and none is ever lost, so this is how you find what a page looked like before. Each entry
has version (the number), title, author, authors (everyone who edited it), agents (the agents that made it, each as by and client), createdAt, fileCount (how many files it has besides its page, when any) and restoredFrom when it was made by restore. Read one with read
(id and version); bring one back with restore. Readers cannot see versions.`

type versionsOut struct {
	// Latest is the newest version number: the baseVersion to send when you publish now.
	Latest   int             `json:"latest"`
	Versions []store.Version `json:"versions"`
}

func (t *tools) versions(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, versionsOut, error) {
	list, err := t.e.svc.Versions(ctx, t.viewer(ctx), in.ID)
	if err != nil {
		return nil, versionsOut{}, t.fail("versions", err)
	}
	out := versionsOut{Versions: make([]store.Version, 0, len(list))}
	for _, v := range list {
		out.Versions = append(out.Versions, v)
		out.Latest = max(out.Latest, v.Number)
	}
	return nil, out, nil
}

const restoreDoc = `Bring back an earlier version of an artifact you may edit (author or editor). It saves a copy of that version as a new newest
version, so nothing is lost: the versions in between stay in the history and you can restore forward again. The
audience does not change. Use it when the person says "go back to the earlier one" or when your own change was wrong;
find the number with versions.
version: the number to bring back.
baseVersion: the newest version you know of (from read or versions). If someone saved a newer one since, the restore is
refused and says which, so you do not discard that save by accident; look at it, then decide.`

type restoreIn struct {
	ID          string `json:"id" jsonschema:"the artifact id"`
	Version     int    `json:"version" jsonschema:"the version number to bring back, from the versions tool"`
	BaseVersion int    `json:"baseVersion,omitempty" jsonschema:"the newest version you know of; the restore is refused if the artifact has a newer one"`
}

func (t *tools) restore(ctx context.Context, _ *mcp.CallToolRequest, in restoreIn) (*mcp.CallToolResult, publishOut, error) {
	if in.Version < 1 {
		return nil, publishOut{}, errors.New("version is required: the number of the version to bring back (see versions)")
	}
	a, err := t.e.svc.Restore(t.asAgent(ctx), t.viewer(ctx), in.ID, in.Version, in.BaseVersion)
	if err != nil {
		return nil, publishOut{}, t.fail("restore", err)
	}
	return nil, publishOut{artifactOut: t.artifact(ctx, a), Next: fmt.Sprintf("Restored: version %d is now a copy of version %d. Give the person the url.", a.Version, in.Version)}, nil
}

// registerVersions adds the tools of this feature to the server.
func (t *tools) registerVersions(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "versions", Description: versionsDoc, Annotations: readOnly("List the versions of an artifact")}, t.versions)
	mcp.AddTool(s, &mcp.Tool{Name: "restore", Description: restoreDoc, Annotations: &mcp.ToolAnnotations{Title: "Restore an earlier version", DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, t.restore)
}
