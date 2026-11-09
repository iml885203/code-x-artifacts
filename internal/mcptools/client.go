package mcptools

import (
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// clients are the agents people use, by what they call themselves in the MCP
// clientInfo they send when a session starts. The name is matched without case,
// by the start of it ("claude-code" and "claude-code-vscode" are both Claude Code).
var clients = []struct{ prefix, name string }{
	{"claude-code", "Claude Code"},
	{"claude-ai", "Claude"},
	{"claude", "Claude"},
	{"codex", "Codex"},
	{"cursor", "Cursor"},
	{"windsurf", "Windsurf"},
	{"gemini", "Gemini CLI"},
	{"copilot", "Copilot"},
	{"visual studio code", "VS Code"},
	{"vscode", "VS Code"},
	{"cline", "Cline"},
	{"zed", "Zed"},
	{"goose", "Goose"},
	{"opencode", "OpenCode"},
	{"hermes", "Hermes"},
}

// ClientName normalizes a self-reported display label, never an authenticated
// software identity. Activity, room presence and version labels share the bound.
func ClientName(info string) string {
	info = strings.TrimSpace(store.BoundedLabel(info, 60))
	lower := strings.ToLower(info)
	if strings.HasPrefix(lower, "codexartifacts") {
		return info
	}
	for _, c := range clients {
		if strings.HasPrefix(lower, c.prefix) {
			return c.name
		}
	}
	return info
}
