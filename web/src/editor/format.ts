// Text marks, quotes, code blocks, rules and to-do items. Every command keeps
// the text it is applied to: applying one only changes how the text is held.
import { toggleMark } from "prosemirror-commands";
import { Fragment, type MarkType, type Node } from "prosemirror-model";
import { TextSelection, type Command, type EditorState } from "prosemirror-state";
import { schema } from "./schema";

export type MarkName = "bold" | "italic" | "underline" | "strike" | "code" | "highlight";

const markType = (name: MarkName): MarkType => {
  const t = schema.marks[name];
  if (t === undefined) throw new Error(`the editor's schema has no mark ${name}`);
  return t;
};

export const toggle = (name: MarkName): Command => toggleMark(markType(name));

/** Whether the whole selection (or the caret's next typing) carries the mark. */
export function markActive(state: EditorState, name: MarkName): boolean {
  const type = markType(name);
  const { from, $from, to, empty } = state.selection;
  if (empty) return type.isInSet(state.storedMarks ?? $from.marks()) !== undefined;
  return state.doc.rangeHasMark(from, to, type);
}

/** The blocks a container (a tab or a quote) holds that the selection touches. */
function touched(state: EditorState): { depth: number; first: number; last: number; start: number; end: number } | null {
  const { $from, from, to: selTo } = state.selection;
  let depth = $from.depth;
  while (depth > 0 && $from.node(depth).type.name !== "tab" && $from.node(depth).type.name !== "quote") depth--;
  const container = $from.node(depth);
  const to = Math.min(selTo, $from.end(depth));
  let at = $from.start(depth);
  let first = -1;
  let last = -1;
  let start = at;
  let end = at;
  container.forEach((child, _o, index) => {
    if (at <= to && at + child.nodeSize >= from) {
      if (first < 0) {
        first = index;
        start = at;
      }
      last = index;
      end = at + child.nodeSize;
    }
    at += child.nodeSize;
  });
  return first < 0 ? null : { depth, first, last, start, end };
}

/** The quote the caret is in, innermost first. */
export function quoteDepth(state: EditorState): number | null {
  const { $from } = state.selection;
  for (let d = $from.depth; d > 0; d--) if ($from.node(d).type.name === "quote") return d;
  return null;
}

/** Quote: wraps the selected blocks in one; when the caret is in a quote, takes the blocks out again. */
export const toggleQuote: Command = (state, dispatch) => {
  const { from, to } = state.selection;
  const inside = quoteDepth(state);
  if (inside !== null) {
    const { $from } = state.selection;
    const start = $from.before(inside);
    const quote = $from.node(inside);
    if (dispatch) {
      const tr = state.tr.replaceWith(start, start + quote.nodeSize, quote.content);
      tr.setSelection(TextSelection.create(tr.doc, from - 1, to - 1));
      dispatch(tr.scrollIntoView());
    }
    return true;
  }
  const range = touched(state);
  if (range === null) return false;
  const { $from } = state.selection;
  const container = $from.node(range.depth);
  const blocks: Node[] = [];
  for (let i = range.first; i <= range.last; i++) blocks.push(container.child(i));
  const quote = schema.nodes.quote;
  if (quote === undefined || !blocks.every((b) => b.type.spec.group?.includes("content_block") === true)) return false;
  if (dispatch) {
    const tr = state.tr.replaceWith(range.start, range.end, quote.create(null, Fragment.from(blocks)));
    tr.setSelection(TextSelection.create(tr.doc, from + 1, to + 1));
    dispatch(tr.scrollIntoView());
  }
  return true;
};

const inlineOf = (code: Node): Node[] => {
  const out: Node[] = [];
  code.forEach((child) => {
    if (child.text === undefined) return;
    child.text.split("\n").forEach((line, i) => {
      if (i > 0 && schema.nodes.hard_break) out.push(schema.nodes.hard_break.create());
      if (line !== "") out.push(schema.text(line));
    });
  });
  return out;
};

const textOf = (block: Node): string => {
  let out = "";
  block.forEach((child) => {
    out += child.type.name === "hard_break" ? "\n" : (child.text ?? "");
  });
  return out;
};

export function inCode(state: EditorState): boolean {
  return state.selection.$from.parent.type.name === "code_block";
}

/** Code block: paragraphs and headings become one (a line break becomes a newline), and back. A block keeps its id. */
export const toggleCodeBlock: Command = (state, dispatch) => {
  const { from, to } = state.selection;
  const blocks: { node: Node; pos: number; parent: Node; index: number }[] = [];
  state.doc.nodesBetween(from, to, (node, pos, parent, index) => {
    if (!node.isTextblock) return true;
    if (parent !== null) blocks.push({ node, pos, parent, index });
    return false;
  });
  const code = schema.nodes.code_block;
  const paragraph = schema.nodes.paragraph;
  if (code === undefined || paragraph === undefined) return false;
  const allCode = blocks.length > 0 && blocks.every((b) => b.node.type === code);
  const changes = blocks.filter((b) => (allCode ? true : b.node.type !== code && b.parent.canReplaceWith(b.index, b.index + 1, code)));
  if (changes.length === 0) return false;
  if (dispatch) {
    const tr = state.tr;
    for (const b of changes.reverse()) {
      const id: unknown = b.node.attrs.id;
      const made = allCode ? paragraph.create({ id }, inlineOf(b.node)) : code.create({ id }, textOf(b.node) === "" ? null : schema.text(textOf(b.node)));
      tr.replaceWith(b.pos, b.pos + b.node.nodeSize, made);
    }
    tr.setSelection(TextSelection.create(tr.doc, from, to));
    dispatch(tr.scrollIntoView());
  }
  return true;
};

/** Rule: a horizontal rule after the block the caret is in, and the caret on the paragraph after it. */
export const insertRule: Command = (state, dispatch) => {
  const range = touched(state);
  const rule = schema.nodes.rule;
  const paragraph = schema.nodes.paragraph;
  if (range === null || rule === undefined || paragraph === undefined) return false;
  const { $from } = state.selection;
  const container = $from.node(range.depth);
  const current = container.child(range.last);
  const emptyHere = current.type === paragraph && current.content.size === 0 && range.first === range.last;
  const next = range.last + 1 < container.childCount ? container.child(range.last + 1) : null;
  const needsParagraph = emptyHere || next?.type !== paragraph;
  if (dispatch) {
    const made = needsParagraph ? [rule.create(), paragraph.create()] : [rule.create()];
    const at = emptyHere ? range.start : range.end;
    const tr = state.tr.replaceWith(at, emptyHere ? range.end : at, made);
    const caret = at + rule.create().nodeSize + 1;
    tr.setSelection(TextSelection.create(tr.doc, caret));
    dispatch(tr.scrollIntoView());
  }
  return true;
};

/** Ticks or unticks the to-do item at the position; false when it is not one. */
export const toggleTask = (itemPos: number): Command => (state, dispatch) => {
  const node = state.doc.nodeAt(itemPos);
  if (node?.type.name !== "list_item" || node.attrs.checked === null) return false;
  dispatch?.(state.tr.setNodeMarkup(itemPos, undefined, { ...node.attrs, checked: node.attrs.checked !== true }));
  return true;
};
