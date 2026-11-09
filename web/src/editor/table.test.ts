import type { EditorView } from "prosemirror-view";
import { redo, undo } from "y-prosemirror";
import * as Y from "yjs";
import { describe, expect, it } from "vitest";
import { webTableClipboard } from "./clipboards";
import { addColumn, addRow, deleteColumn, deleteRow, deleteTable, insertTable, maxCells, maxColumns, nextCell, previousCell, setAlign, tableCommand, tableInfo, toggleHeader, type TableTool } from "./table";
import { join, outline, para, person, reload, room, select, text, typeAt } from "./testRoom";

type Align = "left" | "center" | "right" | null;
const cell = (s: string, align: Align = null) => ({ type: "table_cell", attrs: { align }, content: [{ type: "paragraph", attrs: { id: null }, content: s === "" ? [] : [text(s)] }] });
const row = (...cells: ReturnType<typeof cell>[]) => ({ type: "table_row", content: cells });
const table = (header: boolean, ...rows: ReturnType<typeof row>[]) => ({ type: "table", attrs: { id: "b_t", header }, content: rows });

/** The first table as rows of `text/alignment`, with `*` in front of a header table. */
function grid(view: EditorView): string[] {
  const out: string[] = [];
  view.state.doc.descendants((node) => {
    if (node.type.name !== "table") return true;
    node.forEach((r) => {
      const cells: string[] = [];
      r.forEach((c) => {
        const a = c.attrs.align as Align;
        cells.push((c.textContent === "" ? "_" : c.textContent) + (a === null ? "" : `/${a[0] ?? ""}`));
      });
      out.push(cells.join(" | "));
    });
    if (node.attrs.header === true) out[0] = `*${out[0] ?? ""}`;
    return false;
  });
  return out;
}

const three = () => room(para("b_1", "before"), table(true, row(cell("a1"), cell("b1"), cell("c1")), row(cell("a2"), cell("b2"), cell("c2")), row(cell("a3"), cell("b3"), cell("c3"))), para("b_2", "after"));
const run = (view: EditorView, command: (s: EditorView["state"], d: EditorView["dispatch"]) => boolean) => command(view.state, view.dispatch);

describe("inserting a table", () => {
  it("puts a 3 by 3 table with a header after the caret's block, in an empty paragraph's place, and survives Undo, Redo and reload", () => {
    const doc = room(para("b_1", "intro"), para("b_2", ""));
    const view = person(doc);
    select(view, "intro", 5);
    expect(run(view, insertTable())).toBe(true);
    expect(grid(view)).toEqual(["*_ | _ | _", "_ | _ | _", "_ | _ | _"]);
    expect(outline(view)).toContain("p(intro)");
    expect(tableInfo(view.state)).toMatchObject({ header: true, rows: 3, columns: 3, align: null });
    expect(view.state.doc.textContent).toBe("intro");

    undo(view.state);
    expect(grid(view)).toEqual([]);
    redo(view.state);
    expect(grid(view)).toHaveLength(3);
    expect(grid(person(reload(doc)))).toHaveLength(3);
  });

  it("replaces an empty paragraph, and is not offered inside a table", () => {
    const view = person(room(para("b_1", "")));
    run(view, insertTable(2, 2));
    expect(outline(view)).toMatch(/^table\[/);
    expect(run(view, insertTable())).toBe(false);
  });
});

describe("the operations on rows, columns, header and alignment", () => {
  const cases: { name: string; tool: TableTool; at: [string, number]; want: string[] }[] = [
    { name: "adds a row above", tool: "rowAbove", at: ["a2", 1], want: ["*a1 | b1 | c1", "_ | _ | _", "a2 | b2 | c2", "a3 | b3 | c3"] },
    { name: "adds a row below", tool: "rowBelow", at: ["a2", 1], want: ["*a1 | b1 | c1", "a2 | b2 | c2", "_ | _ | _", "a3 | b3 | c3"] },
    { name: "adds a column on the left", tool: "columnLeft", at: ["b2", 1], want: ["*a1 | _ | b1 | c1", "a2 | _ | b2 | c2", "a3 | _ | b3 | c3"] },
    { name: "adds a column on the right", tool: "columnRight", at: ["b2", 1], want: ["*a1 | b1 | _ | c1", "a2 | b2 | _ | c2", "a3 | b3 | _ | c3"] },
    { name: "deletes the row", tool: "deleteRow", at: ["a2", 1], want: ["*a1 | b1 | c1", "a3 | b3 | c3"] },
    { name: "deletes the column", tool: "deleteColumn", at: ["b2", 1], want: ["*a1 | c1", "a2 | c2", "a3 | c3"] },
    { name: "switches the header off", tool: "header", at: ["b2", 1], want: ["a1 | b1 | c1", "a2 | b2 | c2", "a3 | b3 | c3"] },
    { name: "aligns the whole column to the right", tool: "right", at: ["b2", 1], want: ["*a1 | b1/r | c1", "a2 | b2/r | c2", "a3 | b3/r | c3"] },
  ];

  it.each(cases)("$name, changes nothing else, and Undo, Redo and reload keep it", ({ tool, at, want }) => {
    const doc = three();
    const view = person(doc);
    const original = grid(view);
    select(view, at[0], at[1]);
    expect(run(view, tableCommand(tool))).toBe(true);
    expect(grid(view)).toEqual(want);
    expect(outline(view)).toMatch(/^p\(before\) table\[.*\] p\(after\)$/);

    undo(view.state);
    expect(grid(view)).toEqual(original);
    redo(view.state);
    expect(grid(view)).toEqual(want);
    expect(grid(person(reload(doc)))).toEqual(want);
  });

  it("aligns left, centre and right one column at a time, and pressing the alignment a column has takes it off", () => {
    const view = person(three());
    select(view, "a1", 1);
    run(view, setAlign("center"));
    select(view, "c3", 1);
    run(view, setAlign("left"));
    expect(grid(view)).toEqual(["*a1/c | b1 | c1/l", "a2/c | b2 | c2/l", "a3/c | b3 | c3/l"]);
    expect(tableInfo(view.state)?.align).toBe("left");
    run(view, setAlign("left"));
    expect(tableInfo(view.state)?.align).toBeNull();
    expect(grid(view)[2]).toBe("a3/c | b3 | c3");
  });

  it("gives a new row the alignment of its columns, and puts the caret in the new cell", () => {
    const view = person(room(table(true, row(cell("h1", "left"), cell("h2", "right")), row(cell("x", "left"), cell("y", "right")))));
    select(view, "y", 1);
    run(view, addRow("below"));
    expect(grid(view)).toEqual(["*h1/l | h2/r", "x/l | y/r", "_/l | _/r"]);
    expect(view.state.selection.$from.parent.type.name).toBe("paragraph");
    expect(tableInfo(view.state)).toMatchObject({ rows: 3 });
  });

  it("deletes the whole table and leaves the blocks around it, and the last block becomes an empty paragraph", () => {
    const view = person(three());
    select(view, "b2", 1);
    run(view, deleteTable);
    expect(outline(view)).toBe("p(before) p(after)");
    const only = person(room(table(true, row(cell("x")))));
    run(only, deleteTable);
    expect(outline(only)).toBe("p()");
  });

  it("will not delete the last row or the last column, and does not grow past the limits", () => {
    const view = person(room(table(true, row(cell("x")))));
    select(view, "x", 1);
    expect(run(view, deleteRow)).toBe(false);
    expect(run(view, deleteColumn)).toBe(false);
    const wide = person(room(table(true, row(...Array.from({ length: maxColumns }, (_, i) => cell(`c${String(i)}`))))));
    select(wide, "c0", 1);
    expect(run(wide, addColumn("right"))).toBe(false);
    const tall = person(room(table(true, ...Array.from({ length: maxCells / 10 }, (_, i) => row(...Array.from({ length: 10 }, (_, j) => cell(`${String(i)}.${String(j)}`)))))));
    select(tall, "0.0", 1);
    expect(run(tall, addRow("below"))).toBe(false);
  });
});

describe("moving between cells", () => {
  it("Tab goes to the next cell and on to the next row, and Shift-Tab goes back", () => {
    const view = person(three());
    select(view, "c1", 2);
    run(view, nextCell);
    expect(view.state.selection.$from.parent.textContent).toBe("a2");
    run(view, previousCell);
    expect(view.state.selection.$from.parent.textContent).toBe("c1");
    run(view, previousCell);
    expect(view.state.selection.$from.parent.textContent).toBe("b1");
  });

  it("Tab in the last cell adds a row and puts the caret in its first cell; Undo takes the row away", () => {
    const view = person(three());
    select(view, "c3", 2);
    expect(run(view, nextCell)).toBe(true);
    expect(grid(view)).toHaveLength(4);
    expect(grid(view)[3]).toBe("_ | _ | _");
    expect(view.state.selection.$from.parent.textContent).toBe("");
    expect(view.state.selection.$from.index(view.state.selection.$from.depth - 2)).toBe(0);
    undo(view.state);
    expect(grid(view)).toHaveLength(3);
  });

  it("is not a command outside a table", () => {
    const view = person(room(para("b_1", "x")));
    expect(run(view, nextCell)).toBe(false);
    expect(run(view, toggleHeader)).toBe(false);
  });
});

describe("a person's own edit in a cell", () => {
  it("types in a cell and keeps the other cells", () => {
    const view = person(three());
    select(view, "b2", 2);
    view.dispatch(view.state.tr.insertText("!"));
    expect(grid(view)[1]).toBe("a2 | b2! | c2");
  });

  it("cannot delete across cells, or from a cell out of the table: the table keeps its shape", () => {
    const view = person(three());
    const before = grid(view);
    const { doc } = view.state;
    let from = 0;
    let to = 0;
    doc.descendants((node, pos) => {
      if (node.isTextblock && node.textContent === "b1") from = pos + 1;
      if (node.isTextblock && node.textContent === "b2") to = pos + 2;
      return true;
    });
    view.dispatch(view.state.tr.delete(from, to));
    expect(grid(view)).toEqual(before);
    let pastTable = 0;
    doc.descendants((node, pos) => {
      if (node.isTextblock && node.textContent === "after") pastTable = pos + 2;
      return true;
    });
    view.dispatch(view.state.tr.delete(from, pastTable));
    expect(grid(view)).toEqual(before);
  });
});

describe("two people in one table", () => {
  it("edit different cells at once and both edits are kept", () => {
    const a = three();
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    select(alice, "a1", 2);
    alice.dispatch(alice.state.tr.insertText("A"));
    select(bob, "c3", 2);
    bob.dispatch(bob.state.tr.insertText("B"));
    for (const v of [alice, bob]) expect(grid(v)).toEqual(["*a1A | b1 | c1", "a2 | b2 | c2", "a3 | b3 | c3B"]);
  });

  it("one adds a row while the other types in a cell: the row and the typing are both kept", () => {
    const a = three();
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    select(alice, "c3", 2);
    run(alice, addRow("below"));
    typeAt(bob, bob.state.selection.from, "");
    select(bob, "b2", 2);
    bob.dispatch(bob.state.tr.insertText("!"));
    for (const v of [alice, bob]) expect(grid(v)).toEqual(["*a1 | b1 | c1", "a2 | b2! | c2", "a3 | b3 | c3", "_ | _ | _"]);
  });

  it("one adds a row in the middle while the other types below it", () => {
    const a = three();
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    select(alice, "a1", 1);
    run(alice, addRow("below"));
    select(bob, "c3", 2);
    bob.dispatch(bob.state.tr.insertText("?"));
    for (const v of [alice, bob]) expect(grid(v)).toEqual(["*a1 | b1 | c1", "_ | _ | _", "a2 | b2 | c2", "a3 | b3 | c3?"]);
  });
});

describe("pasting a table", () => {
  it("becomes one table block: the header, the alignment of a column, every row as wide as the widest, a cell's paragraphs and marks in one paragraph", () => {
    const view = person(room(para("b_1", "")));
    view.pasteHTML(webTableClipboard, new Event("paste") as ClipboardEvent);
    expect(grid(view)).toEqual(["*Item | Cost/r | Notes", "Desk | 120/r | First and second with a link", "Chair and lamp | Chair and lamp/r | both", "Plant | _/r | _"]);
    const json = JSON.stringify(view.state.doc.toJSON());
    for (const gone of ["steal()", "onclick", "style", "colspan", "border-collapse"]) expect(json, gone).not.toContain(gone);
    expect(view.dom.querySelectorAll("a[href='https://example.com/n']")).toHaveLength(1);
    expect(view.dom.querySelectorAll("strong")).toHaveLength(1);
    expect(tableInfo(view.state)).toMatchObject({ rows: 4, columns: 3 });
  });

  it("inside a table cell flattens to the cell's one paragraph and keeps the table whole", () => {
    const view = person(three());
    select(view, "b2", 2);
    view.pasteHTML("<p>one</p><p>two</p>", new Event("paste") as ClipboardEvent);
    expect(grid(view)).toHaveLength(3);
    expect(grid(view)[1]).toMatch(/^a2 \| b.*\| c2$/);
  });
});
