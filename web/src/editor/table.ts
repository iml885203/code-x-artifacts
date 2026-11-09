// Tables: insert, rows and columns on any side, delete, the header switch and
// column alignment. Every command changes the cells it names and leaves the
// others (and their ids' absence) as they are; a table's cells are plain
// paragraphs the model has no ids for.
//
// Alignment belongs to a column, not a cell: Markdown can say it only that way
// (docs/doc-format.md), so a table the editor makes always survives the trip.
import type { Node, NodeType } from "prosemirror-model";
import { Plugin, TextSelection, type Command, type EditorState, type Transaction } from "prosemirror-state";
import { ySyncPluginKey } from "y-prosemirror";
import { schema } from "./schema";

/** The limits a doc may hold (internal/docfmt/model.go). */
export const maxColumns = 30;
export const maxCells = 2000;

export type Align = "left" | "center" | "right";

export type TableTool = "insert" | "rowAbove" | "rowBelow" | "columnLeft" | "columnRight" | "deleteRow" | "deleteColumn" | "deleteTable" | "header" | Align;

/** What the toolbar shows about the table the caret is in. */
export interface TableInfo {
  header: boolean;
  align: Align | null;
  rows: number;
  columns: number;
}

const type = (name: string): NodeType => {
  const t = schema.nodes[name];
  if (t === undefined) throw new Error(`the editor's schema has no node ${name}`);
  return t;
};

/** Where the caret is in a table: the table's position, and the row and column of the cell. */
interface Place {
  table: Node;
  /** Position before the table. */
  at: number;
  row: number;
  column: number;
}

function placeOf(state: EditorState): Place | null {
  const { $from } = state.selection;
  let cell = -1;
  let tableDepth = -1;
  for (let d = $from.depth; d > 0; d--) {
    const name = $from.node(d).type.name;
    if (name === "table_cell" && cell < 0) cell = d;
    if (name === "table") {
      tableDepth = d;
      break;
    }
  }
  if (cell < 0 || tableDepth < 0) return null;
  return { table: $from.node(tableDepth), at: $from.before(tableDepth), row: $from.index(tableDepth), column: $from.index(tableDepth + 1) };
}

const alignOf = (cell: Node): Align | null => {
  const a: unknown = cell.attrs.align;
  return a === "left" || a === "center" || a === "right" ? a : null;
};

export function tableInfo(state: EditorState): TableInfo | null {
  const place = placeOf(state);
  if (place === null) return null;
  const first = place.table.firstChild;
  const cell = place.table.child(place.row).maybeChild(place.column);
  return { header: place.table.attrs.header === true, align: cell === null ? null : alignOf(cell), rows: place.table.childCount, columns: first?.childCount ?? 0 };
}

const emptyCell = (align: Align | null): Node => {
  const made = type("table_cell").createAndFill({ align });
  if (made === null) throw new Error("a table cell could not be made");
  return made;
};

/** Position of the row at `index` of a table at `at`, and of its cell at `column`. */
function rowAt(table: Node, at: number, index: number): number {
  let pos = at + 1;
  for (let i = 0; i < index; i++) pos += table.child(i).nodeSize;
  return pos;
}

function cellAt(table: Node, at: number, row: number, column: number): number {
  let pos = rowAt(table, at, row) + 1;
  const r = table.child(row);
  for (let i = 0; i < column; i++) pos += r.child(i).nodeSize;
  return pos;
}

/** The caret in the first text of the cell at the row and column. */
function caretIn(tr: Transaction, at: number, row: number, column: number): Transaction {
  const table = tr.doc.nodeAt(at);
  if (table === null) return tr;
  const r = Math.min(row, table.childCount - 1);
  const c = Math.min(column, table.child(r).childCount - 1);
  return tr.setSelection(TextSelection.create(tr.doc, cellAt(table, at, r, c) + 2));
}

/** Inserts a table of `rows` by `columns` after the block the caret is in (in place of an empty paragraph), with a header row, the caret in its first cell. */
export const insertTable = (rows = 3, columns = 3): Command => (state, dispatch) => {
  const { $from } = state.selection;
  const depth = $from.depth;
  if (placeOf(state) !== null || !$from.parent.isTextblock || depth < 1) return false;
  const container = $from.node(depth - 1);
  const index = $from.index(depth - 1);
  const table = type("table");
  if (!container.canReplaceWith(index, index + 1, table)) return false;
  if (dispatch) {
    const made = table.create({ header: true }, Array.from({ length: rows }, () => type("table_row").create(null, Array.from({ length: columns }, () => emptyCell(null)))));
    const current = $from.parent;
    const replace = current.type.name === "paragraph" && current.content.size === 0;
    const from = replace ? $from.before(depth) : $from.after(depth);
    // Typing continues after a table that ends the container, so one paragraph follows it there.
    const last = index === container.childCount - 1;
    const tr = state.tr.replaceWith(from, replace ? $from.after(depth) : from, last ? [made, type("paragraph").create()] : made);
    dispatch(caretIn(tr, from, 0, 0).scrollIntoView());
  }
  return true;
};

/** Adds a row above or below the caret's row; the new row's cells take the alignment of their columns. */
export const addRow = (where: "above" | "below"): Command => (state, dispatch) => {
  const place = placeOf(state);
  const width = place?.table.firstChild?.childCount ?? 0;
  if (place === null || (place.table.childCount + 1) * width > maxCells) return false;
  if (dispatch) {
    const reference = place.table.child(place.row);
    const row = type("table_row").create(null, Array.from({ length: reference.childCount }, (_, c) => emptyCell(alignOf(reference.child(c)))));
    const index = where === "above" ? place.row : place.row + 1;
    const tr = state.tr.insert(rowAt(place.table, place.at, index), row);
    dispatch(caretIn(tr, place.at, index, place.column).scrollIntoView());
  }
  return true;
};

/** Adds a column left or right of the caret's, with no alignment of its own. */
export const addColumn = (where: "left" | "right"): Command => (state, dispatch) => {
  const place = placeOf(state);
  const width = place?.table.firstChild?.childCount ?? 0;
  if (place === null || width >= maxColumns || place.table.childCount * (width + 1) > maxCells) return false;
  if (dispatch) {
    const index = where === "left" ? place.column : place.column + 1;
    const tr = state.tr;
    for (let r = place.table.childCount - 1; r >= 0; r--) {
      tr.insert(cellAt(place.table, place.at, r, index), emptyCell(null));
    }
    dispatch(caretIn(tr, place.at, place.row, index).scrollIntoView());
  }
  return true;
};

/** Deletes the caret's row; the last row cannot be deleted, the table can. */
export const deleteRow: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null || place.table.childCount < 2) return false;
  if (dispatch) {
    const from = rowAt(place.table, place.at, place.row);
    const tr = state.tr.delete(from, from + place.table.child(place.row).nodeSize);
    dispatch(caretIn(tr, place.at, place.row, place.column).scrollIntoView());
  }
  return true;
};

/** Deletes the caret's column; the last column cannot be deleted, the table can. */
export const deleteColumn: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null || (place.table.firstChild?.childCount ?? 0) < 2) return false;
  if (dispatch) {
    const tr = state.tr;
    for (let r = place.table.childCount - 1; r >= 0; r--) {
      const from = cellAt(place.table, place.at, r, place.column);
      tr.delete(from, from + place.table.child(r).child(place.column).nodeSize);
    }
    dispatch(caretIn(tr, place.at, place.row, place.column).scrollIntoView());
  }
  return true;
};

/** Deletes the table; the caret goes to the block after it, or the one before. */
export const deleteTable: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null) return false;
  if (dispatch) {
    const tr = state.tr;
    const end = place.at + place.table.nodeSize;
    const parent = tr.doc.resolve(place.at).parent;
    const only = parent.childCount === 1;
    if (only) tr.replaceWith(place.at, end, type("paragraph").create());
    else tr.delete(place.at, end);
    const target = Math.min(place.at, tr.doc.content.size);
    dispatch(tr.setSelection(TextSelection.near(tr.doc.resolve(target), -1)).scrollIntoView());
  }
  return true;
};

/** Turns the table's header row on or off. */
export const toggleHeader: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null) return false;
  dispatch?.(state.tr.setNodeMarkup(place.at, undefined, { ...place.table.attrs, header: place.table.attrs.header !== true }));
  return true;
};

/** Aligns every cell of the caret's column; the alignment the column already has is taken off again. */
export const setAlign = (align: Align): Command => (state, dispatch) => {
  const place = placeOf(state);
  if (place === null) return false;
  if (dispatch) {
    const current = alignOf(place.table.child(place.row).child(place.column));
    const next = current === align ? null : align;
    const tr = state.tr;
    for (let r = 0; r < place.table.childCount; r++) {
      const at = cellAt(place.table, place.at, r, place.column);
      const cell = place.table.child(r).child(place.column);
      tr.setNodeMarkup(at, undefined, { ...cell.attrs, align: next });
    }
    dispatch(tr);
  }
  return true;
};

/** Tab goes to the next cell, and in the last cell adds a row; Shift-Tab goes back. */
export const nextCell: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null) return false;
  const width = place.table.child(place.row).childCount;
  if (place.column + 1 < width) {
    dispatch?.(caretIn(state.tr, place.at, place.row, place.column + 1).scrollIntoView());
    return true;
  }
  if (place.row + 1 < place.table.childCount) {
    dispatch?.(caretIn(state.tr, place.at, place.row + 1, 0).scrollIntoView());
    return true;
  }
  // At the cell limit the table cannot grow and the caret stays; Tab never leaves the table.
  addRow("below")(state, dispatch && ((tr) => {
    dispatch(caretIn(tr, place.at, place.row + 1, 0));
  }));
  return true;
};

export const previousCell: Command = (state, dispatch) => {
  const place = placeOf(state);
  if (place === null) return false;
  if (place.column > 0) dispatch?.(caretIn(state.tr, place.at, place.row, place.column - 1).scrollIntoView());
  else if (place.row > 0) dispatch?.(caretIn(state.tr, place.at, place.row - 1, place.table.child(place.row - 1).childCount - 1).scrollIntoView());
  return true;
};

export const tableCommand = (tool: TableTool): Command => {
  switch (tool) {
    case "insert":
      return insertTable();
    case "rowAbove":
      return addRow("above");
    case "rowBelow":
      return addRow("below");
    case "columnLeft":
      return addColumn("left");
    case "columnRight":
      return addColumn("right");
    case "deleteRow":
      return deleteRow;
    case "deleteColumn":
      return deleteColumn;
    case "deleteTable":
      return deleteTable;
    case "header":
      return toggleHeader;
    default:
      return setAlign(tool);
  }
};

/** The cell a position is in, or -1 when it is not in one. */
function cellStart(doc: Node, pos: number): number {
  const $pos = doc.resolve(pos);
  for (let d = $pos.depth; d > 0; d--) if ($pos.node(d).type.name === "table_cell") return $pos.before(d);
  return -1;
}

/** A person's own deletion never reaches across cells: a range that starts in one cell and ends in another (or outside the table) would tear the table apart. Edits that arrive from the room are not judged here, they are already someone's decision. */
export const keepCellsApart = new Plugin({
  filterTransaction: (tr) => {
    if (tr.getMeta(ySyncPluginKey) !== undefined) return true;
    return tr.steps.every((step, i) => {
      const before = tr.docs[i];
      const { from, to } = step.toJSON() as { from?: unknown; to?: unknown };
      if (before === undefined || typeof from !== "number" || typeof to !== "number" || from === to) return true;
      return cellStart(before, from) === cellStart(before, to);
    });
  },
});
