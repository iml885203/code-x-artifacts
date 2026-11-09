// Where a comment on a doc points (docs/editor.md). An anchor is a tab, a block and the words, so an agent reads what it is
// about; the browser that makes it adds two Yjs relative positions (start and end), which follow the words as other people edit,
// wherever the block ends up. When the positions are gone (the words were deleted, or the block was moved, which in a room is
// a delete and an insert) the block id and the words find them again; when that fails too, the words are gone and the
// comment is "changed". Only the anchor's own fields are read here: the server keeps it as given and never looks inside.
import { TextSelection, type EditorState } from "prosemirror-state";
import { absolutePositionToRelativePosition, relativePositionToAbsolutePosition, ySyncPluginKey } from "y-prosemirror";
import * as Y from "yjs";
import { tabElement } from "../tabs";

export interface DocAnchor {
  tab: string;
  block: string;
  quote: string;
  /** Base64 of a Yjs relative position: stuck to the first and to the last character of the words. Absent in an anchor an agent wrote. */
  start?: string;
  end?: string;
}

/** The longest quote kept: an anchor is at most 2000 bytes (service.MaxAnchorBytes) and a quote names the place, not the page. */
export const maxQuoteChars = 300;

/** Whether the words are still there: `changed` is the "original text changed" of the panel. */
export type Standing = "found" | "changed";

const isText = (v: unknown): v is string => typeof v === "string";

/** The doc anchor a comment carries, or null for any other kind of anchor (or a malformed one). */
export function readAnchor(raw: Record<string, unknown> | undefined): DocAnchor | null {
  if (raw === undefined) return null;
  const { tab, block, quote, start, end } = raw;
  if (!isText(tab) || tab === "" || !isText(block) || block === "" || !isText(quote)) return null;
  return { tab, block, quote, ...(isText(start) ? { start } : {}), ...(isText(end) ? { end } : {}) };
}

const encode = (rel: Y.RelativePosition): string => btoa(String.fromCharCode(...Y.encodeRelativePosition(rel)));

function decode(text: string | undefined): Y.RelativePosition | null {
  if (text === undefined) return null;
  try {
    return Y.decodeRelativePosition(Uint8Array.from(atob(text), (c) => c.charCodeAt(0)));
  } catch {
    return null;
  }
}

type Mapping = Parameters<typeof absolutePositionToRelativePosition>[2];

/** The binding of the view to its tab's fragment, which is how a place in the view and a place in the room turn into each other. */
function bindingOf(state: EditorState) {
  const sync = ySyncPluginKey.getState(state) as { binding?: { doc: Y.Doc; type: Y.XmlFragment; mapping: Mapping } } | undefined;
  return sync?.binding ?? null;
}

/** The anchor of what is selected, or null for a caret or a selection that is not text. */
export function anchorOfSelection(state: EditorState): DocAnchor | null {
  const { selection, doc } = state;
  const binding = bindingOf(state);
  const tab: unknown = doc.firstChild?.attrs.id;
  if (!binding || !(selection instanceof TextSelection) || selection.empty || !isText(tab)) return null;
  const block: unknown = selection.$from.depth >= 2 ? selection.$from.node(2).attrs.id : undefined;
  const quote = Array.from(doc.textBetween(selection.from, selection.to, "\n", "\n")).slice(0, maxQuoteChars).join("");
  if (!isText(block) || quote.trim() === "") return null;
  const start = absolutePositionToRelativePosition(selection.from, binding.type, binding.mapping) as Y.RelativePosition;
  // The end sticks to the last character, so words typed right after the quote are not drawn into it (the start sticks to the first).
  const stop = absolutePositionToRelativePosition(selection.to, binding.type, binding.mapping) as Y.RelativePosition;
  const place = Y.createAbsolutePositionFromRelativePosition(stop, binding.doc);
  const end = place === null ? stop : Y.createRelativePositionFromTypeIndex(place.type, place.index, -1);
  return { tab, block, quote, start: encode(start), end: encode(end) };
}

interface Span {
  start: Y.RelativePosition;
  end: Y.RelativePosition;
}

/** Whether the words between two positions have been deleted: both ends have fallen to one place, or into something that is gone. */
function gone(doc: Y.Doc, { start, end }: Span): boolean {
  const a = Y.createAbsolutePositionFromRelativePosition(start, doc);
  const b = Y.createAbsolutePositionFromRelativePosition(end, doc);
  return a === null || b === null || (a.type === b.type && a.index >= b.index);
}

interface Piece {
  text: Y.XmlText;
  chars: string;
}

/** The runs of text of a block, in order; the parts of a block that are not text (a line break, a list item's edge) are a newline between two runs. */
function pieces(node: Y.XmlElement | Y.XmlText | Y.XmlHook, into: Piece[] = []): Piece[] {
  if (node instanceof Y.XmlText) {
    const delta = node.toDelta() as { insert?: unknown }[];
    const chars = delta.map((op) => (typeof op.insert === "string" ? op.insert : "")).join("");
    into.push({ text: node, chars });
  } else if (node instanceof Y.XmlElement) {
    for (const child of node.toArray()) pieces(child, into);
  }
  return into;
}

/** The words found again by the block's id and the quote, as positions in whatever the block is made of now. */
function byQuote(doc: Y.Doc, a: DocAnchor): Span | null {
  const tab = tabElement(doc, a.tab);
  if (tab === null || a.quote === "") return null;
  const block = tab.toArray().find((n): n is Y.XmlElement => n instanceof Y.XmlElement && n.getAttribute("id") === a.block);
  if (block === undefined) return null;
  const runs = pieces(block);
  const at = runs.map((r) => r.chars).join("\n").indexOf(a.quote);
  if (at < 0) return null;
  /** The run and the place in it of the character at `index` of the joined text. */
  const place = (index: number): { run: Y.XmlText; offset: number } => {
    let before = 0;
    for (const r of runs) {
      if (index <= before + r.chars.length) return { run: r.text, offset: index - before };
      before += r.chars.length + 1;
    }
    throw new Error("the quote was found past the end of the block");
  };
  const first = place(at);
  const last = place(at + a.quote.length - 1);
  return { start: Y.createRelativePositionFromTypeIndex(first.run, first.offset), end: Y.createRelativePositionFromTypeIndex(last.run, last.offset + 1, -1) };
}

/** Where the words are in the room now, or null when they are gone. */
function locate(doc: Y.Doc, a: DocAnchor): Span | null {
  const start = decode(a.start);
  const end = decode(a.end);
  if (start !== null && end !== null && !gone(doc, { start, end })) return { start, end };
  return byQuote(doc, a);
}

/** Whether the words an anchor points at are still in the room, in any tab. */
export function standing(doc: Y.Doc, a: DocAnchor): Standing {
  return locate(doc, a) === null ? "changed" : "found";
}

/** The words of the anchor in the tab the editor shows, as a range of its document; null in another tab or when they are gone. */
export function rangeOf(state: EditorState, a: DocAnchor): { from: number; to: number } | null {
  const binding = bindingOf(state);
  if (!binding || state.doc.firstChild?.attrs.id !== a.tab) return null;
  const found = locate(binding.doc, a);
  if (found === null) return null;
  const from = relativePositionToAbsolutePosition(binding.doc, binding.type, found.start, binding.mapping);
  const to = relativePositionToAbsolutePosition(binding.doc, binding.type, found.end, binding.mapping);
  const size = state.doc.content.size;
  return from === null || to === null || from >= to || to > size ? null : { from, to };
}
