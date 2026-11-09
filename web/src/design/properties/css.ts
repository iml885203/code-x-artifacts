// The small pieces of CSS the properties panel reads and writes: lengths,
// colours with alpha, shadows. Pure functions; the panel and its tests share them.

export interface Rgba {
  r: number;
  g: number;
  b: number;
  /** 0 to 1. */
  a: number;
}

const byte = (n: number) => Math.min(255, Math.max(0, Math.round(n)));
const hex2 = (n: number) => byte(n).toString(16).padStart(2, "0");

/** A colour as the browser reports it (`rgb(1, 2, 3)`, `rgba(1, 2, 3, 0.5)`) or as a person types it (`#abc`, `#aabbcc`, `#aabbccdd`, the same functions); null for anything else. */
export function parseColor(text: string): Rgba | null {
  const s = text.trim().toLowerCase();
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])([0-9a-f])?$/.exec(s);
  if (short) {
    const [, r, g, b, a] = short;
    return { r: parseInt(`${r ?? ""}${r ?? ""}`, 16), g: parseInt(`${g ?? ""}${g ?? ""}`, 16), b: parseInt(`${b ?? ""}${b ?? ""}`, 16), a: a ? parseInt(`${a}${a}`, 16) / 255 : 1 };
  }
  const long = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})?$/.exec(s);
  if (long) {
    const [, r, g, b, a] = long;
    return { r: parseInt(r ?? "0", 16), g: parseInt(g ?? "0", 16), b: parseInt(b ?? "0", 16), a: a ? parseInt(a, 16) / 255 : 1 };
  }
  const fn = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)(?:\s*[,/]\s*([\d.]+)(%?))?\s*\)$/.exec(s);
  if (fn) {
    const [, r, g, b, a, pct] = fn;
    const alpha = a === undefined ? 1 : pct ? Number(a) / 100 : Number(a);
    return { r: byte(Number(r)), g: byte(Number(g)), b: byte(Number(b)), a: Math.min(1, Math.max(0, alpha)) };
  }
  return null;
}

/** The alpha as it is written: at most three decimals, none for a whole number. */
const alphaText = (a: number) => String(Math.round(a * 1000) / 1000);

/** `#rrggbb` when opaque, `rgba(r, g, b, a)` when not, so alpha is always written as rgba(). */
export function formatColor(c: Rgba): string {
  if (c.a >= 1) return `#${hex2(c.r)}${hex2(c.g)}${hex2(c.b)}`;
  return `rgba(${String(byte(c.r))}, ${String(byte(c.g))}, ${String(byte(c.b))}, ${alphaText(Math.max(0, c.a))})`;
}

/** `#rrggbb` of a colour, ignoring alpha (what a colour input holds). */
export const hexOf = (c: Rgba) => `#${hex2(c.r)}${hex2(c.g)}${hex2(c.b)}`;

/** What a person typed in a length box, as CSS: a bare number is pixels; a number with a unit, a percentage and the keywords given are kept; anything else is refused (null). */
export function toLength(text: string, keywords: readonly string[] = []): string | null {
  const s = text.trim().toLowerCase();
  if (keywords.includes(s)) return s;
  if (/^-?\d+(\.\d+)?$/.test(s)) return `${s}px`;
  if (/^-?\d+(\.\d+)?(px|%|em|rem|vw|vh)$/.test(s)) return s;
  return null;
}

/** The number of pixels in a computed length (`16px`), or null for `auto`, `normal`, percentages and the like. */
export function pixels(value: string | undefined): number | null {
  const m = /^(-?\d+(?:\.\d+)?)px$/.exec((value ?? "").trim());
  return m ? Number(m[1]) : null;
}

/** A number as it is written in CSS: at most two decimals, none for a whole number. */
export const num = (n: number) => String(Math.round(n * 100) / 100);

/** Free text for a property (a font stack): refused when it could end the declaration or the rule. */
export function safeText(text: string): string | null {
  const s = text.trim();
  if (s === "" || /[;{}<>\\]|url\(|expression\(|@import/i.test(s)) return null;
  return s;
}

/** Splits at the commas that are not inside parentheses. */
export function splitTop(text: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c === "(") depth++;
    else if (c === ")") depth = Math.max(0, depth - 1);
    else if (c === "," && depth === 0) {
      out.push(text.slice(start, i));
      start = i + 1;
    }
  }
  out.push(text.slice(start));
  return out.map((p) => p.trim());
}

export interface Shadow {
  x: number;
  y: number;
  blur: number;
  spread: number;
  color: Rgba;
}

/** One drop shadow (`rgba(0, 0, 0, 0.3) 0px 2px 6px 0px` as the browser reports it, or the same with the colour last); null for none, for several, for `inset` and for what is not understood. */
export function parseShadow(text: string): Shadow | null {
  const s = text.trim();
  if (s === "" || s === "none" || splitTop(s).length !== 1 || /\binset\b/i.test(s)) return null;
  const found: { color: Rgba | null } = { color: null };
  const rest = s.replace(/rgba?\([^)]*\)|#[0-9a-f]{3,8}\b/i, (m) => {
    found.color = parseColor(m);
    return " ";
  });
  const lengths = rest.trim().split(/\s+/).map(pixels);
  if (lengths.length < 2 || lengths.length > 4 || lengths.includes(null)) return null;
  const [x, y, blur, spread] = lengths;
  // A shadow with no colour is the element's text colour; the panel does not guess it.
  if (!found.color) return null;
  return { x: x ?? 0, y: y ?? 0, blur: blur ?? 0, spread: spread ?? 0, color: found.color };
}

export function formatShadow(s: Shadow): string {
  const parts = [`${num(s.x)}px`, `${num(s.y)}px`, `${num(s.blur)}px`];
  if (s.spread !== 0) parts.push(`${num(s.spread)}px`);
  return `${parts.join(" ")} ${formatColor(s.color)}`;
}
