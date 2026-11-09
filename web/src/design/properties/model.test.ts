// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  boxProps,
  lengthText,
  readBox,
  readGrow,
  readLayout,
  readOpacity,
  readPosition,
  readShadow,
  readSize,
  readText,
  writeAlign,
  writeBoxAll,
  writeBoxPart,
  writeDirection,
  writeGap,
  writeGrow,
  writeJustify,
  writeOpacity,
  writePosition,
  writeShadow,
  writeSize,
} from "./model";

describe("size", () => {
  it("writes Fixed as pixels, Hug as the content's size and Fill as all the room", () => {
    expect(writeSize("width", "fixed", 120.5)).toEqual({ width: "120.5px" });
    expect(writeSize("height", "hug", 0)).toEqual({ height: "fit-content" });
    expect(writeSize("width", "fill", 0)).toEqual({ width: "100%" });
  });

  it("reads the mode from what the element declares and the number from what the browser computed", () => {
    const computed = { width: "300px" };
    expect(readSize("width", computed, [["width", "300px"]], 1)).toEqual({ mode: "fixed", px: "300" });
    expect(readSize("width", computed, [["width", "100%"]], 1)).toEqual({ mode: "fill", px: "300" });
    expect(readSize("width", computed, [["width", "fit-content"]], 1)).toEqual({ mode: "hug", px: "300" });
    // A size that comes from a class shows what it computes to.
    expect(readSize("width", computed, [], 1)).toEqual({ mode: "fixed", px: "300" });
    expect(readSize("width", { width: "auto" }, [], 42)).toEqual({ mode: "fixed", px: "42" });
  });

  it("grows with flex-grow and takes it away when off", () => {
    expect(writeGrow(true)).toEqual({ "flex-grow": "1" });
    expect(writeGrow(false)).toEqual({ "flex-grow": "" });
    expect(readGrow({ "flex-grow": "1" })).toBe(true);
    expect(readGrow({ "flex-grow": "0" })).toBe(false);
  });
});

describe("position and layout", () => {
  it("changes only the mode, so the offset stays", () => {
    expect(writePosition("relative")).toEqual({ position: "relative" });
    expect(readPosition({ position: "absolute" })).toBe("absolute");
    expect(readPosition({ position: "static" })).toBe("other");
  });

  it("makes a container of an element that is not one, and keeps the display of one that is", () => {
    expect(writeDirection("column", { display: "block" })).toEqual({ "flex-direction": "column", display: "flex" });
    expect(writeDirection("row", { display: "inline-block" })).toEqual({ "flex-direction": "row", display: "inline-flex" });
    expect(writeDirection("row", { display: "flex" })).toEqual({ "flex-direction": "row" });
  });

  it("writes align, justify and gap as CSS and reads them back as the same choice", () => {
    expect(writeAlign("start")).toEqual({ "align-items": "flex-start" });
    expect(writeAlign("end")).toEqual({ "align-items": "flex-end" });
    expect(writeJustify("between")).toEqual({ "justify-content": "space-between" });
    expect(writeGap("12px")).toEqual({ gap: "12px" });
    expect(readLayout({ display: "flex", "flex-direction": "column", "align-items": "flex-end", "justify-content": "space-between", gap: "12px 12px" })).toEqual({
      flex: true,
      wrap: "nowrap",
      direction: "column",
      align: "end",
      justify: "between",
      gap: "12",
    });
    expect(readLayout({ display: "block", "align-items": "normal", "justify-content": "normal", gap: "normal" })).toMatchObject({ flex: false, align: "stretch", justify: "start", gap: "0" });
  });
});

describe("boxes of four", () => {
  it("names the parts of padding, margin and the radius", () => {
    expect(boxProps("padding").parts).toEqual(["padding-top", "padding-right", "padding-bottom", "padding-left"]);
    expect(boxProps("radius").parts).toEqual(["border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"]);
  });

  it("writes all sides as the shorthand and takes the parts away so nothing overrides it", () => {
    expect(writeBoxAll("padding", "10px")).toEqual({ padding: "10px", "padding-top": "", "padding-right": "", "padding-bottom": "", "padding-left": "" });
    expect(writeBoxAll("radius", "8px")["border-radius"]).toBe("8px");
  });

  it("writes one side as four parts, keeping the other three as they are, and takes the shorthand away", () => {
    expect(writeBoxPart("margin", 1, "5px", ["1", "2", "3", "4"])).toEqual({ margin: "", "margin-top": "1px", "margin-right": "5px", "margin-bottom": "3px", "margin-left": "4px" });
    // A part that is not pixels keeps its text.
    expect(writeBoxPart("radius", 0, "1px", ["50%", "0", "0", "0"])["border-top-right-radius"]).toBe("0px");
    expect(writeBoxPart("radius", 1, "1px", ["50%", "0", "0", "0"])["border-top-left-radius"]).toBe("50%");
  });

  it("reads the four values and whether they are one", () => {
    expect(readBox("padding", { "padding-top": "8px", "padding-right": "8px", "padding-bottom": "8px", "padding-left": "8px" })).toEqual({ values: ["8", "8", "8", "8"], same: true });
    expect(readBox("padding", { "padding-top": "8px", "padding-right": "12px", "padding-bottom": "8px", "padding-left": "12px" }).same).toBe(false);
  });
});

describe("appearance", () => {

  it("writes opacity as a number between 0 and 1 from a percentage", () => {
    expect(writeOpacity(50)).toEqual({ opacity: "0.5" });
    expect(writeOpacity(150)).toEqual({ opacity: "1" });
    expect(writeOpacity(-1)).toEqual({ opacity: "0" });
    expect(readOpacity({ opacity: "0.35" })).toBe("35");
  });

  it("writes a shadow and takes it away, and says when it cannot edit the one there is", () => {
    expect(writeShadow({ x: 1, y: 2, blur: 3, spread: 0, color: { r: 0, g: 0, b: 0, a: 0.5 } })).toEqual({ "box-shadow": "1px 2px 3px rgba(0, 0, 0, 0.5)" });
    expect(writeShadow(null)).toEqual({ "box-shadow": "" });
    expect(readShadow({ "box-shadow": "none" })).toEqual({ shadow: null, editable: true });
    expect(readShadow({ "box-shadow": "rgba(0, 0, 0, 0.3) 0px 2px 6px 0px" }).shadow?.blur).toBe(6);
    expect(readShadow({ "box-shadow": "0 0 1px red, 0 0 2px blue" })).toEqual({ shadow: null, editable: false });
  });
});

describe("text", () => {
  it("reads what the browser computed", () => {
    expect(
      readText({ "font-family": "Inter, sans-serif", "font-size": "44px", "font-weight": "700", "font-style": "italic", "text-align": "center", "line-height": "normal", "letter-spacing": "0.5px", color: "rgb(1, 2, 3)" }),
    ).toEqual({ family: "Inter, sans-serif", size: "44", weight: "700", italic: true, align: "center", lineHeight: "normal", letterSpacing: "0.5", color: { r: 1, g: 2, b: 3, a: 1 } });
    expect(readText({ "text-align": "start" }).align).toBe("left");
    expect(lengthText("50%")).toBe("50%");
  });
});
