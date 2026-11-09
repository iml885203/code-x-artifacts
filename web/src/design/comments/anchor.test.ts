// @vitest-environment node
import { describe, expect, it } from "vitest";
import { anchorAt, readAnchor, resolveAnchor } from "./anchor";

const frame = { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 };
const anchor = { type: "design" as const, frameId: "hero", pid: "heading", x: 100, y: 80 };

describe("design comment source anchors", () => {
  it("keeps source identity without a floating pin when its element disappears and reconnects the restored identity", () => {
    const before = JSON.stringify(anchor);
    expect(resolveAnchor(anchor, frame, { x: 30, y: 40, w: 200, h: 60 })).toEqual({ x: 230, y: 40, fallback: false });
    expect(resolveAnchor(anchor, frame)).toBeNull();
    expect(JSON.stringify(anchor)).toBe(before);
    expect(readAnchor(anchor)?.pid).toBe("heading");
    expect(resolveAnchor(anchor, frame, { x: 30, y: 40, w: 200, h: 60 })).toEqual({ x: 230, y: 40, fallback: false });
  });

  it("clamps an intentional position to the resized frame and never resolves in another frame", () => {
    expect(resolveAnchor({ type: "design", frameId: "hero", x: 700, y: 500 }, { ...frame, w: 200, h: 100 })).toEqual({ x: 200, y: 100, fallback: false });
    expect(resolveAnchor(anchor, { ...frame, id: "other" })).toBeNull();
  });

  it.each([undefined, {}, { block: "doc", quote: "words" }, { ...anchor, x: -1 }, { ...anchor, x: Infinity }, { ...anchor, y: NaN }, { ...anchor, y: 8193 }, { ...anchor, pid: "" }, { ...anchor, frameId: "x".repeat(257) }, { ...anchor, type: "other" }])("ignores a malformed or different kind of anchor: %j", (raw) => {
    expect(readAnchor(raw)).toBeNull();
  });

  it("creates a frame or source-element anchor without serializing rendered markup", () => {
    expect(anchorAt(frame, { x: 900, y: -10 })).toEqual({ type: "design", frameId: "hero", name: "Hero", x: 800, y: 0 });
    expect(anchorAt(frame, { x: 100, y: 80 }, "heading", "Title")).toEqual({ ...anchor, name: "Title" });
  });

  it("creates a valid frame position when source geometry or an identifier cannot be represented", () => {
    const made = anchorAt(frame, { x: NaN, y: Infinity }, "x".repeat(257));
    expect(readAnchor(made)).toEqual({ type: "design", frameId: "hero", name: "Hero", x: 0, y: 0 });
  });

  it("ignores unsafe element geometry while preserving intentional frame positions", () => {
    expect(resolveAnchor(anchor, frame, { x: NaN, y: 0, w: 10, h: 10 })).toBeNull();
    expect(resolveAnchor(anchor, frame, { x: 0, y: 0, w: -10, h: 10 })).toBeNull();
    expect(resolveAnchor({ type: "design", frameId: "hero", x: 100, y: 80 }, frame)?.fallback).toBe(false);
  });
});
