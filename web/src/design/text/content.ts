// What an element's text is in the room (docs/design-room.md, "Formatted text"):
// one Y.Text under its `rich` entry, or the Y.Text of its only child, a text
// node, or nothing yet. Elements that hold anything else are not text.
import * as Y from "yjs";
import { htmlOfDelta, htmlOfText } from "./html";
import type { DeltaOp } from "./units";
import type { FrameTree } from "../tree";

const nodeKey = (file: string, pid: string, field: string) => `${file}|n|${pid}|${field}`;

/** Elements that have no content a person types into, or whose content is not shown as text. */
const NOT_TEXT = new Set(["html", "head", "body", "script", "style", "title", "textarea", "iframe", "noscript", "template", "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"]);

export type Content =
  | { kind: "rich"; pid: string; text: Y.Text }
  /** Plain text: in the text node `node`, or in none yet (an empty element). */
  | { kind: "plain"; pid: string; node: string | null; text: Y.Text | null };

/** The text content of an element, or null when it is not an element that is only text. */
export function contentOf(map: Y.Map<unknown>, file: string, tree: FrameTree, pid: string): Content | null {
  const node = tree.nodes.get(pid);
  if (node?.kind !== "el" || pid === tree.root || NOT_TEXT.has(node.tag.toLowerCase()) || map.get(nodeKey(file, pid, "ns"))) return null;
  const rich = map.get(nodeKey(file, pid, "rich"));
  if (rich instanceof Y.Text) return node.children.length === 0 ? { kind: "rich", pid, text: rich } : null;
  if (node.children.length === 0) return { kind: "plain", pid, node: null, text: null };
  const only = node.children[0];
  if (node.children.length !== 1 || only === undefined || tree.nodes.get(only)?.kind !== "text") return null;
  const text = map.get(nodeKey(file, only, "text"));
  return text instanceof Y.Text ? { kind: "plain", pid, node: only, text } : null;
}

/** The content as the delta of a text. */
export function deltaOf(content: Content): DeltaOp[] {
  if (content.kind === "rich") return content.text.toDelta() as DeltaOp[];
  const text = content.text?.toJSON() ?? "";
  return text === "" ? [] : [{ insert: text }];
}

export const htmlOfContent = (content: Content): string => (content.kind === "rich" ? htmlOfDelta(deltaOf(content)) : htmlOfText(content.text?.toJSON() ?? ""));

/** What each element that is only text shows, by id. */
export function textsOf(map: Y.Map<unknown>, file: string, tree: FrameTree): Map<string, string> {
  const out = new Map<string, string>();
  for (const pid of tree.nodes.keys()) {
    const content = contentOf(map, file, tree, pid);
    if (content && (content.kind === "rich" || content.node !== null)) out.set(pid, htmlOfContent(content));
  }
  return out;
}
