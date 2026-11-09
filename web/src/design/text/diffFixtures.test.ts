import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { diffUnits } from "./diff";
import { unitsOfDelta, type DeltaOp, type Unit } from "./units";

// The diff of one formatted text against another is written twice, here and in
// internal/designroom (mergerich.go), because the editor and the server are not
// one program. Every file under internal/designroom/testdata/diff is a case both
// run: what the least change between `before` and `after` is.
const dir = join(dirname(fileURLToPath(import.meta.url)), "../../../../internal/designroom/testdata/diff");

interface Case {
  name: string;
  before: DeltaOp[];
  after: DeltaOp[];
  want: { start: number; end: number; insert: DeltaOp[]; formats: { index: number; attrs: Record<string, string | null> }[] };
}

const cases = readdirSync(dir)
  .filter((f) => f.endsWith(".json"))
  .sort()
  .map((f) => JSON.parse(readFileSync(join(dir, f), "utf8")) as Case);

/** Units as the runs of a delta: consecutive characters with the same formats are one run, an embed is its own. */
function deltaOfUnits(units: Unit[]): DeltaOp[] {
  const out: DeltaOp[] = [];
  let last: Unit | undefined;
  for (const u of units) {
    const attributes = Object.keys(u.attrs).length > 0 ? { attributes: u.attrs } : {};
    const prev = out[out.length - 1];
    if (u.embed) out.push({ insert: u.embed, ...attributes });
    else if (last && !last.embed && typeof prev?.insert === "string" && JSON.stringify(last.attrs) === JSON.stringify(u.attrs)) prev.insert += u.c;
    else out.push({ insert: u.c, ...attributes });
    last = u;
  }
  return out;
}

describe("the diff of a formatted text", () => {
  it("has the cases", () => {
    expect(cases.length).toBeGreaterThan(10);
  });

  it.each(cases.map((c) => [c.name, c] as const))("is the least change the shared fixtures say for %s", (_name, c) => {
    const d = diffUnits(unitsOfDelta(c.before), unitsOfDelta(c.after));
    // The fixtures list a format change per unit; the editor writes one per stretch of units.
    const formats = d.formats.flatMap((f) => Array.from({ length: f.length }, (_, i) => ({ index: f.index + i, attrs: f.attrs })));
    expect({ start: d.start, end: d.end, insert: deltaOfUnits(d.insert), formats }).toEqual(c.want);
  });
});
