// The text of one element as a list of units, one per position of the Y.Text that
// holds it (a UTF-16 code unit, or one embed), each with the formats it is
// inside. The room's text and what the frame says it now looks like are both put
// in this form, so what changed is a difference between two lists.

/** Format attributes by name (`f:<pid>`), each a JSON string, as designroom writes them. */
export type Attrs = Record<string, string>;

export const FORMAT = "f:";
const EMBED = "\u0000";

export interface Unit {
  /** The code unit, or for an embed EMBED and a key that says which kind and, for markup kept as written, what it is. */
  c: string;
  /** The embed as it goes into the Y.Text (`{br}` or `{raw}`). */
  embed?: Record<string, string>;
  attrs: Attrs;
}

/** A run of a Y.Text's delta. */
export interface DeltaOp {
  insert: string | Record<string, unknown>;
  attributes?: Record<string, unknown>;
}

/** What the frame says about the content of the element it was editing, in order. */
export interface Run {
  /** Characters. */
  s?: string;
  /** A line break. */
  br?: true;
  /** Markup that is kept as it was written. */
  raw?: string;
  /** The inline elements the run is inside, outermost first; `p` is the element's id. */
  f: { t: string; p: string }[];
}

/** The same markup in the browser's own spelling, so that two ways of writing one thing are one thing. */
export function canonical(markup: string): string {
  return new DOMParser().parseFromString(`<!doctype html><body>${markup}`, "text/html").body.innerHTML;
}

const formatsOf = (attributes: Record<string, unknown> | undefined): Attrs => {
  const out: Attrs = {};
  for (const [k, v] of Object.entries(attributes ?? {})) if (k.startsWith(FORMAT) && typeof v === "string") out[k] = v;
  return out;
};

function embedUnit(embed: Record<string, unknown>, attrs: Attrs): Unit | null {
  if (typeof embed.br === "string") return { c: `${EMBED}br`, embed: { br: embed.br }, attrs };
  if (typeof embed.raw === "string") return { c: `${EMBED}raw:${canonical(embed.raw)}`, embed: { raw: embed.raw }, attrs };
  return null; // an embed that is not ours takes a position that no one here edits
}

/** The units of a Y.Text's delta. */
export function unitsOfDelta(delta: DeltaOp[]): Unit[] {
  const units: Unit[] = [];
  for (const op of delta) {
    const attrs = formatsOf(op.attributes);
    if (typeof op.insert === "string") {
      for (let i = 0; i < op.insert.length; i++) units.push({ c: op.insert.charAt(i), attrs });
    } else {
      units.push(embedUnit(op.insert, attrs) ?? { c: `${EMBED}?`, attrs });
    }
  }
  return units;
}

interface Known {
  t: string;
  rows: unknown;
  depth: number;
  value: string;
}

/** The inline elements the text already has, by id. */
function knownFormats(units: Unit[]): Map<string, Known> {
  const known = new Map<string, Known>();
  for (const u of units) {
    for (const [key, value] of Object.entries(u.attrs)) {
      const pid = key.slice(FORMAT.length);
      if (known.has(pid)) continue;
      try {
        const f = JSON.parse(value) as { t?: unknown; d?: unknown; a?: unknown };
        if (typeof f.t === "string" && typeof f.d === "number") known.set(pid, { t: f.t, rows: f.a, depth: f.d, value });
      } catch {
        // a format that is not JSON is not one this reader knows
      }
    }
  }
  return known;
}

/**
 * The units a frame's runs say, formats spelled as the room has them: an element
 * the text already had keeps its attributes exactly (the frame may have
 * respelled them), at the depth it now has; one the frame made has just its id.
 */
export function unitsOfRuns(runs: Run[], before: Unit[]): Unit[] {
  const known = knownFormats(before);
  const units: Unit[] = [];
  for (const run of runs) {
    const attrs: Attrs = {};
    run.f.forEach((f, depth) => {
      const had = known.get(f.p);
      attrs[FORMAT + f.p] = had?.depth === depth ? had.value : JSON.stringify({ t: had?.t ?? f.t, d: depth, a: had?.rows ?? [["", "data-pid", f.p]] });
    });
    if (run.s !== undefined) {
      for (let i = 0; i < run.s.length; i++) units.push({ c: run.s.charAt(i), attrs });
    } else if (run.br) {
      units.push({ c: `${EMBED}br`, embed: { br: "[]" }, attrs });
    } else if (run.raw !== undefined) {
      units.push({ c: `${EMBED}raw:${canonical(run.raw)}`, embed: { raw: run.raw }, attrs });
    }
  }
  return units;
}

/** Whether the content says more than characters: there is a format or an embed in it. */
export const isFormatted = (units: Unit[]) => units.some((u) => u.embed !== undefined || Object.keys(u.attrs).length > 0);

/** What a frame says its content is, checked: anything else is not something to write into the room. */
export function parseRuns(value: unknown): Run[] | null {
  if (!Array.isArray(value)) return null;
  const runs: Run[] = [];
  for (const r of value as unknown[]) {
    if (typeof r !== "object" || r === null) return null;
    const run = r as Record<string, unknown>;
    const f = run.f;
    if (!Array.isArray(f) || !(f as unknown[]).every((x) => typeof x === "object" && x !== null && typeof (x as { t?: unknown }).t === "string" && typeof (x as { p?: unknown }).p === "string")) return null;
    const formats = (f as { t: string; p: string }[]).map(({ t, p }) => ({ t, p }));
    if (typeof run.s === "string") runs.push({ s: run.s, f: formats });
    else if (run.br === 1) runs.push({ br: true, f: formats });
    else if (typeof run.raw === "string") runs.push({ raw: run.raw, f: formats });
    else return null;
  }
  return runs;
}
