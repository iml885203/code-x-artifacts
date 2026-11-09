import { expect, it } from "vitest";
import * as Y from "yjs";
import { DesignEdits } from "../edits";
import { FILE, seed, key } from "../test-room";
import type { Target } from "../controller";
import { paintOf, writePaint } from "./paint";

const target: Target = { file: FILE, frameId: "hero", pid: "box", report: { pid: "box", tag: "div", attrs: [], inline: [], rect: {x:0,y:0,w:30,h:40}, computed: {"background-color":"rgb(20, 40, 60)"} } };
it("edits the current fill channels before a frame report and undoes each action independently", () => {
  const doc = new Y.Doc(); seed(doc); const edits = new DesignEdits(doc);
  writePaint(edits, [target], "fill", "#336699");
  writePaint(edits, [target], "fill", (color) => ({ ...color, a: 0.5 }));
  expect(paintOf(edits, target, "fill").value).toBe("rgba(51, 102, 153, 0.5)");
  expect(edits.map.get(key("box","s|left"))).toBe(" 10px");
  edits.undo(); expect(paintOf(edits, target, "fill").value).toBe("#336699");
  edits.undo(); expect(paintOf(edits, target, "fill").value).toBe("rgb(20, 40, 60)");
});

it("keeps No fill distinct from Reset and does not claim historical inline restoration", () => {
  const doc = new Y.Doc(); seed(doc); const edits = new DesignEdits(doc);
  writePaint(edits, [target], "fill", "none");
  expect(edits.map.get(key("box","s|background-color"))).toBe("transparent !important");
  expect(edits.map.get(key("box","s|background-image"))).toBe("none !important");
  writePaint(edits, [target], "fill", null);
  expect(edits.map.get(key("box","s|background-color"))).toBe("");
  expect(edits.map.get(key("box","s|background-image"))).toBe("");
});
