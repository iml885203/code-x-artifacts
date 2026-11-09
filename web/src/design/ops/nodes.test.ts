import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { DesignEdits, ROOM_MAP } from "../edits";
import { FILE, key, seed } from "../test-room";
import { readTree } from "../tree";
import { copyOf, duplicate, group, insertImage, insertShape, paste, restack, ungroup, type Item } from "./nodes";

// The room of test-room.ts: a body "b" holding "box" (absolute at 10,20, 30 wide; it holds the paragraph "para" with the text "Hello") and "h1".
function setup() {
  const doc = new Y.Doc();
  seed(doc);
  const map = doc.getMap(ROOM_MAP);
  const edits = new DesignEdits(doc);
  const items = (...pids: string[]): Item[] => pids.map((pid) => ({ file: FILE, pid, box: null }));
  const at = (pid: string, left: number, top: number, width = 30, height = 20): Item => ({ file: FILE, pid, box: { left, top, width, height } });
  const kids = (parent: string) => {
    const tree = readTree(map, FILE);
    return (tree.nodes.get(parent)?.children ?? []).filter((c) => tree.nodes.get(c)?.kind === "el");
  };
  const field = (pid: string, name: string) => map.get(key(pid, name));
  const body = () => kids("b");
  /** Every element's place and style, for comparing a room before and after. */
  const shape = () => {
    const tree = readTree(map, FILE);
    return [...tree.nodes.values()]
      .filter((n) => n.kind === "el")
      .map((n) => ({ pid: n.pid, parent: n.parent, at: kids(n.parent).indexOf(n.pid), style: [...map.keys()].filter((k) => k.startsWith(`${FILE}|n|${n.pid}|s|`)).map((k) => `${k}=${String(map.get(k))}`).sort() }))
      .sort((a, b) => (a.pid < b.pid ? -1 : 1));
  };
  return { doc, map, edits, items, at, kids, field, body, shape };
}

const refs = (o: unknown) => (o as { done: { pid: string }[] }).done.map((r) => r.pid);

describe("duplicating", () => {
  it("makes a copy with new ids right after the original, with its styles and everything inside it, and leaves the original as it was", () => {
    const { map, items, kids, field, body } = setup();
    const before = JSON.stringify([...map.entries()].map(([k, v]) => [k, String(v)]));
    const [copy] = refs(duplicate(map, items("box")));
    expect(copy).toBeDefined();
    expect(body()).toEqual(["box", copy, "h1"]);
    expect(field(copy ?? "", "s|left")).toBe(" 10px");
    expect(field(copy ?? "", "s|width")).toBe("30px");
    const inside = kids(copy ?? "");
    expect(inside).toHaveLength(1);
    expect(inside[0]).not.toBe("para");
    const text = kids(inside[0] ?? "");
    expect(text).toEqual([]);
    const tree = readTree(map, FILE);
    expect(tree.nodes.get(inside[0] ?? "")?.children.map((c) => tree.nodes.get(c)?.text)).toEqual(["Hello"]);
    // Nothing that was there changed.
    const after = JSON.stringify([...map.entries()].filter(([k]) => before.includes(JSON.stringify(k).slice(1, -1))).map(([k, v]) => [k, String(v)]));
    expect(JSON.parse(after)).toEqual(JSON.parse(before));
  });

  it("puts a copy of an element positioned absolutely a little to the side and below, and one in the flow in the same place", () => {
    const { map, items, at, field } = setup();
    const [moved] = refs(duplicate(map, [at("box", 10, 20)]));
    expect(field(moved ?? "", "s|left")).toBe("26px");
    expect(field(moved ?? "", "s|top")).toBe("36px");
    const [flow] = refs(duplicate(map, items("h1")));
    expect(field(flow ?? "", "s|left")).toBeUndefined();
  });

  it("copies the text itself: changing the text of the copy leaves the original's", () => {
    const { map, items, kids } = setup();
    const [copy] = refs(duplicate(map, items("box")));
    const para = kids(copy ?? "")[0] ?? "";
    const tree = readTree(map, FILE);
    const copied = tree.nodes.get(para)?.children[0] ?? "";
    (map.get(key(copied, "text")) as Y.Text).insert(0, "Oh, ");
    expect((map.get(key("~t1", "text")) as Y.Text).toJSON()).toBe("Hello");
  });

  it("copies only the outer one when an element and something inside it are both chosen, and nothing for the body or what is not there", () => {
    const { map, items, body } = setup();
    expect(refs(duplicate(map, items("box", "para")))).toHaveLength(1);
    expect(body()).toHaveLength(3);
    expect(duplicate(map, items("b", "nothere"))).toEqual({ refused: "nothing" });
  });

  it("gives the inline elements of a formatted text new ids too, so no two elements of a frame share one", () => {
    const { map, items, kids } = setup();
    const rich = new Y.Text();
    rich.applyDelta([
      { insert: "A ", attributes: {} },
      { insert: "bold", attributes: { "f:e9": '{"t":"b","d":0,"a":[["","data-pid","e9"]]}' } },
      { insert: { br: '[["","data-pid","e10"]]' } },
      { insert: { raw: '<mark data-pid="e11">m</mark>' } },
    ]);
    map.set(key("h1", "rich"), rich);
    interface Run { insert: string | { br?: string; raw?: string }; attributes?: Record<string, string> }
    const originalDelta = structuredClone(rich.toDelta() as Run[]);
    const originalEntries = [...map.entries()].map(([k, v]) => [k, v instanceof Y.Text ? structuredClone(v.toDelta() as unknown[]) : v] as const);
    const originalIDs = new Set([...readTree(map, FILE).nodes.values()].filter((n) => n.kind === "el").map((n) => n.pid).concat(["e9", "e10", "e11"]));
    // Valid fresh ids may contain an old id as a substring.
    const random = crypto.getRandomValues.bind(crypto);
    let next = 0;
    const ids = vi.spyOn(crypto, "getRandomValues").mockImplementation((bytes) => {
      if (!(bytes instanceof Uint8Array) || bytes.length !== 9) return random(bytes);
      bytes.set([0, 0, 4, 35, 0, 0, 0, 0, next++]);
      return bytes;
    });
    let copy: string | undefined;
    try {
      [copy] = refs(duplicate(map, items("h1")));
    } finally {
      ids.mockRestore();
    }
    const text = map.get(key(copy ?? "", "rich")) as Y.Text;
    expect(text).toBeInstanceOf(Y.Text);
    expect(text).not.toBe(rich);
    const runs = text.toDelta() as Run[];
    const formats = Object.entries(runs.find((r) => r.insert === "bold")?.attributes ?? {});
    expect(formats).toHaveLength(1);
    const [formatKey, formatValue] = formats[0] ?? ["", "{}"];
    const format = JSON.parse(formatValue) as { t: string; d: number; a: string[][] };
    const formatID = format.a.find((a) => a[1] === "data-pid")?.[2] ?? "";
    expect(formatKey).toBe(`f:${formatID}`);
    expect(format).toEqual({ t: "b", d: 0, a: [["", "data-pid", formatID]] });
    const br = runs[2]?.insert as { br: string };
    const brAttrs = JSON.parse(br.br) as string[][];
    const brID = brAttrs.find((a) => a[1] === "data-pid")?.[2] ?? "";
    expect(brAttrs).toEqual([["", "data-pid", brID]]);
    const raw = runs[3]?.insert as { raw: string };
    const markup = new DOMParser().parseFromString(raw.raw, "text/html");
    expect(markup.body.children).toHaveLength(1);
    const mark = markup.body.firstElementChild;
    expect(mark?.tagName).toBe("MARK");
    expect(mark?.textContent).toBe("m");
    const rawID = mark?.getAttribute("data-pid") ?? "";
    expect(mark?.getAttributeNames()).toEqual(["data-pid"]);
    const copiedIDs = [copy ?? "", formatID, brID, rawID];
    expect(copiedIDs.every((id) => id.length > 0)).toBe(true);
    expect(new Set(copiedIDs).size).toBe(copiedIDs.length);
    expect(copiedIDs.filter((id) => originalIDs.has(id))).toEqual([]);
    expect(runs).toEqual([
      { insert: "A " },
      { insert: "bold", attributes: { [`f:${formatID}`]: formatValue } },
      { insert: { br: br.br } },
      { insert: { raw: raw.raw } },
    ]);
    expect(kids(copy ?? "")).toEqual([]);
    const originalsNow = () => originalEntries.map(([k]) => {
      const v = map.get(k);
      return [k, v instanceof Y.Text ? v.toDelta() as unknown[] : v];
    });
    expect(rich.toDelta()).toEqual(originalDelta);
    expect(originalsNow()).toEqual(originalEntries);
    text.insert(0, "Copy: ");
    expect((text.toDelta() as Run[])[0]?.insert).toBe("Copy: A ");
    expect(rich.toDelta()).toEqual(originalDelta);
    expect(originalsNow()).toEqual(originalEntries);
  });
});

describe("copying and pasting", () => {
  it("pastes what was copied when it was copied, at the end of the body, further to the side each time", () => {
    const { map, items, at, field, body } = setup();
    const copied = copyOf(map, [at("box", 10, 20)]);
    map.set(key("box", "s|left"), "500px");
    const [first] = refs(paste(map, copied, FILE, 1));
    const [second] = refs(paste(map, copied, FILE, 2));
    expect(body()).toEqual(["box", "h1", first, second]);
    expect(field(first ?? "", "s|left")).toBe("26px");
    expect(field(second ?? "", "s|left")).toBe("42px");
    expect(copyOf(map, items("b"))).toEqual([]);
  });

  it("pastes into another frame of the design with new ids", () => {
    const { doc, map, items } = setup();
    const other = "frames/other.html";
    doc.transact(() => {
      map.set(`${other}|shell|root`, "ob");
      map.set(`${other}|n|ob|kind`, "el");
      map.set(`${other}|n|ob|parent`, "");
      map.set(`${other}|n|ob|order`, "");
    });
    const [made] = refs(paste(map, copyOf(map, items("h1")), other, 1));
    expect(map.get(`${other}|n|${made ?? ""}|parent`)).toBe("ob");
    expect(map.get(`${other}|n|${made ?? ""}|kind`)).toBe("el");
  });

  it("has nothing to paste into a frame that has no body", () => {
    const { map, items } = setup();
    expect(paste(map, copyOf(map, items("h1")), "frames/none.html", 1)).toEqual({ refused: "nothing" });
  });
});

describe("grouping", () => {
  it("wraps the elements in a new box where the first of them was, and Ungroup gives the same tree and styles back", () => {
    const { map, at, body, shape, kids } = setup();
    const before = shape();
    const [wrapper] = refs(group(map, [at("box", 10, 20), at("h1", 100, 50, 40, 10)]));
    expect(body()).toEqual([wrapper]);
    expect(kids(wrapper ?? "")).toEqual(["box", "h1"]);
    expect(refs(ungroup(map, [at(wrapper ?? "", 0, 0)]).valueOf())).toEqual(["box", "h1"]);
    expect(body()).toEqual(["box", "h1"]);
    // The same parents and places; what differs is the explicit offsets the group wrote for the box that was positioned.
    expect(shape().map(({ pid, parent, at: i }) => ({ pid, parent, at: i }))).toEqual(before.map(({ pid, parent, at: i }) => ({ pid, parent, at: i })));
  });

  it("puts a box over elements that are all positioned absolutely, at the place they were, and gives the elements places inside it", () => {
    const { map, at, field } = setup();
    const [wrapper] = refs(group(map, [at("box", 10, 20, 30, 40), at("h1", 100, 50, 40, 10)]));
    const w = wrapper ?? "";
    expect(field(w, "s|position")).toBe("absolute");
    expect([field(w, "s|left"), field(w, "s|top"), field(w, "s|width"), field(w, "s|height")]).toEqual(["10px", "20px", "130px", "40px"]);
    expect([field("box", "s|left"), field("box", "s|top")]).toEqual([" 0px", "0px"]);
    expect([field("h1", "s|left"), field("h1", "s|top")]).toEqual(["90px", "30px"]);
    expect([field("box", "s|position"), field("h1", "s|position")]).toEqual(["absolute", "absolute"]);
  });

  it("leaves the box without a place of its own when an element is in the flow", () => {
    const { map, at, items, field } = setup();
    const [wrapper] = refs(group(map, [at("box", 10, 20), ...items("h1")]));
    expect(field(wrapper ?? "", "s|position")).toBeUndefined();
    expect(field("box", "s|left")).toBe(" 10px");
  });

  it("gives back the places the elements had when the box is taken away", () => {
    const { map, at, field } = setup();
    const [wrapper] = refs(group(map, [at("box", 10, 20, 30, 40), at("h1", 100, 50, 40, 10)]));
    ungroup(map, [at(wrapper ?? "", 10, 20, 130, 40)]);
    expect([field("box", "s|left"), field("box", "s|top"), field("h1", "s|left"), field("h1", "s|top")]).toEqual([" 10px", "20px", "100px", "50px"]);
  });

  it("refuses elements that are not in one parent, and nothing", () => {
    const { map, items } = setup();
    expect(group(map, items("para", "h1"))).toEqual({ refused: "apart" });
    expect(group(map, items("b"))).toEqual({ refused: "nothing" });
  });

  it("takes away a box that has only white space between its elements, and refuses one with text of its own, saying why", () => {
    const { map, items, body } = setup();
    const [wrapper] = refs(group(map, items("box", "h1")));
    map.set(key("~sp", "kind"), "text");
    map.set(key("~sp", "parent"), wrapper ?? "");
    map.set(key("~sp", "order"), "z");
    map.set(key("~sp", "text"), new Y.Text(" \n "));
    expect(refs(ungroup(map, items(wrapper ?? "")))).toEqual(["box", "h1"]);
    expect(body()).toEqual(["box", "h1"]);
    expect(map.has(key("~sp", "kind"))).toBe(false);
    expect(map.has(key(wrapper ?? "", "kind"))).toBe(false);

    map.set(key("~t9", "kind"), "text");
    map.set(key("~t9", "parent"), "box");
    map.set(key("~t9", "order"), "z");
    map.set(key("~t9", "text"), new Y.Text("words"));
    expect(ungroup(map, items("box"))).toEqual({ refused: "text" });
    expect(body()).toEqual(["box", "h1"]);
    expect(ungroup(map, items("h1"))).toEqual({ refused: "nothing" });
  });
});

describe("stacking order", () => {
  const three = () => {
    const s = setup();
    s.map.set(key("c", "kind"), "el");
    s.map.set(key("c", "tag"), "div");
    s.map.set(key("c", "parent"), "b");
    s.map.set(key("c", "order"), "x");
    return s;
  };

  it.each([
    ["forward", ["box"], ["h1", "box", "c"]],
    ["forward", ["h1"], ["box", "c", "h1"]],
    ["forward", ["c"], ["box", "h1", "c"]],
    ["backward", ["c"], ["box", "c", "h1"]],
    ["backward", ["box"], ["box", "h1", "c"]],
    ["front", ["box"], ["h1", "c", "box"]],
    ["back", ["c"], ["c", "box", "h1"]],
    ["forward", ["box", "h1"], ["c", "box", "h1"]],
    ["backward", ["h1", "c"], ["h1", "c", "box"]],
    ["front", ["box", "c"], ["h1", "box", "c"]],
  ] as const)("%s of %j gives %j", (how, chosen, want) => {
    const { map, body } = three();
    const wrote = restack(map, chosen.map((pid) => ({ file: FILE, pid })), how);
    expect(body()).toEqual(want);
    const same = JSON.stringify(want) === JSON.stringify(["box", "h1", "c"]);
    expect(wrote).toBe(!same);
  });

  it("does not touch the text between elements or the elements of another parent", () => {
    const { map, kids } = three();
    restack(map, [{ file: FILE, pid: "box" }], "front");
    expect(kids("box")).toEqual(["para"]);
    expect(map.get(key("~t1", "parent"))).toBe("para");
  });
});

describe("inserting", () => {
  it("adds a text box, a rectangle and an oval at the end of the body, positioned where it was told, each on top of the last", () => {
    const { map, body, field, kids } = setup();
    const text = insertShape(map, FILE, "text", { left: 12, top: 34 }, "Text");
    const rect = insertShape(map, FILE, "rect", { left: 50, top: 60 }, "Text");
    const oval = insertShape(map, FILE, "oval", { left: 70, top: 80 }, "Text");
    expect(body()).toEqual(["box", "h1", text?.pid, rect?.pid, oval?.pid]);
    expect([field(text?.pid ?? "", "s|position"), field(text?.pid ?? "", "s|left"), field(text?.pid ?? "", "s|top"), field(text?.pid ?? "", "s|font-size")]).toEqual(["absolute", "12px", "34px", "24px"]);
    const tree = readTree(map, FILE);
    expect(tree.nodes.get(tree.nodes.get(text?.pid ?? "")?.children[0] ?? "")?.text).toBe("Text");
    expect(map.get(key(tree.nodes.get(text?.pid ?? "")?.children[0] ?? "", "text"))).toBeInstanceOf(Y.Text);
    expect([field(rect?.pid ?? "", "s|width"), field(rect?.pid ?? "", "s|height"), field(rect?.pid ?? "", "s|background-color")]).toEqual(["160px", "120px", "#d9d9d9"]);
    expect(field(rect?.pid ?? "", "s|border-radius")).toBeUndefined();
    expect(field(oval?.pid ?? "", "s|border-radius")).toBe("50%");
    expect(kids(rect?.pid ?? "")).toEqual([]);
  });

  it("adds an image with its address and alt, in the picture's proportions and no longer than 320 on a side", () => {
    const { map, field } = setup();
    const big = insertImage(map, FILE, { src: "/files/abc/photo.png", alt: "A photo", width: 1280, height: 640 }, { left: 40, top: 40 });
    const small = insertImage(map, FILE, { src: "/files/abc/dot.png", alt: "", width: 20, height: 10 }, { left: 0, top: 0 });
    expect(JSON.parse(field(big?.pid ?? "", "attrs") as string)).toEqual([["", "src", "/files/abc/photo.png"], ["", "alt", "A photo"]]);
    expect(field(big?.pid ?? "", "tag")).toBe("img");
    expect([field(big?.pid ?? "", "s|width"), field(big?.pid ?? "", "s|height")]).toEqual(["320px", "160px"]);
    expect([field(small?.pid ?? "", "s|width"), field(small?.pid ?? "", "s|height")]).toEqual(["20px", "10px"]);
  });

  it("adds nothing to a frame that is not in the room", () => {
    const { map } = setup();
    expect(insertShape(map, "frames/none.html", "rect", { left: 0, top: 0 }, "Text")).toBeNull();
  });
});
