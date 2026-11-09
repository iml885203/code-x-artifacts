// @vitest-environment node
import { expect, it } from "vitest";
import * as Y from "yjs";
import { DesignEdits, ROOM_MAP } from "./edits";
import { FILE, key, seed } from "./test-room";

function room() {
  const doc = new Y.Doc(); seed(doc);
  const map = doc.getMap(ROOM_MAP);
  map.set(key("box", "rest"), "background-color:background-color;background:red !important;transform:rotate(2deg)");
  map.set(key("box", "s|background-color"), "blue !important");
  return { doc, map, edits: new DesignEdits(doc) };
}

it("writes a covered fill to its independent override and preserves the authored slot through Reset and Undo", () => {
  const { map, edits } = room();
  const rest = map.get(key("box", "rest"));
  edits.apply([{ file: FILE, pid: "box", style: { "background-color": "green !important" } }]);
  expect(map.get(key("box", "s|background-color"))).toBe("blue !important");
  expect(map.get(key("box", "override|background-color"))).toBe("green !important");
  expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("green !important");
  edits.apply([{ file: FILE, pid: "box", style: { "background-color": "" } }]);
  expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("");
  expect(map.get(key("box", "s|background-color"))).toBe("blue !important");
  expect(map.get(key("box", "rest"))).toBe(rest);
  edits.undo(); expect(map.get(key("box", "override|background-color"))).toBe("green !important");
  edits.undo(); expect(map.has(key("box", "override|background-color"))).toBe(false);
  expect(map.get(key("box", "s|background-color"))).toBe("blue !important");
});

it("keeps one important suffix for explicit priority and inherits effective priority on subsequent edits", () => {
  const { map, edits } = room();
  edits.apply([{ file: FILE, pid: "box", style: { "background-color": "green !important" } }]);
  expect(map.get(key("box", "override|background-color"))).toBe("green !important");
  edits.apply([{ file: FILE, pid: "box", style: { "background-color": "yellow" } }]);
  expect(map.get(key("box", "override|background-color"))).toBe("yellow !important");
});

it("merges concurrent first overrides without sharing a baseline transaction and leaves later peer paint on own Undo", () => {
  const alice = room(); const bobDoc = new Y.Doc();
  Y.applyUpdate(bobDoc, Y.encodeStateAsUpdate(alice.doc));
  const bob = new DesignEdits(bobDoc);
  alice.edits.apply([{ file: FILE, pid: "box", style: { "background-color": "green" } }]);
  bob.apply([{ file: FILE, pid: "box", style: { "background-image": "none !important" } }]);
  Y.applyUpdate(bobDoc, Y.encodeStateAsUpdate(alice.doc));
  Y.applyUpdate(alice.doc, Y.encodeStateAsUpdate(bobDoc));
  expect(alice.edits.frame(FILE).styles.get("box")?.get("background-image")).toBe("none !important");
  bob.apply([{ file: FILE, pid: "box", style: { "background-color": "yellow" } }]);
  Y.applyUpdate(alice.doc, Y.encodeStateAsUpdate(bobDoc));
  alice.edits.undo();
  expect(alice.edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("yellow !important");
  expect(alice.map.get(key("box", "s|background-color"))).toBe("blue !important");
});

it.each([
  ["background-color", "background", true], ["background-color", "background", false],
  ["border-top-color", "border", true], ["border-top-color", "border", false],
] as const)("inherits priority for %s from %s only when the effective cascade is important (%s)", (prop, shorthand, important) => {
  const { map, edits } = room();
  map.set(key("box", `s|${prop}`), "blue");
  map.set(key("box", "rest"), `${prop}:${prop};${shorthand}:${shorthand === "border" ? "2px solid " : ""}red${important ? "!important" : ""}`);
  edits.apply([{ file: FILE, pid: "box", style: { [prop]: "green" } }]);
  expect(map.get(key("box", `override|${prop}`))).toBe(`green${important ? " !important" : ""}`);
  expect(map.get(key("box", `s|${prop}`))).toBe("blue");
  edits.undo();
  expect(map.has(key("box", `override|${prop}`))).toBe(false);
});
