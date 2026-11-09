package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const pinIntro = `Pins are the artifacts a person keeps at the top of their overview. They are the caller's own: nobody else sees
them, and list and read report pinned for you. `

const pinDoc = pinIntro + `pin pins an artifact you can open (your own, or one someone shared with you), so it is listed first in your overview.
Pinning twice does nothing more. Use it when the person asks to keep a page at hand.`

const unpinDoc = pinIntro + `unpin removes your pin from an artifact; the artifact itself is untouched. Unpinning what is not pinned does nothing.`

type pinOut struct {
	ID string `json:"id"`
	// Pinned is whether the artifact is pinned for you after the call.
	Pinned bool `json:"pinned"`
}

func (t *tools) setPin(pinned bool, name string) func(context.Context, *mcp.CallToolRequest, idIn) (*mcp.CallToolResult, pinOut, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, pinOut, error) {
		a, err := t.e.svc.Pin(ctx, t.viewer(ctx), in.ID, pinned)
		if err != nil {
			return nil, pinOut{}, t.fail(name, err)
		}
		return nil, pinOut{ID: a.ID, Pinned: a.Pinned}, nil
	}
}

// registerPins adds the tools of this feature to the server.
func (t *tools) registerPins(s *mcp.Server) {
	annotations := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(false)}
	}
	mcp.AddTool(s, &mcp.Tool{Name: "pin", Description: pinDoc, Annotations: annotations("Pin an artifact")}, t.setPin(true, "pin"))
	mcp.AddTool(s, &mcp.Tool{Name: "unpin", Description: unpinDoc, Annotations: annotations("Unpin an artifact")}, t.setPin(false, "unpin"))
}
