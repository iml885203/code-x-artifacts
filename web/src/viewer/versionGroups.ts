import type { VersionRow } from "../api";

/** How far back from a group's newest version another still belongs to it. */
export const groupWindowMs = 10 * 60 * 1000;

/** Versions the list shows as one entry: the newest, and the older ones made by the same people shortly before. */
export interface VersionGroup {
  /** The version the entry shows. */
  newest: VersionRow;
  /** Every version of the group, newest first, `newest` included: none is ever left out. */
  rows: VersionRow[];
}

/** The people behind a version: everyone who edited it and every agent, as a comparable key. */
function peopleOf(row: VersionRow): string {
  const people = new Set(row.authors?.length ? row.authors : [row.author]);
  const agents = new Set((row.agents ?? []).map((a) => `${a.by}\u0000${a.client}`));
  return JSON.stringify([[...people].sort(), [...agents].sort()]);
}

/**
 * Collapses a history, newest first, for reading: consecutive versions made by
 * the same people within `groupWindowMs` of the group's newest become one entry.
 * It only changes what the list shows, never what is stored or reachable. A
 * restore stands alone: it is a choice someone made, not an idle save.
 */
export function groupVersions(rows: VersionRow[]): VersionGroup[] {
  const groups: VersionGroup[] = [];
  for (const row of rows) {
    const group = groups.at(-1);
    if (group && joins(group, row)) group.rows.push(row);
    else groups.push({ newest: row, rows: [row] });
  }
  return groups;
}

function joins(group: VersionGroup, row: VersionRow): boolean {
  if (group.newest.restoredFrom || row.restoredFrom) return false;
  if (peopleOf(group.newest) !== peopleOf(row)) return false;
  return Date.parse(group.newest.createdAt) - Date.parse(row.createdAt) <= groupWindowMs;
}
