import { toggleMark } from "prosemirror-commands";
import { render, screen, fireEvent } from "@testing-library/react";
import { translator } from "../../../i18n";
import { catalogues } from "../../../test-support";
import { date } from "./index";
import { DOMParser } from "prosemirror-model";
import { NodeSelection, TextSelection, type Command } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { redo, undo } from "y-prosemirror";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { features } from "../../features";
import { schema } from "../../schema";
import { blocks, join, outline, para, person, reload, room, select, text, typeAt } from "../../testRoom";
import type * as Y from "yjs";
import { UndoKeeper } from "../../undoKeeper";
import { toggleList } from "../../lists";
import { cancelDate, chooseDate, datePickerKey, insertDate, validDate } from "./commands";

const run = (view: EditorView, command: Command) => command(view.state, view.dispatch, view);
let registeredHere = false;
beforeAll(() => {
  if (!features.some((feature) => feature.id === date.id)) { features.push(date); registeredHere = true; }
});
afterAll(() => {
  if (registeredHere) features.splice(features.indexOf(date), 1);
});
const datePerson = (doc: Y.Doc, editable = true, undoManager?: Y.UndoManager) => person(doc, editable, "t1", undoManager);

const field = (date = "2026-10-24") => ({ type: "date", attrs: { date } });
const paragraph = () => ({ type: "paragraph", attrs: { id: "p" }, content: [text("Due "), field(), text(" now"), field("2026-12-01")] });
const dates = (view: EditorView): string[] => {
  const values: string[] = [];
  view.state.doc.descendants((node) => { if (node.type.name === "date") values.push(node.attrs.date as string); });
  return values;
};
function pickFirst(view: EditorView): void {
  view.state.doc.descendants((node, pos) => {
    if (node.type.name === "date" && datePickerKey.getState(view.state) === null) {
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, pos)));
      run(view, insertDate);
    }
  });
}

describe("a date in the shared document", () => {
  it("probes and cancels without writing, then submits as its own Undo step between typing with the tab's shared manager", () => {
    const doc = room(para("p", "Due "));
    const keeper = new UndoKeeper();
    const view = datePerson(doc, true, keeper.of(doc, "t1"));
    select(view, "Due ", 4);
    typeAt(view, view.state.selection.from, "before ");
    const before = view.state.doc.toJSON() as unknown;
    expect(insertDate(view.state)).toBe(true);
    expect(datePickerKey.getState(view.state)).toBeNull();
    expect(view.state.doc.toJSON()).toEqual(before);
    expect(run(view, insertDate)).toBe(true);
    expect(run(view, cancelDate)).toBe(true);
    expect(view.state.doc.toJSON()).toEqual(before);
    run(view, insertDate);
    expect(run(view, chooseDate("2026-10-24"))).toBe(true);
    const at = view.state.selection.to;
    typeAt(view, at, " after");
    undo(view.state);
    expect(dates(view)).toEqual(["2026-10-24"]);
    expect(view.state.doc.textContent).toBe("Due before ");
    undo(view.state);
    expect(dates(view)).toEqual([]);
    expect(view.state.doc.textContent).toBe("Due before ");
    undo(view.state);
    expect(view.state.doc.textContent).toBe("Due ");
    redo(view.state); redo(view.state);
    expect(dates(person(reload(doc)))).toEqual(["2026-10-24"]);
    expect(blocks(view)[0]?.attrs).toMatchObject({ id: "p" });
    keeper.release();
  });

  it("follows the insertion caret while a peer types before it and leaves the peer's typing after Undo", () => {
    const a = room(para("p", "Due now"));
    const b = reload(a);
    join(a, b);
    const view = datePerson(a);
    const peer = person(b);
    select(view, "Due now", 4);
    run(view, insertDate);
    select(peer, "Due now", 0);
    typeAt(peer, peer.state.selection.from, "Peer: ");
    expect(run(view, chooseDate("2026-10-24"))).toBe(true);
    expect(view.dom.textContent).toBe("Peer: Due 2026-10-24now");
    expect(dates(peer)).toEqual(["2026-10-24"]);
    undo(view.state);
    expect(view.state.doc.textContent).toBe("Peer: Due now");
  });

  it("changes the same existing date after peer edits and refuses a deleted target instead of changing its neighbour", () => {
    const a = room(paragraph());
    const b = reload(a);
    join(a, b);
    const view = datePerson(a);
    const peer = person(b);
    pickFirst(view);
    select(peer, "Due ", 0);
    typeAt(peer, peer.state.selection.from, "Peer: ");
    expect(run(view, chooseDate("2026-10-25"))).toBe(true);
    expect(dates(peer)).toEqual(["2026-10-25", "2026-12-01"]);
    undo(view.state);
    expect(dates(view)).toEqual(["2026-10-24", "2026-12-01"]);
    expect(view.state.doc.textContent).toBe("Peer: Due  now");
    pickFirst(view);
    let target = -1;
    peer.state.doc.descendants((node, pos) => { if (node.type.name === "date" && target < 0) target = pos; });
    peer.dispatch(peer.state.tr.delete(target, target + 1));
    const before = view.state.doc.toJSON() as unknown;
    expect(run(view, chooseDate("2026-11-01"))).toBe(false);
    expect(view.state.doc.toJSON()).toEqual(before);
    expect(dates(view)).toEqual(["2026-12-01"]);
  });

  it("opens a selected date with Enter while leaving list Enter and readers on their own paths", () => {
    const view = datePerson(room(paragraph()));
    let pos = 0;
    view.state.doc.descendants((node, at) => { if (node.type.name === "date" && pos === 0) pos = at; });
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, pos)));
    const before = view.state.doc.toJSON() as unknown;
    fireEvent.keyDown(view.dom, { key: "Enter", code: "Enter" });
    expect(datePickerKey.getState(view.state)?.initial).toBe("2026-10-24");
    expect(view.state.doc.toJSON()).toEqual(before);
    run(view, cancelDate);
    fireEvent.keyDown(view.dom, { key: "Enter", code: "Enter", ctrlKey: true });
    expect(datePickerKey.getState(view.state)).toBeNull();
    fireEvent.keyDown(view.dom, { key: "Enter", code: "Enter", isComposing: true });
    expect(datePickerKey.getState(view.state)).toBeNull();

    const doc = room(para("p_list", "one"));
    const list = datePerson(doc);
    select(list, "one", 3);
    run(list, toggleList("bullet_list"));
    fireEvent.keyDown(list.dom, { key: "Enter", code: "Enter" });
    expect(outline(list)).toBe("ul[li[p(one)] li[p()]]");
    expect(outline(person(reload(doc)))).toBe("ul[li[p(one)] li[p()]]");
    expect(datePickerKey.getState(list.state)).toBeNull();

    const reader = datePerson(room(paragraph()), false);
    reader.dispatch(reader.state.tr.setSelection(NodeSelection.create(reader.state.doc, pos)));
    const readOnly = reader.state.doc.toJSON() as unknown;
    fireEvent.keyDown(reader.dom, { key: "Enter", code: "Enter" });
    expect(datePickerKey.getState(reader.state)).toBeFalsy();
    expect(reader.state.doc.toJSON()).toEqual(readOnly);
  });

  it("keeps the atom bare when formatting spans the date", () => {
    const view = datePerson(room(paragraph()));
    const bold = schema.marks.bold;
    if (!bold) throw new Error("missing bold");
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2, view.state.doc.content.size - 2)));
    run(view, toggleMark(bold));
    view.state.doc.descendants((node) => { if (node.type.name === "date") expect(node.marks).toEqual([]); });
    expect(dates(view)).toEqual(["2026-10-24", "2026-12-01"]);
  });

  it("keeps readers static and rejects invalid values and code blocks", () => {
    const reader = datePerson(room(paragraph()), false);
    expect(reader.dom.querySelectorAll("input")).toHaveLength(0);
    expect(reader.dom.querySelector("time")?.textContent).toBe("2026-10-24");
    expect(run(reader, insertDate)).toBe(false);
    const view = datePerson(room(para("p", "x")));
    select(view, "x", 1);
    run(view, insertDate);
    expect(run(view, chooseDate("2026-02-29"))).toBe(false);
    expect(dates(view)).toEqual([]);
    const code = datePerson(room({ type: "code_block", attrs: { id: "c" }, content: [text("code")] }));
    select(code, "code", 0);
    expect(insertDate(code.state)).toBe(false);
  });
});

describe("calendar values and clipboard", () => {
  it.each(["0001-01-01", "9999-12-31", "2024-02-29", "2000-02-29"])("accepts %s", (value) => { expect(validDate(value)).toBe(true); });
  it.each(["0000-01-01", "1900-02-29", "2026-04-31", "2026-13-01", "2026-1-01", "2026-01-01T00:00:00Z", "", null, 42])("refuses %s", (value) => { expect(validDate(value)).toBe(false); });
  it("parses only valid dates and never adopts arbitrary HTML attributes", () => {
    const root = document.createElement("div");
    const time = document.createElement("time");
    time.setAttribute("datetime", "2026-10-24");
    time.setAttribute("onclick", "evil()");
    time.textContent = "ignored";
    root.append(time);
    const parsed = DOMParser.fromSchema(schema).parse(root);
    expect(parsed.firstChild?.firstChild?.firstChild?.toJSON()).toEqual(field());
    time.setAttribute("datetime", "2026-02-29");
    expect(DOMParser.fromSchema(schema).parse(root).textContent).toBe("ignored");
  });
});

describe("the date picker row", () => {
  it("focuses each opened date but keeps draft focus through peer changes before opening date B", () => {
    const doc = room(paragraph());
    const other = reload(doc);
    join(doc, other);
    const view = datePerson(doc);
    const peer = person(other);
    pickFirst(view);
    const row = date.rows?.[0];
    if (!row) throw new Error("Date has no picker row");
    const ctx = { run: (command: Command) => { run(view, command); }, openLink: () => undefined, addImages: () => undefined };
    const t = translator(catalogues.en);
    const ui = render(<>{row.render(view.state, ctx, t)}</>);
    const input = screen.getByLabelText(t("editor.dateValue"));
    expect(input).toHaveFocus();
    expect(dates(view)).toEqual(["2026-10-24", "2026-12-01"]);
    fireEvent.change(input, { target: { value: "2027-01-01" } });
    const cancel = screen.getByRole("button", { name: "Cancel" });
    cancel.focus();
    select(peer, "Due ", 0);
    typeAt(peer, peer.state.selection.from, "Peer: ");
    ui.rerender(<>{row.render(view.state, ctx, t)}</>);
    expect(cancel).toHaveFocus();
    expect(input).toHaveValue("2027-01-01");
    expect(view.state.doc.textContent).toBe("Peer: Due  now");
    let last = 0;
    view.state.doc.descendants((node, pos) => { if (node.type.name === "date") last = pos; });
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, last)));
    run(view, insertDate);
    ui.rerender(<>{row.render(view.state, ctx, t)}</>);
    expect(screen.getByLabelText(t("editor.dateValue"))).toHaveValue("2026-12-01");
    expect(screen.getByLabelText(t("editor.dateValue"))).toHaveFocus();
    fireEvent.change(screen.getByLabelText(t("editor.dateValue")), { target: { value: "2026-12-02" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(dates(view)).toEqual(["2026-10-24", "2026-12-02"]);
  });
});
