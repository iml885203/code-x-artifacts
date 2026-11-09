import { describe, expect, it } from "vitest";
import type { VersionRow } from "../api";
import { groupVersions } from "./versionGroups";

const at = (minutes: number) => new Date(Date.UTC(2026, 9, 20, 10, 0, 0) + minutes * 60_000).toISOString();
const row = (version: number, minutes: number, who: string[], over: Partial<VersionRow> = {}): VersionRow => ({
  version,
  title: "Plan",
  author: who[0] ?? "",
  authors: who,
  createdAt: at(minutes),
  ...over,
});
/** The versions of each group, as numbers. */
const shape = (rows: VersionRow[]) => groupVersions(rows).map((g) => g.rows.map((r) => r.version));

describe("groupVersions", () => {
  it("puts same-author versions within ten minutes into one group shown by its newest", () => {
    const groups = groupVersions([row(4, 30, ["alice"]), row(3, 25, ["alice"]), row(2, 21, ["alice"]), row(1, 0, ["alice"])]);
    expect(groups.map((g) => [g.newest.version, g.rows.map((r) => r.version)])).toEqual([[4, [4, 3, 2]], [1, [1]]]);
  });

  it.each([
    ["different authors", [row(2, 5, ["bob"]), row(1, 4, ["alice"])], [[2], [1]]],
    ["the same people in another order", [row(2, 5, ["alice", "bob"]), row(1, 4, ["bob", "alice"])], [[2, 1]]],
    ["a person with company and the person alone", [row(2, 5, ["alice", "bob"]), row(1, 4, ["alice"])], [[2], [1]]],
    ["exactly ten minutes apart", [row(2, 10, ["alice"]), row(1, 0, ["alice"])], [[2, 1]]],
    ["more than ten minutes apart", [row(2, 11, ["alice"]), row(1, 0, ["alice"])], [[2], [1]]],
    ["an agent's version and the same person's own", [row(2, 5, ["alice"], { agents: [{ by: "alice", client: "Claude Code" }] }), row(1, 4, ["alice"])], [[2], [1]]],
    ["the same agent twice", [row(2, 5, ["alice"], { agents: [{ by: "alice", client: "Claude Code" }] }), row(1, 4, ["alice"], { agents: [{ by: "alice", client: "Claude Code" }] })], [[2, 1]]],
    ["a restore and the version before it", [row(2, 5, ["alice"], { restoredFrom: 1 }), row(1, 4, ["alice"])], [[2], [1]]],
    ["a version and the restore before it", [row(2, 5, ["alice"]), row(1, 4, ["alice"], { restoredFrom: 0 }), row(0, 3, ["alice"], { restoredFrom: 1 })], [[2, 1], [0]]],
    ["a row with no authors list falls back to its author", [row(2, 5, ["alice"], { authors: [] }), { version: 1, title: "Plan", author: "alice", createdAt: at(4) }], [[2, 1]]],
  ])("%s", (_name, rows, want) => {
    expect(shape(rows)).toEqual(want);
  });

  it("measures the window from the group's newest, so a long steady stretch is split instead of swallowed whole", () => {
    // One a minute for 25 minutes: each is within ten minutes of its neighbour, but a group spans ten.
    const rows = Array.from({ length: 26 }, (_, i) => row(26 - i, 25 - i, ["alice"]));
    const groups = groupVersions(rows);
    expect(groups.length).toBe(3);
    expect(groups.map((g) => g.rows.length)).toEqual([11, 11, 4]);
  });

  it("never drops or repeats a version: every one is in exactly one group, in order", () => {
    const rows = [row(7, 90, ["a"]), row(6, 85, ["a"]), row(5, 80, ["b"]), row(4, 40, ["a"]), row(3, 39, ["a"], { restoredFrom: 1 }), row(2, 38, ["a"]), row(1, 0, ["a"])];
    expect(groupVersions(rows).flatMap((g) => g.rows.map((r) => r.version))).toEqual([7, 6, 5, 4, 3, 2, 1]);
  });

  it("gives no groups for no versions", () => {
    expect(groupVersions([])).toEqual([]);
  });
});
