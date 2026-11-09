import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import type { EditorView } from "prosemirror-view";
import { TextSelection } from "prosemirror-state";
import { join, para, person, room, tabbed, typeAt } from "../testRoom";
import { anchorOfSelection, readAnchor, rangeOf, standing, type DocAnchor } from "./anchor";

/** What the anchor marks in this person's editor. */
const marked = (view: EditorView, anchor: DocAnchor): string | null => {
  const range = rangeOf(view.state, anchor);
  return range === null ? null : view.state.doc.textBetween(range.from, range.to, "\n", "\n");
};

/** Selects exactly these words, in the first block that holds them. */
function selectWords(view: EditorView, words: string): void {
  let from = -1;
  view.state.doc.descendants((node, pos) => {
    if (from < 0 && node.isTextblock && node.textContent.includes(words)) from = pos + 1 + node.textContent.indexOf(words);
    return from < 0;
  });
  if (from < 0) throw new Error(`no block holds "${words}"`);
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, from, from + words.length)));
}

/** Alice and Bob in the same room, Alice having commented on `quote` in the first block that holds it. */
function commented(quote: string, ...blocks: unknown[]) {
  const a = room(...blocks);
  const b = new Y.Doc();
  join(a, b);
  const alice = person(a);
  const bob = person(b);
  selectWords(alice, quote);
  const anchor = anchorOfSelection(alice.state);
  if (anchor === null) throw new Error("the selection made no anchor");
  return { a, b, alice, bob, anchor };
}

describe("an anchor in a doc", () => {
  it("says the tab, the block and the words, and none for a caret", () => {
    const view = person(room(para("b_1", "Hello"), para("b_2", "The second block")));
    selectWords(view, "second");
    expect(anchorOfSelection(view.state)).toMatchObject({ tab: "t1", block: "b_2", quote: "second" });
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, view.state.selection.from)));
    expect(anchorOfSelection(view.state)).toBeNull();
  });

  it("marks the same words in both browsers", () => {
    const { alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox"));
    expect(marked(alice, anchor)).toBe("brown fox");
    expect(marked(bob, anchor)).toBe("brown fox");
  });

  it("moves with the words when another person types before them", () => {
    const { a, alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox"), para("b_2", "Last"));
    typeAt(bob, 2, "Once upon a time. ");
    expect(marked(alice, anchor)).toBe("brown fox");
    expect(marked(bob, anchor)).toBe("brown fox");
    expect(standing(a, anchor)).toBe("found");
  });

  it("stays on the words when the block is moved", () => {
    const { a, alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox"), para("b_2", "Last"));
    // A move is a delete and an insert somewhere else: the words are new in the room, the block keeps its id.
    const first = bob.state.doc.firstChild?.firstChild;
    if (!first) throw new Error("no first block");
    const end = bob.state.doc.content.size - 1;
    bob.dispatch(bob.state.tr.insert(end, first).delete(1, 1 + first.nodeSize));
    expect(bob.state.doc.firstChild?.firstChild?.attrs.id).toBe("b_2");
    expect(marked(alice, anchor)).toBe("brown fox");
    expect(marked(bob, anchor)).toBe("brown fox");
    expect(standing(a, anchor)).toBe("found");
  });

  it("keeps its quote and is changed when the words are deleted", () => {
    const { a, alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox jumps"));
    selectWords(bob, "brown fox");
    bob.dispatch(bob.state.tr.deleteSelection());
    expect(marked(alice, anchor)).toBeNull();
    expect(standing(a, anchor)).toBe("changed");
    expect(anchor.quote).toBe("brown fox");
  });

  it("is changed when its whole block is deleted, even in a tab nobody has open", () => {
    const doc = tabbed({ id: "t1", title: "One", blocks: [para("b_1", "Keep")] }, { id: "t2", title: "Two", blocks: [para("b_9", "Only here")] });
    const there = { tab: "t2", block: "b_9", quote: "Only here" };
    expect(standing(doc, there)).toBe("found");
    const second = person(doc, true, "t2");
    selectWords(second, "Only here");
    second.dispatch(second.state.tr.deleteSelection());
    expect(standing(doc, there)).toBe("changed");
  });

  it("does not draw in the words typed right after it or right before it", () => {
    const { alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox"));
    selectWords(bob, "brown fox");
    const { from, to } = bob.state.selection;
    typeAt(bob, to, " jumps");
    typeAt(bob, from, "very ");
    expect(bob.state.doc.textContent).toBe("The quick very brown fox jumps");
    expect(marked(alice, anchor)).toBe("brown fox");
  });

  it("still marks what is left when only some of the words are deleted", () => {
    const { alice, bob, anchor } = commented("brown fox", para("b_1", "The quick brown fox"));
    selectWords(bob, "fox");
    bob.dispatch(bob.state.tr.deleteSelection());
    expect(marked(alice, anchor)).toBe("brown ");
  });

  it("is found from the block and the quote alone, as an agent writes it", () => {
    const view = person(room(para("b_1", "One"), para("b_2", "Say hello to the world")));
    const agent = { tab: "t1", block: "b_2", quote: "hello" };
    expect(marked(view, agent)).toBe("hello");
    expect(marked(view, { ...agent, block: "b_1" })).toBeNull();
    expect(marked(view, { ...agent, quote: "goodbye" })).toBeNull();
  });
});

describe("reading an anchor from a comment", () => {
  it("takes a doc anchor and refuses what is not one", () => {
    expect(readAnchor({ tab: "t1", block: "b_2", quote: "hi" })).toEqual({ tab: "t1", block: "b_2", quote: "hi" });
    expect(readAnchor({ frame: "f1", element: "e2" })).toBeNull();
    expect(readAnchor({ tab: "t1", block: "b_2", quote: 3 })).toBeNull();
    expect(readAnchor(undefined)).toBeNull();
  });
});
