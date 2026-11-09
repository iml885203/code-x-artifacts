import { describe, expect, it } from "vitest";
import { layoutPage, readDesign, GAP_X, MARGIN } from "./manifest";
import { clampZoom, fit, panBy, stepZoom, zoomAt } from "./view";

describe("zoom", () => {
  it("keeps the canvas point under the pointer where it is", () => {
    const before = { x: 40, y: 20, z: 1 };
    const after = zoomAt(before, 2, 300, 200);
    // The canvas point under (300, 200) is the same before and after.
    expect((300 - before.x) / before.z).toBeCloseTo((300 - after.x) / after.z);
    expect((200 - before.y) / before.z).toBeCloseTo((200 - after.y) / after.z);
    expect(after.z).toBe(2);
  });

  it("stays between 25% and 200% when asked for more or less", () => {
    expect(clampZoom(0.01)).toBe(0.25);
    expect(clampZoom(9)).toBe(2);
    expect(zoomAt({ x: 0, y: 0, z: 1 }, 50, 0, 0).z).toBe(2);
  });

  it("steps through 25, 50, 75, 100, 125, 150 and 200 and stops at the ends", () => {
    expect(stepZoom(1, 1)).toBe(1.25);
    expect(stepZoom(1, -1)).toBe(0.75);
    expect(stepZoom(2, 1)).toBe(2);
    expect(stepZoom(0.25, -1)).toBe(0.25);
    // From between steps it goes to the next one in that direction.
    expect(stepZoom(0.6, 1)).toBe(0.75);
    expect(stepZoom(0.6, -1)).toBe(0.5);
  });
});

describe("fit", () => {
  it("shows a big page whole and centred, and does not enlarge a small one past 100%", () => {
    const big = fit(4000, 2000, 1000, 800);
    expect(big.z).toBeCloseTo(0.25);
    expect(big.x + (4000 * big.z) / 2).toBeCloseTo(500);
    expect(big.y + (2000 * big.z) / 2).toBeCloseTo(400);
    expect(fit(200, 100, 1000, 800).z).toBe(1);
  });

  it("keeps a page that is as tall as the window out of the band at the top, which is for the bars", () => {
    const tall = fit(1000, 2000, 2000, 800, 80);
    expect(tall.z).toBeCloseTo(0.36);
    expect(tall.y).toBe(80);
    expect(tall.y + 2000 * tall.z).toBeCloseTo(800);
    // A page with room to spare is still centred.
    expect(fit(400, 200, 1000, 800, 80).y).toBeCloseTo(300);
  });

  it("may go below the smallest step for a page too big for it", () => {
    expect(fit(40000, 40000, 800, 600).z).toBeLessThan(0.25);
  });
});

describe("pan", () => {
  it("moves the page by the drag", () => {
    expect(panBy({ x: 10, y: 10, z: 1 }, -4, 6)).toEqual({ x: 6, y: 16, z: 1 });
  });
});

const design = {
  version: 1,
  pages: [
    {
      id: "home",
      name: "Home",
      rows: [
        [
          { id: "a", name: "A", file: "frames/a.html", w: 100, h: 50 },
          { id: "b", name: "B", file: "frames/b.html", w: 40, h: 80 },
        ],
        [{ id: "c", name: "C", file: "frames/c.html", w: 60, h: 60 }],
      ],
    },
  ],
};

describe("the layout of a page", () => {
  it("puts frames of a row side by side with their tops aligned and rows one under the other", () => {
    const page = readDesign(design)?.pages[0];
    if (!page) throw new Error("not a design");
    const l = layoutPage(page);
    const at = (id: string) => l.frames.find((p) => p.frame.id === id);
    expect(at("a")).toMatchObject({ x: MARGIN, y: MARGIN });
    expect(at("b")).toMatchObject({ x: MARGIN + 100 + GAP_X, y: MARGIN });
    expect(at("c")?.x).toBe(MARGIN);
    expect(at("c")?.y).toBeGreaterThan(MARGIN + 80);
    expect(l.width).toBe(MARGIN + 100 + GAP_X + 40 + MARGIN);
  });

  it("refuses what is not a design rather than guessing", () => {
    expect(readDesign(null)).toBeNull();
    expect(readDesign({ pages: [] })).toBeNull();
    expect(readDesign({ pages: [{ id: "p", name: "P", rows: [[{ id: "a" }]] }] })).toBeNull();
  });
});
