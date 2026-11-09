// What other people have selected, as awareness carries it (docs/realtime.md):
// each browser publishes `selection` next to the `user` the server rewrites with
// the authenticated name. Whatever arrives is another browser's word, so it is
// read defensively and a name is only ever taken from `user`.
import type { Awareness } from "y-protocols/awareness";
import type { Selected } from "./useSelection";

/** The most elements one person's selection puts into awareness; the server refuses a message over 4 KB. */
export const MAX_PUBLISHED = 50;
const MAX_ID = 64;

type Field = Record<string, string[]>;

/** The selection as published: the selected pids of each frame. Null when nothing is selected, so the field goes away. */
export function selectionField(selected: Selected[]): Field | null {
  const field: Field = {};
  for (const s of selected.slice(0, MAX_PUBLISHED)) (field[s.frameId] ??= []).push(s.pid);
  return Object.keys(field).length === 0 ? null : field;
}

export function publishSelection(awareness: Awareness, selected: Selected[]): void {
  awareness.setLocalStateField("selection", selectionField(selected));
}

export interface Peer {
  client: number;
  name: string;
  color: unknown;
  agent: boolean;
  selection: Selected[];
  /** Set on what an agent changed (presenceAgent.ts): the server's time of that announcement, which tells one announcement from the next. */
  at?: number;
}

const isId = (v: unknown): v is string => typeof v === "string" && v !== "" && v.length <= MAX_ID;

export function selectionOf(raw: unknown): Selected[] {
  if (typeof raw !== "object" || raw === null) return [];
  const out: Selected[] = [];
  for (const [frameId, pids] of Object.entries(raw)) {
    if (!isId(frameId) || !Array.isArray(pids)) continue;
    for (const pid of pids as unknown[]) if (isId(pid) && out.length < MAX_PUBLISHED) out.push({ frameId, pid });
  }
  return out;
}

/** The other people in the room with the name the server gave them, and what they have selected. A state without a name is nobody. */
export function peersOf(awareness: Awareness, self: number): Peer[] {
  const peers: Peer[] = [];
  for (const [client, state] of awareness.getStates()) {
    if (client === self) continue;
    const { user, selection } = state as { user?: { name?: unknown; color?: unknown; agent?: unknown }; selection?: unknown };
    if (typeof user?.name !== "string" || user.name === "") continue;
    peers.push({ client, name: user.name, color: user.color, agent: user.agent === true, selection: selectionOf(selection) });
  }
  return peers.sort((a, b) => a.client - b.client);
}
