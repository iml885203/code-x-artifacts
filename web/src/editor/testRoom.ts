// What the tests of the editor's view share: a room made from blocks, a person's
// editor on a copy of it, and the relay that joins two copies.
import type { Node } from "prosemirror-model";
import { TextSelection } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { prosemirrorJSONToYXmlFragment } from "y-prosemirror";
import { Awareness } from "y-protocols/awareness";
import * as Y from "yjs";
import { afterEach } from "vitest";
import { schema } from "./schema";
import { orderName, tabFragmentName } from "./tabs";
import type { Hooks } from "./feature";
import { mount, type LiveView } from "./view";

const open: LiveView[] = [];
afterEach(() => {
  for (const v of open.splice(0)) v.destroy();
});

export const text = (s: string) => ({ type: "text", text: s });
export const para = (id: string, s: string) => ({ type: "paragraph", attrs: { id }, content: s === "" ? [] : [text(s)] });
export const docJSON = (...blocks: unknown[]) => ({ type: "doc", content: [{ type: "tab", attrs: { id: "t1", title: "" }, content: blocks }] });

export function room(...blocks: unknown[]): Y.Doc {
  return tabbed({ id: "t1", title: "", blocks });
}

/** A room of tabs as the editor keeps it: the order of the ids and a fragment per tab (docs/doc-room.md). */
export function tabbed(...tabs: { id: string; title: string; blocks: unknown[] }[]): Y.Doc {
  const doc = new Y.Doc();
  for (const t of tabs) {
    const json = { type: "doc", content: [{ type: "tab", attrs: { id: t.id, title: t.title }, content: t.blocks }] };
    prosemirrorJSONToYXmlFragment(schema, json, doc.getXmlFragment(tabFragmentName(t.id)));
  }
  doc.getArray(orderName).push(tabs.map((t) => t.id));
  return doc;
}

/** One person's editor on a copy of the room, joined to the others by handing every update on. */
export function person(doc: Y.Doc, editable = true, tab?: string, undo?: Y.UndoManager, hooks: Partial<Hooks> = {}): EditorView {
  const parent = document.createElement("div");
  document.body.append(parent);
  const live = mount(parent, doc, new Awareness(doc), editable, "Document", hooks, tab, undo);
  open.push({
    view: live.view,
    destroy: () => {
      live.destroy();
      parent.remove();
    },
  });
  return live.view;
}

export function join(a: Y.Doc, b: Y.Doc): void {
  Y.applyUpdate(b, Y.encodeStateAsUpdate(a), "relay");
  const relay = (to: Y.Doc) => (u: Uint8Array, origin: unknown) => {
    if (origin !== "relay") Y.applyUpdate(to, u, "relay");
  };
  a.on("update", relay(b));
  b.on("update", relay(a));
}

/** The room as a new browser would load it: the same content, nothing else. */
export function reload(doc: Y.Doc): Y.Doc {
  const fresh = new Y.Doc();
  Y.applyUpdate(fresh, Y.encodeStateAsUpdate(doc));
  return fresh;
}

export function typeAt(view: EditorView, pos: number, s: string): void {
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, pos)).insertText(s));
}

/** The caret (or selection) in the text of the first block that holds `inside`, from `a` to `b` characters in. */
export function select(view: EditorView, inside: string, a = 0, b = a): void {
  const at = rangeOf(view, inside, inside);
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, at.from + a, at.from + b)));
}

/** The text of the first block that holds `first` through the last block that holds `last`. */
function rangeOf(view: EditorView, first: string, last: string): { from: number; to: number } {
  const found: { from?: number; to?: number } = {};
  view.state.doc.descendants((node, pos) => {
    if (!node.isTextblock) return true;
    if (found.from === undefined && node.textContent.includes(first)) found.from = pos + 1;
    if (found.from !== undefined && node.textContent.includes(last) && (found.to === undefined || first !== last)) found.to = pos + node.nodeSize - 1;
    return false;
  });
  if (found.from === undefined || found.to === undefined) throw new Error(`no blocks hold "${first}" and "${last}"`);
  return { from: found.from, to: found.to };
}

/** A selection from the start of the block that holds `first` to the end of the block that holds `last`. */
export function selectBetween(view: EditorView, first: string, last: string): void {
  const { from, to } = rangeOf(view, first, last);
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, from, to)));
}

/** The first tab of the document as a short outline: `p(text)`, `ul[li[p(a)]]`... */
export function outline(view: EditorView): string {
  const short: Record<string, string> = { bullet_list: "ul", numbered_list: "ol", task_list: "todo", list_item: "li", quote: "q", code_block: "code", rule: "hr", paragraph: "p", heading: "h" };
  const one = (n: Node): string => {
    const name = short[n.type.name] ?? n.type.name;
    if (n.type.name === "rule") return name;
    const mark = n.type.name === "list_item" && n.attrs.checked !== null ? (n.attrs.checked === true ? "x" : "_") : "";
    if (n.isTextblock) return `${name}${n.type.name === "heading" ? String(n.attrs.level) : ""}(${n.textContent})`;
    const inner: string[] = [];
    n.forEach((c) => inner.push(one(c)));
    return `${name}${mark}[${inner.join(" ")}]`;
  };
  const out: string[] = [];
  view.state.doc.firstChild?.forEach((c) => out.push(one(c)));
  return out.join(" ");
}

/** Every block of the first tab, as the JSON the room holds. */
export const blocks = (view: EditorView): Record<string, unknown>[] => {
  const tab = view.state.doc.toJSON() as { content: { content?: Record<string, unknown>[] }[] };
  return tab.content[0]?.content ?? [];
};
