import { Awareness } from "y-protocols/awareness";
import * as Y from "yjs";
import { describe, expect, it } from "vitest";
import { join, para, room, typeAt } from "../testRoom";
import { mount } from "../view";
import { anchorOfSelection, type DocAnchor } from "./anchor";
import { redraw, type Marking } from "./marks";
import { TextSelection } from "prosemirror-state";

/** An editor on `doc` that draws these threads; `opened` collects what a click on a mark asks for. */
function editor(doc: Y.Doc, editable: boolean, threads: { id: string; anchor: DocAnchor }[], opened: string[] = [], active: string | null = null) {
  const parent = document.createElement("div");
  document.body.append(parent);
  const marking: Marking = { threads: () => threads, active: () => active, open: (id) => opened.push(id), compose: () => undefined };
  const live = mount(parent, doc, new Awareness(doc), editable, "Document", { comments: marking });
  return { view: live.view, destroy: () => { live.destroy(); parent.remove(); } };
}

function commentOn(words: string) {
  const a = room(para("b_1", "The quick brown fox"), para("b_2", "Second"));
  const b = new Y.Doc();
  join(a, b);
  const author = editor(a, true, []);
  const at = author.view.state.doc.textContent.indexOf(words) + 2;
  author.view.dispatch(author.view.state.tr.setSelection(TextSelection.create(author.view.state.doc, at, at + words.length)));
  const anchor = anchorOfSelection(author.view.state);
  if (anchor === null) throw new Error("no anchor");
  return { a, b, anchor, author };
}

const marked = (view: { dom: HTMLElement }) => Array.from(view.dom.querySelectorAll("[data-comment]")).map((e) => e.textContent);

describe("the words that have a comment", () => {
  it("are marked for a reader as well as for an editor, and follow the text another person types before them", () => {
    const { a, b, anchor, author } = commentOn("brown fox");
    const threads = [{ id: "c1", anchor }];
    const reader = editor(b, false, threads);
    const editing = editor(a, true, threads);
    expect(marked(reader.view)).toEqual(["brown fox"]);
    expect(marked(editing.view)).toEqual(["brown fox"]);

    typeAt(author.view, 2, "Once upon a time. ");
    expect(marked(reader.view)).toEqual(["brown fox"]);
    expect(marked(editing.view)).toEqual(["brown fox"]);
    for (const e of [author, reader, editing]) e.destroy();
  });

  it("draw the open thread's words stronger, once the editor is told to draw again", () => {
    const { a, anchor, author } = commentOn("brown fox");
    let active: string | null = null;
    const parent = document.createElement("div");
    document.body.append(parent);
    const live = mount(parent, a, new Awareness(a), true, "Document", { comments: { threads: () => [{ id: "c1", anchor }], active: () => active, open: () => undefined, compose: () => undefined } });
    expect(live.view.dom.querySelector(".ed-commented-active")).toBeNull();
    active = "c1";
    live.view.dispatch(live.view.state.tr.setMeta(redraw, true));
    expect(live.view.dom.querySelector(".ed-commented-active")?.textContent).toBe("brown fox");
    live.destroy();
    parent.remove();
    author.destroy();
  });

  it("ask for their thread when they are clicked, and draw nothing once the words are gone", () => {
    const { a, anchor, author } = commentOn("brown fox");
    const opened: string[] = [];
    const reader = editor(a, false, [{ id: "c1", anchor }], opened);
    reader.view.dom.querySelector("[data-comment]")?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(opened).toEqual(["c1"]);

    const at = author.view.state.doc.textContent.indexOf("brown fox") + 2;
    author.view.dispatch(author.view.state.tr.delete(at, at + "brown fox".length));
    expect(marked(reader.view)).toEqual([]);
    for (const e of [author, reader]) e.destroy();
  });

  it("ask to comment on the selection with Cmd-Alt-M and with nothing else", () => {
    const { a, author } = commentOn("brown fox");
    let composed = 0;
    const parent = document.createElement("div");
    document.body.append(parent);
    const live = mount(parent, a, new Awareness(a), true, "Document", { comments: { threads: () => [], active: () => null, open: () => undefined, compose: () => { composed++; } } });
    const press = (init: KeyboardEventInit) => live.view.dom.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init }));
    press({ code: "KeyM", altKey: true, metaKey: true });
    press({ code: "KeyM", altKey: true, ctrlKey: true });
    press({ code: "KeyM", metaKey: true });
    press({ code: "KeyM" });
    expect(composed).toBe(2);
    live.destroy();
    parent.remove();
    author.destroy();
  });
});
