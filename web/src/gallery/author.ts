import type { Artifact } from "../api";

/** Names are presentation; identity comparisons always use author. */
export function authorName(a: Artifact): string {
  const name = a.authorDisplayName?.trim();
  return name === undefined || name === "" ? a.author : name;
}
