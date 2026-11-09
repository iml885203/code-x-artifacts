// @vitest-environment node
import { describe, expect, it } from "vitest";
import { moved, resized, startOf, type Box } from "./geometry";

const box: Box = { left: 100, top: 50, width: 200, height: 100 };

describe("moving", () => {
  it("adds the distance, in the frame's pixels, to where the element is", () => {
    expect(moved(box, 60, 30)).toEqual({ left: "160px", top: "80px" });
    expect(moved(box, -120.4, 0.6)).toEqual({ left: "-20px", top: "51px" });
  });

  it("is nothing when the distance rounds to nothing", () => {
    expect(moved(box, 0.3, -0.4)).toEqual({});
  });
});

describe("resizing a positioned element", () => {
  it("moves the corner that is held and keeps the opposite one where it is", () => {
    expect(resized(box, "se", 20, 10, false, true)).toEqual({ width: "220px", height: "110px" });
    expect(resized(box, "nw", 20, 10, false, true)).toEqual({ left: "120px", top: "60px", width: "180px", height: "90px" });
    expect(resized(box, "ne", 20, -10, false, true)).toEqual({ top: "40px", width: "220px", height: "110px" });
    expect(resized(box, "sw", -20, 10, false, true)).toEqual({ left: "80px", width: "220px", height: "110px" });
  });

  it("never makes it smaller than a pixel, and the opposite corner still stays", () => {
    expect(resized(box, "nw", 500, 500, false, true)).toEqual({ left: "299px", top: "149px", width: "1px", height: "1px" });
    expect(resized(box, "se", -500, -500, false, true)).toEqual({ width: "1px", height: "1px" });
  });

  it("keeps the ratio with Shift: the bigger change leads, from every corner", () => {
    expect(resized(box, "se", 40, 5, true, true)).toEqual({ width: "240px", height: "120px" });
    expect(resized(box, "se", 4, 30, true, true)).toEqual({ width: "260px", height: "130px" });
    expect(resized(box, "nw", -40, -5, true, true)).toEqual({ left: "60px", top: "30px", width: "240px", height: "120px" });
    expect(resized(box, "ne", 40, 0, true, true)).toEqual({ top: "30px", width: "240px", height: "120px" });
  });
});

describe("resizing an element that is in the flow", () => {
  it("writes the size only: its place is the flow's", () => {
    expect(resized(box, "se", 20, 10, false, false)).toEqual({ width: "220px", height: "110px" });
  });
});

describe("where an element starts, from what the frame reported", () => {
  const computed = { left: "48px", top: "300.5px", width: "220px", height: "auto" };
  const rect = { x: 48, y: 300.5, w: 220, h: 31.25 };

  it("reads the computed pixels, and falls back on the rectangle for a size that is auto", () => {
    expect(startOf(computed, rect)).toEqual({ left: 48, top: 300.5, width: 220, height: 31.25 });
  });

  it("is nothing for a place that is not in pixels", () => {
    expect(startOf({ ...computed, left: "auto" }, rect)).toBeNull();
  });
});
