// @vitest-environment node
import { describe, expect, it } from "vitest";
import { clampSlide, gapAt, moveTarget } from "./order";

describe("where a dragged slide lands", () => {
  it.each([
    // from, gap, count, to
    [1, 4, 3, 3],
    [1, 3, 3, 2],
    [3, 1, 3, 1],
    [2, 1, 3, 1],
    [2, 4, 3, 3],
    [1, 1, 3, null],
    [1, 2, 3, null],
    [2, 3, 3, null],
    [3, 4, 3, null],
    [0, 2, 3, null],
    [4, 1, 3, null],
    [1, 5, 3, null],
    [1, 0, 3, null],
  ])("slide %i dropped at gap %i of %i becomes %s", (from, gap, count, to) => {
    expect(moveTarget(from, gap, count)).toBe(to);
  });

  it("takes the gap before a slide on its first half and after it on the second", () => {
    expect(gapAt(3, 10, 0, 100)).toBe(3);
    expect(gapAt(3, 49, 0, 100)).toBe(3);
    expect(gapAt(3, 50, 0, 100)).toBe(4);
    expect(gapAt(3, 99, 0, 100)).toBe(4);
  });

  it("keeps a selection inside the deck", () => {
    expect(clampSlide(4, 3)).toBe(3);
    expect(clampSlide(0, 3)).toBe(1);
    expect(clampSlide(2, 3)).toBe(2);
    expect(clampSlide(2, 0)).toBe(1);
  });
});
