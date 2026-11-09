// @vitest-environment node
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { ROOM_MAP } from "./edits";
import { FILE, key, seed } from "./test-room";
import { between, elementOrder, placement, readTree } from "./tree";

const room = () => {
  const doc = new Y.Doc();
  seed(doc);
  return doc.getMap(ROOM_MAP);
};

describe("order keys", () => {
  it("are the keys internal/designroom makes for the same neighbours", () => {
    // Taken from designroom's between: a move written here must sort where the server sorts it.
    const cases: [string, string, string][] = [
      ["", "", "V"],
      ["V", "", "k"],
      ["", "V", "F"],
      ["V", "g", "a"],
      ["V", "W", "VV"],
      ["V", "V1", "V0V"],
      ["zz", "", "zzV"],
      ["A", "B", "AV"],
      ["Vz", "W", "VzV"],
      ["g", "g", "q"],
    ];
    for (const [a, b, want] of cases) expect(between(a, b)).toBe(want);
  });
});

describe("a frame's tree", () => {
  it("lists the elements in order with what a row shows, and keeps the text inside", () => {
    const tree = readTree(room(), FILE);
    expect(tree.root).toBe("b");
    expect(tree.nodes.get("b")?.children).toEqual(["box", "h1"]);
    expect(tree.nodes.get("box")?.children).toEqual(["para"]);
    expect(tree.nodes.get("para")?.children).toEqual(["~t1"]);
    expect(tree.nodes.get("~t1")).toMatchObject({ kind: "text", text: "Hello" });
    expect([...elementOrder(tree)]).toEqual([
      ["b", ["box", "h1"]],
      ["box", ["para"]],
    ]);
  });

  it("reads a name from the data-name attribute only", () => {
    const map = room();
    map.set(key("h1", "attrs"), JSON.stringify([["", "class", "t"], ["", "data-name", "Headline"], ["", "data-pid", "h1"]]));
    expect(readTree(map, FILE).nodes.get("h1")?.name).toBe("Headline");
    map.set(key("h1", "attrs"), JSON.stringify([["", "title", "data-name"]]));
    expect(readTree(map, FILE).nodes.get("h1")?.name).toBe("");
    map.set(key("h1", "attrs"), "not json");
    expect(readTree(map, FILE).nodes.get("h1")?.name).toBe("");
  });

  it("knows an element is hidden whether the room or its own markup says display:none", () => {
    const map = room();
    expect(readTree(map, FILE).nodes.get("h1")?.hidden).toBe(false);
    map.set(key("h1", "s|display"), "none");
    expect(readTree(map, FILE).nodes.get("h1")).toMatchObject({ hidden: true, authoredHidden: false });
    map.set(key("h1", "s|display"), " none !important");
    expect(readTree(map, FILE).nodes.get("h1")?.hidden).toBe(true);
    map.set(key("h1", "s|display"), "flex");
    expect(readTree(map, FILE).nodes.get("h1")?.hidden).toBe(false);
    map.delete(key("h1", "s|display"));
    map.set(key("h1", "rest"), "color:red;display:none");
    expect(readTree(map, FILE).nodes.get("h1")).toMatchObject({ hidden: true, authoredHidden: true });
    // The editor's own state wins over what the markup or a display says.
    map.set(key("h1", "hidden"), "0");
    expect(readTree(map, FILE).nodes.get("h1")?.hidden).toBe(false);
    map.set(key("h1", "rest"), "color:red");
    map.set(key("h1", "s|display"), "flex");
    map.set(key("h1", "hidden"), "1");
    expect(readTree(map, FILE).nodes.get("h1")?.hidden).toBe(true);
  });

  it("puts an element whose parent is gone at the end of the body, as the server reads it", () => {
    const map = room();
    for (const k of [...map.keys()]) if (k.startsWith(`${FILE}|n|box|`)) map.delete(k);
    const tree = readTree(map, FILE);
    expect(tree.nodes.get("b")?.children).toEqual(["h1", "para"]);
  });
});

describe("where a move puts an element", () => {
  it("is between the neighbours it goes between, in any parent", () => {
    const tree = readTree(room(), FILE);
    const first = placement(tree, "h1", "b", "box");
    expect(first?.parent).toBe("b");
    expect((first?.order ?? "") < (tree.nodes.get("box")?.order ?? "")).toBe(true);
    const into = placement(tree, "h1", "box", null);
    expect(into?.parent).toBe("box");
    expect((into?.order ?? "") > (tree.nodes.get("para")?.order ?? "")).toBe(true);
  });

  it("is refused into itself, into something inside it, for the body, for text and for a target that is not there", () => {
    const tree = readTree(room(), FILE);
    expect(placement(tree, "box", "box", null)).toBeNull();
    expect(placement(tree, "box", "para", null)).toBeNull();
    expect(placement(tree, "b", "box", null)).toBeNull();
    expect(placement(tree, "~t1", "b", null)).toBeNull();
    expect(placement(tree, "h1", "nowhere", null)).toBeNull();
    expect(placement(tree, "h1", "b", "nowhere")).toBeNull();
    expect(placement(tree, "ghost", "b", null)).toBeNull();
  });
});
