// @vitest-environment node
import { describe, expect, it } from "vitest";
import { applyChange, editable, firstPlace, insertables, multiline, newElement, textAt, withText, type Slide, type Words } from "./fields";

const slide: Slide = {
  title: "Targets",
  notes: "n",
  elements: [
    { type: "text", text: "Line one" },
    { type: "table", columns: ["Metric", "Now"], rows: [["Revenue", { text: "$1.1M", tone: "warning" }], ["Churn", { tone: "success" }]] },
    { type: "stats", items: [{ value: 42, label: "answers" }] },
    { type: "steps", items: ["Queue", { title: "Charge", text: "Idempotent" }] },
    { type: "columns", columns: [{ title: "Left", elements: [{ type: "text", text: "in a column" }] }, { elements: [{ type: "bullets", items: ["a"] }] }] },
  ],
};

const words: Words = { text: "New text", point: "New point", column: (n) => `Column ${String(n)}`, card: (n) => `Card ${String(n)}`, cardText: "What it is", statValue: "42", statLabel: "What it counts" };

describe("the strings of a slide by their place", () => {
  it("reads every kind of place, and nothing where the slide has no string", () => {
    const cases: [string, string | null][] = [
      ["title", "Targets"],
      ["elements/0/text", "Line one"],
      ["elements/1/columns/1", "Now"],
      ["elements/1/rows/0/1", "$1.1M"],
      ["elements/1/rows/1/1", ""],
      ["elements/2/items/0/value", "42"],
      ["elements/3/items/0/title", "Queue"],
      ["elements/3/items/1/text", "Idempotent"],
      ["elements/4/columns/0/elements/0/text", "in a column"],
      ["elements/4/columns/1/elements/0/items/0", "a"],
      ["elements/9/text", null],
      ["elements/0/type/x", null],
      ["elements/1/rows/0/7", null],
      ["elements/3/items/0/text", null],
      ["notes/0", null],
      ["elements", null],
      ["elements/-1/text", null],
    ];
    for (const [path, want] of cases) expect([path, textAt(slide, path)]).toEqual([path, want]);
  });

  it("writes a new slide with only that string changed and leaves the one it was given alone", () => {
    const before = structuredClone(slide);
    const cell = withText(slide, "elements/1/rows/0/1", "$1.4M");
    expect(cell).not.toBe(slide);
    expect(slide).toEqual(before);
    expect((cell?.elements as unknown[])[1]).toEqual({ type: "table", columns: ["Metric", "Now"], rows: [["Revenue", { text: "$1.4M", tone: "warning" }], ["Churn", { tone: "success" }]] });
    expect((cell?.elements as unknown[])[0]).toBe((slide.elements as unknown[])[0]);
    expect(textAt(withText(slide, "elements/1/rows/1/1", "ok"), "elements/1/rows/1/1")).toBe("ok");
    expect(textAt(withText(slide, "elements/2/items/0/value", "43"), "elements/2/items/0/value")).toBe("43");
    expect(textAt(withText(slide, "elements/3/items/0/title", "Accept"), "elements/3/items/0")).toBe("Accept");
    expect(withText(slide, "title", "中文標題")?.title).toBe("中文標題");
    expect(withText(slide, "elements/9/text", "x")).toBeNull();
    expect(withText(slide, "", "x")).toBeNull();
  });

  it("takes exactly the places the deck's page marks, by element type, as editable", () => {
    const every: Slide = {
      title: "T",
      elements: [
        { type: "text", text: "a" },
        { type: "bullets", items: ["a"] },
        { type: "table", columns: ["h"], rows: [["c", { text: "t", tone: "success" }]] },
        { type: "stats", items: [{ value: 7, label: "l" }] },
        { type: "quote", text: "q", by: "b" },
        { type: "steps", items: ["s", { title: "st", text: "sx" }] },
        { type: "columns", columns: [{ title: "ct", elements: [{ type: "numbered", items: ["n"] }] }, { elements: [{ type: "text", text: "in" }] }] },
        { type: "cards", items: [{ title: "c", text: "ct", items: ["ci"] }, { title: "d" }] },
      ],
    };
    const marked = ["title", "elements/0/text", "elements/1/items/0", "elements/2/columns/0", "elements/2/rows/0/0", "elements/2/rows/0/1", "elements/3/items/0/value", "elements/3/items/0/label",
      "elements/4/text", "elements/4/by", "elements/5/items/0/title", "elements/5/items/1/title", "elements/5/items/1/text", "elements/6/columns/0/title", "elements/6/columns/0/elements/0/items/0",
      "elements/6/columns/1/elements/0/text", "elements/7/items/0/title", "elements/7/items/0/text", "elements/7/items/0/items/0", "elements/7/items/1/title"];
    for (const path of marked) expect([path, editable(every, path)]).toEqual([path, true]);
    const code: Slide = { title: "C", elements: [{ type: "code", code: "x", language: "go" }, { type: "progress", items: [{ label: "l", value: 5, text: "half" }] }] };
    for (const path of ["elements/0/code", "elements/0/language", "elements/0/type", "elements/1/items/0/text", "elements/1/type", "elements/2/columns/0", "notes"]) expect([path, editable({ ...every, ...code, notes: "n" }, path)]).toEqual([path, false]);
    expect(editable(code, "elements/1/items/0/label")).toBe(true);
  });

  it("lets only a text element's text hold line breaks", () => {
    expect(multiline(slide, "elements/0/text")).toBe(true);
    expect(multiline(slide, "elements/4/columns/0/elements/0/text")).toBe(true);
    expect(multiline(slide, "title")).toBe(false);
    expect(multiline(slide, "elements/3/items/1/text")).toBe(false);
  });
});

describe("the insert tool's elements", () => {
  it("starts each kind with content the format accepts, and its first place to type", () => {
    expect(insertables.map((k) => newElement(k, words))).toEqual([
      { type: "text", text: "New text" },
      { type: "bullets", items: ["New point"] },
      { type: "table", columns: ["Column 1", "Column 2"], rows: [["", ""], ["", ""]] },
      { type: "cards", items: [{ title: "Card 1", text: "What it is" }, { title: "Card 2", text: "What it is" }] },
      { type: "stats", items: [{ value: "42", label: "What it counts" }] },
    ]);
    for (const k of insertables) {
      const added = applyChange(slide, { op: "insert", kind: k, element: newElement(k, words) });
      const at = (added?.elements as unknown[]).length - 1;
      expect(at).toBe(5);
      expect(textAt(added, firstPlace(k, at))).not.toBeNull();
    }
  });

  it("adds at the end, to a title slide with no elements too, and not to a full slide", () => {
    const element = newElement("text", words);
    expect(applyChange({ layout: "title", title: "Hi" }, { op: "insert", kind: "text", element })).toEqual({ layout: "title", title: "Hi", elements: [element] });
    const full = { title: "Full", elements: Array.from({ length: 8 }, () => element) };
    expect(applyChange(full, { op: "insert", kind: "text", element })).toBeNull();
    expect(applyChange(slide, { op: "text", path: "elements/9/text", text: "x" })).toBeNull();
  });
});
