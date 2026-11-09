// @vitest-environment node
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import type { Target } from "../controller";
import { DesignEdits, ROOM_MAP } from "../edits";
import { FILE, seed } from "../test-room";
import { DesignOps } from "./DesignOps";

function setup() {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  const ops = new DesignOps(edits);
  const target = (pid: string, absolute = true): Target => ({
    frameId: "hero",
    file: FILE,
    pid,
    report: { pid, tag: "div", rect: { x: 10, y: 20, w: 30, h: 40 }, attrs: [], inline: [], computed: absolute ? { position: "absolute", left: "10px", top: "20px", width: "30px", height: "40px" } : { position: "static" } },
  });
  const room = () => JSON.stringify([...doc.getMap(ROOM_MAP).entries()].map(([k, v]) => [k, v instanceof Y.Text ? v.toJSON() : v]).sort());
  return { edits, ops, target, room };
}

// Every action is one step of the person's own history: one Undo puts the room back exactly, one Redo does it again.
describe("what the keys and buttons do is one step of Undo each", () => {
  const actions: [string, (s: ReturnType<typeof setup>) => unknown][] = [
    ["duplicate", ({ ops, target }) => ops.duplicate([target("box"), target("h1", false)])],
    ["paste", ({ ops, target }) => (ops.copy([target("box")]), ops.paste(FILE))],
    ["group", ({ ops, target }) => ops.group([target("box"), target("h1")])],
    ["bring forward", ({ ops, target }) => ops.restack([target("box")], "forward")],
    ["send to back", ({ ops, target }) => ops.restack([target("h1")], "back")],
    ["insert a text box", ({ ops }) => ops.insertShape(FILE, "text", { left: 5, top: 6 }, "Text")],
    ["insert a rectangle", ({ ops }) => ops.insertShape(FILE, "rect", { left: 5, top: 6 }, "Text")],
    ["insert an image", ({ ops }) => ops.insertImage(FILE, { src: "/files/a/b.png", alt: "b", width: 10, height: 10 }, { left: 0, top: 0 })],
  ];
  it.each(actions)("%s", (_name, act) => {
    const s = setup();
    const before = s.room();
    act(s);
    const after = s.room();
    expect(after).not.toBe(before);
    expect(s.edits.steps()).toEqual({ undo: 1, redo: 0 });
    expect(s.edits.undo()).toBe(true);
    expect(s.room()).toBe(before);
    expect(s.edits.redo()).toBe(true);
    expect(s.room()).toBe(after);
  });

  it("ungroup, after a group, is a second step and its Undo gives the group back", () => {
    const s = setup();
    const made = s.ops.group([s.target("box"), s.target("h1")]);
    const grouped = s.room();
    const [wrapper] = (made as { done: { pid: string }[] }).done;
    s.ops.ungroup([s.target(wrapper?.pid ?? "")]);
    expect(s.edits.steps().undo).toBe(2);
    s.edits.undo();
    expect(s.room()).toBe(grouped);
  });

  it("an action that is refused or changes nothing is not a step", () => {
    const s = setup();
    expect(s.ops.group([s.target("para"), s.target("h1")])).toEqual({ refused: "apart" });
    expect(s.ops.ungroup([s.target("h1")])).toEqual({ refused: "nothing" });
    expect(s.ops.restack([s.target("h1")], "front")).toBe(false);
    expect(s.ops.paste(FILE)).toEqual({ refused: "nothing" });
    expect(s.ops.copy([s.target("b")])).toBe(false);
    expect(s.edits.steps()).toEqual({ undo: 0, redo: 0 });
  });

  it("pastes what was copied even after the original is deleted, and again further to the side", () => {
    const s = setup();
    s.ops.copy([s.target("box")]);
    s.edits.apply([{ file: FILE, pid: "box", remove: true }]);
    expect(s.ops.canPaste).toBe(true);
    expect(s.ops.copiedFrom).toBe(FILE);
    const first = (s.ops.paste(FILE) as { done: { pid: string }[] }).done[0]?.pid ?? "";
    const second = (s.ops.paste(FILE) as { done: { pid: string }[] }).done[0]?.pid ?? "";
    const map = s.edits.map;
    expect(map.get(`${FILE}|n|${first}|s|left`)).toBe("26px");
    expect(map.get(`${FILE}|n|${second}|s|left`)).toBe("42px");
  });
});
