// Tabs of a doc in the editor (docs/doc-room.md): the order of a doc's tabs is a shared array of tab ids, and each tab is a
// fragment of its own that holds the tab element and its blocks. The four things a person does to tabs (add, rename, delete,
// move) are edits of that array and of the tab element's title, so moving a tab only reorders ids: what other people are
// typing in the tab is never re-created, and the server's four doc_edit tab ops write the same shape.
import * as Y from "yjs";
import { newBlockId } from "./ids";

/** The shared array of tab ids, in order. */
export const orderName = "tabs";
/** The name of the shared fragment that holds a tab. */
export const tabFragmentName = (id: string): string => `tab:${id}`;

export interface TabInfo {
  id: string;
  title: string;
}

/** The longest title the format allows (docfmt.MaxTitleRunes): a longer one would keep the room from ever being saved. */
export const maxTitleLength = 200;

/** `t_` and eight characters, like the ids the server makes. */
export function newTabId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return "t_" + Array.from(bytes, (b) => "abcdefghijklmnopqrstuvwxyz0123456789"[b % 36]).join("");
}

/**
 * What a title may be so that the doc it is in stays one the server can save: one line, not blank, not too long, nothing
 * the format refuses. A title that is not allowed is null, and the caller keeps the old one.
 */
export function cleanTitle(raw: string): string | null {
  const title = raw.trim();
  if (title === "" || /[\r\n\0]/.test(title) || title.includes("-->") || Array.from(title).length > maxTitleLength) return null;
  return title;
}

/** The ids in order, each once: two people who move a tab at once can leave an id in the array twice. */
function idsOf(doc: Y.Doc): string[] {
  const seen = new Set<string>();
  const ids: string[] = [];
  for (const id of doc.getArray<string>(orderName).toArray()) {
    if (typeof id === "string" && !seen.has(id)) {
      seen.add(id);
      ids.push(id);
    }
  }
  return ids;
}

/** The element of a tab: the one child of its fragment, when it has arrived. */
export function tabElement(doc: Y.Doc, id: string): Y.XmlElement | null {
  const first = doc.getXmlFragment(tabFragmentName(id)).get(0);
  return first instanceof Y.XmlElement && first.nodeName === "tab" ? first : null;
}

/** The tabs in order, with their titles. */
export function tabsOf(doc: Y.Doc): TabInfo[] {
  return idsOf(doc).map((id) => ({ id, title: tabElement(doc, id)?.getAttribute("title") ?? "" }));
}

/** A new tab (an empty heading 1 to type its page's title in) after the tab named, or at the end; its id. */
export function addTab(doc: Y.Doc, title: string, after?: string, id: string = newTabId()): string | null {
  const clean = cleanTitle(title);
  if (clean === null) return null;
  doc.transact(() => {
    const tab = new Y.XmlElement("tab");
    tab.setAttribute("id", id);
    tab.setAttribute("title", clean);
    const heading = new Y.XmlElement("heading");
    heading.setAttribute("id", newBlockId());
    heading.setAttribute("level", 1 as unknown as string);
    tab.insert(0, [heading]);
    doc.getXmlFragment(tabFragmentName(id)).insert(0, [tab]);
    const order = doc.getArray<string>(orderName);
    order.insert(after === undefined ? order.length : rawIndexAfter(order, after), [id]);
  });
  return id;
}

/** Where in the array a new id goes to come right after the one named (past a duplicate of it, if there is one). */
function rawIndexAfter(order: Y.Array<string>, id: string): number {
  const raw = order.toArray();
  const last = raw.lastIndexOf(id);
  return last < 0 ? raw.length : last + 1;
}

export function renameTab(doc: Y.Doc, id: string, title: string): boolean {
  const el = tabElement(doc, id);
  const clean = cleanTitle(title);
  if (!el || clean === null) return false;
  if (el.getAttribute("title") !== clean) el.setAttribute("title", clean);
  return true;
}

/** Deletes a tab with everything in it. The last tab stays: a doc has at least one. */
export function deleteTab(doc: Y.Doc, id: string): boolean {
  const ids = idsOf(doc);
  if (ids.length < 2 || !ids.includes(id)) return false;
  doc.transact(() => {
    const order = doc.getArray<string>(orderName);
    // Every place the id is in (a duplicate from two moves at once goes too).
    for (let i = order.length - 1; i >= 0; i--) if (order.get(i) === id) order.delete(i, 1);
    const fragment = doc.getXmlFragment(tabFragmentName(id));
    if (fragment.length > 0) fragment.delete(0, fragment.length);
  });
  return true;
}

/** Moves a tab one place up (-1) or down (1): the id moves in the array, and the tab's content is not touched. */
export function moveTab(doc: Y.Doc, id: string, by: -1 | 1): boolean {
  const ids = idsOf(doc);
  const at = ids.indexOf(id);
  const neighbour = ids[at + by];
  if (at < 0 || neighbour === undefined) return false;
  doc.transact(() => {
    const order = doc.getArray<string>(orderName);
    order.delete(order.toArray().indexOf(id), 1);
    const place = order.toArray().indexOf(neighbour);
    order.insert(by < 0 ? place : place + 1, [id]);
  });
  return true;
}
