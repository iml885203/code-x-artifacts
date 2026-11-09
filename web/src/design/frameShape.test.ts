// @vitest-environment node
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { ROOM_MAP } from "./edits";
import { LIVE_ATTRS, needsLoading, shapeOf, SHOWN_LIVE, SHOWN_ON_LOAD } from "./frameShape";
import { FILE, key, seed } from "./test-room";

const repo = join(dirname(fileURLToPath(import.meta.url)), "../../..");
const read = (path: string) => readFileSync(join(repo, path), "utf8");

// The fields of a node the server reads from the room (internal/designroom, node.set), and the two it reads as the editor's.
function roomFields(): string[] {
  const source = read("internal/designroom/designroom.go");
  const body = /func \(n \*node\) set\(field string, v any\) error \{([\s\S]*?)\n\}/.exec(source)?.[1] ?? "";
  const fields = [...body.matchAll(/case "([^"]+)":/g)].map((m) => m[1] ?? "");
  const editor = /func \(n \*node\) setEditorField\([\s\S]*?\n\}/.exec(source)?.[0] ?? "";
  if (editor.includes('field == "hidden"')) fields.push("hidden");
  if (editor.includes('strings.CutPrefix(field, "s|")')) fields.push("s|");
  if (editor.includes('strings.CutPrefix(field, "override|")')) fields.push("override|");
  return fields.sort();
}

describe("what an open frame is told and what only loading it shows", () => {
  it("covers every field of a node the room holds, each exactly once", () => {
    const fields = roomFields();
    expect(fields.length).toBeGreaterThan(8);
    expect([...Object.keys(SHOWN_LIVE), ...SHOWN_ON_LOAD].sort()).toEqual(fields);
  });

  it("counts as live only what the frame's inspector applies, and the image attributes it is allowed to change", () => {
    const apply = read("internal/kind/inspector/apply.js");
    for (const part of new Set(Object.values(SHOWN_LIVE))) expect(apply).toContain(`m.${part}`);
    const valid = read("internal/kind/inspector/valid.js");
    const allowed = /\[("[^"]+"(?:,\s*"[^"]+")*)\]\.includes\(x\[1\]\)/.exec(valid)?.[1] ?? "";
    expect(JSON.parse(`[${allowed}]`)).toEqual([...LIVE_ATTRS]);
  });

  it("asks to load the frame again for its head, an attribute, a tag or a style it does not own, and not for what it is told", () => {
    const doc = new Y.Doc();
    seed(doc);
    const map = doc.getMap(ROOM_MAP);
    const before = shapeOf(map, FILE);
    const after = (change: (m: Y.Map<unknown>) => void) => {
      const d = new Y.Doc();
      Y.applyUpdate(d, Y.encodeStateAsUpdate(doc));
      const m = d.getMap(ROOM_MAP);
      d.transact(() => {
        change(m);
      });
      return needsLoading(before, shapeOf(m, FILE));
    };
    expect(after((m) => m.set(`${FILE}|shell|pre`, "<head><style>p{}</style></head>"))).toBe(true);
    expect(after((m) => m.set(key("box", "attrs"), JSON.stringify([["", "class", "card"]])))).toBe(true);
    expect(after((m) => m.set(key("h1", "tag"), "h2"))).toBe(true);
    expect(after((m) => m.set(key("box", "rest"), "transition:all 1s"))).toBe(true);

    expect(after((m) => m.set(key("box", "s|left"), "99px"))).toBe(false);
    expect(after((m) => m.set(key("box", "hidden"), "1"))).toBe(false);
    expect(after((m) => m.set(key("h1", "order"), "A"))).toBe(false);
    expect(after((m) => m.set(key("box", "attrs"), JSON.stringify([["", "data-name", "Card"]])))).toBe(false);
    expect(after((m) => { (m.get(key("~t1", "text")) as Y.Text).insert(0, "Oh "); })).toBe(false);
    expect(after((m) => {
      m.set(key("new", "kind"), "el");
      m.set(key("new", "tag"), "section");
      m.set(key("new", "parent"), "b");
      m.set(key("new", "order"), "z");
    })).toBe(false);
    expect(after((m) => { for (const k of [...m.keys()]) if (k.startsWith(key("h1", ""))) m.delete(k); })).toBe(false);
  });

  it("asks for a text that is not the whole of an element, which the frame is not told", () => {
    const doc = new Y.Doc();
    seed(doc);
    const map = doc.getMap(ROOM_MAP);
    doc.transact(() => {
      map.set(key("em", "kind"), "el");
      map.set(key("em", "tag"), "div");
      map.set(key("em", "parent"), "para");
      map.set(key("em", "order"), "Z");
    });
    const before = shapeOf(map, FILE);
    (map.get(key("~t1", "text")) as Y.Text).insert(0, "Oh ");
    expect(needsLoading(before, shapeOf(map, FILE))).toBe(true);
  });
});
