// @vitest-environment node
import { describe, expect, it } from "vitest";
import { place, shows } from "./bubblePlace";

const bubble = { width: 200, height: 36 };
const area = { width: 600, height: 2000, scrollTop: 0, viewHeight: 500 };

describe("the bubble over a selection", () => {
  it("sits above the first line of the selection, centred on it", () => {
    expect(place({ x: 300, top: 100, bottom: 120 }, bubble, area)).toEqual({ left: 200, top: 56 });
  });

  it("stays inside the area on the left and the right, the page's edges", () => {
    expect(place({ x: 5, top: 100, bottom: 120 }, bubble, area).left).toBe(0);
    expect(place({ x: 598, top: 100, bottom: 120 }, bubble, area).left).toBe(400);
  });

  it("goes below the selection when there is no room above it in view", () => {
    expect(place({ x: 300, top: 20, bottom: 40 }, bubble, area).top).toBe(48);
    expect(place({ x: 300, top: 1010, bottom: 1030 }, bubble, { ...area, scrollTop: 1000 }).top).toBe(1038);
  });

  it("stays above when the room above is exactly enough, and never outside an area narrower than itself", () => {
    expect(place({ x: 300, top: 44, bottom: 60 }, bubble, area).top).toBe(0);
    expect(place({ x: 10, top: 100, bottom: 120 }, bubble, { ...area, width: 50 }).left).toBe(0);
  });

  it("shows only for selected words and not while typing", () => {
    expect(shows(true, false)).toBe(true);
    expect(shows(false, false)).toBe(false);
    expect(shows(true, true)).toBe(false);
  });
});
