// @vitest-environment node
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { DesignEdits, ROOM_MAP, frameState } from "./edits";
import { FILE, key, seed } from "./test-room";

/** Two people's replicas of one room, and a relay that makes them equal. */
function pair() {
  const a = new Y.Doc();
  seed(a);
  const b = new Y.Doc();
  Y.applyUpdate(b, Y.encodeStateAsUpdate(a));
  const relay = () => {
    Y.applyUpdate(b, Y.encodeStateAsUpdate(a, Y.encodeStateVector(b)));
    Y.applyUpdate(a, Y.encodeStateAsUpdate(b, Y.encodeStateVector(a)));
  };
  return { a, b, ea: new DesignEdits(a), eb: new DesignEdits(b), relay };
}

const styleOf = (e: DesignEdits, pid: string) => Object.fromEntries(e.frame(FILE).styles.get(pid) ?? []);

describe("what the room holds of a frame", () => {
  it("lists the elements and the owned style each has, and the body", () => {
    const doc = new Y.Doc();
    seed(doc);
    const s = frameState(doc.getMap(ROOM_MAP), FILE);
    expect(s.root).toBe("b");
    expect([...s.alive].sort()).toEqual(["b", "box", "h1", "para"]);
    expect(Object.fromEntries(s.styles.get("box") ?? [])).toEqual({ left: " 10px", top: "20px", width: "30px" });
    expect(s.styles.has("h1")).toBe(false);
  });

  it("ignores what is left of an element someone deleted (an entry with no kind)", () => {
    const doc = new Y.Doc();
    seed(doc);
    doc.getMap(ROOM_MAP).set(key("ghost", "s|left"), "5px");
    const s = frameState(doc.getMap(ROOM_MAP), FILE);
    expect(s.alive.has("ghost")).toBe(false);
    expect(s.styles.has("ghost")).toBe(false);
  });
});

describe("one history step per user action", () => {
  it("writes the owned entries the room reads, keeping the spacing of the value it replaces", () => {
    const { a, ea } = pair();
    ea.apply([{ file: FILE, pid: "box", style: { left: "110px", top: "25px" } }]);
    const map = a.getMap(ROOM_MAP);
    expect(map.get(key("box", "s|left"))).toBe(" 110px");
    expect(map.get(key("box", "s|top"))).toBe("25px");
    expect(map.get(key("box", "s|width"))).toBe("30px");
  });

  it("keeps an !important the value had", () => {
    const doc = new Y.Doc();
    seed(doc);
    doc.getMap(ROOM_MAP).set(key("h1", "s|width"), " 220px !important");
    new DesignEdits(doc).apply([{ file: FILE, pid: "h1", style: { width: "300px" } }]);
    expect(doc.getMap(ROOM_MAP).get(key("h1", "s|width"))).toBe(" 300px !important");
  });

  it("is one step for a change of any number of properties and elements, and Undo then Redo returns the same values", () => {
    const { ea } = pair();
    const before = { box: styleOf(ea, "box"), h1: styleOf(ea, "h1") };
    expect(ea.apply([{ file: FILE, pid: "box", style: { left: "110px", top: "25px" } }, { file: FILE, pid: "h1", style: { left: "1px", top: "2px" } }])).toBe(true);
    const after = { box: styleOf(ea, "box"), h1: styleOf(ea, "h1") };
    expect(ea.steps()).toEqual({ undo: 1, redo: 0 });
    expect(ea.undo()).toBe(true);
    expect({ box: styleOf(ea, "box"), h1: styleOf(ea, "h1") }).toEqual(before);
    expect(ea.steps()).toEqual({ undo: 0, redo: 1 });
    expect(ea.redo()).toBe(true);
    expect({ box: styleOf(ea, "box"), h1: styleOf(ea, "h1") }).toEqual(after);
    expect(ea.undo()).toBe(true);
    expect(ea.undo()).toBe(false);
  });

  it("makes no step for a change that changes nothing", () => {
    const { ea } = pair();
    expect(ea.apply([{ file: FILE, pid: "box", style: { top: "20px" } }])).toBe(false);
    expect(ea.apply([])).toBe(false);
    expect(ea.steps().undo).toBe(0);
  });

  it("keeps two actions made one after the other as two steps", () => {
    const { ea } = pair();
    ea.apply([{ file: FILE, pid: "box", style: { left: "1px" } }]);
    ea.apply([{ file: FILE, pid: "box", style: { left: "2px" } }]);
    expect(ea.steps().undo).toBe(2);
    ea.undo();
    expect(styleOf(ea, "box").left).toBe(" 1px");
  });

  it("takes an element that has no entry back to having none when the first change is undone", () => {
    const { ea, a } = pair();
    ea.apply([{ file: FILE, pid: "h1", style: { left: "5px", top: "6px" } }]);
    expect(styleOf(ea, "h1")).toEqual({ left: "5px", top: "6px" });
    ea.undo();
    expect(styleOf(ea, "h1")).toEqual({});
    expect(a.getMap(ROOM_MAP).has(key("h1", "s|left"))).toBe(false);
  });
});

describe("Undo is your own", () => {
  it("does not undo another person's change to the same property of the same element", () => {
    const { ea, eb, relay } = pair();
    ea.apply([{ file: FILE, pid: "box", style: { left: "100px" } }]);
    relay();
    eb.apply([{ file: FILE, pid: "box", style: { left: "200px" } }]);
    relay();
    expect(styleOf(ea, "box").left).toBe(" 200px");
    ea.undo();
    relay();
    expect(styleOf(ea, "box").left).toBe(" 200px");
    expect(styleOf(eb, "box").left).toBe(" 200px");
  });

  it("undoes only what you changed of an element the other person changed in another property", () => {
    const { ea, eb, relay } = pair();
    ea.apply([{ file: FILE, pid: "box", style: { left: "100px" } }]);
    eb.apply([{ file: FILE, pid: "box", style: { top: "90px" } }]);
    relay();
    ea.undo();
    relay();
    expect(styleOf(ea, "box")).toEqual({ left: " 10px", top: "90px", width: "30px" });
    expect(styleOf(eb, "box")).toEqual(styleOf(ea, "box"));
    // The other person has nothing of yours to undo, and has their own step.
    expect(eb.steps().undo).toBe(1);
    eb.undo();
    relay();
    expect(styleOf(ea, "box").top).toBe("20px");
  });

  it("has no history of its own for what another person did before you came or while you were here", () => {
    const { ea, eb, relay } = pair();
    eb.apply([{ file: FILE, pid: "box", style: { left: "300px" } }]);
    relay();
    expect(ea.steps()).toEqual({ undo: 0, redo: 0 });
    expect(ea.undo()).toBe(false);
    expect(styleOf(ea, "box").left).toBe(" 300px");
  });
});

describe("deleting elements", () => {
  it("removes each element and everything inside it, in one step", () => {
    const { ea, a } = pair();
    expect(ea.apply([{ file: FILE, pid: "box", remove: true }])).toBe(true);
    const keys = [...a.getMap(ROOM_MAP).keys()];
    expect(keys.filter((k) => k.startsWith(`${FILE}|n|box|`) || k.startsWith(`${FILE}|n|para|`) || k.startsWith(`${FILE}|n|~t1|`))).toEqual([]);
    expect(ea.frame(FILE).alive.has("h1")).toBe(true);
    expect(ea.steps().undo).toBe(1);
  });

  it("is undone as a whole, and the paragraph's text comes back with it", () => {
    const { ea, a } = pair();
    ea.apply([{ file: FILE, pid: "box", remove: true }]);
    expect(ea.undo()).toBe(true);
    const s = ea.frame(FILE);
    expect([...s.alive].sort()).toEqual(["b", "box", "h1", "para"]);
    expect(Object.fromEntries(s.styles.get("box") ?? [])).toEqual({ left: " 10px", top: "20px", width: "30px" });
    const text = a.getMap(ROOM_MAP).get(key("~t1", "text"));
    if (!(text instanceof Y.Text)) throw new Error("the text did not come back as a text");
    expect(text.toJSON()).toBe("Hello");
    expect(a.getMap(ROOM_MAP).get(key("para", "parent"))).toBe("box");
    expect(ea.redo()).toBe(true);
    expect(ea.frame(FILE).alive.has("box")).toBe(false);
  });

  it("never deletes the body, or what is not there", () => {
    const { ea } = pair();
    expect(ea.apply([{ file: FILE, pid: "b", remove: true }, { file: FILE, pid: "nothing", remove: true }])).toBe(false);
    expect(ea.frame(FILE).alive.has("b")).toBe(true);
  });
});

describe("watching the room", () => {
  it("tells of a change you made, one another person made and an undo, and stops when told to", () => {
    const { ea, eb, relay } = pair();
    let calls = 0;
    const stop = ea.onChange(() => calls++);
    ea.apply([{ file: FILE, pid: "box", style: { left: "1px" } }]);
    expect(calls).toBe(1);
    eb.apply([{ file: FILE, pid: "h1", style: { top: "1px" } }]);
    relay();
    expect(calls).toBe(2);
    ea.undo();
    expect(calls).toBe(3);
    stop();
    ea.redo();
    expect(calls).toBe(3);
  });
});

/** Everything the room holds, as a plain object, to see what a change touched. */
const snapshot = (doc: Y.Doc) => Object.fromEntries([...doc.getMap(ROOM_MAP).entries()].map(([k, v]) => [k, v instanceof Y.Text ? v.toJSON() : v]));
const changedKeys = (before: Record<string, unknown>, after: Record<string, unknown>) =>
  [...new Set([...Object.keys(before), ...Object.keys(after)])].filter((k) => before[k] !== after[k]).sort();

describe("renaming, hiding and reordering", () => {
  it("a rename changes the element's data-name and no other entry, and Undo takes it back", () => {
    const { a, ea } = pair();
    a.getMap(ROOM_MAP).set(key("box", "attrs"), JSON.stringify([["", "id", "box"], ["", "class", "card"]]));
    const before = snapshot(a);
    expect(ea.rename(FILE, "box", "  Hero card ")).toBe(true);
    const after = snapshot(a);
    expect(changedKeys(before, after)).toEqual([key("box", "attrs")]);
    expect(JSON.parse(after[key("box", "attrs")] as string)).toEqual([["", "id", "box"], ["", "class", "card"], ["", "data-name", "Hero card"]]);
    expect(ea.rename(FILE, "box", "Hero card")).toBe(false);
    expect(ea.rename(FILE, "box", "")).toBe(true);
    expect(JSON.parse(snapshot(a)[key("box", "attrs")] as string)).toEqual([["", "id", "box"], ["", "class", "card"]]);
    ea.undo();
    expect(JSON.parse(snapshot(a)[key("box", "attrs")] as string)).toContainEqual(["", "data-name", "Hero card"]);
    ea.undo();
    expect(snapshot(a)).toEqual(before);
  });

  it("hiding is a state of its own and keeps the element, showing takes it away again, each one step", () => {
    const { a, ea } = pair();
    const before = snapshot(a);
    expect(ea.setHidden(FILE, "h1", true)).toBe(true);
    expect(changedKeys(before, snapshot(a))).toEqual([key("h1", "hidden")]);
    expect(snapshot(a)[key("h1", "hidden")]).toBe("1");
    expect(ea.frame(FILE).hidden.get("h1")).toBe("1");
    expect(ea.frame(FILE).alive.has("h1")).toBe(true);
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(true);
    expect(ea.setHidden(FILE, "h1", true)).toBe(false);
    expect(ea.setHidden(FILE, "h1", false)).toBe(true);
    expect(snapshot(a)).toEqual(before);
    expect(ea.steps().undo).toBe(2);
    ea.undo();
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(true);
  });

  it("hiding and showing an element keeps the display the panel wrote", () => {
    const { a, ea } = pair();
    a.getMap(ROOM_MAP).set(key("h1", "s|display"), "flex");
    const before = snapshot(a);
    ea.setHidden(FILE, "h1", true);
    expect(snapshot(a)[key("h1", "s|display")]).toBe("flex");
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(true);
    ea.setHidden(FILE, "h1", false);
    expect(snapshot(a)).toEqual(before);
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(false);
  });

  it("shows an element an older room hid through its display entry by taking that entry away", () => {
    const { a, ea } = pair();
    a.getMap(ROOM_MAP).set(key("h1", "s|display"), "none");
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(true);
    expect(ea.setHidden(FILE, "h1", false)).toBe(true);
    expect(snapshot(a)[key("h1", "s|display")]).toBeUndefined();
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(false);
  });

  it("shows again an element whose own markup hides it with the state 0, and never hides the body", () => {
    const { a, ea } = pair();
    a.getMap(ROOM_MAP).set(key("h1", "rest"), "display:none");
    expect(ea.setHidden(FILE, "h1", false)).toBe(true);
    expect(ea.tree(FILE).nodes.get("h1")?.hidden).toBe(false);
    expect(snapshot(a)[key("h1", "hidden")]).toBe("0");
    expect(ea.setHidden(FILE, "b", true)).toBe(false);
    expect(ea.rename(FILE, "b", "page")).toBe(false);
  });

  it("a move writes the element's parent and order and nothing else, in one step", () => {
    const { a, ea } = pair();
    const before = snapshot(a);
    expect(ea.move(FILE, "h1", "b", "box")).toBe(true);
    expect(changedKeys(before, snapshot(a))).toEqual([key("h1", "order")]);
    expect(ea.tree(FILE).nodes.get("b")?.children).toEqual(["h1", "box"]);
    expect(ea.move(FILE, "h1", "box", null)).toBe(true);
    expect(ea.tree(FILE).nodes.get("box")?.children).toEqual(["para", "h1"]);
    expect(ea.tree(FILE).nodes.get("h1")?.parent).toBe("box");
    ea.undo();
    expect(ea.tree(FILE).nodes.get("b")?.children).toEqual(["h1", "box"]);
    ea.undo();
    expect(snapshot(a)).toEqual(before);
  });

  it("refuses to put an element inside itself or something inside it, and leaves the room as it was", () => {
    const { a, ea } = pair();
    const before = snapshot(a);
    expect(ea.move(FILE, "box", "box", null)).toBe(false);
    expect(ea.move(FILE, "box", "para", null)).toBe(false);
    expect(ea.move(FILE, "b", "h1", null)).toBe(false);
    expect(snapshot(a)).toEqual(before);
    expect(ea.steps().undo).toBe(0);
  });

  it("a move that leaves the element where it is is not a step", () => {
    const { ea } = pair();
    expect(ea.move(FILE, "box", "b", "h1")).toBe(false);
    expect(ea.move(FILE, "h1", "b", null)).toBe(false);
    expect(ea.steps().undo).toBe(0);
  });

  it("while one person reorders an element another changes its style, and both stay", () => {
    const { a, b, ea, eb, relay } = pair();
    expect(ea.move(FILE, "box", "b", null)).toBe(true);
    eb.apply([{ file: FILE, pid: "box", style: { left: "99px" } }]);
    relay();
    for (const e of [ea, eb]) {
      expect(e.tree(FILE).nodes.get("b")?.children).toEqual(["h1", "box"]);
      expect(styleOf(e, "box").left).toBe(" 99px");
    }
    expect(snapshot(a)).toEqual(snapshot(b));
  });
});
