// Links: what the Cmd+K popover reads and changes. An address is checked with
// the same list the server validates (schema.ts `safeHref`); the person is told
// why one is refused instead of it being dropped.
import type { MarkType, Node } from "prosemirror-model";
import { TextSelection, type Command, type EditorState } from "prosemirror-state";
import { safeHref, schema } from "./schema";

const linkType = (): MarkType => {
  const t = schema.marks.link;
  if (t === undefined) throw new Error("the editor's schema has no mark link");
  return t;
};

export type Checked = { href: string } | { refused: "empty" } | { refused: "scheme"; address: string };

/** The address as it will be stored, or why it cannot be. */
export function checkAddress(input: string): Checked {
  const address = input.trim();
  if (address === "") return { refused: "empty" };
  const href = safeHref(address);
  return href === null ? { refused: "scheme", address } : { href };
}

/** A link the caret or selection is in: where it starts and ends, and where it points. */
export interface LinkRange {
  from: number;
  to: number;
  href: string;
  text: string;
}

/** The whole run of one link around a position: neighbouring text with the same address belongs to it. */
function runAround(doc: Node, pos: number): LinkRange | null {
  const $pos = doc.resolve(pos);
  const parent = $pos.parent;
  if (!parent.inlineContent) return null;
  const start = $pos.start();
  const runs: { from: number; to: number; href: string }[] = [];
  parent.forEach((child, offset) => {
    const mark = child.marks.find((m) => m.type === linkType());
    if (mark === undefined) return;
    const from = start + offset;
    const last = runs.at(-1);
    const href = String(mark.attrs.href);
    if (last?.to === from && last.href === href) last.to = from + child.nodeSize;
    else runs.push({ from, to: from + child.nodeSize, href });
  });
  const hit = runs.find((r) => pos > r.from && pos < r.to) ?? runs.find((r) => pos >= r.from && pos <= r.to);
  return hit === undefined ? null : { ...hit, text: doc.textBetween(hit.from, hit.to) };
}

/** The link the caret is in (an empty selection), or the link a selection lies within. */
export function linkAt(state: EditorState): LinkRange | null {
  const { from, to, empty } = state.selection;
  const run = runAround(state.doc, from);
  if (run === null) return null;
  return empty || to <= run.to ? run : null;
}

/** What the popover starts with: the link being changed, or the selected text with no address. */
export function linkDraft(state: EditorState): LinkRange | null {
  const found = linkAt(state);
  if (found !== null) return found;
  const { from, to, empty, $from } = state.selection;
  if (!$from.parent.inlineContent) return null;
  return { from, to, href: "", text: empty ? "" : state.doc.textBetween(from, to) };
}

/** Makes `text` a link to `href` over the range: the range's text is replaced when the text changed, and other marks of its first character stay. */
export const setLink = (range: { from: number; to: number }, text: string, href: string): Command => (state, dispatch) => {
  const checked = checkAddress(href);
  if (!("href" in checked) || !state.doc.resolve(range.from).parent.inlineContent) return false;
  if (dispatch) {
    const shown = text === "" ? checked.href : text;
    const tr = state.tr;
    let to = range.to;
    if (state.doc.textBetween(range.from, range.to) !== shown) {
      const others = state.doc.resolve(range.from).marks().filter((m) => m.type !== linkType());
      tr.replaceWith(range.from, range.to, schema.text(shown, others));
      to = range.from + shown.length;
    }
    tr.removeMark(range.from, to, linkType()).addMark(range.from, to, linkType().create({ href: checked.href }));
    tr.setSelection(TextSelection.create(tr.doc, to));
    dispatch(tr.scrollIntoView());
  }
  return true;
};

/** Takes the link off the text and leaves the text. */
export const removeLink = (range: { from: number; to: number }): Command => (state, dispatch) => {
  dispatch?.(state.tr.removeMark(range.from, range.to, linkType()));
  return true;
};
