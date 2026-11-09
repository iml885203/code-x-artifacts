// What changed between the units a Y.Text had and the units the frame says it has
// now, as the least that has to be written: the stretch in the middle that is
// replaced, and the formats that differ on what stays. Written to the text at the
// positions the text has now, even if other people changed it in the meantime.
import * as Y from "yjs";
import type { Attrs, Unit } from "./units";

export interface FormatChange {
  index: number;
  length: number;
  /** A name with a value sets that format; a name with null takes it away. */
  attrs: Record<string, string | null>;
}

export interface Diff {
  /** The units [start, end) of the old content are replaced by `insert`. */
  start: number;
  end: number;
  insert: Unit[];
  /** The formats of units that stay, by their position in the old content. */
  formats: FormatChange[];
}

const sameAttrs = (a: Attrs, b: Attrs) => {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k]);
};

const isHigh = (c: string) => c >= "\ud800" && c <= "\udbff";
const isLow = (c: string) => c >= "\udc00" && c <= "\udfff";

export function diffUnits(before: Unit[], after: Unit[]): Diff {
  const most = Math.min(before.length, after.length);
  let head = 0;
  while (head < most && before[head]?.c === after[head]?.c) head++;
  // A character of two units is kept or replaced whole.
  if (head > 0 && head < most && isHigh(before[head - 1]?.c ?? "")) head--;
  let tail = 0;
  while (tail < most - head && before[before.length - 1 - tail]?.c === after[after.length - 1 - tail]?.c) tail++;
  if (tail > 0 && isLow(before[before.length - tail]?.c ?? "")) tail--;
  const start = head;
  const end = before.length - tail;
  const formats: FormatChange[] = [];
  const keep = (from: number, to: number, shift: number) => {
    let open: FormatChange | null = null;
    for (let i = from; i < to; i++) {
      const was = before[i]?.attrs ?? {};
      const now = after[i + shift]?.attrs ?? {};
      const change: FormatChange["attrs"] = {};
      for (const [k, v] of Object.entries(now)) if (was[k] !== v) change[k] = v;
      for (const k of Object.keys(was)) if (!(k in now)) change[k] = null;
      if (Object.keys(change).length === 0) {
        open = null;
      } else if (open && sameAttrs(open.attrs as Attrs, change as Attrs)) {
        open.length++;
      } else {
        open = { index: i, length: 1, attrs: change };
        formats.push(open);
      }
    }
  };
  keep(0, start, 0);
  keep(end, before.length, after.length - before.length);
  return { start, end, insert: after.slice(start, after.length - tail), formats };
}

/**
 * Where a position of the text as it was is now, after the changes other people
 * made (the deltas of the text's observer, in order). A change exactly at the
 * position pushes it along when `after` is true and leaves it when not.
 */
export function throughChanges(index: number, changes: readonly (readonly DeltaStep[])[], after: boolean): number {
  let at = index;
  for (const delta of changes) at = through(at, delta, after);
  return at;
}

export interface DeltaStep {
  retain?: number;
  insert?: string | object;
  delete?: number;
}

function through(index: number, delta: readonly DeltaStep[], after: boolean): number {
  let old = 0;
  let now = 0;
  for (const op of delta) {
    if (op.retain !== undefined) {
      if (index < old + op.retain) return now + (index - old);
      old += op.retain;
      now += op.retain;
    } else if (op.insert !== undefined) {
      const length = typeof op.insert === "string" ? op.insert.length : 1;
      if (index === old) return after ? now + length : now;
      now += length;
    } else if (op.delete !== undefined) {
      if (index < old + op.delete) return now;
      old += op.delete;
    }
  }
  return now + (index - old);
}

const runsOf = (units: Unit[]) => {
  const runs: { units: Unit[] }[] = [];
  for (const u of units) {
    const last = runs[runs.length - 1];
    const prev = last?.units[0];
    if (last && prev && u.embed === undefined && prev.embed === undefined && sameAttrs(prev.attrs, u.attrs)) last.units.push(u);
    else runs.push({ units: [u] });
  }
  return runs;
};

/** An id for an element the editor makes, that no one else is making at the same moment. */
export function freshId(prefix: string): string {
  const bytes = crypto.getRandomValues(new Uint8Array(5));
  return prefix + Array.from(bytes, (b) => b.toString(36).padStart(2, "0")).join("").slice(0, 8);
}

/** Writes the difference into the text, in one go: formats of what stays first, then the replaced stretch. */
export function writeDiff(text: Y.Text, diff: Diff, map: (index: number, after: boolean) => number): void {
  for (const f of diff.formats) {
    const from = map(f.index, true);
    const to = map(f.index + f.length, false);
    if (to > from) text.format(from, to - from, f.attrs);
  }
  const from = map(diff.start, true);
  const to = Math.max(from, map(diff.end, false));
  if (to > from) text.delete(from, to - from);
  let at = from;
  for (const run of runsOf(diff.insert)) {
    const first = run.units[0];
    if (!first) continue;
    // Always given a set of formats, even none: an insert without one takes those around it.
    if (first.embed) {
      text.insertEmbed(at, first.embed.br === "[]" ? { br: JSON.stringify([["", "data-pid", freshId("x")]]) } : first.embed, first.attrs);
      at += 1;
    } else {
      const chars = run.units.map((u) => u.c).join("");
      text.insert(at, chars, first.attrs);
      at += chars.length;
    }
  }
}
