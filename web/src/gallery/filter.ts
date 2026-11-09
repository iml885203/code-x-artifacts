import type { Artifact } from "../api";

export type Filter = "all" | "mine" | "shared" | "pinned";

export const filters: readonly Filter[] = ["all", "mine", "shared", "pinned"];

export function isFilter(v: string): v is Filter {
  return (filters as readonly string[]).includes(v);
}

function inFilter(a: Artifact, filter: Filter, user: string): boolean {
  switch (filter) {
    case "all":
      return true;
    case "mine":
      return a.author === user;
    case "shared":
      return a.author !== user;
    case "pinned":
      return a.pinned;
  }
}

/**
 * The cards to show: the chosen filter, then a case-insensitive match of the
 * search over title and author. Pinned ones come first; each group keeps the
 * order the server gave (newest first).
 */
export function visibleArtifacts(list: readonly Artifact[], filter: Filter, query: string, user: string): Artifact[] {
  const matching = list.filter((a) => inFilter(a, filter, user) && matches(a, query));
  return [...matching.filter((a) => a.pinned), ...matching.filter((a) => !a.pinned)];
}

/** Where the "Recently opened" list is. */
export const recentPath = "/recent";

export function isRecentPath(path: string): boolean {
  return path.split(/[?#]/)[0] === recentPath;
}

/** The artifacts the person opened that the search matches, most recently opened first, as the server orders `?order=opened`. */
export function recentlyOpened(list: readonly Artifact[], query: string): Artifact[] {
  const opened = list.flatMap((a) => (a.openedAt !== undefined && matches(a, query) ? [{ a, at: Date.parse(a.openedAt) }] : []));
  return opened.sort((x, y) => y.at - x.at).map(({ a }) => a);
}

function matches(a: Artifact, query: string): boolean {
  return `${a.title} ${a.author}`.toLowerCase().includes(query.trim().toLowerCase());
}

/** How many artifacts each filter holds, whatever is typed in the search. */
export function countByFilter(list: readonly Artifact[], user: string): Record<Filter, number> {
  const count = (filter: Filter) => list.filter((a) => inFilter(a, filter, user)).length;
  return { all: list.length, mine: count("mine"), shared: count("shared"), pinned: count("pinned") };
}
