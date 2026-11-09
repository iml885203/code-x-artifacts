// The editor's operations on a frame's elements, as writes into the room's map
// (docs/design-room.md): duplicate and paste, group and ungroup, stacking order,
// and inserting a text box, a shape or an image. Each is called inside one
// transaction by DesignOps, so it is one step of the person's history. They
// write the same entries the server reads (nodeKey in room.ts) and return what
// they made so the canvas can select it; what cannot be done is refused with the
// reason, writing nothing.
import * as Y from "yjs";
import type { Box } from "../geometry";
import { between, parseAttrs, readTree, type FrameTree } from "../tree";
import { capture, elementsOf, keepSpacing, lastOrder, newPid, newTextId, nodeKey, pixels, restore, rootOf, type Snapshot } from "./room";

export interface Ref {
  file: string;
  pid: string;
}

/** An element the operation is asked to work on, with where its frame says it is when it is positioned absolutely. */
export interface Item extends Ref {
  /** Its place and size in pixels in its containing block; null unless the element is positioned absolutely. */
  box: Box | null;
}

export type Refusal = "apart" | "nothing" | "text";
export type Outcome = { done: Ref[] } | { refused: Refusal };

/** How far a copy is from what it was made from, in the frame's pixels. */
export const COPY_OFFSET = 16;

const px = (n: number) => `${String(Math.round(n))}px`;
const kidsOf = (tree: FrameTree, pid: string) => tree.nodes.get(pid)?.children ?? [];

/** The items that are not inside another of the items, in the order of their frame's tree. */
function outermost(map: Y.Map<unknown>, items: Item[]): Item[] {
  const trees = new Map<string, FrameTree>();
  const treeOf = (file: string) => {
    let t = trees.get(file);
    if (!t) trees.set(file, (t = readTree(map, file)));
    return t;
  };
  const chosen = new Set(items.map((i) => `${i.file}\0${i.pid}`));
  return items.filter((i) => {
    const tree = treeOf(i.file);
    const node = tree.nodes.get(i.pid);
    if (node?.kind !== "el" || i.pid === tree.root) return false;
    for (let at = node.parent, steps = 0; at !== "" && steps <= tree.nodes.size; steps++) {
      if (chosen.has(`${i.file}\0${at}`)) return false;
      at = tree.nodes.get(at)?.parent ?? "";
    }
    return true;
  });
}

/** What copying an element takes: everything inside it, and where it was when it is positioned absolutely. */
export interface Copied {
  file: string;
  snapshot: Snapshot;
  left?: number;
  top?: number;
}

export function copyOf(map: Y.Map<unknown>, items: Item[]): Copied[] {
  const out: Copied[] = [];
  for (const i of outermost(map, items)) {
    const snapshot = capture(map, i.file, i.pid);
    if (snapshot) out.push({ file: i.file, snapshot, ...(i.box ? { left: i.box.left, top: i.box.top } : {}) });
  }
  return out;
}

/** Makes a copy of each element right after the original, a little to the side when it is positioned absolutely. */
export function duplicate(map: Y.Map<unknown>, items: Item[]): Outcome {
  const made: Ref[] = [];
  for (const i of outermost(map, items)) {
    const tree = readTree(map, i.file);
    const node = tree.nodes.get(i.pid);
    const snapshot = capture(map, i.file, i.pid, tree);
    if (!node || !snapshot) continue;
    const siblings = kidsOf(tree, node.parent);
    const next = tree.nodes.get(siblings[siblings.indexOf(i.pid) + 1] ?? "")?.order ?? "";
    const pid = restore(map, i.file, snapshot, {
      parent: node.parent,
      order: between(node.order, next),
      ...(i.box ? { left: i.box.left + COPY_OFFSET, top: i.box.top + COPY_OFFSET } : {}),
    });
    made.push({ file: i.file, pid });
  }
  return made.length === 0 ? { refused: "nothing" } : { done: made };
}

/**
 * Puts what was copied at the end of the body of `file`, each a little further
 * from where it was for every time it has been put there (`times`, from 1).
 */
export function paste(map: Y.Map<unknown>, copied: Copied[], file: string, times: number): Outcome {
  const root = rootOf(map, file);
  if (root === "" || copied.length === 0) return { refused: "nothing" };
  const made: Ref[] = [];
  for (const c of copied) {
    const tree = readTree(map, file);
    const pid = restore(map, file, c.snapshot, {
      parent: root,
      order: lastOrder(tree, root),
      ...(c.left !== undefined && c.top !== undefined ? { left: c.left + COPY_OFFSET * times, top: c.top + COPY_OFFSET * times } : {}),
    });
    made.push({ file, pid });
  }
  return { done: made };
}

/**
 * Wraps the elements, which have to be children of one parent, in a new box that
 * takes the place of the first of them. When every one is positioned absolutely the
 * box is positioned where they are, over all of them, and they are placed inside
 * it, so nothing moves on the canvas; Ungroup gives the places back.
 */
export function group(map: Y.Map<unknown>, items: Item[]): Outcome {
  const top = outermost(map, items);
  const first = top[0];
  if (!first) return { refused: "nothing" };
  const tree = readTree(map, first.file);
  const parent = tree.nodes.get(first.pid)?.parent ?? "";
  if (top.some((i) => i.file !== first.file || tree.nodes.get(i.pid)?.parent !== parent)) return { refused: "apart" };
  const members = [...top].sort((a, b) => ((tree.nodes.get(a.pid)?.order ?? "") < (tree.nodes.get(b.pid)?.order ?? "") ? -1 : 1));
  const { file } = first;
  const box = new Map<string, Box>();
  for (const m of members) if (m.box) box.set(m.pid, m.box);
  const positioned = members.every((m) => box.has(m.pid));
  const wrapper = newPid();
  const at = (field: string) => nodeKey(file, wrapper, field);
  map.set(at("kind"), "el");
  map.set(at("tag"), "div");
  map.set(at("parent"), parent);
  map.set(at("order"), tree.nodes.get(members[0]?.pid ?? "")?.order ?? "");
  map.set(at("attrs"), JSON.stringify([["", "data-name", "Group"]]));
  if (positioned) {
    const boxes = [...box.values()];
    const left = Math.round(Math.min(...boxes.map((b) => b.left)));
    const topEdge = Math.round(Math.min(...boxes.map((b) => b.top)));
    const right = Math.round(Math.max(...boxes.map((b) => b.left + b.width)));
    const bottom = Math.round(Math.max(...boxes.map((b) => b.top + b.height)));
    map.set(at("s|position"), "absolute");
    map.set(at("s|left"), px(left));
    map.set(at("s|top"), px(topEdge));
    map.set(at("s|width"), px(right - left));
    map.set(at("s|height"), px(bottom - topEdge));
    for (const m of members) {
      const b = box.get(m.pid);
      if (!b) continue;
      // Said outright, so that taking the box away knows these places are inside it (a class may be what made the element absolute).
      if (map.get(nodeKey(file, m.pid, "s|position")) !== "absolute") map.set(nodeKey(file, m.pid, "s|position"), "absolute");
      map.set(nodeKey(file, m.pid, "s|left"), keepSpacing(map.get(nodeKey(file, m.pid, "s|left")), px(b.left - left)));
      map.set(nodeKey(file, m.pid, "s|top"), keepSpacing(map.get(nodeKey(file, m.pid, "s|top")), px(b.top - topEdge)));
    }
  }
  for (const m of members) map.set(nodeKey(file, m.pid, "parent"), wrapper);
  return { done: [{ file, pid: wrapper }] };
}

const blank = (s: string) => s.trim() === "";

/**
 * Takes each box away and leaves what was inside it where the box was, in the
 * same order. A box that is positioned absolutely at a place and whose elements are
 * too gives its place to them first, so nothing moves on the canvas. A box with text
 * of its own is refused: taking it away would lose the text.
 */
export function ungroup(map: Y.Map<unknown>, items: Item[]): Outcome {
  const made: Ref[] = [];
  let refused: Refusal | null = null;
  for (const i of outermost(map, items)) {
    const tree = readTree(map, i.file);
    const node = tree.nodes.get(i.pid);
    if (!node) continue;
    const members = elementsOf(tree, i.pid);
    const hasText = map.has(nodeKey(i.file, i.pid, "rich")) || kidsOf(tree, i.pid).some((k) => tree.nodes.get(k)?.kind === "text" && !blank(tree.nodes.get(k)?.text ?? ""));
    if (members.length === 0) {
      refused ??= "nothing";
      continue;
    }
    if (hasText) {
      refused = "text";
      continue;
    }
    const siblings = kidsOf(tree, node.parent);
    const at = siblings.indexOf(i.pid);
    const before = tree.nodes.get(siblings[at - 1] ?? "")?.order ?? "";
    const after = tree.nodes.get(siblings[at + 1] ?? "")?.order ?? "";
    const left = pixels(map, i.file, i.pid, "left");
    const topEdge = pixels(map, i.file, i.pid, "top");
    const gives = map.get(nodeKey(i.file, i.pid, "s|position")) === "absolute" && left !== null && topEdge !== null;
    let key = before;
    for (const m of members) {
      key = between(key, after);
      map.set(nodeKey(i.file, m, "parent"), node.parent);
      map.set(nodeKey(i.file, m, "order"), key);
      if (gives && map.get(nodeKey(i.file, m, "s|position")) === "absolute") {
        const l = pixels(map, i.file, m, "left");
        const t = pixels(map, i.file, m, "top");
        if (l !== null && t !== null) {
          map.set(nodeKey(i.file, m, "s|left"), keepSpacing(map.get(nodeKey(i.file, m, "s|left")), px(l + left)));
          map.set(nodeKey(i.file, m, "s|top"), keepSpacing(map.get(nodeKey(i.file, m, "s|top")), px(t + topEdge)));
        }
      }
      made.push({ file: i.file, pid: m });
    }
    // The box goes, with the text between its elements (white space only, or it was refused).
    for (const gone of [i.pid, ...kidsOf(tree, i.pid).filter((k) => !members.includes(k))]) {
      const prefix = nodeKey(i.file, gone, "");
      for (const k of [...map.keys()]) if (k.startsWith(prefix)) map.delete(k);
    }
  }
  if (made.length > 0) return { done: made };
  return { refused: refused ?? "nothing" };
}

export type Stack = "forward" | "backward" | "front" | "back";

/** The order the elements of a parent are in once `chosen` have moved. */
function stacked(els: string[], chosen: Set<string>, how: Stack): string[] {
  const out = [...els];
  if (how === "front") return [...out.filter((e) => !chosen.has(e)), ...out.filter((e) => chosen.has(e))];
  if (how === "back") return [...out.filter((e) => chosen.has(e)), ...out.filter((e) => !chosen.has(e))];
  if (how === "forward") {
    for (let i = out.length - 2; i >= 0; i--) {
      const here = out[i] ?? "";
      const above = out[i + 1] ?? "";
      if (chosen.has(here) && !chosen.has(above)) [out[i], out[i + 1]] = [above, here];
    }
  } else {
    for (let i = 1; i < out.length; i++) {
      const here = out[i] ?? "";
      const below = out[i - 1] ?? "";
      if (chosen.has(here) && !chosen.has(below)) [out[i - 1], out[i]] = [here, below];
    }
  }
  return out;
}

/**
 * Moves the elements up or down among their siblings, one step or to the end
 * (a later element in the file paints over an earlier one). They keep their order
 * among themselves. False, with nothing written, when they are where they would go.
 */
export function restack(map: Y.Map<unknown>, items: Ref[], how: Stack): boolean {
  let wrote = false;
  const files = [...new Set(items.map((i) => i.file))];
  for (const file of files) {
    const tree = readTree(map, file);
    const byParent = new Map<string, Set<string>>();
    for (const i of items) {
      const node = i.file === file ? tree.nodes.get(i.pid) : undefined;
      if (node?.kind !== "el" || i.pid === tree.root) continue;
      byParent.set(node.parent, (byParent.get(node.parent) ?? new Set()).add(i.pid));
    }
    for (const [parent, chosen] of byParent) {
      const els = elementsOf(tree, parent);
      const next = stacked(els, chosen, how);
      if (next.every((e, i) => e === els[i])) continue;
      const order = (e: string) => tree.nodes.get(e)?.order ?? "";
      let key = "";
      next.forEach((e, i) => {
        if (!chosen.has(e)) {
          key = order(e);
          return;
        }
        const above = next.slice(i + 1).find((o) => !chosen.has(o));
        key = between(key, above === undefined ? "" : order(above));
        map.set(nodeKey(file, e, "order"), key);
      });
      wrote = true;
    }
  }
  return wrote;
}

export type Shape = "text" | "rect" | "oval";

export interface At {
  left: number;
  top: number;
}

/** What a new shape looks like: its size and the declarations that are not a place. */
const SHAPES: Record<Shape, { width?: number; height?: number; style: Record<string, string> }> = {
  text: { style: { "font-size": "24px" } },
  rect: { width: 160, height: 120, style: { "background-color": "#d9d9d9" } },
  oval: { width: 160, height: 120, style: { "background-color": "#d9d9d9", "border-radius": "50%" } },
};

function appendElement(map: Y.Map<unknown>, file: string, tag: string, attrs: [string, string][], style: Record<string, string>): string | null {
  const root = rootOf(map, file);
  if (root === "") return null;
  const tree = readTree(map, file);
  const pid = newPid();
  const at = (field: string) => nodeKey(file, pid, field);
  map.set(at("kind"), "el");
  map.set(at("tag"), tag);
  map.set(at("parent"), root);
  map.set(at("order"), lastOrder(tree, root));
  map.set(at("attrs"), JSON.stringify(attrs.map(([k, v]) => ["", k, v])));
  for (const [prop, value] of Object.entries(style)) map.set(at(`s|${prop}`), value);
  return pid;
}

/** Adds a text box, a rectangle or an oval at the end of the body (on top of what is there), placed absolutely. */
export function insertShape(map: Y.Map<unknown>, file: string, shape: Shape, at: At, text: string): Ref | null {
  const spec = SHAPES[shape];
  const size = spec.width && spec.height ? { width: px(spec.width), height: px(spec.height) } : {};
  const pid = appendElement(map, file, "div", [], { position: "absolute", left: px(at.left), top: px(at.top), ...size, ...spec.style });
  if (!pid) return null;
  if (shape === "text") {
    const id = newTextId();
    map.set(nodeKey(file, id, "kind"), "text");
    map.set(nodeKey(file, id, "parent"), pid);
    map.set(nodeKey(file, id, "order"), "V");
    map.set(nodeKey(file, id, "text"), new Y.Text(text));
  }
  return { file, pid };
}

export interface Picture {
  /** The address of a file of the artifact: /files/<id>/<name>. */
  src: string;
  alt: string;
  /** The picture's own size, to give the image the same proportions. */
  width: number;
  height: number;
}

/** The longest side a picture is first shown with; the person resizes it from there. */
export const PICTURE_SIDE = 320;

/** Adds an image at the end of the body, as big as PICTURE_SIDE allows and in the picture's proportions. */
export function insertImage(map: Y.Map<unknown>, file: string, picture: Picture, at: At): Ref | null {
  const scale = Math.min(1, PICTURE_SIDE / Math.max(picture.width, picture.height, 1));
  const pid = appendElement(
    map,
    file,
    "img",
    [
      ["src", picture.src],
      ["alt", picture.alt],
    ],
    { position: "absolute", left: px(at.left), top: px(at.top), width: px(Math.max(1, picture.width * scale)), height: px(Math.max(1, picture.height * scale)), "object-fit": "cover" },
  );
  return pid ? { file, pid } : null;
}

/** Gives an element an attribute (an empty or null value takes `src` and `alt` differently: null takes it away, "" is an empty alt) and changes no other. False when it already has it so. */
export function setAttribute(map: Y.Map<unknown>, file: string, pid: string, name: "alt" | "src", value: string | null): boolean {
  const entry = nodeKey(file, pid, "attrs");
  if (map.get(nodeKey(file, pid, "kind")) !== "el") return false;
  const rows = parseAttrs(map.get(entry));
  const at = rows.findIndex(([ns, key]) => ns === "" && key === name);
  const now = at < 0 ? null : (rows[at]?.[2] ?? null);
  if (now === value) return false;
  if (value === null) rows.splice(at, 1);
  else if (at < 0) rows.push(["", name, value]);
  else rows[at] = ["", name, value];
  map.set(entry, JSON.stringify(rows));
  return true;
}

/** The properties of a picture fill that go with its address, as a person chooses how it fits. */
export const FITS = {
  cover: { "background-size": "cover" },
  contain: { "background-size": "contain" },
  original: { "background-size": "auto" },
} as const;

export type Fit = keyof typeof FITS;

/** Fills the element with a picture: the address and, for a fill that was not there, a fit that covers, centred and not repeated. */
export function fill(map: Y.Map<unknown>, file: string, pid: string, src: string): void {
  const at = (prop: string) => nodeKey(file, pid, `s|${prop}`);
  map.set(at("background-image"), `url(${src})`);
  const has = (prop: string) => typeof map.get(at(prop)) === "string" && map.get(at(prop)) !== "";
  if (!has("background-size")) map.set(at("background-size"), "cover");
  if (!has("background-position")) map.set(at("background-position"), "center");
  if (!has("background-repeat")) map.set(at("background-repeat"), "no-repeat");
}
