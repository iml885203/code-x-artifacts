// Where the person is typing an @ and what to do with the pick: pure functions of the editor's state, so
// the list around them (suggest.ts) has no rule of its own.
import type { Node } from "prosemirror-model";
import { Plugin, TextSelection, type EditorState, type Transaction } from "prosemirror-state";
import { AddMarkStep, Mapping } from "prosemirror-transform";
import type { Person } from "../../../api";
import { schema } from "../../schema";

/** The @ and the text typed after it, up to the caret. */
export interface Trigger {
  /** Where the @ is. */
  from: number;
  /** The caret. */
  to: number;
  query: string;
}

/** Longest text after the @ that still looks for someone: a name has spaces, a sentence is longer. */
export const MAX_QUERY = 30;

// The @ starts the text or follows a space or a break (an address like a@b.co is not a mention); the text
// after it does not start with a space, so "@ " is just an @.
const typing = new RegExp(`(?:^|[\\s\\uFFFC])@([^\\s@\\uFFFC][^@\\uFFFC]{0,${String(MAX_QUERY - 1)}})?$`, "u");

/** The @ the caret is right after, or null: only a caret (not a selection) in text, never in a code block. */
export function triggerAt(state: EditorState): Trigger | null {
  const { selection } = state;
  if (!selection.empty) return null;
  const { $from } = selection;
  const block = $from.parent;
  if (!block.inlineContent || block.type.spec.code === true) return null;
  const found = typing.exec(block.textBetween(0, $from.parentOffset, undefined, "￼"));
  if (found === null) return null;
  const query = found[1] ?? "";
  return { from: $from.pos - query.length - 1, to: $from.pos, query };
}

/** The transaction that turns the @ and what was typed after it into the person, with a space after to go on typing from; null when the schema cannot hold it there. */
export function pick(state: EditorState, at: Trigger, person: Pick<Person, "user" | "name">): Transaction | null {
  const type = schema.nodes.mention;
  if (type === undefined) throw new Error("the editor's schema has no mention");
  const node = type.create({ user: person.user, name: person.name });
  const $at = state.doc.resolve(at.from);
  if (!$at.parent.canReplaceWith($at.index(), $at.index(), type)) return null;
  const tr = state.tr.replaceWith(at.from, at.to, [node, schema.text(" ")]);
  return tr.setSelection(TextSelection.create(tr.doc, at.from + node.nodeSize + 1)).scrollIntoView();
}

/** Whether the document still holds a mention of this account. */
export function holds(doc: Node, user: string): boolean {
  let found = false;
  doc.descendants((node) => {
    if (node.type.name === "mention" && node.attrs.user === user) found = true;
    return !found;
  });
  return found;
}

/** The ranges of the new document that the transactions touched: what a step wrote, and what a mark was added over (a mark step moves nothing, so its map is empty). */
function touched(trs: readonly Transaction[]): [number, number][] {
  const steps = trs.flatMap((tr) => tr.steps);
  const maps = steps.map((step) => step.getMap());
  const out: [number, number][] = [];
  steps.forEach((step, i) => {
    const later = new Mapping(maps.slice(i + 1));
    const add = (start: number, end: number) => {
      out.push([later.map(start, -1), later.map(end, 1)]);
    };
    if (step instanceof AddMarkStep) add(step.from, step.to);
    else maps[i]?.forEach((_from, _to, start, end) => {
      add(start, end);
    });
  });
  return out;
}

/** A mention has no marks (docs/doc-format.md): the room keeps marks on text only, so a mark that landed on one (a bold over a selection, a paste inside bold text) would be gone after a reload. */
export const bareMentions = new Plugin({
  appendTransaction: (trs, _before, after) => {
    if (!trs.some((tr) => tr.docChanged)) return null;
    let fix: Transaction | null = null;
    for (const [from, to] of touched(trs)) {
      after.doc.nodesBetween(Math.max(0, from), Math.min(after.doc.content.size, to), (node, pos) => {
        if (node.type.name === "mention" && node.marks.length > 0) fix = (fix ?? after.tr).removeMark(pos, pos + node.nodeSize);
        return true;
      });
    }
    return fix;
  },
});
