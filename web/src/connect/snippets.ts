// The lines each client needs to reach CodeXArtifacts's MCP endpoint. The address is
// the server's own (the boot block), never a constant, so a copy of CodeXArtifacts on
// another host writes commands that work there.

/** What stands in for the token until step 1 is done. */
export const tokenPlaceholder = "<YOUR_TOKEN>";

export type Client = "claude" | "codex" | "other";

export const clients: readonly Client[] = ["claude", "codex", "other"];

/** The one-line command that adds CodeXArtifacts to Claude Code. */
export function claudeCommand(baseUrl: string, token: string): string {
  return `claude mcp add --transport http pictor ${baseUrl}/api/mcp --header "Authorization: Bearer ${token}"`;
}

/** Codex reads the token from the environment, so config.toml never holds the secret. */
export function codexEnv(token: string): string {
  return `export PICTOR_TOKEN=${token}`;
}

export function codexCommand(baseUrl: string): string {
  return `codex mcp add pictor --url ${baseUrl}/api/mcp --bearer-token-env-var PICTOR_TOKEN`;
}

/** The same server as the command above, for ~/.codex/config.toml. */
export function codexConfig(baseUrl: string): string {
  return `[mcp_servers.pictor]\nurl = "${baseUrl}/api/mcp"\nbearer_token_env_var = "PICTOR_TOKEN"`;
}

/** The shape Cursor and most other clients take for a remote MCP server. */
export function mcpJson(baseUrl: string, token: string): string {
  return JSON.stringify({ mcpServers: { pictor: { url: `${baseUrl}/api/mcp`, headers: { Authorization: `Bearer ${token}` } } } }, null, 2);
}
