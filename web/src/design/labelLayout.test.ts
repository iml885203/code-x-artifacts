// @vitest-environment node
import { describe, expect, it } from "vitest";
import { labelLayout } from "./labelLayout";
import { GAP_Y } from "./manifest";

describe("where the labels of a page keep their actions", () => {
  it("keeps Comment and Play in the labels while every label holds two 44px targets and the row gap holds the label", () => {
    expect(labelLayout(1, [390, 800])).toEqual({ actions: "row", name: "above" });
    expect(labelLayout(0.5, [390, 800])).toEqual({ actions: "row", name: "above" });
  });

  it("moves them to the strip as soon as one frame is too narrow on screen, however wide the others are", () => {
    // A 100px frame at 50% is 50px wide on screen.
    expect(labelLayout(0.5, [800, 100]).actions).toBe("dock");
    expect(labelLayout(0.5, [800, 176]).actions).toBe("row");
    expect(labelLayout(0.5, [800, 174]).actions).toBe("dock");
  });

  it("moves them to the strip when the row gap on screen is shorter than the label, whatever the frame heights", () => {
    // 168 x zoom is the gap; a two-line label is 70px, a one-line label 50px.
    expect(labelLayout(70 / GAP_Y, [195 / (70 / GAP_Y)]).actions).toBe("row");
    expect(labelLayout(69 / GAP_Y, [195 / (69 / GAP_Y)]).actions).toBe("dock");
    expect(labelLayout(50 / GAP_Y, [400 / (50 / GAP_Y)]).actions).toBe("row");
    expect(labelLayout(49 / GAP_Y, [400 / (49 / GAP_Y)]).actions).toBe("dock");
  });

  it("puts the name on the frame's top edge only when even a name line does not fit the gap, as for 200px-high frames fitted at 10% and 5%", () => {
    expect(labelLayout(0.2, [390])).toEqual({ actions: "dock", name: "above" });
    expect(labelLayout(26 / GAP_Y, [390]).name).toBe("above");
    expect(labelLayout(25 / GAP_Y, [390]).name).toBe("inside");
    expect(labelLayout(0.1, [800, 390])).toEqual({ actions: "dock", name: "inside" });
    expect(labelLayout(0.05, [800, 390])).toEqual({ actions: "dock", name: "inside" });
  });
});
