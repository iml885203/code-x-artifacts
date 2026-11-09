package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

const whoamiDoc = `Say who you are acting as on this CodeXArtifacts server and what the server allows.
Use it to check the connection, to learn your username (the "author" of what you publish), and to see
whether visibility "public" is available (publicLinks). No arguments.`

type noArgs struct{}

type whoamiOut struct {
	service.Profile
	// PublicLinks says whether visibility "public" can be used on this server.
	PublicLinks bool `json:"publicLinks"`
}

func (t *tools) whoami(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, whoamiOut, error) {
	me, err := t.e.svc.Me(ctx, t.viewer(ctx))
	if err != nil {
		return nil, whoamiOut{}, t.fail("whoami", err)
	}
	return nil, whoamiOut{Profile: me, PublicLinks: t.e.svc.PublicLinks()}, nil
}

const kindsDoc = `List the kinds of artifact this server can host, each with its authoring guide.
Call this before you write a page you have not written on this server yet. The guide tells you what valid
content is, what the page may and may not do (for html: the sandbox rules and the window.pictor API for who is
viewing and for shared data), and how to choose who can see it. No arguments.`

type kindOut struct {
	Name string `json:"name"`
	// Guide is the authoring manual, in Markdown.
	Guide string `json:"guide"`
	// ExportFormats are the formats the artifact can be downloaded in, if any.
	ExportFormats []string `json:"exportFormats"`
	// FormatVersions are the content format versions the publish tool's format
	// argument accepts, oldest first; empty when the kind has none.
	FormatVersions []string `json:"formatVersions"`
	// CurrentFormat is the version a new artifact gets; empty when the kind has none.
	CurrentFormat string `json:"currentFormat"`
}

type kindsOut struct {
	Kinds []kindOut `json:"kinds"`
	// Default is the kind used when publish does not name one.
	Default string `json:"default"`
}

func (t *tools) kinds(context.Context, *mcp.CallToolRequest, noArgs) (*mcp.CallToolResult, kindsOut, error) {
	out := kindsOut{Default: kind.Default, Kinds: []kindOut{}}
	for _, name := range kind.Names() {
		k, err := kind.Lookup(name)
		if err != nil {
			return nil, out, t.fail("kinds", fmt.Errorf("lookup registered kind %s: %w", name, err))
		}
		formats := []string{}
		if x, ok := k.(kind.Exporter); ok {
			formats = x.Formats()
		}
		out.Kinds = append(out.Kinds, kindOut{
			Name: name, Guide: k.Guide(), ExportFormats: formats,
			FormatVersions: append([]string{}, k.FormatVersions()...), CurrentFormat: kind.CurrentFormat(k),
		})
	}
	return nil, out, nil
}

// registerIdentity adds the tools of this feature to the server.
func (t *tools) registerIdentity(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "whoami", Description: whoamiDoc, Annotations: readOnly("Who am I")}, t.whoami)
	mcp.AddTool(s, &mcp.Tool{Name: "kinds", Description: kindsDoc, Annotations: readOnly("Artifact kinds and how to write them")}, t.kinds)
}
