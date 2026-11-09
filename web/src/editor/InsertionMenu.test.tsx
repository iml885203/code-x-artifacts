import { useState } from "react";
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { NodeSelection, type EditorState, type Command } from "prosemirror-state";
import { undo, redo } from "y-prosemirror";
import { renderIn } from "../test-support";
import { InsertionMenu } from "./InsertionMenu";
import { InsertRow } from "./InsertRow";
import { join, outline, para, person, reload, room, select, text } from "./testRoom";

function show(slash = false, block: unknown = para("b_one", "keep these words"), editable = true) {
  const blocks: unknown[] = Array.isArray(block) ? block as unknown[] : [block];
  const doc = room(...blocks);
  let changed: (state: EditorState) => void = () => undefined;
  let opened = () => undefined;
  const view = person(doc, editable, undefined, undefined, { onUpdate: (next) => { changed(next); }, onInsert: () => { opened(); } });
  if (view.state.doc.textContent === "keep these words") select(view, "keep", 5, 10);
  else select(view, view.state.doc.firstChild?.firstChild?.textContent ?? "", 0);
  function Host() {
    const [open, setOpen] = useState(false);
    const [state, update] = useState(view.state);
    changed = update;
    opened = () => { setOpen(true); };
    const ctx = { run: (command: Command) => { view.focus(); command(view.state, view.dispatch, view); }, openLink: () => undefined, addImages: () => undefined };
    return <><InsertRow state={state} ctx={ctx} />{open && <InsertionMenu state={state} ctx={ctx} onClose={() => { setOpen(false); view.focus(); }} onCancel={(query) => { view.dispatch(view.state.tr.insertText(`/${query}`)); }} />}</>;
  }
  renderIn(<Host />);
  if (slash) act(() => { view.someProp("handleTextInput", (handle) => handle(view, view.state.selection.from, view.state.selection.to, "/", () => view.state.tr)); });
  return { view, doc };
}

describe("inserting document blocks", () => {
  it("keeps the selected words when a heading is chosen with the keyboard, persists it and undoes once", async () => {
    const { view, doc } = show();
    await userEvent.click(screen.getByRole("button", { name: "Insert block" }));
    await userEvent.type(screen.getByRole("combobox", { name: "Find a block" }), "Heading 2{ArrowDown}{ArrowUp}{Enter}");
    expect(outline(view)).toBe("h2(keep these words)");
    expect(outline(person(reload(doc)))).toBe("h2(keep these words)");
    await userEvent.click(screen.getByRole("button", { name: "More" }));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(outline(view)).toBe("p(keep these words)");
  });

  it("offers the same blocks from slash without saving the query, and restores literal slash text on Escape", async () => {
    const { view, doc } = show(true);
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "Quote{Enter}");
    expect(outline(person(reload(doc)))).toBe("q[p(keep these words)]");
    act(() => { select(view, "keep", 0);
      view.someProp("handleTextInput", (handle) => handle(view, view.state.selection.from, view.state.selection.to, "/", () => view.state.tr)); });
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "unknown{Escape}");
    expect(view.state.doc.textContent).toBe("/unknownkeep these words");
    expect(screen.queryByRole("combobox", { name: "Find a block" })).not.toBeInTheDocument();
  });

  it("maps the caret through peer edits while discovery is open and undoes only the chosen block change", async () => {
    const { view, doc } = show();
    const peerDoc = reload(doc);
    join(doc, peerDoc);
    const peer = person(peerDoc);
    act(() => {
      select(view, "keep", 0);
      view.dispatch(view.state.tr.insertText("mine "));
      select(view, "keep", 10, 15);
      view.someProp("handleTextInput", (handle) => handle(view, view.state.selection.from, view.state.selection.to, "/", () => view.state.tr));
    });
    const input = await screen.findByRole("combobox", { name: "Find a block" });
    await userEvent.type(input, "Heading 2");
    expect(person(reload(doc)).state.doc.textContent).toBe("mine keep these words");
    act(() => { select(peer, "keep", 0); peer.dispatch(peer.state.tr.insertText("peer ")); });
    await userEvent.keyboard("{Enter}");
    expect(outline(view)).toBe("h2(peer mine keep these words)");
    expect(view.state.selection.$from.parent.textBetween(0, view.state.selection.$from.parentOffset)).toBe("peer mine keep ");
    act(() => { undo(view.state); });
    expect(outline(view)).toBe("p(peer mine keep these words)");
    act(() => { redo(view.state); });
    expect(outline(person(reload(doc)))).toBe("h2(peer mine keep these words)");
  });

  it("wraps keyboard navigation and cancels an unmatched query as literal text after a peer edit", async () => {
    const { doc } = show(true);
    const peerDoc = reload(doc);
    join(doc, peerDoc);
    const peer = person(peerDoc);
    const input = await screen.findByRole("combobox", { name: "Find a block" });
    await userEvent.keyboard("{ArrowUp}");
    expect(screen.getByRole("option", { name: "Quote" })).toHaveAttribute("aria-selected", "true");
    await userEvent.type(input, "no-such-block{ArrowDown}{Enter}");
    expect(screen.getByText("No matching blocks")).toBeVisible();
    act(() => { select(peer, "keep", 0); peer.dispatch(peer.state.tr.insertText("peer ")); });
    expect(person(reload(doc)).state.doc.textContent).toBe("peer keep these words");
    await userEvent.keyboard("{Escape}");
    expect(person(reload(doc)).state.doc.textContent).toBe("peer keep /no-such-block words");
  });

  it("closes plus discovery by pressing its trigger again and by Escape without changing the document", async () => {
    const { doc } = show();
    const trigger = screen.getByRole("button", { name: "Insert block" });
    await userEvent.click(trigger);
    expect(screen.getByRole("combobox", { name: "Find a block" })).toHaveFocus();
    await userEvent.click(trigger);
    expect(screen.queryByRole("combobox", { name: "Find a block" })).not.toBeInTheDocument();
    await userEvent.click(trigger);
    await userEvent.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
    expect(person(reload(doc)).state.doc.textContent).toBe("keep these words");
  });

  it("opens slash discovery after a hard line break without putting its query into the room", async () => {
    const { view, doc } = show(false, { type: "paragraph", attrs: { id: "b_line" }, content: [text("hello"), { type: "hard_break" }] });
    act(() => {
      select(view, "hello", 6);
      view.someProp("handleTextInput", (handle) => handle(view, view.state.selection.from, view.state.selection.to, "/", () => view.state.tr));
    });
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "Heading 2");
    expect(person(reload(doc)).state.doc.textContent).toBe("hello");
    await userEvent.keyboard("{Enter}");
    expect(outline(person(reload(doc)))).toBe("h2(hello)");
    expect(view.state.doc.firstChild?.firstChild?.lastChild?.type.name).toBe("hard_break");
  });

  it("focuses the anchored slash search after its hidden positioning pass", () => {
    const view = person(room(para("b_focus", "words")));
    view.focus();
    const nativeFocus = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "focus")?.value as HTMLElement["focus"];
    // jsdom has no visibility-aware native focus; the browser refuses hidden targets.
    vi.spyOn(HTMLElement.prototype, "focus").mockImplementation(function (this: HTMLElement, options?: FocusOptions) {
      if (getComputedStyle(this).visibility !== "hidden") nativeFocus.call(this, options);
    });
    renderIn(<InsertionMenu state={view.state} anchor={{ current: view }}
      ctx={{ run: (command) => { command(view.state, view.dispatch, view); }, openLink: () => undefined, addImages: () => undefined }} onClose={() => undefined} />);
    expect(screen.getByRole("combobox", { name: "Find a block" })).toHaveFocus();
    expect(view.state.doc.textContent).toBe("words");
    const elsewhere = document.createElement("input");
    document.body.append(elsewhere);
    elsewhere.focus();
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(elsewhere).toHaveFocus();
    elsewhere.remove();
  });

  it("uses the actual input range when the browser caret moves before the editor selection catches up", async () => {
    const { view, doc } = show(false, [
      { type: "heading", attrs: { id: "b_title", level: 1 }, content: [text("Plan")] },
      para("b_target", "First paragraph."),
    ]);
    expect(view.state.selection.from).toBe(2);
    act(() => { view.someProp("handleTextInput", (handle) => handle(view, 8, 8, "/", () => view.state.tr)); });
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "Heading 2{Enter}");
    expect(outline(person(reload(doc)))).toBe("h1(Plan) h2(First paragraph.)");
    expect(view.state.selection.$from.parent.textContent).toBe("First paragraph.");
  });

  it("opens at ordinary input even when the stale selection is a code block node", async () => {
    const { view, doc } = show(false, [
      { type: "code_block", attrs: { id: "b_code", lang: null }, content: [text("code")] },
      para("b_target", "First paragraph."),
    ]);
    act(() => {
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, 1)));
      view.someProp("handleTextInput", (handle) => handle(view, 8, 8, "/", () => view.state.tr));
    });
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "Heading 2{Enter}");
    expect(outline(person(reload(doc)))).toBe("code(code) h2(First paragraph.)");
  });

  it.each<[string, unknown]>([
    ["code", { type: "code_block", attrs: { id: "b_code", lang: null }, content: [text("target")] }],
    ["link", { type: "paragraph", attrs: { id: "b_link" }, content: [{ ...text("target"), marks: [{ type: "link", attrs: { href: "https://example.com" } }] }] }],
  ])("leaves input into actual %s alone when the stale selection is ordinary text", (_name, target) => {
    const { view } = show(false, [para("b_start", "Plan"), target]);
    let handled: unknown;
    act(() => { handled = view.someProp("handleTextInput", (handle) => handle(view, 8, 8, "/", () => view.state.tr)); });
    expect(handled).toBeFalsy();
    expect(view.state.selection.from).toBe(2);
    expect(screen.queryByRole("combobox", { name: "Find a block" })).not.toBeInTheDocument();
  });

  it.each<[string, unknown, boolean]>([
    ["code blocks", { type: "code_block", attrs: { id: "b_code", lang: null }, content: [text("code")] }, true],
    ["read-only documents", para("b_p", "reader"), false],
    ["linked text", { type: "paragraph", attrs: { id: "b_link" }, content: [{ ...text("linked"), marks: [{ type: "link", attrs: { href: "https://example.com" } }] }] }, true],
    ["inline code", { type: "paragraph", attrs: { id: "b_inline" }, content: [{ ...text("code"), marks: [{ type: "code" }] }] }, true],
  ])("leaves slash as ordinary typing in %s", (_name, block, editable) => {
    const { view } = show(false, block, editable);
    let handled: unknown;
    act(() => { handled = view.someProp("handleTextInput", (handle) => handle(view, view.state.selection.from, view.state.selection.to, "/", () => view.state.tr)); });
    expect(handled).toBeFalsy();
    expect(screen.queryByRole("combobox", { name: "Find a block" })).not.toBeInTheDocument();
    expect(view.state.doc.textContent).not.toContain("/");
  });
});
