import type { Awareness } from "y-protocols/awareness";
import type { Person } from "./people";

/** Who a live room says is here, from awareness: one entry per person (or agent), under the name the server gave them, you first. */
export function peopleOfAwareness(awareness: Awareness, self: number): Person[] {
  const people: Person[] = [];
  const seen = new Set<string>();
  for (const [client, state] of awareness.getStates()) {
    const user = (state as { user?: { id?: unknown; name?: unknown; color?: unknown; agent?: unknown } }).user;
    if (typeof user?.name !== "string" || user.name === "") continue;
    const agent = user.agent === true;
    // The server says who the entry is; a name stands in for an id only from a server that did not.
    const id = typeof user.id === "string" && user.id !== "" ? user.id : user.name;
    const key = `${agent ? "agent" : "person"}:${id}`;
    if (client !== self && seen.has(key)) continue;
    seen.add(key);
    people.push({ id, name: user.name, color: user.color, agent, self: client === self });
  }
  return people.sort((a, b) => Number(b.self) - Number(a.self) || a.name.localeCompare(b.name));
}
