import { call } from "../api";

export interface ActivityToken {
  id: string;
  name: string;
  revoked: boolean;
  lastUsed?: string;
  lastConnected?: string;
}
export interface AgentActivityItem {
  id: string;
  tokenID: string;
  tokenLabel: string;
  revoked: boolean;
  transport: "mcp" | "http";
  client: string;
  clientVersion?: string;
  occurredAt: string;
  operation: "connect" | "publish" | "restore" | "doc_edit" | "board_edit";
  target?: { id: string; title: string; url: string };
}
export interface AgentActivityPage {
  items: AgentActivityItem[];
  tokens: ActivityToken[];
  nextCursor?: string;
}

export async function readAgentActivity(tokenID: string, cursor: string, signal: AbortSignal): Promise<AgentActivityPage> {
  const query = new URLSearchParams({ limit: "50" });
  if (tokenID !== "") query.set("tokenID", tokenID);
  if (cursor !== "") query.set("cursor", cursor);
  const response = await call("GET", `/api/agent-activity?${query.toString()}`, undefined, signal);
  const page = await response.json() as AgentActivityPage;
  if (!Array.isArray(page.items) || !Array.isArray(page.tokens)) throw new Error("invalid activity page");
  return page;
}

/** Links are current service-authorized destinations, never historical or external URLs. */
export function activityTarget(item: AgentActivityItem) {
  const target = item.target;
  return target && /^[a-zA-Z0-9_-]+$/.test(target.id) && target.url === `/a/${target.id}` ? target : undefined;
}
