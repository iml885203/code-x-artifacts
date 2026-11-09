// The rules of a design's pages and frames that people see (docs/design-format.md):
// the limits, the sizes offered, and what a change to the structure comes to.
// The server holds the same limits (internal/designfmt) and checks the room again.

export const MAX_PAGES = 20;
export const MAX_FRAMES = 60;
export const MAX_SIDE = 8192;
export const MIN_SIDE = 1;
export const MAX_NAME = 80;

/** The sizes offered next to the width and height. A4 is 210 by 297 mm at 96 dpi. */
export const PRESETS = [
  { id: "desktop", w: 1440, h: 900 },
  { id: "tablet", w: 834, h: 1194 },
  { id: "phone", w: 390, h: 844 },
  { id: "a4", w: 794, h: 1123 },
] as const;

/** The kinds of refusal, so the screen can say each in the person's language. */
export type Refusal =
  | { rule: "pages"; limit: number }
  | { rule: "frames"; limit: number }
  | { rule: "name"; limit: number }
  | { rule: "side"; field: "w" | "h"; min: number; max: number }
  | { rule: "lastPage" }
  | { rule: "lastFrame" }
  | { rule: "missing" };

/** What a change to the structure came to: it was made, it would have changed nothing, or a rule refused it and nothing was written. */
export type Outcome = { kind: "changed"; page?: string; frame?: string } | { kind: "same" } | { kind: "refused"; why: Refusal };

export const refused = (why: Refusal): Outcome => ({ kind: "refused", why });
export const same: Outcome = { kind: "same" };

/** A name with the number n in it: the first of "Page 2", "Page 3"… from `from` on that no one has. */
export function numbered(pattern: (n: number) => string, taken: string[], from: number): string {
  const have = new Set(taken);
  for (let n = from; ; n++) if (!have.has(pattern(n))) return pattern(n);
}

/** A name as it is kept (trimmed), "" for none, or the refusal when it is too long. */
export function cleanName(raw: string): string | Refusal {
  const name = raw.trim();
  if (Array.from(name).length > MAX_NAME) return { rule: "name", limit: MAX_NAME };
  return name;
}

/** Whole pixels from 1 to 8192, or the refusal that names both limits. */
export function sideProblem(field: "w" | "h", n: number): Refusal | null {
  return Number.isInteger(n) && n >= MIN_SIDE && n <= MAX_SIDE ? null : { rule: "side", field, min: MIN_SIDE, max: MAX_SIDE };
}
