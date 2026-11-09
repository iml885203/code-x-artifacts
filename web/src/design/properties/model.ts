// What the properties panel shows of an element and the declarations each of its
// controls writes. The frame reports the element's computed values and its inline
// style (inspect.ts); a control writes inline CSS through the room (edits.ts). Pure
// functions: every control is a row of the table test in model.test.ts.
import { formatColor, formatShadow, num, parseColor, parseShadow, pixels, type Rgba, type Shadow } from "./css";

export type Computed = Record<string, string | undefined>;
/** Declarations to write: a value, or "" to take the declaration away. */
export type Declarations = Record<string, string>;

const inlineOf = (inline: [string, string][], prop: string) => inline.find(([name]) => name === prop)?.[1];

/** A computed length as text for a box: pixels without the unit, anything else (a percentage, `normal`) as the browser says it. */
export function lengthText(value: string | undefined): string {
  const px = pixels(value);
  return px === null ? (value ?? "") : num(px);
}

// Size ----------------------------------------------------------------------

export type Axis = "width" | "height";
export type SizeMode = "fixed" | "hug" | "fill";

const HUG = ["fit-content", "max-content", "min-content", "auto"];

export function readSize(axis: Axis, computed: Computed, inline: [string, string][], box: number) {
  const own = inlineOf(inline, axis)?.replace(/\s*!important\s*$/i, "").trim().toLowerCase();
  const mode: SizeMode = own === "100%" ? "fill" : own !== undefined && HUG.includes(own) ? "hug" : "fixed";
  return { mode, px: num(pixels(computed[axis]) ?? box) };
}

/** Fixed is the pixels, Hug is the content's own size, Fill is all the room the parent gives. Switching to Fixed from another mode keeps the size the element has now. */
export function writeSize(axis: Axis, mode: SizeMode, px: number): Declarations {
  return { [axis]: mode === "fixed" ? `${num(px)}px` : mode === "hug" ? "fit-content" : "100%" };
}

export const readGrow = (computed: Computed) => Number(computed["flex-grow"] ?? "0") > 0;
export const writeGrow = (on: boolean): Declarations => ({ "flex-grow": on ? "1" : "" });

// Position and layout --------------------------------------------------------

export type Position = "absolute" | "relative" | "other";

export function readPosition(computed: Computed): Position {
  const p = computed.position;
  return p === "absolute" ? "absolute" : p === "relative" ? "relative" : "other";
}

/** Only the mode changes: the offset (left, top) stays as it is, so a relative element is moved by it from its place in the flow. */
export const writePosition = (mode: "absolute" | "relative"): Declarations => ({ position: mode });

export type Direction = "row" | "column";
export type Distribution = "start" | "center" | "end" | "stretch" | "between";

const CSS_OF: Record<Distribution, string> = { start: "flex-start", center: "center", end: "flex-end", stretch: "stretch", between: "space-between" };

function distributionOf(value: string | undefined, fallback: Distribution): Distribution {
  switch (value) {
    case "flex-start":
    case "start":
    case "left":
    case "self-start":
      return "start";
    case "center":
      return "center";
    case "flex-end":
    case "end":
    case "right":
    case "self-end":
      return "end";
    case "stretch":
      return "stretch";
    case "space-between":
      return "between";
    default:
      return fallback;
  }
}

export function readLayout(computed: Computed) {
  const flex = computed.display === "flex" || computed.display === "inline-flex";
  return {
    flex,
    wrap: computed["flex-wrap"] ?? "nowrap",
    direction: computed["flex-direction"]?.startsWith("column") ? ("column" as const) : ("row" as const),
    align: distributionOf(computed["align-items"], "stretch"),
    justify: distributionOf(computed["justify-content"], "start"),
    gap: lengthText(computed.gap === "normal" ? "0px" : computed.gap?.split(" ")[0]),
  };
}

/** A container is a flex box: choosing a direction makes the element one when it is not. */
export function writeDirection(direction: Direction, computed: Computed): Declarations {
  const out: Declarations = { "flex-direction": direction };
  if (computed.display !== "flex" && computed.display !== "inline-flex") out.display = computed.display?.startsWith("inline") ? "inline-flex" : "flex";
  return out;
}

export const writeAlign = (value: Distribution): Declarations => ({ "align-items": CSS_OF[value] });
export const writeJustify = (value: Distribution): Declarations => ({ "justify-content": CSS_OF[value] });
export const writeGap = (css: string): Declarations => ({ gap: css });

// Boxes of four: padding, margin, the corners of the radius ------------------------

export type Box = "padding" | "margin" | "radius";

const SIDES = ["top", "right", "bottom", "left"] as const;
const CORNERS = ["top-left", "top-right", "bottom-right", "bottom-left"] as const;

/** The shorthand and the four parts of a box, in the order top, right, bottom, left (corners: top-left, top-right, bottom-right, bottom-left). */
export function boxProps(box: Box): { all: string; parts: string[] } {
  if (box === "radius") return { all: "border-radius", parts: CORNERS.map((c) => `border-${c}-radius`) };
  return { all: box, parts: SIDES.map((s) => `${box}-${s}`) };
}

export function readBox(box: Box, computed: Computed) {
  const { parts } = boxProps(box);
  const values = parts.map((p) => lengthText(computed[p]));
  return { values, same: values.every((v) => v === values[0]) };
}

/** One value for all four. The parts are taken away so that nothing overrides the shorthand. */
export function writeBoxAll(box: Box, css: string): Declarations {
  const { all, parts } = boxProps(box);
  return { [all]: css, ...Object.fromEntries(parts.map((p) => [p, ""])) };
}

/** One part changed: all four are written as parts, and the shorthand (which would otherwise be read first) is taken away. */
export function writeBoxPart(box: Box, index: number, css: string, current: string[]): Declarations {
  const { all, parts } = boxProps(box);
  const out: Declarations = { [all]: "" };
  parts.forEach((p, i) => {
    out[p] = i === index ? css : `${current[i] ?? "0"}${/^-?[\d.]+$/.test(current[i] ?? "0") ? "px" : ""}`;
  });
  return out;
}

// Appearance ------------------------------------------------------------------------

/** A colour the frame reports; transparent when the element has none (alpha 0). */
export function readColor(value: string | undefined): Rgba {
  return parseColor(value ?? "") ?? { r: 0, g: 0, b: 0, a: 1 };
}

export const writeColor = (prop: "background-color" | "color" | "border-color", c: Rgba): Declarations => ({ [prop]: formatColor(c) });

export const writeOpacity = (percent: number): Declarations => ({ opacity: num(Math.min(100, Math.max(0, percent)) / 100) });
export const readOpacity = (computed: Computed) => num(Number(computed.opacity ?? "1") * 100);

export function readShadow(computed: Computed): { shadow: Shadow | null; editable: boolean } {
  const text = computed["box-shadow"] ?? "none";
  const shadow = parseShadow(text);
  return { shadow, editable: shadow !== null || text === "none" || text === "" };
}

export const writeShadow = (s: Shadow | null): Declarations => ({ "box-shadow": s ? formatShadow(s) : "" });
export const NO_SHADOW: Shadow = { x: 0, y: 4, blur: 8, spread: 0, color: { r: 0, g: 0, b: 0, a: 0.25 } };

// Text ---------------------------------------------------------------------------------

export function readText(computed: Computed) {
  const weight = computed["font-weight"] ?? "400";
  return {
    family: computed["font-family"] ?? "",
    size: lengthText(computed["font-size"]),
    weight: /^\d+$/.test(weight) ? weight : weight === "bold" ? "700" : "400",
    italic: computed["font-style"] === "italic" || computed["font-style"] === "oblique",
    align: textAlignOf(computed["text-align"]),
    lineHeight: lengthText(computed["line-height"]),
    letterSpacing: lengthText(computed["letter-spacing"]),
    color: readColor(computed.color),
  };
}

export type Align = "left" | "center" | "right" | "justify";
function textAlignOf(value: string | undefined): Align {
  return value === "center" || value === "right" || value === "justify" ? value : "left";
}

export type Wrap = "nowrap" | "wrap" | "wrap-reverse";
export const writeWrap = (wrap: Wrap): Declarations => ({ "flex-wrap": wrap });

/** No fill suppresses color and images. Reset removes declarations, revealing the current cascade, without retaining historical inline styling. */
export const writeFillNone = (): Declarations => ({ "background-color": "transparent !important", "background-image": "none !important" });
export const writeFillReset = (): Declarations => Object.fromEntries(["background-color", "background-image", "background-size", "background-position", "background-repeat"].map(p => [p, ""]));

export type BorderSide = typeof SIDES[number];
export type BorderPart = "width" | "style" | "color";
export function readBorderSides(computed: Computed) {
  const sides = SIDES.map(side => ({
    side,
    width: lengthText(computed[`border-${side}-style`] === "none" ? "0px" : computed[`border-${side}-width`] ?? "0px"),
    style: computed[`border-${side}-style`] ?? "none",
    color: computed[`border-${side}-color`] ?? "rgb(0, 0, 0)",
  }));
  const mixed = Object.fromEntries((["width", "style", "color"] as const).map(part => [part, sides.some(s => s[part] !== sides[0]?.[part])])) as Record<BorderPart, boolean>;
  return { sides, mixed };
}

/** A longhand overrides an important shorthand at the same priority; other declarations retain their spelling and order. */
export function preservePriority(prop: string, value: string, inline: [string, string][]): string {
  if (!value || /!important\s*$/i.test(value)) return value;
  const family = prop.startsWith("border-") ? ["border", `border-${prop.split("-")[1] ?? ""}`, `border-${prop.split("-").at(-1) ?? ""}`] : prop.startsWith("background-") ? ["background"] : [];
  return inline.some(([p,v]) => (p === prop || family.includes(p)) && /!important\s*$/i.test(v)) ? `${value} !important` : value;
}

export function writeBorderSide(side: BorderSide, part: BorderPart, value: string, computed: Computed, inline: [string, string][] = []): Declarations {
  const prop = `border-${side}-${part}`;
  const out: Declarations = { [prop]: preservePriority(prop, value, inline) };
  if (part === "width" && (!computed[`border-${side}-style`] || computed[`border-${side}-style`] === "none")) {
    const style = `border-${side}-style`;
    out[style] = preservePriority(style, "solid", inline);
  }
  return out;
}

/** Room values precede delayed frame reports when a width edit decides whether it must make a border visible. */
export function currentBorderStyle(side: BorderSide, current: ReadonlyMap<string,string> | undefined, computed: Computed): string {
  const clean = (value: string | undefined) => value?.replace(/\s*!important\s*$/i, "").trim();
  const own = clean(current?.get(`border-${side}-style`));
  const sharedValue = current?.get("border-style");
  const ownImportant = /!important\s*$/i.test(current?.get(`border-${side}-style`) ?? "");
  if (own && (ownImportant || !/!important\s*$/i.test(sharedValue ?? ""))) return own;
  const shared = clean(sharedValue)?.split(/\s+/);
  if (shared?.length) {
    const index = SIDES.indexOf(side);
    return [shared[0],shared[1] ?? shared[0],shared[2] ?? shared[0],shared[3] ?? shared[1] ?? shared[0]][index] ?? "none";
  }
  return computed[`border-${side}-style`] ?? "none";
}
