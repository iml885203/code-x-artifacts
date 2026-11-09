// A frame's tree as the room holds it (docs/design-room.md): flat nodes with a
// parent and a fractional order, read here into what the outline shows and what a
// move needs. The spelling of the keys and the order arithmetic are internal/
// designroom's: the same keys, and `between` gives the same key for the same
// neighbours, so a move written here is read there.
import * as Y from "yjs";

const DIGITS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
const BASE = DIGITS.length;
const digitAt = (s: string, i: number) => (i >= s.length ? 0 : DIGITS.indexOf(s.charAt(i)));

/** An order key strictly after a and before b (an empty a is the start, an empty b the end): designroom's `between`. */
export function between(a: string, b: string): string {
  if (b !== "" && a >= b) b = ""; // no room between equal keys: go after a
  let out = "";
  let constrained = b !== "";
  for (let i = 0; ; i++) {
    const lo = digitAt(a, i);
    const hi = constrained ? digitAt(b, i) : BASE;
    if (lo + 1 < hi) return out + DIGITS.charAt(Math.floor((lo + hi) / 2));
    out += DIGITS.charAt(lo);
    if (constrained && lo < hi) constrained = false;
  }
}

export interface TreeNode {
  pid: string;
  kind: "el" | "text" | "comment";
  tag: string;
  parent: string;
  order: string;
  /** The `data-name` the element was given, or "". */
  name: string;
  /** What the first text inside the element says, for a row to show when it has no name. */
  text: string;
  /** The element has `display:none`, written by the outline or by whoever wrote the frame. */
  hidden: boolean;
  /** Its own markup says `display:none`, which only a declaration that replaces it can undo. */
  authoredHidden: boolean;
  /** Everything inside it, text and comments too, in order. */
  children: string[];
}

export interface FrameTree {
  root: string;
  nodes: Map<string, TreeNode>;
}

/** The value of a `display` declaration without its !important, lower case. */
const displayOf = (value: string) => value.replace(/!important\s*$/i, "").trim().toLowerCase();

/** Whether a style attribute (the skeleton the room keeps, with its slots) declares display:none. */
function declaresNone(skeleton: string): boolean {
  let none = false;
  for (const part of skeleton.split(";")) {
    const colon = part.indexOf(":");
    if (colon > 0 && part.slice(0, colon).trim().toLowerCase() === "display") none = displayOf(part.slice(colon + 1)) === "none";
  }
  return none;
}

/** The `data-name` in an element's attribute list entry (a JSON array of [namespace, key, value]). */
export function nameIn(attrs: unknown): string {
  for (const [ns, key, value] of parseAttrs(attrs)) if (ns === "" && key === "data-name") return value;
  return "";
}

export function parseAttrs(attrs: unknown): [string, string, string][] {
  if (typeof attrs !== "string" || attrs === "") return [];
  try {
    const rows: unknown = JSON.parse(attrs);
    if (!Array.isArray(rows)) return [];
    return rows.filter((r): r is [string, string, string] => Array.isArray(r) && r.length === 3 && r.every((c) => typeof c === "string"));
  } catch {
    return [];
  }
}

/** The tree of one frame. An element whose parent is gone (someone deleted it as another moved something into it) sits at the end of the body, as the server reads it. */
export function readTree(map: Y.Map<unknown>, file: string): FrameTree {
  const prefix = `${file}|n|`;
  const fields = new Map<string, Map<string, unknown>>();
  map.forEach((value, key) => {
    if (!key.startsWith(prefix)) return;
    const rest = key.slice(prefix.length);
    const bar = rest.indexOf("|");
    if (bar < 0) return;
    const pid = rest.slice(0, bar);
    let own = fields.get(pid);
    if (!own) fields.set(pid, (own = new Map<string, unknown>()));
    own.set(rest.slice(bar + 1), value);
  });
  const str = (v: unknown) => (typeof v === "string" ? v : "");
  const rootValue = map.get(`${file}|shell|root`);
  const root = typeof rootValue === "string" ? rootValue : "";
  const nodes = new Map<string, TreeNode>();
  for (const [pid, f] of fields) {
    const kind = f.get("kind");
    if (kind !== "el" && kind !== "text" && kind !== "comment") continue;
    const display = f.get("s|display");
    const state = f.get("hidden");
    const text = f.get("text");
    nodes.set(pid, {
      pid,
      kind,
      tag: str(f.get("tag")),
      parent: str(f.get("parent")),
      order: str(f.get("order")),
      name: nameIn(f.get("attrs")),
      text: text instanceof Y.Text ? text.toJSON() : str(text),
      // The editor's own state wins; without one, a display written by an older client or the element's own markup says.
      hidden: state === "1" ? true : state === "0" ? false : typeof display === "string" ? displayOf(display) === "none" : declaresNone(str(f.get("rest"))),
      authoredHidden: declaresNone(str(f.get("rest"))),
      children: [],
    });
  }
  const members: Record<string, TreeNode[]> = {};
  for (const n of nodes.values()) {
    if (n.pid === root) continue;
    if (!nodes.has(n.parent)) {
      n.parent = root;
      n.order = `~${n.pid}`;
    }
    (members[n.parent] ??= []).push(n);
  }
  for (const [parent, list] of Object.entries(members)) {
    list.sort((a, b) => (a.order < b.order ? -1 : a.order > b.order ? 1 : a.pid < b.pid ? -1 : 1));
    const holder = nodes.get(parent);
    if (holder) holder.children = list.map((n) => n.pid);
  }
  return { root, nodes };
}

/** The element children of each element, in order: what a frame is told to show when someone reorders. */
export function elementOrder(tree: FrameTree): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const n of tree.nodes.values()) {
    if (n.kind !== "el") continue;
    const kids = n.children.filter((c) => tree.nodes.get(c)?.kind === "el");
    if (kids.length > 0) out.set(n.pid, kids);
  }
  return out;
}

/** Whether `pid` is `of` or inside it. */
export function isWithin(tree: FrameTree, pid: string, of: string): boolean {
  for (let at: string | undefined = pid, steps = 0; at !== undefined && steps <= tree.nodes.size; steps++) {
    if (at === of) return true;
    at = tree.nodes.get(at)?.parent;
  }
  return false;
}

export interface Placement {
  parent: string;
  order: string;
}

/**
 * Where `pid` goes to be a child of `parent` right before `before` (the end
 * when null). Null when the move is refused: the body, something that is not an
 * element, or into itself or something inside it.
 */
export function placement(tree: FrameTree, pid: string, parent: string, before: string | null): Placement | null {
  const moved = tree.nodes.get(pid);
  const target = tree.nodes.get(parent);
  if (moved?.kind !== "el" || pid === tree.root || target?.kind !== "el") return null;
  if (isWithin(tree, parent, pid)) return null;
  const siblings = target.children.filter((c) => c !== pid);
  let at = siblings.length;
  if (before !== null) {
    at = siblings.indexOf(before);
    if (at < 0) return null;
  }
  const lo = at > 0 ? (tree.nodes.get(siblings[at - 1] ?? "")?.order ?? "") : "";
  const hi = at < siblings.length ? (tree.nodes.get(siblings[at] ?? "")?.order ?? "") : "";
  return { parent, order: between(lo, hi) };
}
