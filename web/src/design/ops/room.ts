// What the editor's operations write into a design's room (docs/design-room.md):
// the entries of the shared map `design` that internal/designroom reads, spelled
// the same way. Nothing here touches a frame or the person's history: the
// functions write into a Y.Map inside whatever transaction the caller opened, so
// an operation is one step of that person's undo.
import * as Y from "yjs";
import { between, readTree, type FrameTree } from "../tree";

const ALPHABET = "abcdefghijklmnopqrstuvwxyz0123456789";

function random(length: number): string {
  const bytes = crypto.getRandomValues(new Uint8Array(length));
  return Array.from(bytes, (b) => ALPHABET.charAt(b % ALPHABET.length)).join("");
}

/** A new element's id: a `data-pid` no element of a frame has (the server's own are `e<number>`). */
export const newPid = (): string => `n${random(9)}`;
/** A new text node's id (the `~` keeps it from ever being a pid). */
export const newTextId = (): string => `~t${random(9)}`;

export const nodeKey = (file: string, pid: string, field: string) => `${file}|n|${pid}|${field}`;

/** The id of a frame's body. */
export function rootOf(map: Y.Map<unknown>, file: string): string {
  const root = map.get(`${file}|shell|root`);
  return typeof root === "string" ? root : "";
}

/** A value an entry holds, as it can be kept and written again: a string, or the characters or the runs of a shared text. */
export type Value = string | { text: string } | { rich: unknown[] };

export interface SavedNode {
  /** The id the node had. */
  pid: string;
  /** The id of its parent; empty for the root of the copy. */
  parent: string;
  /** Its place among its siblings, which a copy of the whole subtree keeps. */
  order: string;
  fields: Record<string, Value>;
}

/** An element with everything inside it, as it was when it was copied: parents before their children. */
export interface Snapshot {
  root: string;
  nodes: SavedNode[];
}

const own = (pid: string, file: string) => `${file}|n|${pid}|`;

/** Everything the room holds of the element and what is inside it; null for what is not an element. */
export function capture(map: Y.Map<unknown>, file: string, pid: string, tree: FrameTree = readTree(map, file)): Snapshot | null {
  const top = tree.nodes.get(pid);
  if (top?.kind !== "el" || pid === tree.root) return null;
  const nodes: SavedNode[] = [];
  const visit = (id: string, parent: string) => {
    const node = tree.nodes.get(id);
    if (!node) return;
    const fields: Record<string, Value> = {};
    const prefix = own(id, file);
    map.forEach((value, key) => {
      if (!key.startsWith(prefix)) return;
      const field = key.slice(prefix.length);
      if (field === "parent" || field === "order") return;
      if (value instanceof Y.Text) fields[field] = field === "rich" ? { rich: value.toDelta() as unknown[] } : { text: value.toJSON() };
      else if (typeof value === "string") fields[field] = value;
    });
    nodes.push({ pid: id, parent, order: node.order, fields });
    for (const child of node.children) visit(child, id);
  };
  visit(pid, "");
  return { root: pid, nodes };
}

/** The data-pid attributes in a piece of markup, each given the id `rename` says. */
const renameIn = (markup: string, rename: (pid: string) => string) => markup.replace(/(data-pid=")([^"]*)(")/g, (_m, a: string, pid: string, c: string) => a + rename(pid) + c);

/** The pids inside an attribute list kept as JSON. */
const renamedList = (json: string, rename: (pid: string) => string) => json.replace(/"data-pid","([^"]*)"/g, (_m, pid: string) => `"data-pid","${rename(pid)}"`);

/** A formatted text's runs with the inline elements' ids renamed: the formats' keys and the ids inside them, and the markup kept as written. */
function renamedRuns(runs: unknown[], rename: (pid: string) => string): unknown[] {
  return runs.map((run) => {
    const r = run as { insert?: unknown; attributes?: Record<string, unknown> };
    const out: Record<string, unknown> = { ...r };
    if (r.attributes) {
      const attributes: Record<string, unknown> = {};
      for (const [name, value] of Object.entries(r.attributes)) {
        if (!name.startsWith("f:") || typeof value !== "string") {
          attributes[name] = value;
          continue;
        }
        attributes[`f:${rename(name.slice(2))}`] = renamedList(value, rename);
      }
      out.attributes = attributes;
    }
    const embed = r.insert;
    const marked = embed as { raw?: unknown; br?: unknown } | undefined;
    if (marked && typeof marked === "object" && typeof marked.raw === "string") out.insert = { ...marked, raw: renameIn(marked.raw, rename) };
    else if (marked && typeof marked === "object" && typeof marked.br === "string") out.insert = { ...marked, br: renamedList(marked.br, rename) };
    return out;
  });
}

export interface Placement {
  parent: string;
  order: string;
  /** Where the copy of an absolutely positioned element goes, in the pixels of its containing block. */
  left?: number;
  top?: number;
}

/** Writes a copy of a snapshot under new ids; returns the copy's root id. */
export function restore(map: Y.Map<unknown>, file: string, snap: Snapshot, at: Placement): string {
  const renamed = new Map<string, string>();
  const rename = (pid: string) => {
    let next = renamed.get(pid);
    if (!next) renamed.set(pid, (next = newPid()));
    return next;
  };
  const ids = new Map(snap.nodes.map((n) => [n.pid, n.fields.kind === "el" ? rename(n.pid) : newTextId()]));
  for (const n of snap.nodes) {
    const id = ids.get(n.pid) ?? "";
    const isRoot = n.pid === snap.root;
    for (const [field, value] of Object.entries(n.fields)) {
      let written: unknown = value;
      if (typeof value === "object" && "text" in value) written = new Y.Text(value.text);
      else if (typeof value === "object") {
        const text = new Y.Text();
        text.applyDelta(renamedRuns(value.rich, rename));
        written = text;
      }
      map.set(nodeKey(file, id, field), written);
    }
    map.set(nodeKey(file, id, "parent"), isRoot ? at.parent : (ids.get(n.parent) ?? ""));
    if (isRoot) {
      map.set(nodeKey(file, id, "order"), at.order);
      if (at.left !== undefined) map.set(nodeKey(file, id, "s|left"), `${String(Math.round(at.left))}px`);
      if (at.top !== undefined) map.set(nodeKey(file, id, "s|top"), `${String(Math.round(at.top))}px`);
    } else {
      // The children keep the order they had among their siblings, which were copied with them.
      map.set(nodeKey(file, id, "order"), n.order);
    }
  }
  return ids.get(snap.root) ?? "";
}

/** The order key for a node placed after every child of `parent`. */
export function lastOrder(tree: FrameTree, parent: string): string {
  const kids = tree.nodes.get(parent)?.children ?? [];
  return between(tree.nodes.get(kids[kids.length - 1] ?? "")?.order ?? "", "");
}

/** The element children of a parent, in order. */
export function elementsOf(tree: FrameTree, parent: string): string[] {
  return (tree.nodes.get(parent)?.children ?? []).filter((c) => tree.nodes.get(c)?.kind === "el");
}

/** The value to write for a changed number: the old value's spacing before it and its !important after it, so only the number differs in the saved file. */
export function keepSpacing(old: unknown, value: string): string {
  if (typeof old !== "string" || value === "") return value;
  const lead = /^\s*/.exec(old)?.[0] ?? "";
  const important = /\s*!important\s*$/i.exec(old)?.[0] ?? "";
  return lead + value + important;
}

/** A style entry of an element as a number of pixels, or null when it is not one. */
export function pixels(map: Y.Map<unknown>, file: string, pid: string, prop: string): number | null {
  const v = map.get(nodeKey(file, pid, `s|${prop}`));
  if (typeof v !== "string") return null;
  const m = /^\s*(-?\d+(?:\.\d+)?)px\s*(?:!important)?\s*$/i.exec(v);
  return m ? Number(m[1]) : null;
}
