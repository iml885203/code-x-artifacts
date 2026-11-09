// What a frame builds an element from: the room's entries of an element the
// frame's file does not have (inserted, duplicated or grouped since the file was
// saved, or by someone else just now). The frame's inspector builds it
// (internal/kind/design_inspector.js, `create`); once built it is shown, styled and
// ordered like any other element.
import * as Y from "yjs";
import type { DesignEdits } from "../edits";
import { isFrameGone, type AttrChange, type Build, type FrameLinks } from "../inspect";
import { parseAttrs, type FrameTree } from "../tree";
import { nodeKey } from "./room";
import { attributes, escapeAttr, escapeText, htmlOfDelta } from "../text/html";
import type { DeltaOp } from "../text/units";

const SLOT = "";

/** The declarations of a style skeleton that are not the editor's (the ones with a slot are its own entries), as written. */
export function foreignDeclarations(skeleton: unknown): string | undefined {
  if (typeof skeleton !== "string" || skeleton === "") return undefined;
  const parts: string[] = [];
  let depth = 0;
  let start = 0;
  let quote = "";
  for (let i = 0; i < skeleton.length; i++) {
    const c = skeleton.charAt(i);
    if (quote !== "") {
      if (c === "\\") i++;
      else if (c === quote) quote = "";
    } else if (c === '"' || c === "'") quote = c;
    else if (c === "(") depth++;
    else if (c === ")") depth = Math.max(depth - 1, 0);
    else if (c === ";" && depth === 0) {
      parts.push(skeleton.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(skeleton.slice(start));
  const kept = parts.filter((p) => p.trim() !== "" && !p.includes(SLOT));
  return kept.length === 0 ? undefined : kept.join(";");
}

const VOID = new Set(["area", "br", "col", "hr", "img", "input", "source", "wbr"]);

/** The markup of an element of the room and of every element inside it that `missing` says the frame lacks. */
function markupOf(map: Y.Map<unknown>, file: string, tree: FrameTree, pid: string, missing: Set<string>): string {
  const node = tree.nodes.get(pid);
  const get = (field: string) => map.get(nodeKey(file, pid, field));
  const tag = node?.tag ?? "div";
  const rows = parseAttrs(get("attrs")).filter(([, key]) => !key.startsWith("\u0000") && key !== "style" && key !== "data-pid");
  const style = foreignDeclarations(get("rest"));
  const open = `<${tag}${attributes(rows)}${style ? ` style="${escapeAttr(style)}"` : ""} data-pid="${escapeAttr(pid)}">`;
  if (VOID.has(tag)) return open;
  let inside = "";
  const rich = get("rich");
  if (rich instanceof Y.Text) inside += htmlOfDelta(rich.toDelta() as DeltaOp[]);
  for (const child of node?.children ?? []) {
    const c = tree.nodes.get(child);
    if (c?.kind === "text") inside += escapeText(c.text);
    else if (c?.kind === "el" && missing.has(child)) inside += markupOf(map, file, tree, child, missing);
  }
  return `${open}${inside}</${tag}>`;
}

/** What the frame must build: each element of the room it does not have, whose parent it has, with what is inside it that it does not have either. */
export function buildsFor(edits: DesignEdits, file: string, have: ReadonlySet<string>): Build[] {
  const state = edits.frame(file);
  const missing = new Set([...state.alive].filter((p) => !have.has(p)));
  if (missing.size === 0) return [];
  const tree = edits.tree(file);
  const out: Build[] = [];
  for (const pid of missing) {
    const node = tree.nodes.get(pid);
    if (!node || missing.has(node.parent) || !have.has(node.parent)) continue;
    const siblings = tree.nodes.get(node.parent)?.children ?? [];
    const before = siblings.slice(siblings.indexOf(pid) + 1).find((s) => tree.nodes.get(s)?.kind === "el" && have.has(s)) ?? null;
    const ns = edits.map.get(nodeKey(file, pid, "ns"));
    out.push({ parent: node.parent, before, html: markupOf(edits.map, file, tree, pid, missing), ...(typeof ns === "string" && ns !== "" ? { ns } : {}) });
  }
  return out;
}

/** The alt and src of each image as the room holds them. */
export function pictures(edits: DesignEdits, file: string): Map<string, { alt: string | null; src: string | null }> {
  const out = new Map<string, { alt: string | null; src: string | null }>();
  const tree = edits.tree(file);
  for (const [pid, node] of tree.nodes) {
    if (node.kind !== "el" || node.tag !== "img") continue;
    const rows = parseAttrs(edits.map.get(nodeKey(file, pid, "attrs")));
    const find = (name: string) => rows.find(([ns, key]) => ns === "" && key === name)?.[2] ?? null;
    out.set(pid, { alt: find("alt"), src: find("src") });
  }
  return out;
}

/** What changed in the images since `before` (the first time an image is seen it only becomes what is compared with). */
export function pictureChanges(now: Map<string, { alt: string | null; src: string | null }>, before: Map<string, { alt: string | null; src: string | null }> | undefined): AttrChange[] {
  const out: AttrChange[] = [];
  for (const [pid, p] of now) {
    const was = before?.get(pid);
    if (!was) continue;
    if (was.alt !== p.alt) out.push([pid, "alt", p.alt]);
    if (was.src !== p.src) out.push([pid, "src", p.src]);
  }
  return out;
}

/**
 * Keeps each frame having the elements the room has: when a frame says it is
 * ready, and after every change to the room, whoever made it. It must come
 * before the hook that shows styles and order (useFramesShowRoom), which only
 * touches elements the frame has: the two see every change in the order they
 * were set up.
 */
export function mountBuilt(edits: DesignEdits, links: FrameLinks, frames: () => { id: string; file: string }[]): () => void {
  const seen = new Map<string, Map<string, { alt: string | null; src: string | null }>>();
  const push = (frameId: string) => {
    const frame = frames().find((f) => f.id === frameId);
    const have = links.has(frameId);
    if (!frame || !have) return;
    const builds = buildsFor(edits, frame.file, have);
    const now = pictures(edits, frame.file);
    const attrs = pictureChanges(now, seen.get(frameId));
    seen.set(frameId, now);
    if (builds.length === 0 && attrs.length === 0) return;
    links.mount(frameId, builds, attrs).catch((err: unknown) => {
      if (!isFrameGone(err)) console.error("build the elements of the room in their frame", err);
    });
  };
  const pushAll = () => {
    for (const f of frames()) push(f.id);
  };
  pushAll();
  const offChange = edits.onChange(pushAll);
  const offReady = links.onReady((frameId) => {
    seen.delete(frameId);
    push(frameId);
  });
  return () => {
    offChange();
    offReady();
  };
}
