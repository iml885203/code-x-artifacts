import type { EditorView } from "prosemirror-view";
import { redo, undo } from "y-prosemirror";
import { describe, expect, it } from "vitest";
import { backspaceItem, indentItem, outdentItem, splitItem, toggleList } from "./lists";
import * as Y from "yjs";
import { join, outline, para, person, reload, room, select, selectBetween, typeAt } from "./testRoom";

const inner = (s: string) => ({ type: "paragraph", attrs: { id: null }, content: [{ type: "text", text: s }] });
const item = (s: string, ...nested: unknown[]) => ({ type: "list_item", attrs: { checked: null }, content: [inner(s), ...nested] });
const task = (s: string, checked: boolean, ...nested: unknown[]) => ({ type: "list_item", attrs: { checked }, content: [inner(s), ...nested] });
const list = (type: string, id: string | null, ...items: unknown[]) => ({ type, attrs: { id }, content: items });

const run = (view: EditorView, command: (s: EditorView["state"], d: EditorView["dispatch"]) => boolean) => command(view.state, view.dispatch);

describe("lists", () => {
  it.each([
    ["bullet_list", "ul"],
    ["numbered_list", "ol"],
    ["task_list", "todo"],
  ] as const)("makes a %s of the paragraphs the selection touches, undoes and redoes it, keeps it on reload, and keeps the text", (kind, name) => {
    const doc = room(para("b_1", "one"), para("b_2", "two"), para("b_3", "three"));
    const view = person(doc);
    const before = view.state.doc.textContent;
    select(view, "two", 1);
    run(view, toggleList(kind));
    const mark = kind === "task_list" ? "_" : "";
    expect(outline(view)).toBe(`p(one) ${name}[li${mark}[p(two)]] p(three)`);
    expect(view.state.doc.textContent).toBe(before);

    undo(view.state);
    expect(outline(view)).toBe("p(one) p(two) p(three)");
    redo(view.state);
    expect(outline(view)).toBe(`p(one) ${name}[li${mark}[p(two)]] p(three)`);
    expect(outline(person(reload(doc)))).toBe(`p(one) ${name}[li${mark}[p(two)]] p(three)`);
  });

  it("makes one list of several selected blocks, keeping the caret where it was", () => {
    const view = person(room(para("b_1", "one"), para("b_2", "two"), para("b_3", "three")));
    selectBetween(view, "one", "two");
    run(view, toggleList("numbered_list"));
    expect(outline(view)).toBe("ol[li[p(one)] li[p(two)]] p(three)");
    expect(view.state.selection.$from.parent.textContent).toBe("one");
    expect(view.state.selection.$to.parent.textContent).toBe("two");
  });

  it("changes the kind of a list that is not of that kind, and turns one that is already of it back into paragraphs", () => {
    const view = person(room(list("bullet_list", "b_1", item("one"), item("two"))));
    selectBetween(view, "one", "two");
    run(view, toggleList("task_list"));
    expect(outline(view)).toBe("todo[li_[p(one)] li_[p(two)]]");
    run(view, toggleList("task_list"));
    expect(outline(view)).toBe("p(one) p(two)");
  });

  it("Tab puts an item under the one above, with the items nested in it, and Shift+Tab brings it back", () => {
    const doc = room(list("bullet_list", "b_1", item("a"), item("b", list("bullet_list", null, item("c"))), item("d")));
    const view = person(doc);
    expect(outline(view)).toBe("ul[li[p(a)] li[p(b) ul[li[p(c)]]] li[p(d)]]");
    select(view, "b", 0);
    expect(run(view, indentItem)).toBe(true);
    expect(outline(view)).toBe("ul[li[p(a) ul[li[p(b) ul[li[p(c)]]]]] li[p(d)]]");
    expect(run(view, outdentItem)).toBe(true);
    expect(outline(view)).toBe("ul[li[p(a)] li[p(b) ul[li[p(c)]]] li[p(d)]]");
  });

  it("cannot indent the first item of a list", () => {
    const view = person(room(list("bullet_list", "b_1", item("a"), item("b"))));
    select(view, "a", 0);
    expect(run(view, indentItem)).toBe(false);
    expect(outline(view)).toBe("ul[li[p(a)] li[p(b)]]");
  });

  it("Shift+Tab in a top-level item makes it a paragraph and splits the list around it", () => {
    const view = person(room(list("numbered_list", "b_1", item("a"), item("b"), item("c"))));
    select(view, "b", 1);
    run(view, outdentItem);
    expect(outline(view)).toBe("ol[li[p(a)]] p(b) ol[li[p(c)]]");
    const ids = new Set<unknown>();
    view.state.doc.descendants((n, _p, parent) => {
      if (n.attrs.id !== undefined && parent?.type.name !== "list_item" && n.type.name !== "list_item" && n.type.name !== "tab") ids.add(n.attrs.id);
    });
    expect(ids.size).toBe(3);
  });

  it("an item takes the kind of the list it is nested in, and a ticked to-do keeps its tick through a move", () => {
    const view = person(room(list("task_list", "b_1", task("a", false), task("b", true))));
    select(view, "b", 0);
    run(view, indentItem);
    expect(outline(view)).toBe("todo[li_[p(a) todo[lix[p(b)]]]]");
    const bullets = person(room(list("bullet_list", "b_2", item("a", list("numbered_list", null, item("n"))), item("b"))));
    select(bullets, "b", 0);
    run(bullets, indentItem);
    expect(outline(bullets)).toBe("ul[li[p(a) ol[li[p(n)] li[p(b)]]]]");
  });

  it("Enter makes the next item, taking the text after the caret and the nested items with it; Enter in an empty item goes up a level", () => {
    const view = person(room(list("bullet_list", "b_1", item("hello world", list("bullet_list", null, item("kid"))))));
    select(view, "hello", 5);
    expect(run(view, splitItem)).toBe(true);
    expect(outline(view)).toBe("ul[li[p(hello)] li[p( world) ul[li[p(kid)]]]]");
    select(view, " world", 0);
    expect(run(view, splitItem)).toBe(true);
    expect(outline(view)).toBe("ul[li[p(hello)] li[p()] li[p( world) ul[li[p(kid)]]]]");
    select(view, "kid", 3);
    run(view, splitItem);
    run(view, splitItem);
    expect(outline(view)).toBe("ul[li[p(hello)] li[p()] li[p( world) ul[li[p(kid)]]] li[p()]]");
  });

  it("Enter in a to-do starts an unticked to-do", () => {
    const view = person(room(list("task_list", "b_1", task("done", true))));
    select(view, "done", 4);
    run(view, splitItem);
    expect(outline(view)).toBe("todo[lix[p(done)] li_[p()]]");
  });

  it("Backspace at the start of an item joins it to the one above, and at the start of the first item makes it a paragraph", () => {
    const view = person(room(list("bullet_list", "b_1", item("one"), item("two"))));
    select(view, "two", 0);
    expect(run(view, backspaceItem)).toBe(true);
    expect(outline(view)).toBe("ul[li[p(onetwo)]]");
    select(view, "onetwo", 0);
    run(view, backspaceItem);
    expect(outline(view)).toBe("p(onetwo)");
  });

  it("does nothing outside a list, so the keys keep their usual meaning", () => {
    const view = person(room(para("b_1", "plain")));
    select(view, "plain", 2);
    expect(run(view, splitItem)).toBe(false);
    expect(run(view, backspaceItem)).toBe(false);
    expect(run(view, indentItem)).toBe(false);
    expect(run(view, outdentItem)).toBe(false);
  });

  it("alice's list change does not touch what bob types in another block, and her Undo leaves it", () => {
    const a = room(para("b_1", "alice's"), para("b_2", "bob's"));
    const b = new Y.Doc();
    join(a, b);
    const alice = person(a);
    const bob = person(b);
    select(alice, "alice's", 1);
    run(alice, toggleList("bullet_list"));
    typeAt(bob, bob.state.doc.content.size - 2, " typing");
    expect(outline(alice)).toBe("ul[li[p(alice's)]] p(bob's typing)");
    expect(outline(bob)).toBe("ul[li[p(alice's)]] p(bob's typing)");
    undo(alice.state);
    expect(outline(alice)).toBe("p(alice's) p(bob's typing)");
    expect(outline(bob)).toBe("p(alice's) p(bob's typing)");
  });
});
