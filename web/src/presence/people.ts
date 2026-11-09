import { colorOf, paletteClass } from "../editor/colors";

/** One person, or one agent, that has the artifact open. */
export interface Person {
  /**
   * Who it is, whatever the source calls them: the account of a person, "<account>/<client>" of an agent.
   * The server gives the same id in the room's awareness and in the viewers list; the name is only what to show.
   */
  id: string;
  name: string;
  agent: boolean;
  /** You: shown first, and as "You". */
  self: boolean;
  /**
   * A signed-out browser on a public link, named by a random nickname it keeps (`guest:<digest>` ids): a browser,
   * not a verified person. Only the server's list has guests; a live room never does.
   */
  guest?: boolean;
  /** A palette name (`c0`…`c7`) when the source of the entry chose one; the name's own colour otherwise. */
  color?: unknown;
}

/** The colour class of a person: the one their source gave, else one picked from their name. */
export function colorClassOf(p: Person): string {
  return paletteClass(p.color ?? colorOf(p.name));
}

/** What a person is, for telling entries apart: a guest is never the person or agent with the same id. */
const kindOf = (p: Person) => (p.guest === true ? "guest" : p.agent ? "agent" : "person");

/** One entry per person, agent or guest, whatever its name. */
export const keyOf = (p: Person) => `${kindOf(p)}:${p.id}`;

/** The name to show for one person two sources both know: not the bare account when the other source knows more, since a source that only knows the account falls back to it. */
function betterName(a: Person, b: Person): string {
  return a.name === a.id && b.name !== b.id ? b.name : a.name;
}

/**
 * The people of several sources as one list: the same account (and kind, and for an agent the same client)
 * is one entry wherever it is found, never matched by the name shown, which each source may spell its own way.
 * It is you if either source says so; the first source's colour wins, and the name is the first one that is
 * more than the account. You come first, then people and agents by name, then guests by name.
 */
export function mergePeople(...sources: Person[][]): Person[] {
  const merged = new Map<string, Person>();
  for (const p of sources.flat()) {
    const seen = merged.get(keyOf(p));
    merged.set(keyOf(p), seen ? { ...seen, name: betterName(seen, p), self: seen.self || p.self, color: seen.color ?? p.color } : p);
  }
  return [...merged.values()].sort((a, b) => Number(b.self) - Number(a.self) || Number(a.guest === true) - Number(b.guest === true) || a.name.localeCompare(b.name));
}

/** The letters in an avatar: the first character of the name, whatever the script. */
export function initialOf(name: string): string {
  return (Array.from(name.trim())[0] ?? "?").toUpperCase();
}
