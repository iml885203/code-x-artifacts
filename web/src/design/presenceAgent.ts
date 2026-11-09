// An agent the server shows in a design's room (docs/realtime.md): it is not a
// connection and has no selection, so the elements it just changed are marked
// with its name for a few seconds, then the mark fades. The name is only ever
// the one in `user`, which the server wrote; `touched` is {frame: [pid…]}, in
// the same shape as a selection.
import type { Awareness } from "y-protocols/awareness";
import { selectionOf, type Peer } from "./presence";

/** How long the marks of an announcement stay, in milliseconds; the stylesheet fades them over the same time. */
export const markMs = 4000;

/** What each announced agent says it changed. Anything that is not an agent's state, or has no name, is not one. */
export function announcements(awareness: Awareness): Peer[] {
  const out: Peer[] = [];
  for (const [client, state] of awareness.getStates()) {
    const { user, touched, at } = state as { user?: { name?: unknown; agent?: unknown }; touched?: unknown; at?: unknown };
    if (user?.agent !== true || typeof user.name !== "string" || user.name === "" || typeof at !== "number") continue;
    const selection = selectionOf(touched);
    if (selection.length > 0) out.push({ client, name: user.name, color: undefined, agent: true, selection, at });
  }
  return out;
}

/**
 * The announcements that are still fresh, by when this browser first saw them:
 * the server's clock and this one are not the same, so a mark is timed here.
 * `seen` is the memory between calls; what is no longer announced is forgotten.
 */
export function freshMarks(all: Peer[], seen: Map<string, number>, now: number): Peer[] {
  const present = new Set<string>();
  const fresh: Peer[] = [];
  for (const a of all) {
    const key = `${String(a.client)}:${String(a.at)}`;
    present.add(key);
    if (!seen.has(key)) seen.set(key, now);
    if (now - (seen.get(key) ?? now) < markMs) fresh.push(a);
  }
  for (const key of seen.keys()) if (!present.has(key)) seen.delete(key);
  return fresh;
}
