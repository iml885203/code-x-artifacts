import { fireEvent, within } from "@testing-library/react";
import { NodeSelection, TextSelection } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { redo, undo } from "y-prosemirror";
import * as Y from "yjs";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { translator } from "../../../i18n";
import { catalogues } from "../../../test-support";
import type { Hooks } from "../../feature";
import { features } from "../../features";
import { toggle } from "../../format";
import { schema } from "../../schema";
import { tabFragmentName } from "../../tabs";
import { join, outline, para, person, reload, room, typeAt } from "../../testRoom";
import { UndoKeeper } from "../../undoKeeper";
import { editDropdown, insertDropdown, openDropdown } from "./commands";
import { encodeConfig, readConfig, type DropdownConfig } from "./config";
import { dropdown, dropdownWords, type DropdownHooks } from "./index";

const words = dropdownWords(translator(catalogues.en));
const hooks: Partial<Hooks> & DropdownHooks = { dropdown: { words: () => words } };
const added = !features.some((feature) => feature.id === "dropdown");
// The isolated module joins the actual registry for production mount, without replacing collaborators.
beforeAll(() => {
  expect(schema.nodes.dropdown, "requires the certified shared-schema dependency").toBeDefined();
  if (added) features.push(dropdown);
});
afterAll(() => { if (added) features.splice(features.indexOf(dropdown), 1); });

const dialog = () => {
  const forms = document.querySelectorAll<HTMLElement>('.ed-dropdown-picker');
  const form = forms[forms.length - 1];
  if (!form) throw new Error("no dropdown editor");
  return form;
};
const save = (form = dialog()) => fireEvent.click(within(form).getByRole("button", { name: words.save }));
const cancel = () => fireEvent.click(within(dialog()).getByRole("button", { name: words.cancel }));
const choose = (index: number, form = dialog()) => fireEvent.change(within(form).getByRole("combobox"), { target: { value: String(index) } });
const open = (view: EditorView) => {
  const entry = dropdown.insertions?.find((item) => item.id === "dropdown");
  if (!entry) throw new Error("no dropdown insertion entry");
  expect(entry.command).toBe(openDropdown);
  expect(entry.command(view.state, view.dispatch, view)).toBe(true);
};
const at = (view: EditorView, index = 0) => {
  const positions: number[] = [];
  view.state.doc.descendants((node, pos) => { if (node.type.name === "dropdown") positions.push(pos); });
  const pos = positions[index];
  if (pos === undefined) throw new Error("no field");
  return pos;
};
const configOf = (view: EditorView, index = 0) => readConfig(view.state.doc.nodeAt(at(view, index))?.attrs.config);
const selectAt = (view: EditorView, pos: number) => { view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, pos))); };
const sharedField = (doc: Y.Doc): Y.XmlElement => {
  const search = (fragment: Y.XmlFragment): Y.XmlElement | undefined => {
    for (const child of fragment.toArray()) {
      if (!(child instanceof Y.XmlElement)) continue;
      if (child.nodeName === "dropdown") return child;
      const found = search(child);
      if (found) return found;
    }
    return undefined;
  };
  const found = search(doc.getXmlFragment(tabFragmentName("t1")));
  if (!found) throw new Error("no shared field");
  return found;
};
const withField = (config: DropdownConfig = { options: ["One", "Two"], value: "One" }) => room({
  type: "paragraph", attrs: { id: "p" }, content: [
    { type: "text", text: "A" }, { type: "dropdown", attrs: { config: encodeConfig(config) } }, { type: "text", text: "B" },
  ],
});

describe("a person editing dropdown fields in the production view", () => {
  it("keeps a formatted dropdown bare while retaining text marks and peer/reload equality", () => {
    const doc = withField();
    const peerDoc = reload(doc);
    join(doc, peerDoc);
    const view = person(doc, true, undefined, undefined, hooks);
    const peer = person(peerDoc, true, undefined, undefined, hooks);
    const identity = sharedField(doc);
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2, 5)));
    expect(toggle("bold")(view.state, view.dispatch, view)).toBe(true);
    expect(view.state.doc.nodeAt(at(view))?.marks.map((mark) => mark.type.name)).toEqual([]);
    const textMarks: { text: string | undefined; marks: string[] }[] = [];
    view.state.doc.descendants((node) => {
      if (node.isText) textMarks.push({ text: node.text, marks: node.marks.map((mark) => mark.type.name) });
    });
    expect(textMarks).toEqual([{ text: "A", marks: ["bold"] }, { text: "B", marks: ["bold"] }]);
    expect(sharedField(doc)).toBe(identity);
    expect(configOf(view)).toEqual({ options: ["One", "Two"], value: "One" });
    expect(peer.state.doc.toJSON()).toEqual(view.state.doc.toJSON());
    expect(person(reload(doc)).state.doc.toJSON()).toEqual(view.state.doc.toJSON());
  });

  it("inserts at the relative caret, changes in place, isolates Undo, reloads and retains peer text", () => {
    const doc = room(para("p", "AB"));
    const peerDoc = reload(doc);
    join(doc, peerDoc);
    const keeper = new UndoKeeper();
    const view = person(doc, true, "t1", keeper.of(doc, "t1"), hooks);
    const peer = person(peerDoc, true, undefined, undefined, hooks);
    selectAt(view, 3);
    open(view);
    typeAt(peer, 2, "peer ");
    save();
    expect(configOf(view)).toEqual({ options: ["Option 1", "Option 2"], value: "Option 1" });
    expect(view.state.doc.textContent).toBe("peer AB");
    expect(peer.state.doc.toJSON()).toEqual(view.state.doc.toJSON());
    const identity = sharedField(doc);
    typeAt(view, 2, "local ");
    fireEvent.click(view.dom.querySelector<HTMLButtonElement>(".ed-dropdown-trigger") ?? view.dom);
    choose(1);
    save();
    expect(configOf(view)?.value).toBe("Option 2");
    expect(sharedField(doc)).toBe(identity);
    expect(undo(view.state)).toBe(true);
    expect(configOf(view)?.value).toBe("Option 1");
    expect(view.state.doc.textContent).toBe("local peer AB");
    expect(redo(view.state)).toBe(true);
    expect(configOf(person(reload(doc), false))).toEqual(configOf(view));
    expect(peer.state.doc.toJSON()).toEqual(view.state.doc.toJSON());
    keeper.release();
  });

  it("resolves current words only on execution through the insertion entry and has no toolbar tool", () => {
    const doc = room(para("p", "AB"));
    let current = dropdownWords(translator(catalogues.en));
    let reads = 0;
    const localized: Partial<Hooks> & DropdownHooks = { dropdown: { words: () => { reads++; return current; } } };
    const view = person(doc, true, undefined, undefined, localized);
    const before = Y.encodeStateAsUpdate(doc);
    expect(openDropdown(view.state)).toBe(true);
    expect(reads).toBe(0);
    expect(dropdown.tools).toBeUndefined();
    current = dropdownWords(translator(catalogues["zh-Hant"]));
    open(view);
    expect(reads).toBe(1);
    expect(dialog()).toHaveAccessibleName(current.label);
    expect(within(dialog()).getByRole("button", { name: current.save })).toBeVisible();
    fireEvent.click(within(dialog()).getByRole("button", { name: current.cancel }));
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
    const unwired = person(reload(doc));
    expect(openDropdown(unwired.state, unwired.dispatch, unwired)).toBe(false);
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
  });

  it("probes, opens and cancels without writing; refuses read-only execution", () => {
    const doc = room(para("p", "AB"));
    const view = person(doc, true, undefined, undefined, hooks);
    const before = Y.encodeStateAsUpdate(doc);
    expect(insertDropdown(words)(view.state)).toBe(true);
    expect(document.querySelector('.ed-dropdown-picker')).toBeNull();
    open(view);
    cancel();
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
    const reader = person(reload(doc), false);
    expect(insertDropdown(words)(reader.state, reader.dispatch, reader)).toBe(false);
    expect(editDropdown(reader, 3, words)).toBe(false);
  });

  it("validates the draft and keeps a selected rename or removal valid", () => {
    const doc = withField();
    const view = person(doc, true, undefined, undefined, hooks);
    expect(editDropdown(view, at(view), words)).toBe(true);
    let form = dialog();
    const before = Y.encodeStateAsUpdate(doc);
    const inputs = within(form).getAllByRole("textbox");
    fireEvent.input(inputs[1] ?? form, { target: { value: "One" } });
    save();
    expect(within(form).getByRole("alert")).toHaveTextContent(words.invalid);
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
    fireEvent.input(inputs[1] ?? form, { target: { value: "Two" } });
    fireEvent.input(inputs[0] ?? form, { target: { value: "Renamed" } });
    save();
    expect(configOf(view)).toEqual({ options: ["Renamed", "Two"], value: "Renamed" });
    editDropdown(view, at(view), words);
    form = dialog();
    fireEvent.click(within(form).getAllByRole("button", { name: (name) => name.startsWith(`${words.remove}:`) })[0] ?? form);
    expect(within(form).getByRole("button", { name: (name) => name.startsWith(`${words.remove}:`) })).toBeDisabled();
    save();
    expect(configOf(view)).toEqual({ options: ["Two"], value: "Two" });
  });

  it("uses fresh peer options for a selection and refuses a removed choice", () => {
    const doc = withField();
    const peerDoc = reload(doc);
    join(doc, peerDoc);
    const view = person(doc, true, undefined, undefined, hooks);
    const peer = person(peerDoc, true, undefined, undefined, hooks);
    editDropdown(view, at(view), words);
    choose(1);
    sharedField(peerDoc).setAttribute("config", encodeConfig({ options: ["One", "Two", "Peer"], value: "Peer" }));
    save();
    expect(configOf(view)).toEqual({ options: ["One", "Two", "Peer"], value: "Two" });
    expect(configOf(peer)).toEqual(configOf(view));
    editDropdown(view, at(view), words);
    choose(1);
    sharedField(peerDoc).setAttribute("config", encodeConfig({ options: ["One", "Peer"], value: "Peer" }));
    const before = Y.encodeStateAsUpdate(doc);
    save();
    expect(within(dialog()).getByRole("alert")).toHaveTextContent(words.changed);
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
    cancel();
  });

  it("rejects stale option edits and never edits the neighbour of a deleted target", () => {
    const doc = withField();
    const view = person(doc, true, undefined, undefined, hooks);
    editDropdown(view, at(view), words);
    fireEvent.input(within(dialog()).getAllByRole("textbox")[0] ?? dialog(), { target: { value: "Mine" } });
    sharedField(doc).setAttribute("config", encodeConfig({ options: ["Peer", "Two"], value: "Peer" }));
    save();
    expect(within(dialog()).getByRole("alert")).toHaveTextContent(words.changed);
    cancel();
    editDropdown(view, at(view), words);
    const pos = at(view);
    view.dispatch(view.state.tr.delete(pos, pos + 1));
    const type = schema.nodes.dropdown;
    if (!type) throw new Error("missing real dropdown schema");
    view.dispatch(view.state.tr.insert(pos, type.create({ config: encodeConfig({ options: ["Neighbour"], value: "Neighbour" }) })));
    const before = Y.encodeStateAsUpdate(doc);
    save();
    expect(within(dialog()).getByRole("alert")).toHaveTextContent(words.missing);
    expect(Y.encodeStateAsUpdate(doc)).toEqual(before);
    expect(configOf(view)?.value).toBe("Neighbour");
    cancel();
  });

  it("concurrent complete configs converge without combining one peer's options with another's value", () => {
    const doc = withField();
    const peerDoc = reload(doc);
    const view = person(doc, true, undefined, undefined, hooks);
    const peer = person(peerDoc, true, undefined, undefined, hooks);
    editDropdown(view, at(view), words);
    fireEvent.input(within(dialog()).getAllByRole("textbox")[0] ?? dialog(), { target: { value: "Mine" } });
    save();
    editDropdown(peer, at(peer), words);
    fireEvent.input(within(dialog()).getAllByRole("textbox")[1] ?? dialog(), { target: { value: "Theirs" } });
    choose(1);
    save();
    const a = Y.encodeStateAsUpdate(doc);
    const b = Y.encodeStateAsUpdate(peerDoc);
    Y.applyUpdate(doc, b, "relay");
    Y.applyUpdate(peerDoc, a, "relay");
    const result = configOf(view);
    expect(configOf(peer)).toEqual(result);
    expect([
      { options: ["Mine", "Two"], value: "Mine" },
      { options: ["One", "Theirs"], value: "Theirs" },
    ]).toContainEqual(result);
    expect(result?.options).toContain(result?.value);
  });

  it("reopening a different field resets the draft and leaves the first field unchanged", () => {
    const view = person(withField(), true, undefined, undefined, hooks);
    const type = schema.nodes.dropdown;
    if (!type) throw new Error("missing real dropdown schema");
    view.dispatch(view.state.tr.insert(at(view) + 1, type.create({ config: encodeConfig({ options: ["Other", "Choice"], value: "Other" }) })));
    editDropdown(view, at(view), words);
    fireEvent.input(within(dialog()).getAllByRole("textbox")[0] ?? dialog(), { target: { value: "Unsubmitted" } });
    editDropdown(view, at(view, 1), words);
    expect(within(dialog()).getAllByRole("textbox")[0]).toHaveValue("Other");
    choose(1);
    save();
    expect(configOf(view)).toEqual({ options: ["One", "Two"], value: "One" });
    expect(configOf(view, 1)).toEqual({ options: ["Other", "Choice"], value: "Choice" });
  });

  it("leaves paragraph and list Enter on their existing routes", () => {
    const paragraph = person(room(para("p", "AB")), true, undefined, undefined, hooks);
    selectAt(paragraph, 3);
    fireEvent.keyDown(paragraph.dom, { key: "Enter" });
    expect(outline(paragraph)).toBe("p(A) p(B)");
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
    const list = person(room({ type: "bullet_list", content: [{ type: "list_item", content: [para("li", "AB")] }] }), true, undefined, undefined, hooks);
    selectAt(list, 5);
    fireEvent.keyDown(list.dom, { key: "Enter" });
    expect(outline(list)).toBe("ul[li[p(A)] li[p(B)]]");
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
  });

  it("ignores modified and composing Enter and never opens or writes for a reader's selected field", () => {
    const doc = withField();
    const view = person(doc, true, undefined, undefined, hooks);
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, at(view))));
    fireEvent.keyDown(view.dom, { key: "Enter", shiftKey: true });
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
    const composing = person(withField(), true, undefined, undefined, hooks);
    composing.dispatch(composing.state.tr.setSelection(NodeSelection.create(composing.state.doc, at(composing))));
    fireEvent.keyDown(composing.dom, { key: "Enter", isComposing: true });
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
    const readerDoc = withField();
    const reader = person(readerDoc, false, undefined, undefined, hooks);
    reader.dispatch(reader.state.tr.setSelection(NodeSelection.create(reader.state.doc, at(reader))));
    const before = Y.encodeStateAsUpdate(readerDoc);
    fireEvent.keyDown(reader.dom, { key: "Enter" });
    expect(document.querySelector(".ed-dropdown-picker")).toBeNull();
    expect(Y.encodeStateAsUpdate(readerDoc)).toEqual(before);
  });

  it("opens from the selected atom with Enter, cancels with Escape and stays noninteractive for readers", () => {
    const view = person(withField(), true, undefined, undefined, hooks);
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, at(view))));
    fireEvent.keyDown(view.dom, { key: "Enter" });
    expect(dialog()).toHaveAccessibleName(words.label);
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(document.querySelector('.ed-dropdown-picker')).toBeNull();
    const reader = person(withField(), false, undefined, undefined, hooks);
    expect(reader.dom.querySelector<HTMLButtonElement>(".ed-dropdown-trigger")).toHaveAttribute("hidden");
    expect(reader.dom.textContent).toContain("One");
  });
});
