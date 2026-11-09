// @vitest-environment node
import { describe, expect, it } from "vitest";
import { formatColor, formatShadow, hexOf, parseColor, parseShadow, pixels, safeText, splitTop, toLength } from "./css";

describe("colours", () => {
  it.each([
    ["#112233", { r: 17, g: 34, b: 51, a: 1 }],
    ["#fa0", { r: 255, g: 170, b: 0, a: 1 }],
    ["rgb(1, 2, 3)", { r: 1, g: 2, b: 3, a: 1 }],
    ["rgba(1, 2, 3, 0.5)", { r: 1, g: 2, b: 3, a: 0.5 }],
    ["rgb(1 2 3 / 50%)", { r: 1, g: 2, b: 3, a: 0.5 }],
    ["rgba(0, 0, 0, 0)", { r: 0, g: 0, b: 0, a: 0 }],
  ])("reads %s", (text, want) => {
    expect(parseColor(text)).toEqual(want);
  });

  it("refuses what is not a colour", () => {
    for (const bad of ["", "red", "#12", "rgb(1,2)", "url(x)"]) expect(parseColor(bad)).toBeNull();
  });

  it("writes an opaque colour as hex and any alpha as rgba(), and reads back what it wrote", () => {
    expect(formatColor({ r: 17, g: 34, b: 51, a: 1 })).toBe("#112233");
    expect(formatColor({ r: 17, g: 34, b: 51, a: 0.5 })).toBe("rgba(17, 34, 51, 0.5)");
    expect(formatColor({ r: 0, g: 0, b: 0, a: 0 })).toBe("rgba(0, 0, 0, 0)");
    for (const text of ["#112233", "rgba(17, 34, 51, 0.25)"]) expect(formatColor(parseColor(text) ?? { r: 0, g: 0, b: 0, a: 1 })).toBe(text);
    expect(hexOf({ r: 255, g: 0, b: 16, a: 0.3 })).toBe("#ff0010");
  });
});

describe("lengths", () => {
  it("takes a bare number as pixels and keeps units and the keywords it was told of", () => {
    expect(toLength("44")).toBe("44px");
    expect(toLength(" 12.5 ")).toBe("12.5px");
    expect(toLength("50%")).toBe("50%");
    expect(toLength("2rem")).toBe("2rem");
    expect(toLength("normal", ["normal"])).toBe("normal");
    expect(toLength("normal")).toBeNull();
    expect(toLength("12;color:red")).toBeNull();
    expect(toLength("")).toBeNull();
  });

  it("reads pixels only from pixels", () => {
    expect(pixels("16px")).toBe(16);
    expect(pixels("-2.5px")).toBe(-2.5);
    expect(pixels("auto")).toBeNull();
    expect(pixels("50%")).toBeNull();
    expect(pixels(undefined)).toBeNull();
  });

  it("refuses free text that could end the declaration", () => {
    expect(safeText('"Helvetica Neue", Arial')).toBe('"Helvetica Neue", Arial');
    for (const bad of ["a;b", "a}", "url(x)", "<b>", ""]) expect(safeText(bad)).toBeNull();
  });

  it("splits only at the commas outside parentheses", () => {
    expect(splitTop("rgba(0, 0, 0, 1) 1px 1px, 2px 2px red")).toEqual(["rgba(0, 0, 0, 1) 1px 1px", "2px 2px red"]);
  });
});

describe("shadows", () => {
  it("reads the browser's form and writes one back that reads the same", () => {
    const s = parseShadow("rgba(0, 0, 0, 0.3) 0px 2px 6px 0px");
    expect(s).toEqual({ x: 0, y: 2, blur: 6, spread: 0, color: { r: 0, g: 0, b: 0, a: 0.3 } });
    expect(formatShadow(s ?? { x: 0, y: 0, blur: 0, spread: 0, color: { r: 0, g: 0, b: 0, a: 1 } })).toBe("0px 2px 6px rgba(0, 0, 0, 0.3)");
    expect(parseShadow("0px 2px 6px #000")?.color).toEqual({ r: 0, g: 0, b: 0, a: 1 });
  });

  it("does not understand none, several, inset or a colourless shadow", () => {
    for (const bad of ["none", "", "0 0 1px red, 0 0 2px blue", "inset 0 0 1px #000", "1px 1px 2px"]) expect(parseShadow(bad)).toBeNull();
  });
});
