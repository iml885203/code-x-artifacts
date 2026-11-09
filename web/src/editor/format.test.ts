import { Selection } from "prosemirror-state";
import { undo, redo } from "y-prosemirror";
import * as Y from "yjs";
import { describe, expect, it } from "vitest";
import { insertRule, markActive, toggle, toggleCodeBlock, toggleQuote, type MarkName } from "./format";
import { toggleList } from "./lists";
import { blocks, join, outline, para, person, reload, room, select, typeAt } from "./testRoom";

const marks: [MarkName, string][] = [
  ["bold", "strong"],
  ["italic", "em"],
  ["underline", "u"],
  ["strike", "s"],
  ["code", "code"],
  ["highlight", "mark"],
];

describe("text marks", () => {
  it.each(marks)("%s applies to the selected text, undoes, redoes and survives a reload, and the text does not change", (name, tag) => {
    const doc = room(para("b_1", "make this loud"));
    const view = person(doc);
    select(view, "make", 5, 9);
    expect(toggle(name)(view.state, view.dispatch)).toBe(true);
    expect(view.dom.querySelector(tag)?.textContent).toBe("this");
    expect(view.state.doc.textContent).toBe("make this loud");
    expect(markActive(view.state, name)).toBe(true);

    undo(view.state);
    expect(view.dom.querySelector(tag)).toBeNull();
    expect(view.state.doc.textContent).toBe("make this loud");
    redo(view.state);
    expect(view.dom.querySelector(tag)?.textContent).toBe("this");
    expect(person(reload(doc)).dom.querySelector(tag)?.textContent).toBe("this");
  });

  it("toggles off again and, with only a caret, marks what is typed next", () => {
    const view = person(room(para("b_1", "word")));
    select(view, "word", 0, 4);
    toggle("bold")(view.state, view.dispatch);
    toggle("bold")(view.state, view.dispatch);
    expect(markActive(view.state, "bold")).toBe(false);
    select(view, "word", 4);
    toggle("italic")(view.state, view.dispatch);
    expect(markActive(view.state, "italic")).toBe(true);
    view.dispatch(view.state.tr.insertText("s"));
    expect(view.dom.querySelector("em")?.textContent).toBe("s");
  });
});

describe("quotes", () => {
  it("wraps the block in a quote and takes it out again; undo, redo and a reload agree; the text is the same", () => {
    const doc = room(para("b_1", "before"), para("b_2", "wise words"), para("b_3", "after"));
    const view = person(doc);
    select(view, "wise", 2);
    toggleQuote(view.state, view.dispatch);
    expect(outline(view)).toBe("p(before) q[p(wise words)] p(after)");
    expect(view.state.doc.textContent).toBe("beforewise wordsafter");
    expect(view.state.selection.$from.parent.textContent).toBe("wise words");

    undo(view.state);
    expect(outline(view)).toBe("p(before) p(wise words) p(after)");
    redo(view.state);
    expect(outline(person(reload(doc)))).toBe("p(before) q[p(wise words)] p(after)");
    select(view, "wise", 2);
    toggleQuote(view.state, view.dispatch);
    expect(outline(view)).toBe("p(before) p(wise words) p(after)");
  });

  it("holds a list too", () => {
    const view = person(room(para("b_1", "one")));
    select(view, "one", 1);
    toggleList("bullet_list")(view.state, view.dispatch);
    toggleQuote(view.state, view.dispatch);
    expect(outline(view)).toBe("q[ul[li[p(one)]]]");
  });
});

describe("code blocks", () => {
  it("makes a code block of a paragraph and a paragraph of it again, keeping the id and the text; undo, redo and reload agree", () => {
    const doc = room(para("b_1", "let x = 1"));
    const view = person(doc);
    select(view, "let", 3);
    toggleCodeBlock(view.state, view.dispatch);
    expect(outline(view)).toBe("code(let x = 1)");
    expect(blocks(view)[0]).toMatchObject({ type: "code_block", attrs: { id: "b_1" } });
    expect(view.dom.querySelector("pre code")?.textContent).toBe("let x = 1");

    undo(view.state);
    expect(outline(view)).toBe("p(let x = 1)");
    redo(view.state);
    expect(outline(person(reload(doc)))).toBe("code(let x = 1)");
    toggleCodeBlock(view.state, view.dispatch);
    expect(blocks(view)[0]).toMatchObject({ type: "paragraph", attrs: { id: "b_1" } });
  });

  it("turns a line break into a newline and back, so the lines stay", () => {
    const view = person(room({ type: "paragraph", attrs: { id: "b_1" }, content: [{ type: "text", text: "one" }, { type: "hard_break" }, { type: "text", text: "two" }] }));
    select(view, "one", 1);
    toggleCodeBlock(view.state, view.dispatch);
    expect(view.state.doc.firstChild?.firstChild?.textContent).toBe("one\ntwo");
    toggleCodeBlock(view.state, view.dispatch);
    expect(JSON.stringify(blocks(view)[0])).toContain("hard_break");
  });

  it("is not offered inside a list item, where a code block cannot be", () => {
    const view = person(room(para("b_1", "item")));
    select(view, "item", 1);
    toggleList("bullet_list")(view.state, view.dispatch);
    expect(toggleCodeBlock(view.state, view.dispatch)).toBe(false);
  });
});

describe("rules", () => {
  it("adds a rule and a paragraph to write in after the block, undoes and redoes it, and keeps it on reload", () => {
    const doc = room(para("b_1", "above"));
    const view = person(doc);
    select(view, "above", 5);
    insertRule(view.state, view.dispatch);
    expect(outline(view)).toBe("p(above) hr p()");
    expect(view.state.selection.$from.parent.type.name).toBe("paragraph");
    expect(view.state.selection.$from.parent.textContent).toBe("");
    expect(view.dom.querySelector("hr")).not.toBeNull();

    undo(view.state);
    expect(outline(view)).toBe("p(above)");
    redo(view.state);
    expect(outline(person(reload(doc)))).toBe("p(above) hr p()");
  });

  it("replaces an empty paragraph with the rule and gives the paragraph after it a new id", () => {
    const view = person(room(para("b_1", "a"), para("b_2", "")));
    view.dispatch(view.state.tr.setSelection(Selection.atEnd(view.state.doc)));
    insertRule(view.state, view.dispatch);
    expect(outline(view)).toBe("p(a) hr p()");
    const ids = blocks(view).map((b) => (b.attrs as { id: unknown }).id);
    expect(new Set(ids).size).toBe(3);
    expect(ids.every((id) => typeof id === "string" && id !== "")).toBe(true);
  });
});

describe("to-do items", () => {
  const todo = (checked: boolean) => ({ type: "task_list", attrs: { id: "b_1" }, content: [{ type: "list_item", attrs: { checked }, content: [{ type: "paragraph", attrs: { id: null }, content: [{ type: "text", text: "ship it" }] }] }] });

  it("a click on the box ticks it; a reload still has it ticked; Undo unticks and Redo ticks again", () => {
    const doc = room(todo(false));
    const view = person(doc);
    const box = view.dom.querySelector<HTMLInputElement>("input.ed-check");
    expect(box?.checked).toBe(false);
    box?.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    expect(outline(view)).toBe("todo[lix[p(ship it)]]");
    expect(view.dom.querySelector<HTMLInputElement>("input.ed-check")?.checked).toBe(true);
    expect(view.dom.querySelector("li.ed-done")).not.toBeNull();

    const later = person(reload(doc));
    expect(outline(later)).toBe("todo[lix[p(ship it)]]");
    expect(later.dom.querySelector<HTMLInputElement>("input.ed-check")?.checked).toBe(true);

    undo(view.state);
    expect(outline(view)).toBe("todo[li_[p(ship it)]]");
    redo(view.state);
    expect(outline(view)).toBe("todo[lix[p(ship it)]]");
  });

  it("is ticked for the others at once, and a person who may only watch cannot tick it", () => {
    const a = room(todo(false));
    const b = new Y.Doc();
    const c = new Y.Doc();
    join(a, b);
    join(a, c);
    const alice = person(a);
    const bob = person(b);
    const carol = person(c, false);
    alice.dom.querySelector<HTMLInputElement>("input.ed-check")?.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    expect(outline(bob)).toBe("todo[lix[p(ship it)]]");
    expect(outline(carol)).toBe("todo[lix[p(ship it)]]");

    const box = carol.dom.querySelector<HTMLInputElement>("input.ed-check");
    box?.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    expect(outline(alice)).toBe("todo[lix[p(ship it)]]");
    expect(box?.checked).toBe(true);
  });

  it("typing next to it does not change it", () => {
    const view = person(room(todo(true), para("b_2", "x")));
    typeAt(view, view.state.doc.content.size - 2, "y");
    expect(outline(view)).toBe("todo[lix[p(ship it)]] p(xy)");
  });
});
