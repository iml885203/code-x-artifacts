import { expect, it } from "vitest";
import * as Y from "yjs";
import { DesignEdits } from "../edits";
import { FILE, seed } from "../test-room";
import { addLine } from "./lines";

it("persists a directed arrow as ordinary SVG and restores exactly one gesture with Undo", () => {
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  const ref = addLine(edits, FILE, { x: 100, y: 120 }, { x: 20, y: 40 }, true, "Arrow");
  expect(ref).not.toBeNull();
  if (!ref) throw new Error("Expected a persisted arrow");
  const entries = [...edits.map.entries()];
  const paths = entries.filter(([key]) => key.endsWith("|attrs"));
  expect(paths.some(([, value]) => String(value).includes(" M") || String(value).includes("M"))).toBe(true);
  expect(edits.frame(FILE).styles.get(ref.pid)?.get("left")).toBe("18px");
  expect(edits.steps().undo).toBe(1);
  edits.undo();
  expect(edits.map.has(`${FILE}|n|${ref.pid}|tag`)).toBe(false);
  edits.redo();
  expect([...edits.map.entries()]).toEqual(entries);
});

it("does not write a cancelled-size line or non-finite coordinates", () => {
  const doc = new Y.Doc(); seed(doc); const edits = new DesignEdits(doc);
  expect(addLine(edits, FILE, { x: 1, y: 1 }, { x: 1, y: 1 }, false, "Line")).toBeNull();
  expect(addLine(edits, FILE, { x: NaN, y: 1 }, { x: 3, y: 4 }, false, "Line")).toBeNull();
  expect(edits.steps().undo).toBe(0);
});

it("keeps short arrowheads inside their SVG bounds and refuses unrepresentable coordinates", () => {
  const doc = new Y.Doc(); seed(doc); const edits = new DesignEdits(doc);
  expect(addLine(edits, FILE, { x: 1e8, y: 0 }, { x: 0, y: 0 }, true, "Arrow")).toBeNull();
  const ref = addLine(edits, FILE, { x: 100, y: 100 }, { x: 101, y: 100 }, true, "Arrow");
  expect(ref).not.toBeNull();
  if (!ref) throw new Error("Expected a persisted arrow");
  expect(parseFloat(edits.frame(FILE).styles.get(ref.pid)?.get("left") ?? "")).toBeLessThan(93);
});
