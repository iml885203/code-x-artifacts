import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { Editor, type Shown, type Target } from "./controller";
import { DesignEdits, ROOM_MAP } from "./edits";
import type { ElementReport } from "./inspect";
import { FILE, key, seed } from "./test-room";

const report = (pid: string, over: Partial<ElementReport> = {}, computed: Record<string, string> = {}): ElementReport => ({
  pid,
  tag: "div",
  rect: { x: 100, y: 50, w: 200, h: 100 },
  attrs: [],
  inline: [],
  computed: { position: "absolute", left: "100px", top: "50px", width: "200px", height: "100px", ...computed },
  ...over,
});
const box: Target = { frameId: "hero", file: FILE, pid: "box", report: report("box") };
const flow: Target = { frameId: "hero", file: FILE, pid: "h1", report: report("h1", {}, { position: "static", left: "auto", top: "auto" }) };

function setup() {
  const doc = new Y.Doc();
  seed(doc);
  doc.getMap(ROOM_MAP).set(key("box", "s|left"), " 100px");
  doc.getMap(ROOM_MAP).set(key("box", "s|top"), "50px");
  const edits = new DesignEdits(doc);
  const apply = vi.fn<Shown["apply"]>(() => Promise.resolve());
  const editor = new Editor(edits, { apply });
  const room = (pid: string, prop: string) => doc.getMap(ROOM_MAP).get(key(pid, `s|${prop}`));
  const shown = () => apply.mock.calls.flatMap(([, styles]) => styles as [string, string, string | null][]);
  return { edits, editor, apply, room, shown };
}

describe("dragging an element", () => {
  it("is one history step however many times the pointer moved, and Undo then Redo returns the same values", () => {
    const { edits, editor, room } = setup();
    const { drag } = editor.move([box]);
    if (!drag) throw new Error("an absolute element could not be dragged");
    for (let i = 1; i <= 40; i++) drag.update(i * 3, i * 1.5);
    expect(room("box", "left")).toBe(" 100px"); // nothing reaches the room until the pointer is released
    expect(edits.steps().undo).toBe(0);
    expect(drag.end()).toBe(true);
    expect(edits.steps().undo).toBe(1);
    const moved = [room("box", "left"), room("box", "top")];
    expect(moved).toEqual([" 220px", "110px"]);
    edits.undo();
    expect([room("box", "left"), room("box", "top")]).toEqual([" 100px", "50px"]);
    edits.redo();
    expect([room("box", "left"), room("box", "top")]).toEqual(moved);
  });

  it("measures in the frame's pixels: the caller divides by the zoom, so 30 by 15 on screen at 50% is 60 by 30", () => {
    const { editor, room } = setup();
    const { drag } = editor.move([box]);
    drag?.update(30 / 0.5, 15 / 0.5);
    drag?.end();
    expect([room("box", "left"), room("box", "top")]).toEqual([" 160px", "80px"]);
  });

  it("shows the element at each position in its frame while dragging, and says where its box is", () => {
    const { editor, shown } = setup();
    const { drag } = editor.move([box]);
    const rects = drag?.update(20, 10);
    expect(rects).toEqual({ "hero/box": { x: 120, y: 60, w: 200, h: 100 } });
    expect(shown()).toEqual([["box", "left", "120px"], ["box", "top", "60px"]]);
    drag?.update(0, 0);
    expect(shown().slice(2)).toEqual([["box", "left", "100px"], ["box", "top", "50px"]]);
  });

  it("is no step and puts the frame back when it ends where it began", () => {
    const { edits, editor, shown } = setup();
    const { drag } = editor.move([box]);
    drag?.update(40, 40);
    drag?.update(0.2, -0.3);
    expect(drag?.end()).toBe(false);
    expect(edits.steps().undo).toBe(0);
    expect(shown()).toContainEqual(["box", "left", " 100px"]);
  });

  it("puts the frame back as the room has it when it is given up", () => {
    const { editor, shown, room } = setup();
    const { drag } = editor.move([box]);
    drag?.update(40, 40);
    drag?.cancel();
    expect(shown().slice(-4)).toEqual([["box", "left", " 100px"], ["box", "top", "50px"], ["box", "width", "30px"], ["box", "height", null]]);
    expect(room("box", "left")).toBe(" 100px");
  });

  it("moves everything selected together, in one step", () => {
    const { edits, editor, room } = setup();
    const other: Target = { frameId: "hero", file: FILE, pid: "para", report: report("para", {}, { left: "5px", top: "6px" }) };
    const { drag } = editor.move([box, other]);
    drag?.update(10, 10);
    drag?.end();
    expect(edits.steps().undo).toBe(1);
    expect([room("box", "left"), room("para", "left"), room("para", "top")]).toEqual([" 110px", "15px", "16px"]);
  });

  it("leaves an element that is in the flow where it is, and says which those were", () => {
    const { editor, room } = setup();
    expect(editor.move([flow])).toEqual({ drag: null, still: [flow] });
    const both = editor.move([box, flow]);
    both.drag?.update(10, 0);
    both.drag?.end();
    expect(both.still).toEqual([flow]);
    expect(room("h1", "left")).toBeUndefined();
    expect(room("box", "left")).toBe(" 110px");
  });

  it("does not drag an element whose place is not in pixels", () => {
    const { editor } = setup();
    const odd: Target = { ...box, report: report("box", {}, { left: "auto" }) };
    expect(editor.move([odd]).drag).toBeNull();
  });
});

describe("resizing an element", () => {
  it("writes the size and the place a held corner changes, as one step, with the box following the pointer", () => {
    const { edits, editor, room } = setup();
    const drag = editor.resize(box, "nw");
    const rects = drag.update(20, 10, false);
    expect(rects).toEqual({ "hero/box": { x: 120, y: 60, w: 180, h: 90 } });
    drag.update(30, 15, false);
    expect(drag.end()).toBe(true);
    expect(edits.steps().undo).toBe(1);
    expect([room("box", "left"), room("box", "top"), room("box", "width"), room("box", "height")]).toEqual([" 130px", "65px", "170px", "85px"]);
  });

  it("keeps the ratio when told to", () => {
    const { editor, room } = setup();
    const drag = editor.resize(box, "se", );
    drag.update(40, 0, true);
    drag.end();
    expect([room("box", "width"), room("box", "height")]).toEqual(["240px", "120px"]);
  });

  it("writes only the size of an element in the flow", () => {
    const { editor, room } = setup();
    const drag = editor.resize(flow, "se");
    drag.update(20, 10, false);
    drag.end();
    expect([room("h1", "left"), room("h1", "top"), room("h1", "width"), room("h1", "height")]).toEqual([undefined, undefined, "220px", "110px"]);
  });
});

describe("deleting", () => {
  it("removes what is selected in one step and Undo brings it back", () => {
    const { edits, editor } = setup();
    expect(editor.remove([box, flow])).toBe(true);
    expect([...edits.frame(FILE).alive].sort()).toEqual(["b"]);
    expect(edits.steps().undo).toBe(1);
    edits.undo();
    expect([...edits.frame(FILE).alive].sort()).toEqual(["b", "box", "h1", "para"]);
  });
});
