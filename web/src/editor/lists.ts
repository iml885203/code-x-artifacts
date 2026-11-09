// Lists in the editor: bullet, numbered and to-do, nested by Tab.
//
// A list item is a paragraph and then at most one list nested under it
// (internal/docroom/interop/schema.mjs). Changing the nesting of an item by
// hand-editing that tree is where bugs live, so every command here does the
// same three steps: read the blocks the selection touches as flat lines (an
// item is a line with a depth), change the lines, build the tree again. A nested
// list has one type, so the items of one run take the type of the first line a
// command touched.
import { Fragment, type Node, type NodeType } from "prosemirror-model";
import { TextSelection, type Command, type EditorState } from "prosemirror-state";
import { schema } from "./schema";

export type ListKind = "bullet_list" | "numbered_list" | "task_list";
const kinds: ListKind[] = ["bullet_list", "numbered_list", "task_list"];

const isKind = (name: string): name is ListKind => (kinds as string[]).includes(name);

interface Item {
  k: "item";
  depth: number;
  type: ListKind;
  /** The paragraph of the item. */
  text: Node;
  checked: boolean;
  /** A command set this line's type on purpose: its nested run follows it. */
  touched: boolean;
  selected: boolean;
  /** The attributes of the top-level list the item came from. */
  list: { id: unknown; start: unknown } | null;
}
/** A paragraph or heading that can become an item. */
interface Text {
  k: "text";
  node: Node;
  selected: boolean;
}
interface Other {
  k: "other";
  node: Node;
}
type Entry = Item | Text | Other;

const type = (name: string): NodeType => {
  const t = schema.nodes[name];
  if (t === undefined) throw new Error(`the editor's schema has no node ${name}`);
  return t;
};

/** The container blocks live in: a tab or a quote. */
function containerDepth(state: EditorState): number {
  const { $from } = state.selection;
  for (let d = $from.depth; d > 0; d--) {
    const name = $from.node(d).type.name;
    if (name === "tab" || name === "quote") return d;
  }
  return 0;
}

interface Collected {
  /** The blocks of the container that the selection touches are replaced as one range. */
  from: number;
  to: number;
  entries: Entry[];
}

function flatten(list: Node, pos: number, depth: number, from: number, to: number, top: Node | null, out: Entry[]): void {
  const listType = list.type.name;
  if (!isKind(listType)) return;
  const topAttrs = top === null ? { id: list.attrs.id as unknown, start: list.attrs.start as unknown } : null;
  let at = pos + 1;
  list.forEach((li) => {
    const text = li.child(0);
    const start = at + 1;
    const selected = from <= start + text.nodeSize - 1 && to >= start + 1;
    out.push({ k: "item", depth, type: listType, text, checked: li.attrs.checked === true, touched: false, selected, list: topAttrs });
    const nested = li.childCount > 1 ? li.child(1) : null;
    if (nested !== null) flatten(nested, start + text.nodeSize, depth + 1, from, to, top ?? list, out);
    at += li.nodeSize;
  });
}

function collect(state: EditorState): Collected | null {
  const { $from, from, to: selTo } = state.selection;
  const depth = containerDepth(state);
  const container = $from.node(depth);
  const base = $from.start(depth);
  const to = Math.min(selTo, $from.end(depth));
  let first = -1;
  let last = -1;
  const entries: Entry[] = [];
  const blocks: { node: Node; start: number }[] = [];
  let at = base;
  container.forEach((child, _o, index) => {
    blocks.push({ node: child, start: at });
    if (at <= to && at + child.nodeSize >= from) {
      if (first < 0) first = index;
      last = index;
    }
    at += child.nodeSize;
  });
  if (first < 0) return null;
  for (let i = first; i <= last; i++) {
    const b = blocks[i];
    if (b === undefined) continue;
    const name = b.node.type.name;
    if (name === "paragraph" || name === "heading") {
      entries.push({ k: "text", node: b.node, selected: b.start + 1 <= to && b.start + b.node.nodeSize - 1 >= from });
    } else if (isKind(name)) {
      flatten(b.node, b.start, 0, from, to, null, entries);
    } else {
      entries.push({ k: "other", node: b.node });
    }
  }
  const startBlock = blocks[first];
  const endBlock = blocks[last];
  if (startBlock === undefined || endBlock === undefined) return null;
  return { from: startBlock.start, to: endBlock.start + endBlock.node.nodeSize, entries };
}

/** An item sits no deeper than one under the line above it, and the first line of a list is at the top. */
function clamp(entries: Entry[]): void {
  let prev = -1;
  for (const e of entries) {
    if (e.k !== "item") {
      prev = -1;
      continue;
    }
    e.depth = Math.max(0, Math.min(e.depth, prev + 1));
    prev = e.depth;
  }
}

function itemNode(line: Item, kind: ListKind, nested: Node | null): Node {
  const attrs = { checked: kind === "task_list" ? line.checked : null };
  return type("list_item").create(attrs, nested === null ? [line.text] : [line.text, nested]);
}

/** The list that starts at lines[from] (all at `depth`), and where the next line after it is. */
function level(lines: Item[], from: number, depth: number, used: Set<unknown>): { list: Node; next: number } {
  const run: Item[] = [];
  let end = from;
  const head = lines[from];
  if (head === undefined) throw new Error("a list needs a first line");
  while (end < lines.length) {
    const line = lines[end];
    if (line === undefined || line.depth < depth) break;
    if (depth === 0 && line.depth === 0 && line.type !== head.type) break;
    if (line.depth === depth) run.push(line);
    end++;
  }
  const kind = depth === 0 ? head.type : (run.find((l) => l.touched) ?? head).type;
  const items: Node[] = [];
  let i = from;
  while (i < end) {
    const line = lines[i];
    if (line === undefined) break;
    i++;
    let nested: Node | null = null;
    const after = lines[i];
    if (i < end && after !== undefined && after.depth > depth) {
      const r = level(lines, i, depth + 1, used);
      nested = r.list;
      i = r.next;
    }
    items.push(itemNode(line, kind, nested));
  }
  let attrs: Record<string, unknown> | null = null;
  if (depth === 0) {
    const id = head.list?.id ?? null;
    const own = id !== null && !used.has(id) ? id : null;
    if (own !== null) used.add(own);
    attrs = { id: own, ...(kind === "numbered_list" ? { start: head.list?.start ?? 1 } : {}) };
  }
  return { list: type(kind).create(attrs, items), next: end };
}

function emit(entries: Entry[]): Fragment {
  const nodes: Node[] = [];
  const used = new Set<unknown>();
  let i = 0;
  while (i < entries.length) {
    const e = entries[i];
    if (e === undefined) break;
    if (e.k === "item") {
      const lines: Item[] = [];
      while (i < entries.length) {
        const n = entries[i];
        if (n?.k !== "item") break;
        lines.push(n);
        i++;
      }
      let at = 0;
      while (at < lines.length) {
        const r = level(lines, at, 0, used);
        nodes.push(r.list);
        at = r.next;
      }
      continue;
    }
    nodes.push(e.node);
    i++;
  }
  return Fragment.from(nodes);
}

const plain = (text: Node): Text => ({ k: "text", node: type("paragraph").create(null, text.content), selected: true });

/** The textblock the position is in, counted from the start of the range, with the offset inside it. */
function placeOf(state: EditorState, from: number, to: number, pos: number): { index: number; offset: number } {
  let index = 0;
  let found = { index: 0, offset: 0 };
  state.doc.nodesBetween(from, to, (node, p) => {
    if (!node.isTextblock) return true;
    if (pos >= p + 1 && pos <= p + node.nodeSize - 1) found = { index, offset: pos - (p + 1) };
    index++;
    return false;
  });
  return found;
}

function posOf(doc: Node, from: number, to: number, place: { index: number; offset: number }): number {
  let index = 0;
  let pos = from + 1;
  doc.nodesBetween(from, to, (node, p) => {
    if (!node.isTextblock) return true;
    if (index === place.index) pos = p + 1 + Math.min(place.offset, node.content.size);
    index++;
    return false;
  });
  return pos;
}

/** Applies `change` to the lines the selection touches; false (and nothing done) when it changes none. */
function transform(change: (entries: Entry[]) => boolean): Command {
  return (state, dispatch) => {
    const got = collect(state);
    if (!got?.entries.some((e) => e.k !== "other" && e.selected)) return false;
    if (!change(got.entries)) return false;
    clamp(got.entries);
    if (dispatch) {
      const fragment = emit(got.entries);
      const anchor = placeOf(state, got.from, got.to, state.selection.anchor);
      const head = placeOf(state, got.from, got.to, state.selection.head);
      const tr = state.tr.replaceWith(got.from, got.to, fragment);
      const end = got.from + fragment.size;
      tr.setSelection(TextSelection.create(tr.doc, posOf(tr.doc, got.from, end, anchor), posOf(tr.doc, got.from, end, head)));
      dispatch(tr.scrollIntoView());
    }
    return true;
  };
}

/** Makes the selected blocks a list of this kind; when they all are one already, makes them paragraphs again. */
export function toggleList(kind: ListKind): Command {
  return transform((entries) => {
    const chosen = entries.filter((e): e is Item | Text => (e.k === "item" || e.k === "text") && e.selected);
    if (chosen.length === 0) return false;
    const already = chosen.every((e) => e.k === "item" && e.type === kind);
    entries.forEach((e, i) => {
      if (e.k === "item" && e.selected) {
        if (already) entries[i] = plain(e.text);
        else {
          e.type = kind;
          e.touched = true;
        }
      } else if (e.k === "text" && e.selected && !already) {
        const id: unknown = e.node.attrs.id;
        entries[i] = { k: "item", depth: 0, type: kind, text: type("paragraph").create(null, e.node.content), checked: false, touched: true, selected: true, list: { id, start: 1 } };
      }
    });
    return true;
  });
}

/** The lines to move: selected items that are not inside another selected item's subtree. */
function roots(entries: Entry[]): number[] {
  const found: number[] = [];
  let until = -1;
  entries.forEach((e, i) => {
    if (i < until || e.k !== "item" || !e.selected) return;
    found.push(i);
    let k = i + 1;
    while (k < entries.length) {
      const n = entries[k];
      if (n?.k !== "item" || n.depth <= e.depth) break;
      k++;
    }
    until = k;
  });
  return found;
}

/** Tab: each selected item goes under the item above it, taking its own nested items along. */
export const indentItem: Command = transform((entries) => {
  let changed = false;
  for (const i of roots(entries)) {
    const line = entries[i];
    const above = entries[i - 1];
    if (line?.k !== "item" || above?.k !== "item" || line.depth > above.depth) continue;
    const was = line.depth;
    line.depth = was + 1;
    for (let k = i + 1; k < entries.length; k++) {
      const n = entries[k];
      if (n?.k !== "item" || n.depth <= was) break;
      n.depth += 1;
    }
    changed = true;
  }
  return changed;
});

/** Shift+Tab: each selected item goes up a level; at the top it becomes a paragraph. The items after it follow it as its nested items. */
export const outdentItem: Command = transform((entries) => {
  const moving = roots(entries);
  for (const i of moving) {
    const line = entries[i];
    if (line?.k !== "item") continue;
    if (line.depth === 0) entries[i] = plain(line.text);
    else line.depth -= 1;
  }
  return moving.length > 0;
});

/** The item the caret is in: its list item node, its depth in the document and the index of it in its list. */
export function itemAt(state: EditorState): { depth: number; node: Node; index: number } | null {
  const { $from } = state.selection;
  for (let d = $from.depth; d > 0; d--) {
    if ($from.node(d).type.name === "list_item") return { depth: d, node: $from.node(d), index: $from.index(d - 1) };
  }
  return null;
}

/** The innermost list the caret is in, for the toolbar. */
export function listAt(state: EditorState): ListKind | null {
  const { $from } = state.selection;
  for (let d = $from.depth; d > 0; d--) {
    const name = $from.node(d).type.name;
    if (isKind(name)) return name;
  }
  return null;
}

/** Enter in an item starts the next item; in an empty one it ends the list level (outdent). */
export const splitItem: Command = (state, dispatch) => {
  const at = itemAt(state);
  const { $from } = state.selection;
  if (at === null || $from.parent.type.name !== "paragraph" || $from.depth !== at.depth + 1) return false;
  if (!state.selection.empty) return false;
  if ($from.parent.content.size === 0) return outdentItem(state, dispatch);
  if (dispatch) {
    const text = $from.parent;
    const offset = $from.parentOffset;
    const first = type("paragraph").create(null, text.content.cut(0, offset));
    const second = type("paragraph").create(null, text.content.cut(offset));
    const nested = at.node.childCount > 1 ? at.node.child(1) : null;
    const attrs = at.node.attrs.checked === null ? at.node.attrs : { ...at.node.attrs, checked: false };
    const one = type("list_item").create(at.node.attrs, [first]);
    const two = type("list_item").create(attrs, nested === null ? [second] : [second, nested]);
    const start = $from.before(at.depth);
    const tr = state.tr.replaceWith(start, start + at.node.nodeSize, [one, two]);
    tr.setSelection(TextSelection.create(tr.doc, start + one.nodeSize + 2));
    dispatch(tr.scrollIntoView());
  }
  return true;
};

/** Backspace at the start of an item: it joins the item above when both are plain, and otherwise goes up a level. */
export const backspaceItem: Command = (state, dispatch) => {
  const at = itemAt(state);
  const { $from } = state.selection;
  if (at === null || !state.selection.empty || $from.parentOffset !== 0 || $from.depth !== at.depth + 1) return false;
  if (at.index > 0 && at.node.childCount === 1 && $from.parent.content.size > 0) {
    const list = $from.node(at.depth - 1);
    const above = list.child(at.index - 1);
    if (above.childCount === 1) {
      if (dispatch) {
        const start = $from.before(at.depth);
        const joined = type("paragraph").create(null, above.child(0).content.append($from.parent.content));
        const merged = type("list_item").create(above.attrs, [joined]);
        const from = start - above.nodeSize;
        const tr = state.tr.replaceWith(from, start + at.node.nodeSize, merged);
        tr.setSelection(TextSelection.create(tr.doc, from + 2 + above.child(0).content.size));
        dispatch(tr.scrollIntoView());
      }
      return true;
    }
  }
  return outdentItem(state, dispatch);
};
