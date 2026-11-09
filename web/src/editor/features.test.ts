// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Hooks } from "./feature";
import { groups } from "./feature";
import { features } from "./features";

const hooks: Hooks = { onUpdate: () => undefined, onLink: () => undefined, onImages: () => undefined, say: () => "", comments: { threads: () => [], active: () => null, open: () => undefined, compose: () => undefined }, hint: () => "", mention: { people: () => Promise.resolve({ people: [], complete: true }), mentioned: () => undefined, words: () => ({ label: "", none: "", incomplete: "" }) } };

describe("the editor's features", () => {
  it("each have an id of their own, and so does each tool and row", () => {
    const unique = (ids: string[]) => {
      expect(new Set(ids).size).toBe(ids.length);
    };
    unique(features.map((f) => f.id));
    unique(features.flatMap((f) => f.insertions?.map((item) => item.id) ?? []));
    unique(features.flatMap((f) => f.tools?.map((t) => t.id) ?? []));
    unique(features.flatMap((f) => f.rows?.map((r) => r.id) ?? []));
  });

  it("put every tool in a toolbar group that exists", () => {
    const known = groups.map((g) => g.id as string);
    for (const tool of features.flatMap((f) => f.tools ?? [])) expect(known, tool.id).toContain(tool.group);
  });

  it("bind no key twice, but Tab: two features on one key would shadow each other by their order", () => {
    const keys = features.flatMap((f) => Object.keys(f.keymap?.(hooks) ?? {}));
    // A table cell holds one paragraph and a list item holds no table, so the caret is never in both: the order of the two does not matter.
    expect(keys.filter((k, i) => keys.indexOf(k) !== i && k !== "Tab" && k !== "Shift-Tab")).toEqual([]);
  });
});
