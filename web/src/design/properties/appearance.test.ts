import { tokenColor, paintColor } from "./palette";
import { describe, expect, it } from "vitest";
import { readBorderSides, readLayout, writeBorderSide, writeFillNone, writeFillReset, writeWrap, preservePriority, currentBorderStyle } from "./model";

describe("faithful appearance edits", () => {
  it("offers actual color tokens and aliases without resolving dimension tokens or cycles as colors", () => {
    const tokens={"--ink":"#112233","--alias":"var(--ink)","--space":"16px","--loop":"var(--loop)"};
    expect(tokenColor("--alias",tokens)).toBe("#112233");
    expect(paintColor("var(--alias) !important",tokens)).toEqual({r:17,g:34,b:51,a:1});
    expect(paintColor("transparent !important",tokens)?.a).toBe(0);
    expect(tokenColor("--space",tokens)).toBeNull();
    expect(tokenColor("--loop",tokens)).toBeNull();
  });
  it("reads wrapping without normalizing unsupported values and writes only wrapping", () => {
    for (const wrap of ["nowrap", "wrap", "wrap-reverse", "inherit"]) {
      expect(readLayout({ display: "flex", "flex-wrap": wrap }).wrap).toBe(wrap);
    }
    expect(writeWrap("wrap")).toEqual({ "flex-wrap": "wrap" });
  });
  it("removes a fill explicitly while reset reveals the current cascade rather than a remembered color", () => {
    expect(writeFillNone()).toEqual({ "background-color": "transparent !important", "background-image": "none !important" });
    expect(writeFillReset()).toEqual({ "background-color": "", "background-image": "", "background-size": "", "background-position": "", "background-repeat": "" });
    const element = document.createElement("div");
    element.style.cssText = "background: red url('/image.png') center/cover; color: blue";
    const apply = (values: Record<string,string>) => { Object.entries(values).forEach(([p,v]) => { if (v) element.style.setProperty(p,v.replace(/\s*!important$/,""),v.endsWith("!important") ? "important" : ""); else element.style.removeProperty(p); }); };
    apply(writeFillNone());
    expect(element.style.backgroundColor).toBe("transparent");
    expect(element.style.backgroundImage).toBe("none");
    apply(writeFillReset());
    expect(element.style.backgroundColor).toBe("");
    expect(element.style.backgroundImage).toBe("");
    expect(element.style.color).toBe("blue");
  });
  it("preserves shorthand priority only when it applies to the edited side", () => {
    const inline: [string,string][] = [["border", "2px solid red !important"], ["border-left-color", "blue !important"]];
    expect(writeBorderSide("right", "color", "green", {}, inline)).toEqual({"border-right-color":"green !important"});
    expect(writeBorderSide("left", "color", "green", {}, inline)).toEqual({"border-left-color":"green !important"});
    expect(writeBorderSide("top", "color", "green", {}, [["border-left", "2px solid red !important"]])).toEqual({"border-top-color":"green"});
    expect(preservePriority("background-color", "", [["background", "red !important"]])).toBe("");
    expect(writeBorderSide("right", "style", "dotted", {})).toEqual({"border-right-style":"dotted"});
  });
  it("shows no border width for a none style and preserves a visible side's width, color and existing dashed style", () => {
    expect(readBorderSides({"border-top-style":"none","border-top-width":"3px","border-top-color":"rgb(0, 0, 0)"}).sides[0]).toMatchObject({style:"none",width:"0"});
    expect(readBorderSides({"border-top-style":"solid","border-top-width":"2px","border-top-color":"rgb(255, 0, 0)"}).sides[0]).toMatchObject({style:"solid",width:"2",color:"rgb(255, 0, 0)"});
    expect(writeBorderSide("top","width","2px",{"border-top-style":"none"})).toEqual({"border-top-width":"2px","border-top-style":"solid"});
    expect(writeBorderSide("top","width","2px",{"border-top-style":"dashed"})).toEqual({"border-top-width":"2px"});
  });
  it("uses an important room border-style shorthand before a weaker side when checking border visibility", () => {
    expect(currentBorderStyle("top",new Map([["border-top-style","none"],["border-style","dashed !important"]]),{"border-top-style":"none"})).toBe("dashed");
    expect(currentBorderStyle("top",new Map([["border-top-style","dotted !important"],["border-style","dashed"]]),{"border-top-style":"none"})).toBe("dotted");
  });
  it("reports asymmetric sides as mixed and changes one side without rewriting its neighbors or shorthand", () => {
    const computed = Object.fromEntries(["top", "right", "bottom", "left"].flatMap((side,i) => [[`border-${side}-width`, `${String(i+1)}px`], [`border-${side}-style`, i === 0 ? "solid" : "dashed"], [`border-${side}-color`, `rgb(${String(i)}, 20, 30)`]]));
    const border = readBorderSides(computed);
    expect(border.mixed).toEqual({ width: true, style: true, color: true });
    expect(border.sides.map(s => s.width)).toEqual(["1","2","3","4"]);
    expect(writeBorderSide("right", "width", "8px", computed)).toEqual({ "border-right-width": "8px" });
    expect(writeBorderSide("left", "color", "var(--accent)", computed)).toEqual({ "border-left-color": "var(--accent)" });
    expect(writeBorderSide("bottom", "width", "2px", { "border-bottom-style": "none" })).toEqual({ "border-bottom-width": "2px", "border-bottom-style": "solid" });
  });
});
