// @vitest-environment node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// The token block of the site's stylesheet, read as written: the contrast of
// the design is computed from these values, so a token cannot drift unseen.
const css = readFileSync(resolve(process.cwd(), "../internal/web/static/style.css"), "utf8");
const rootBlock = /:root \{([\s\S]*?)\n\}/.exec(css)?.[1] ?? "";

type Theme = "light" | "dark";

function splitTop(list: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < list.length; i++) {
    if (list[i] === "(") depth++;
    if (list[i] === ")") depth--;
    if (list[i] === "," && depth === 0) {
      parts.push(list.slice(from, i).trim());
      from = i + 1;
    }
  }
  parts.push(list.slice(from).trim());
  return parts;
}

const declared = new Map<string, string>();
for (const [, name = "", value = ""] of rootBlock.matchAll(/^\s*(--[\w-]+):\s*([^;]+);/gm)) declared.set(name, value.trim());

function valueOf(name: string, theme: Theme): string {
  const raw = declared.get(name);
  if (raw === undefined) throw new Error(`no token ${name}`);
  const ld = /^light-dark\((.*)\)$/s.exec(raw);
  let v = raw;
  if (ld) {
    const both = splitTop(ld[1] ?? "");
    expect(both, `${name} has a value for each theme`).toHaveLength(2);
    v = both[theme === "light" ? 0 : 1] ?? "";
  }
  const ref = /^var\((--[\w-]+)\)$/.exec(v);
  return ref?.[1] ? valueOf(ref[1], theme) : v;
}

// oklch / hex to linear sRGB, then WCAG relative luminance.
function luminance(colour: string): number {
  let rgb: number[];
  const hex = /^#([0-9a-f]{6})$/i.exec(colour);
  const ok = /^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\)$/.exec(colour);
  if (hex) {
    rgb = [0, 2, 4].map((i) => {
      const c = parseInt((hex[1] ?? "").slice(i, i + 2), 16) / 255;
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
  } else if (ok) {
    const [L, C, h] = [Number(ok[1]), Number(ok[2]), (Number(ok[3]) * Math.PI) / 180] as [number, number, number];
    const a = C * Math.cos(h);
    const b = C * Math.sin(h);
    const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
    const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
    const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
    rgb = [
      4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
      -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
      -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
    ].map((x) => Math.min(1, Math.max(0, x)));
  } else {
    throw new Error(`cannot read colour ${colour}`);
  }
  const [r, g, b] = rgb as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(fg: string, bg: string, theme: Theme): number {
  const a = luminance(valueOf(fg, theme));
  const b = luminance(valueOf(bg, theme));
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

const themes: Theme[] = ["light", "dark"];
const surfaces = ["--night", "--plate", "--raised"];
const kinds = ["web", "doc", "slides", "design", "board", "dashboard", "motion", "system"];

describe("the colour tokens", () => {
  it.each(themes)("give every colour token a value in the %s theme", (theme) => {
    const colourTokens = [...declared].filter(([, v]) => /oklch\(|#[0-9a-f]{6}|light-dark\(/i.test(v)).map(([k]) => k);
    expect(colourTokens.length).toBeGreaterThan(30);
    for (const name of colourTokens) {
      if (name === "--float") continue; // a shadow, not a colour pair; its two values are checked below
      expect(valueOf(name, theme), name).toBeTruthy();
    }
    expect(valueOf("--float", theme)).toMatch(/^0 8px 24px/);
  });

  it.each(themes)("keep reading text at 4.5:1 on every surface in the %s theme", (theme) => {
    for (const fg of ["--text", "--text-2", "--muted", "--danger", "--star-text"]) {
      for (const bg of surfaces) expect(contrast(fg, bg, theme), `${fg} on ${bg}`).toBeGreaterThanOrEqual(4.5);
    }
    expect(contrast("--on-star", "--star", theme), "--on-star on --star").toBeGreaterThanOrEqual(4.5);
    expect(contrast("--text", "--star-wash", theme), "--text on --star-wash").toBeGreaterThanOrEqual(4.5);
    expect(contrast("--text", "--field", theme), "--text on --field").toBeGreaterThanOrEqual(4.5);
  });

  it.each(themes)("keep the edge of every control at 3:1 against its surface in the %s theme", (theme) => {
    for (const bg of [...surfaces, "--field"]) expect(contrast("--edge", bg, theme), `--edge on ${bg}`).toBeGreaterThanOrEqual(3);
    expect(contrast("--star-edge", "--night", theme), "--star-edge on --night").toBeGreaterThanOrEqual(3);
  });

  it.each(themes)("keep the connected light visible at 3:1 on every surface in the %s theme", (theme) => {
    for (const bg of surfaces) expect(contrast("--connected", bg, theme), `--connected on ${bg}`).toBeGreaterThanOrEqual(3);
  });

  it.each(themes)("keep each kind's glyph at 4.5:1 on its tile in the %s theme", (theme) => {
    for (const k of kinds) expect(contrast(`--kind-${k}-fg`, `--kind-${k}-bg`, theme), k).toBeGreaterThanOrEqual(4.5);
  });

  it("makes the kinds' tiles lighter to darker on light, so they differ by more than hue", () => {
    const tile = (k: string) => luminance(valueOf(`--kind-${k}-bg`, "light"));
    expect(tile("web")).toBeGreaterThan(tile("doc"));
    expect(tile("doc")).toBeGreaterThan(tile("slides"));
    expect(tile("slides")).toBeGreaterThan(tile("design"));
    expect(tile("design")).toBeGreaterThan(tile("board"));
  });
});
