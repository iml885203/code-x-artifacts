import { DOMParser } from "prosemirror-model";
import { NodeSelection, TextSelection } from "prosemirror-state";
import { describe, expect, it } from "vitest";
import { person, room, text } from "./testRoom";
import { schema } from "./schema";
import { selectAllInTab } from "./view";

const date = { type: "date", attrs: { date: "2026-10-24" } };
const config = JSON.stringify({ options: ["Ready", "Later"], value: "Ready" });
const dropdown = { type: "dropdown", attrs: { config } };
const paragraph = (...content: unknown[]) => ({ type: "paragraph", attrs: { id: "b_1" }, content });

describe.each([true, false])("field clipboard (editable=%s)", (editable) => {
  it.each([{ field: date, expected: "2026-10-24" }, { field: dropdown, expected: "Ready" }])("copies a selected field alone as $expected", ({ field, expected }) => {
      const view = person(room(paragraph(field)), editable);
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, 2)));
      expect(view.serializeForClipboard(view.state.selection.content()).text).toBe(expected);
  });

  it("copies fields among text and retains their HTML configuration", () => {
    const view = person(room(paragraph(text("Due "), date, text(" / "), dropdown, text(" now"))), editable);
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2)));
    expect(selectAllInTab(view.state, view.dispatch)).toBe(true);
    const copied = view.serializeForClipboard(view.state.selection.content());
    expect(copied.text).toBe("Due 2026-10-24 / Ready now");
    const parsed = DOMParser.fromSchema(schema).parseSlice(copied.dom);
    const fields: unknown[] = [];
    parsed.content.descendants((node) => {
      if (node.type.name === "date" || node.type.name === "dropdown") fields.push(node.toJSON());
    });
    expect(fields).toEqual([date, dropdown]);
  });

  it("omits invalid field attributes and keeps markup-like labels as plain text", () => {
    const label = '<img src=x onerror="steal()">';
    const view = person(room(paragraph(
      { type: "date", attrs: { date: "2026-02-30" } },
      { type: "dropdown", attrs: { config: JSON.stringify({ options: ["Ready"], value: "Other" }) } },
      { type: "dropdown", attrs: { config: JSON.stringify({ options: [label], value: label }) } },
    )), editable);
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2)));
    expect(selectAllInTab(view.state, view.dispatch)).toBe(true);
    const copied = view.serializeForClipboard(view.state.selection.content());
    expect(copied.text).toBe(label);
    expect(copied.dom.querySelector("img, script, [onerror]")).toBeNull();
    expect(copied.dom.querySelector(".ed-dropdown:last-child")?.textContent).toBe(label);
  });

  it("preserves the installed default for other leaves and block separators", () => {
    const view = person(room(
      paragraph(text("Before"), { type: "hard_break" }, { type: "mention", attrs: { user: "alice", name: "Alice" } }, text("after")),
      { type: "rule", attrs: { id: "b_2" } },
      { type: "paragraph", attrs: { id: "b_3" }, content: [text("Next")] },
    ), editable);
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2)));
    expect(selectAllInTab(view.state, view.dispatch)).toBe(true);
    const slice = view.state.selection.content();
    const expected = slice.content.textBetween(0, slice.content.size, "\n\n");
    expect(expected).toBe("Beforeafter\n\nNext");
    expect(view.serializeForClipboard(slice).text).toBe(expected);
  });
});
