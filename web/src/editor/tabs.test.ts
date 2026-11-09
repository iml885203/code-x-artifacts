import * as Y from "yjs";
import { describe, expect, it } from "vitest";
import { addTab, cleanTitle, deleteTab, maxTitleLength, moveTab, renameTab, tabElement, tabsOf } from "./tabs";
import { UndoKeeper } from "./undoKeeper";
import { join, para, person, reload, select, tabbed } from "./testRoom";
import { undo } from "y-prosemirror";

const three = () =>
  tabbed(
    { id: "t1", title: "One", blocks: [para("b_1", "first")] },
    { id: "t2", title: "Two", blocks: [para("b_2", "second")] },
    { id: "t3", title: "Three", blocks: [para("b_3", "third")] },
  );
const titles = (doc: Y.Doc) => tabsOf(doc).map((t) => t.title);
const textOf = (doc: Y.Doc, id: string) => tabElement(doc, id)?.toJSON().replace(/<[^>]*>/g, "");

describe("adding a tab", () => {
  it("puts it after the tab named, or at the end, starts it with an empty heading 1, and gives it an id of its own", () => {
    const doc = three();
    const first = addTab(doc, "Untitled tab", "t2");
    const last = addTab(doc, "Last");
    expect(titles(doc)).toEqual(["One", "Two", "Untitled tab", "Three", "Last"]);
    expect(new Set([first, last, "t1", "t2", "t3"]).size).toBe(5);
    expect(first).toMatch(/^t_[a-z0-9]{8}$/);

    const view = person(doc, true, first ?? "");
    expect(view.state.doc.child(0).childCount).toBe(1);
    expect(view.state.doc.child(0).child(0).type.name).toBe("heading");
    expect(view.state.doc.child(0).child(0).attrs.level).toBe(1);
    expect(view.state.doc.textContent).toBe("");
  });

  it("refuses a title the doc could not be saved with", () => {
    const doc = three();
    for (const bad of ["", "   ", "a\nb", "x".repeat(maxTitleLength + 1), "a --> b"]) expect(addTab(doc, bad), JSON.stringify(bad)).toBeNull();
    expect(titles(doc)).toEqual(["One", "Two", "Three"]);
  });

  it("stays after a reload, with the other tabs' text", () => {
    const doc = three();
    addTab(doc, "New", "t1", "t_new");
    const again = reload(doc);
    expect(titles(again)).toEqual(["One", "New", "Two", "Three"]);
    expect(textOf(again, "t2")).toBe("second");
  });
});

describe("renaming a tab", () => {
  it("changes only its title: the id and the text stay, also after a reload", () => {
    const doc = three();
    expect(renameTab(doc, "t2", "  Plan B ")).toBe(true);
    expect(titles(reload(doc))).toEqual(["One", "Plan B", "Three"]);
    expect(textOf(doc, "t2")).toBe("second");
  });

  it("refuses a blank, two-line or too long title and an unknown tab, and changes nothing", () => {
    const doc = three();
    expect(renameTab(doc, "t2", "")).toBe(false);
    expect(renameTab(doc, "t2", "a\nb")).toBe(false);
    expect(renameTab(doc, "t2", "x".repeat(maxTitleLength + 1))).toBe(false);
    expect(renameTab(doc, "nope", "x")).toBe(false);
    expect(titles(doc)).toEqual(["One", "Two", "Three"]);
  });

  it("takes a title as long as the format allows, in characters not bytes", () => {
    expect(cleanTitle("你".repeat(maxTitleLength))).not.toBeNull();
    expect(cleanTitle("😀".repeat(maxTitleLength))).not.toBeNull();
    expect(cleanTitle("😀".repeat(maxTitleLength + 1))).toBeNull();
  });
});

describe("deleting a tab", () => {
  it("removes it with its text, also after a reload", () => {
    const doc = three();
    expect(deleteTab(doc, "t2")).toBe(true);
    expect(titles(reload(doc))).toEqual(["One", "Three"]);
    expect(tabElement(doc, "t2")).toBeNull();
  });

  it("keeps the last tab: a doc has at least one", () => {
    const doc = tabbed({ id: "t1", title: "Only", blocks: [para("b_1", "x")] });
    expect(deleteTab(doc, "t1")).toBe(false);
    expect(deleteTab(three(), "nope")).toBe(false);
    expect(titles(doc)).toEqual(["Only"]);
  });
});

describe("moving a tab", () => {
  it("moves it up and down one place with its content, and keeps the order after a reload", () => {
    const doc = three();
    expect(moveTab(doc, "t3", -1)).toBe(true);
    expect(moveTab(doc, "t1", 1)).toBe(true);
    expect(titles(doc)).toEqual(["Three", "One", "Two"]);
    expect(titles(reload(doc))).toEqual(titles(doc));
    expect(textOf(doc, "t3")).toBe("third");
    expect(textOf(doc, "t1")).toBe("first");
  });

  it("does nothing at the ends or for an unknown tab", () => {
    const doc = three();
    expect(moveTab(doc, "t1", -1)).toBe(false);
    expect(moveTab(doc, "t3", 1)).toBe(false);
    expect(moveTab(doc, "nope", 1)).toBe(false);
    expect(titles(doc)).toEqual(["One", "Two", "Three"]);
  });

  it("is a change of the order alone: the tab's own fragment is not written", () => {
    const doc = three();
    const writes: string[] = [];
    for (const id of ["t1", "t2", "t3"]) doc.getXmlFragment(`tab:${id}`).observeDeep(() => writes.push(id));
    moveTab(doc, "t2", 1);
    expect(titles(doc)).toEqual(["One", "Three", "Two"]);
    expect(writes).toEqual([]);
  });
});

describe("two people", () => {
  it("one moves a tab while another types in it: the order changes and every character stays, in both editors", () => {
    const a = three();
    const b = reload(a);
    join(a, b);
    const alice = person(a, true, "t2");
    const bob = person(b, true, "t2");
    select(bob, "second", 6);
    bob.dispatch(bob.state.tr.insertText(" and more"));
    moveTab(a, "t2", -1);
    bob.dispatch(bob.state.tr.insertText("!"));
    expect(titles(a)).toEqual(["Two", "One", "Three"]);
    expect(titles(b)).toEqual(["Two", "One", "Three"]);
    for (const view of [alice, bob]) expect(view.state.doc.textContent).toBe("second and more!");
  });

  it("one adds a tab while another types in a different tab: both see it and the text is intact", () => {
    const a = three();
    const b = reload(a);
    join(a, b);
    const bob = person(b, true, "t1");
    select(bob, "first", 5);
    bob.dispatch(bob.state.tr.insertText(" and bob"));
    addTab(a, "Notes", "t1", "t_notes");
    bob.dispatch(bob.state.tr.insertText("!"));
    for (const doc of [a, b]) {
      expect(titles(doc)).toEqual(["One", "Notes", "Two", "Three"]);
      expect(textOf(doc, "t1")).toBe("first and bob!");
    }
  });

  it("two people moving tabs at once end with every tab once, in the same order", () => {
    const a = three();
    const b = reload(a);
    join(a, b);
    moveTab(a, "t3", -1);
    moveTab(b, "t1", 1);
    expect(titles(a)).toEqual(titles(b));
    expect([...titles(a)].sort()).toEqual(["One", "Three", "Two"]);
  });
});

describe("Undo across tabs", () => {
  it("still takes back what was typed in a tab after going to another tab and back", () => {
    const doc = three();
    const keeper = new UndoKeeper();
    const open = (tab: string) => person(doc, true, tab, keeper.of(doc, tab));

    let view = open("t1");
    select(view, "first", 5);
    view.dispatch(view.state.tr.insertText(" typed"));
    expect(textOf(doc, "t1")).toBe("first typed");
    view.destroy();

    const other = open("t2");
    select(other, "second", 6);
    other.dispatch(other.state.tr.insertText(" in two"));
    other.destroy();

    view = open("t1");
    expect(textOf(doc, "t1")).toBe("first typed");
    undo(view.state);
    expect(textOf(doc, "t1")).toBe("first");
    // The other tab's typing is not part of this tab's history.
    expect(textOf(doc, "t2")).toBe("second in two");
    const again = open("t2");
    undo(again.state);
    expect(textOf(doc, "t2")).toBe("second");
  });

  it("does not undo what another person typed, and lets go of the history of a tab that is gone", () => {
    const a = three();
    const b = reload(a);
    join(a, b);
    const keeper = new UndoKeeper();
    const mine = person(a, true, "t1", keeper.of(a, "t1"));
    const theirs = person(b, true, "t1");
    select(theirs, "first", 5);
    theirs.dispatch(theirs.state.tr.insertText(" bob"));
    select(mine, "first bob", 9);
    mine.dispatch(mine.state.tr.insertText(" alice"));
    mine.destroy();
    const back = person(a, true, "t1", keeper.of(a, "t1"));
    undo(back.state);
    expect(textOf(a, "t1")).toBe("first bob");
    keeper.keepOnly(["t2"]);
    expect(keeper.of(a, "t1")).not.toBe(undefined);
  });
});
