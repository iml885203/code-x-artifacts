// How the library is arranged: the groups of a list of artifacts under each
// "group by" choice, and the two choices a person keeps (how to group, grid or
// list). Pure rules apart from the storage, which tolerates being unavailable.
import type { Artifact } from "../api";
import { authorName } from "./author";
import { displayedKinds, displayOf, reservedKinds, type DisplayedKind } from "../kinds/kinds";

export type GroupBy = "none" | "date" | "owner" | "kind";
export type View = "grid" | "list";

export const groupChoices: readonly GroupBy[] = ["none", "date", "owner", "kind"];
export const views: readonly View[] = ["grid", "list"];

export const isGroupBy = (v: string): v is GroupBy => (groupChoices as readonly string[]).includes(v);
export const isView = (v: string): v is View => (views as readonly string[]).includes(v);

export type DateKey = "today" | "yesterday" | "week" | "earlier";
export type KindKey = DisplayedKind;

/** The kinds in the order the groups come: the four people see, then any whose look is only reserved. */
export const kindOrder: readonly KindKey[] = [...displayedKinds, ...reservedKinds];
const dateOrder: readonly DateKey[] = ["today", "yesterday", "week", "earlier"];

export type Group =
  | { by: "none"; key: "all"; items: Artifact[] }
  | { by: "date"; key: DateKey; items: Artifact[] }
  | { by: "owner"; key: string; owner: string; items: Artifact[] }
  | { by: "kind"; key: KindKey; items: Artifact[] };

const slotOf = (a: Artifact): KindKey => displayOf(a.kind);

/** Whole local calendar days from `then` to `now`; a time in the future counts as today. */
function daysAgo(then: Date, now: Date): number {
  const day = (d: Date) => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate());
  return Math.max(0, Math.round((day(now) - day(then)) / 86_400_000));
}

function dateKeyOf(iso: string, now: Date): DateKey {
  const days = daysAgo(new Date(iso), now);
  if (days === 0) return "today";
  if (days === 1) return "yesterday";
  return days < 7 ? "week" : "earlier";
}

/** Pinned ones first; each half keeps the order it came in (newest first from the server). */
function pinnedFirst(items: readonly Artifact[]): Artifact[] {
  return [...items.filter((a) => a.pinned), ...items.filter((a) => !a.pinned)];
}

/**
 * The groups of `list` under `by`, each with its pinned items first. Every item
 * is in exactly one group and a group with nothing in it is not returned.
 * Owners come with the viewer's own first, then the others by name.
 */
export function groupArtifacts(list: readonly Artifact[], by: GroupBy, now: Date, user: string): Group[] {
  switch (by) {
    case "none":
      return list.length === 0 ? [] : [{ by, key: "all", items: pinnedFirst(list) }];
    case "date":
      return dateOrder.flatMap((key) => {
        const items = list.filter((a) => dateKeyOf(a.updatedAt, now) === key);
        return items.length === 0 ? [] : [{ by, key, items: pinnedFirst(items) }];
      });
    case "kind":
      return kindOrder.flatMap((key) => {
        const items = list.filter((a) => slotOf(a) === key);
        return items.length === 0 ? [] : [{ by, key, items: pinnedFirst(items) }];
      });
    case "owner": {
      const names = new Map(list.map((a) => [a.author, authorName(a)]));
      const owners = [...names.keys()].sort((a, b) => Number(b === user) - Number(a === user) || (names.get(a) ?? a).localeCompare(names.get(b) ?? b) || a.localeCompare(b));
      return owners.map((owner) => ({ by, key: `owner:${owner}`, owner, items: pinnedFirst(list.filter((a) => a.author === owner)) }));
    }
  }
}

// What a person chose is theirs, so the key carries their name: two people on one browser keep their own.
const storageKey = (user: string, what: "group" | "view") => `pictor.library.${what}.${user}`;

function read<T extends string>(user: string, what: "group" | "view", ok: (v: string) => v is T, fallback: T): T {
  try {
    const v = localStorage.getItem(storageKey(user, what));
    return v !== null && ok(v) ? v : fallback;
  } catch {
    // Storage can be blocked or throw (a private window): the default is a fine answer.
    return fallback;
  }
}

function write(user: string, what: "group" | "view", value: string) {
  try {
    localStorage.setItem(storageKey(user, what), value);
  } catch {
    // Not remembered, still chosen.
  }
}

export const readGroupBy = (user: string): GroupBy => read(user, "group", isGroupBy, "none");
export const readView = (user: string): View => read(user, "view", isView, "grid");
export const writeGroupBy = (user: string, by: GroupBy) => {
  write(user, "group", by);
};
export const writeView = (user: string, view: View) => {
  write(user, "view", view);
};
