import { afterEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact } from "../test-support";
import { groupArtifacts, readGroupBy, readView, writeGroupBy, writeView, type GroupBy } from "./group";

// Wednesday 21 October 2026, early afternoon in the machine's own time zone.
const now = new Date(2026, 9, 21, 13, 0, 0);
const at = (daysBack: number, hour = 9) => new Date(2026, 9, 21 - daysBack, hour, 0, 0).toISOString();

const item = (title: string, over: Partial<Artifact> = {}) => artifact({ title, author: "alice", updatedAt: at(0), ...over });
const titles = (groups: ReturnType<typeof groupArtifacts>) => groups.map((g) => [g.key, g.items.map((a) => a.title)]);

describe("grouping the library", () => {
  const list = [
    item("web", { kind: "html", updatedAt: at(0), author: "bob" }),
    item("react page", { kind: "react", updatedAt: at(1), author: "alice" }),
    item("doc", { kind: "doc", updatedAt: at(3), author: "carol" }),
    item("slides", { kind: "slides", updatedAt: at(7), author: "alice" }),
    item("design", { kind: "design", updatedAt: at(40), author: "bob" }),
  ];

  it.each<GroupBy>(["none", "date", "owner", "kind"])("puts every item in exactly one group when grouped by %s", (by) => {
    const groups = groupArtifacts(list, by, now, "alice");
    const all = groups.flatMap((g) => g.items.map((a) => a.title));
    expect([...all].sort()).toEqual(list.map((a) => a.title).sort());
  });

  it("keeps one group, in the order given, when not grouped, and none for an empty library", () => {
    expect(titles(groupArtifacts(list, "none", now, "alice"))).toEqual([["all", list.map((a) => a.title)]]);
    expect(groupArtifacts([], "none", now, "alice")).toEqual([]);
  });

  it("groups by edit date: today, yesterday, the rest of the week, earlier", () => {
    expect(titles(groupArtifacts(list, "date", now, "alice"))).toEqual([
      ["today", ["web"]],
      ["yesterday", ["react page"]],
      ["week", ["doc"]],
      ["earlier", ["slides", "design"]],
    ]);
  });

  it.each([
    ["just after midnight today", new Date(2026, 9, 21, 0, 0, 1).toISOString(), "today"],
    ["a minute before midnight yesterday", new Date(2026, 9, 20, 23, 59, 0).toISOString(), "yesterday"],
    ["six days back", at(6), "week"],
    ["seven days back", at(7), "earlier"],
    ["a time after now (a clock that is ahead)", new Date(2026, 9, 22, 8, 0, 0).toISOString(), "today"],
  ])("puts %s in %s", (_name, updatedAt, key) => {
    expect(groupArtifacts([item("x", { updatedAt })], "date", now, "alice").map((g) => g.key)).toEqual([key]);
  });

  it("groups by owner with your own first and the others by name, under their own name", () => {
    const groups = groupArtifacts(list, "owner", now, "alice");
    expect(titles(groups)).toEqual([
      ["owner:alice", ["react page", "slides"]],
      ["owner:bob", ["web", "design"]],
      ["owner:carol", ["doc"]],
    ]);
    expect(groups.map((g) => (g.by === "owner" ? g.owner : ""))).toEqual(["alice", "bob", "carol"]);
  });

  it("groups by kind in the order web, document, slides, design, and web covers html and react", () => {
    expect(titles(groupArtifacts(list, "kind", now, "alice"))).toEqual([
      ["web", ["web", "react page"]],
      ["doc", ["doc"]],
      ["slides", ["slides"]],
      ["design", ["design"]],
    ]);
  });

  it("does not draw a group with nothing in it, and puts a kind this bundle does not know with the web pages", () => {
    expect(groupArtifacts([item("odd", { kind: "mystery" })], "kind", now, "alice").map((g) => g.key)).toEqual(["web"]);
    expect(groupArtifacts([item("only today")], "date", now, "alice").map((g) => g.key)).toEqual(["today"]);
  });

  it.each<GroupBy>(["none", "date", "owner", "kind"])("puts pinned ones first inside every group when grouped by %s", (by) => {
    const pinned = [item("a"), item("b"), item("c", { pinned: true }), item("d", { pinned: true })];
    for (const g of groupArtifacts(pinned, by, now, "alice")) expect(g.items.map((a) => a.title)).toEqual(["c", "d", "a", "b"]);
  });
});

describe("the choices a person keeps", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("restores the group and the view that person chose, and not another person's", () => {
    writeGroupBy("alice", "owner");
    writeView("alice", "list");
    expect([readGroupBy("alice"), readView("alice")]).toEqual(["owner", "list"]);
    expect([readGroupBy("bob"), readView("bob")]).toEqual(["none", "grid"]);
  });

  it("falls back to no grouping and the grid for a value it does not know", () => {
    localStorage.setItem("pictor.library.group.alice", "colour");
    localStorage.setItem("pictor.library.view.alice", "wall");
    expect([readGroupBy("alice"), readView("alice")]).toEqual(["none", "grid"]);
  });

  it("falls back, and does not throw, when the storage throws", () => {
    const broken = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    };
    vi.stubGlobal("localStorage", broken);
    expect([readGroupBy("alice"), readView("alice")]).toEqual(["none", "grid"]);
    expect(() => {
      writeGroupBy("alice", "kind");
      writeView("alice", "list");
    }).not.toThrow();
  });
});
