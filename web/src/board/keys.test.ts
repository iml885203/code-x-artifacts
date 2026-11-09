import { describe, expect, it } from "vitest";
import { nextZoom, zoomedAt, zoomKeyOf } from "./keys";

const key = (k: string, over: Partial<{ metaKey: boolean; ctrlKey: boolean; altKey: boolean; target: EventTarget | null }> = {}) => ({
  key: k, metaKey: false, ctrlKey: false, altKey: false, target: document.body, ...over,
});

describe("a board's zoom keys", () => {
  it("are the Design canvas's: + and = in, - out, 0 fits, 1 is 100%", () => {
    expect(["+", "=", "-", "0", "1"].map((k) => zoomKeyOf(key(k)))).toEqual(["in", "in", "out", "fit", "actual"]);
  });

  it("leave everything else, a key with a modifier and typing in a field to the editor", () => {
    expect(zoomKeyOf(key("r"))).toBeNull();
    expect(zoomKeyOf(key("v"))).toBeNull();
    expect(zoomKeyOf(key("=", { ctrlKey: true }))).toBeNull();
    expect(zoomKeyOf(key("1", { metaKey: true }))).toBeNull();
    expect(zoomKeyOf(key("0", { target: document.createElement("textarea") }))).toBeNull();
  });

  it("step through the Design canvas's steps, and 1 is 100% from anywhere", () => {
    expect(nextZoom("in", 1)).toBe(1.25);
    expect(nextZoom("out", 1)).toBe(0.75);
    expect(nextZoom("in", 2)).toBe(2);
    expect(nextZoom("actual", 0.4)).toBe(1);
  });

  it("keep the point at the middle of the screen where it is", () => {
    const view = { zoom: 1, scrollX: -100, scrollY: 50, width: 800, height: 600 };
    const next = zoomedAt(view, 2);
    const middle = (v: { zoom: number; scrollX: number; scrollY: number }) => [400 / v.zoom - v.scrollX, 300 / v.zoom - v.scrollY];
    expect(middle(next)).toEqual(middle(view));
    expect(next.zoom).toBe(2);
  });
});
