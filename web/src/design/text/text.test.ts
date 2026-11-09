// @vitest-environment node
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { diffUnits, throughChanges, writeDiff } from "./diff";
import { htmlOfDelta } from "./html";
import { parseRuns, unitsOfDelta, unitsOfRuns, type DeltaOp, type Run } from "./units";

const bold = (pid: string, depth = 0) => ({ [`f:${pid}`]: JSON.stringify({ t: "b", d: depth, a: [["", "data-pid", pid]] }) });
/** A text with these runs written into it, and what it reads back as. */
const textOf = (delta: DeltaOp[]) => {
  const doc = new Y.Doc();
  const t = doc.getText("t");
  t.applyDelta(delta);
  return t;
};

// The least change between two texts (words, emoji, formats) is a case of internal/designroom/testdata/diff, run by diffFixtures.test.ts.
describe("what changed between two texts", () => {
  it("sees a break that stayed as the same break, whatever the frame says of its attributes", () => {
    const before = unitsOfDelta([{ insert: "a" }, { insert: { br: '[["","data-pid","e4"]]' } }, { insert: "b" }]);
    const after = unitsOfRuns([{ s: "a", f: [] }, { br: true, f: [] }, { s: "bc", f: [] }], before);
    const d = diffUnits(before, after);
    expect({ start: d.start, end: d.end, insert: d.insert.map((u) => u.c).join("") }).toEqual({ start: 3, end: 3, insert: "c" });
  });
});

describe("the formats of what the frame says", () => {
  it("keeps the spelling the room has for an element that was there, and gives one the frame made an id of its own", () => {
    const old = { "f:e3": JSON.stringify({ t: "b", d: 0, a: [["", "data-pid", "e3"], ["", "style", "color:red"]] }) };
    const before = unitsOfDelta([{ insert: "x", attributes: old }]);
    const after = unitsOfRuns(
      [
        { s: "x", f: [{ t: "b", p: "e3" }] },
        { s: "y", f: [{ t: "i", p: "n9" }] },
      ],
      before,
    );
    expect(after[0]?.attrs).toEqual(old);
    expect(after[1]?.attrs).toEqual({ "f:n9": JSON.stringify({ t: "i", d: 0, a: [["", "data-pid", "n9"]] }) });
  });

  it("reads an element's depth from where it now is, and keeps the text of a value Go escaped as it was", () => {
    const escaped = '{"t":"b","d":0,"a":[["","title","a\\u003cb"]]}';
    const before = unitsOfDelta([{ insert: "x", attributes: { "f:e3": escaped } }]);
    expect(unitsOfRuns([{ s: "x", f: [{ t: "b", p: "e3" }] }], before)[0]?.attrs["f:e3"]).toBe(escaped);
    const deeper = unitsOfRuns([{ s: "x", f: [{ t: "i", p: "e5" }, { t: "b", p: "e3" }] }], before)[0]?.attrs["f:e3"];
    expect(JSON.parse(deeper ?? "{}")).toEqual({ t: "b", d: 1, a: [["", "title", "a<b"]] });
  });

  it("is not text when what the frame says is not a list of runs with the elements they are in", () => {
    // The frame's break is a 1.
    const wire = [{ s: "a", f: [] }, { br: 1, f: [] }, { raw: "<mark>m</mark>", f: [{ t: "b", p: "e3" }] }];
    const good: Run[] = [{ s: "a", f: [] }, { br: true, f: [] }, { raw: "<mark>m</mark>", f: [{ t: "b", p: "e3" }] }];
    expect(parseRuns(wire)).toEqual(good);
    for (const bad of ["x", [1], [{ s: "a" }], [{ s: "a", f: [{ t: "b" }] }], [{ f: [] }], [{ br: true, f: [] }], [{ s: 4, f: [] }]]) expect(parseRuns(bad)).toBeNull();
  });
});

describe("where a position is after other people's changes", () => {
  const cases: [string, number, DeltaOp[], boolean, number][] = [
    ["before an insert", 8, [{ retain: 3 } as never, { insert: "xyz" }], true, 11],
    ["after an insert", 2, [{ retain: 3 } as never, { insert: "xyz" }], true, 2],
    ["exactly at an insert, when it is to stay behind", 3, [{ retain: 3 } as never, { insert: "xyz" }], true, 6],
    ["exactly at an insert, when it is to stay in front", 3, [{ retain: 3 } as never, { insert: "xyz" }], false, 3],
    ["inside a deletion", 5, [{ retain: 3 } as never, { delete: 4 } as never], true, 3],
    ["after a deletion", 9, [{ retain: 3 } as never, { delete: 4 } as never], true, 5],
    ["past an embed", 4, [{ insert: { br: "[]" } }], true, 5],
  ];
  for (const [name, index, delta, after, want] of cases) {
    it(`finds a position ${name}`, () => {
      expect(throughChanges(index, [delta as never], after)).toBe(want);
    });
  }

  it("goes through changes one after the other", () => {
    expect(throughChanges(5, [[{ insert: "ab" }], [{ retain: 4 }, { delete: 1 }]], true)).toBe(6);
  });
});

describe("the markup of a text", () => {
  it("opens a format where its stretch starts and closes it where it ends, nested by depth, with the text escaped", () => {
    const delta: DeltaOp[] = [
      { insert: "a <b> & " },
      { insert: "bold ", attributes: bold("e3") },
      { insert: "both", attributes: { ...bold("e3"), "f:e4": JSON.stringify({ t: "i", d: 1, a: [["", "data-pid", "e4"]] }) } },
      { insert: { br: '[["","data-pid","e5"]]' } },
      { insert: { raw: "<mark>m</mark>" } },
      { insert: "end" },
    ];
    expect(htmlOfDelta(delta)).toBe('a &lt;b&gt; &amp; <b data-pid="e3">bold <i data-pid="e4">both</i></b><br data-pid="e5"><mark>m</mark>end');
  });

  it("makes a link to the outside open in a new tab that knows nothing of the page, as a published one does", () => {
    const link = (rows: string[][]) => htmlOfDelta([{ insert: "go", attributes: { "f:e6": JSON.stringify({ t: "a", d: 0, a: rows }) } }]);
    expect(link([["", "href", "https://x.test/?a=1&b=2"]])).toBe('<a href="https://x.test/?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">go</a>');
    expect(link([["", "href", "mailto:a@x.test"], ["", "target", "_self"], ["", "rel", "opener"]])).toBe('<a href="mailto:a@x.test" target="_blank" rel="noopener noreferrer">go</a>');
    expect(link([["", "href", "#top"]])).toBe('<a href="#top">go</a>');
  });

  it("shows nothing that is not text: an element text does not hold, an event attribute, a name that is not one", () => {
    const f = (t: string, rows: string[][]) => ({ insert: "x", attributes: { "f:e7": JSON.stringify({ t, d: 0, a: rows }) } });
    expect(htmlOfDelta([f("script", [])])).toBe("x");
    expect(htmlOfDelta([f("b", [["", "onclick", "x()"], ["", 'a"b', "v"], ["svg", "k", "v"]])])).toBe("<b>x</b>");
  });
});

describe("writing a difference into a text", () => {
  const through = (i: number) => i;
  const write = (text: Y.Text, before: DeltaOp[], runs: Run[]) => {
    const was = unitsOfDelta(before);
    writeDiff(text, diffUnits(was, unitsOfRuns(runs, was)), through);
  };

  it("replaces the middle and gives what it inserts the formats the frame says, not those around it", () => {
    const before: DeltaOp[] = [{ insert: "one " }, { insert: "two", attributes: bold("e3") }, { insert: " three" }];
    const t = textOf(before);
    write(t, before, [{ s: "one ", f: [] }, { s: "2", f: [{ t: "b", p: "e3" }] }, { s: " plain ", f: [] }, { s: "three", f: [] }]);
    expect(htmlOfDelta(t.toDelta() as DeltaOp[])).toBe('one <b data-pid="e3">2</b> plain three');
  });

  it("writes a break and a made bold word, and a line typed after the break is not bold", () => {
    const t = textOf([{ insert: "ab" }]);
    write(t, [{ insert: "ab" }], [{ s: "a", f: [{ t: "b", p: "x1" }] }, { br: true, f: [] }, { s: "b", f: [] }]);
    expect(htmlOfDelta(t.toDelta() as DeltaOp[])).toMatch(/^<b data-pid="x1">a<\/b><br data-pid="x[0-9a-z]+">b$/);
  });
});
