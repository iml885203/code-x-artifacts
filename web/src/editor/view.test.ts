import { splitBlock } from "prosemirror-commands";
import { TextSelection } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { undo, redo } from "y-prosemirror";
import * as Y from "yjs";
import { describe, expect, it } from "vitest";
import { blockCommand } from "./block";
import { selectAllInTab } from "./view";
import { join, para, person, room, text, typeAt } from "./testRoom";

const json = (view: EditorView) => view.state.doc.toJSON() as { content: { content: { attrs: Record<string, unknown>; type: string; content?: { text: string }[] }[] }[] };
const blocks = (view: EditorView) => json(view).content[0]?.content ?? [];
const textOf = (view: EditorView, n: number) => blocks(view)[n]?.content?.map((c) => c.text).join("") ?? "";

describe("the editor's view", () => {
  it("shows what the room holds and keeps the ids", () => {
    const view = person(room(para("b_1", "Hello"), { type: "heading", attrs: { id: "b_2", level: 2 }, content: [text("Title")] }));
    expect(blocks(view).map((b) => [b.type, b.attrs.id])).toEqual([["paragraph", "b_1"], ["heading", "b_2"]]);
    expect(view.dom.querySelector("h2")?.textContent).toBe("Title");
  });

  it("gives the second half of a split block an id of its own, and every block has one", () => {
    const view = person(room(para("b_1", "Hello world")));
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 8)));
    splitBlock(view.state, view.dispatch);
    const ids = blocks(view).map((b) => b.attrs.id);
    expect(ids).toHaveLength(2);
    expect(ids[0]).toBe("b_1");
    expect(ids[1]).toMatch(/^b_[a-z0-9]{8}$/);
    expect(blocks(view).map((_, i) => textOf(view, i))).toEqual(["Hello ", "world"]);
  });

  it("starts with the caret in the text, and select all then paste keeps the tab", () => {
    const view = person(room(para("b_1", "one"), para("b_2", "two")));
    expect(view.state.selection.$from.parent.inlineContent).toBe(true);
    expect(selectAllInTab(view.state, view.dispatch)).toBe(true);
    view.pasteHTML("<p>new</p>", new Event("paste") as ClipboardEvent);
    expect(json(view).content[0]).toMatchObject({ attrs: { id: "t1" } });
    expect(blocks(view).map((_, i) => textOf(view, i))).toEqual(["new"]);
  });

  it("makes a heading of a paragraph and back, keeping the id", () => {
    const view = person(room(para("b_1", "Title")));
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 3)));
    blockCommand(3)(view.state, view.dispatch);
    expect(blocks(view)[0]).toMatchObject({ type: "heading", attrs: { id: "b_1", level: 3 } });
    blockCommand("paragraph")(view.state, view.dispatch);
    expect(blocks(view)[0]).toMatchObject({ type: "paragraph", attrs: { id: "b_1" } });
  });

  it("edits of two people in different blocks both reach both editors", () => {
    const a = room(para("b_1", "one"), para("b_2", "two"));
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    typeAt(alice, 2, "A:");
    typeAt(bob, bob.state.doc.content.size - 2, ":B");
    expect([textOf(alice, 0), textOf(alice, 1)]).toEqual(["A:one", "two:B"]);
    expect([textOf(bob, 0), textOf(bob, 1)]).toEqual(["A:one", "two:B"]);
  });

  it("undoes only the person's own typing and leaves what another person wrote in place", () => {
    const a = room(para("b_1", "mid"));
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    typeAt(alice, 2, "alice ");
    typeAt(bob, bob.state.doc.content.size - 2, " bob");
    expect(textOf(alice, 0)).toBe("alice mid bob");

    undo(alice.state);
    expect(textOf(alice, 0)).toBe("mid bob");
    expect(textOf(bob, 0)).toBe("mid bob");

    redo(alice.state);
    expect(textOf(alice, 0)).toBe("alice mid bob");
  });

  it("cannot be typed into when it is read-only, and still shows other people's edits", () => {
    const a = room(para("b_1", "read"));
    const b = new Y.Doc();
    join(a, b);
    const writer = person(a);
    const reader = person(b, false);
    expect(reader.editable).toBe(false);
    typeAt(writer, 2, "live ");
    expect(textOf(reader, 0)).toBe("live read");
  });
});

describe("a hostile clipboard", () => {
  const hostile = [
    "<p onclick=\"steal()\">plain</p>",
    "<script>window.stolen = 1</script><p>after script</p>",
    "<img src=x onerror=\"window.stolen = 2\"><p>after image</p>",
    "<p><a href=\"javascript:alert(3)\">click me</a></p>",
    "<p style=\"background:url(//evil.example/x)\" class=\"evil\">styled</p>",
    "<style>p { display:none }</style><p>after style</p>",
    "<iframe src=\"//evil.example\"></iframe><object data=\"x\"></object>",
    "<div onmouseover=\"window.stolen = 4\"><span style=\"color:red\">in a div</span></div>",
    "<table><tr><td><ul><li>a cell with a list</li></ul></td></tr></table>",
  ].join("");

  it("produces a document with none of the script, handlers, styles or addresses, and only blocks the schema's paste rules name", () => {
    const view = person(room(para("b_1", "")));
    view.pasteHTML(hostile, new Event("paste") as ClipboardEvent);
    const doc = JSON.stringify(view.state.doc.toJSON());
    for (const gone of ["window.stolen", "steal()", "alert(3)", "javascript:", "evil.example", "display:none", "href", "onerror", "onclick", "<", "marks"]) {
      expect(doc, gone).not.toContain(gone);
    }
    const types = new Set<string>();
    view.state.doc.descendants((n) => {
      types.add(n.type.name);
    });
    expect([...types].sort()).toEqual(["paragraph", "tab", "table", "table_cell", "table_row", "text"]);
    const dom = view.dom.innerHTML;
    expect(view.dom.querySelectorAll("a, img, script, style, iframe, object, [style], [class=evil]")).toHaveLength(0);
    expect(dom).not.toMatch(/onerror|onclick|javascript:|<script|<iframe|<img|<style| style=/i);
    const pasted = view.state.doc.textContent;
    for (const kept of ["plain", "after script", "after image", "click me", "styled", "after style", "in a div", "a cell with a list"]) expect(pasted).toContain(kept);
  });
});

describe("a person's caret when someone else's edit arrives", () => {
  it("stays where the person put it, even when the browser has moved it before ProseMirror was told", () => {
    const a = room(para("b_1", "First paragraph."), para("b_2", "Second paragraph."));
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    bob.dom.tabIndex = 0;
    bob.focus();

    // Bob clicks into the second paragraph: the browser moves its selection, and the selectionchange
    // event that tells ProseMirror has not been delivered yet when Alice's edit arrives.
    const second = bob.dom.querySelectorAll("p")[1]?.firstChild;
    if (!second) throw new Error("no second paragraph");
    document.getSelection()?.setBaseAndExtent(second, 6, second, 6);
    typeAt(alice, 2, "Alice. ");

    expect(bob.state.selection.$from.parent.textContent).toBe("Second paragraph.");
    expect(bob.state.selection.$from.parentOffset).toBe(6);
  });

  describe("the hint of an empty doc", () => {
    const hint = { hint: () => "Start writing…" };
    const shown = (view: EditorView) => view.dom.querySelector(".ed-empty")?.getAttribute("data-hint") ?? null;

    it("is in the first block of an empty doc, and gone at the first character", () => {
      const view = person(room(para("b_1", "")), true, undefined, undefined, hint);
      expect(shown(view)).toBe("Start writing…");
      typeAt(view, 1, "H");
      expect(shown(view)).toBeNull();
    });

    it("is in an empty heading too, which is where a new doc starts", () => {
      const view = person(room({ type: "heading", attrs: { id: "b_1", level: 1 }, content: [] }), true, undefined, undefined, hint);
      expect(view.dom.querySelector("h1.ed-empty")?.getAttribute("data-hint")).toBe("Start writing…");
    });

    it("never shows in a doc with content, even when the block holding the caret is empty, nor to a reader", () => {
      expect(shown(person(room(para("b_1", "Words")), true, undefined, undefined, hint))).toBeNull();
      expect(shown(person(room(para("b_1", ""), para("b_2", "")), true, undefined, undefined, hint))).toBeNull();
      expect(shown(person(room(para("b_1", "")), false, undefined, undefined, hint))).toBeNull();
    });
  });
});
