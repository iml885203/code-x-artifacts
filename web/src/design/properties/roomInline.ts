import type { Tokens } from "../tokens";
import { parseColor, type Rgba } from "./css";
import { paintColor, tokenColor } from "./palette";
import type { FrameState } from "../edits";

/** Covered authored slots are retained for reset; only the projected cascade tells controls which declaration wins. */
export function roomInline(frame: FrameState, pid: string): Map<string, string> {
  const source = frame.inline.get(pid);
  if (source === undefined) return frame.styles.get(pid) ?? new Map<string, string>();
  const style = document.createElement("div").style;
  style.cssText = source;
  return new Map(Array.from(style, prop => [prop, style.getPropertyValue(prop) + (style.getPropertyPriority(prop) ? " !important" : "")]));
}

/** Normalize browser-supported literals before channel arithmetic; preserve token lookup. */
export function roomPaintColor(source: string | undefined, tokens: Tokens): Rgba | null {
  const direct = paintColor(source, tokens);
  if (direct) return direct;
  const value = (source ?? "").replace(/\s*!important\s*$/i, "").trim();
  const binding = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
  const literal = binding ? tokenColor(binding, tokens) ?? "" : value;
  const probe = document.createElement("span");
  // Context-dependent values need the inspector's computed fallback, not host inheritance.
  if (!literal || /\b(var|currentcolor|inherit|initial|unset|revert)\b/i.test(literal)) return null;
  probe.style.color = literal;
  if (!probe.style.color) return null;
  probe.style.display = "none";
  document.body.append(probe);
  const normalized = getComputedStyle(probe).color;
  probe.remove();
  const parsed = parseColor(normalized);
  if (parsed) return parsed;
  // Modern color spaces may remain serialized in their original syntax. The browser
  // converts these to the picker's sRGB bytes without guessing at CSS grammar.
  if (typeof CSS === "undefined" || !CSS.supports("color", literal)) return null;
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = 1;
  const context = canvas.getContext("2d");
  if (!context) return null;
  context.fillStyle = literal;
  context.fillRect(0, 0, 1, 1);
  const [r = 0, g = 0, b = 0, a = 0] = context.getImageData(0, 0, 1, 1).data;
  return {r, g, b, a: a / 255};
}
