// What a frame cannot be told while it is open. A frame of someone in the room
// is loaded from the room (the live address, docs/design-room.md) and then kept
// up to date by its inspector: styles, hiding, element order, the text of
// elements that are only text, the elements the room has and it does not, and
// the alt and src of images. Everything else the room holds of a frame is only
// shown when the frame is loaded again, so a change to it, and nothing else, is
// a reason to load the frame again.
import * as Y from "yjs";
import { textsOf } from "./text/content";
import { parseAttrs, readTree } from "./tree";

/** The entries of a node that the inspector shows as they change, and the part of `apply` that does it. */
export const SHOWN_LIVE: Readonly<Record<string, string>> = {
  "s|": "styles",
  "override|": "inline",
  hidden: "styles",
  parent: "order",
  order: "order",
  rich: "texts",
  text: "texts",
};

/** The entries of a node that only loading the frame again shows. */
export const SHOWN_ON_LOAD: readonly string[] = ["kind", "tag", "ns", "attrs", "rest", "data"];

/** The attributes of an image the inspector changes in place (`apply.attrs`). */
export const LIVE_ATTRS: readonly string[] = ["alt", "src"];

/** The outline's name for an element: the frame does not show it, so a rename is no reason to load the frame again. */
const NAME = "data-name";

export interface FrameShape {
  shell: string;
  /** For each node, what of it the frame shows only when it is loaded. */
  nodes: Map<string, string>;
}

const nodeKey = (file: string, pid: string, field: string) => `${file}|n|${pid}|${field}`;

const valueOf = (v: unknown): string => (v instanceof Y.Text ? v.toJSON() : typeof v === "string" ? v : "");

/** What of a frame the room holds that its inspector cannot show while it is open. */
export function shapeOf(map: Y.Map<unknown>, file: string): FrameShape {
  const shell: string[] = [];
  const prefix = `${file}|shell|`;
  map.forEach((value, key) => {
    if (key.startsWith(prefix)) shell.push(`${key.slice(prefix.length)}=${valueOf(value)}`);
  });
  shell.sort();
  const tree = readTree(map, file);
  const texts = textsOf(map, file, tree);
  const nodes = new Map<string, string>();
  for (const [pid, node] of tree.nodes) {
    const get = (field: string) => valueOf(map.get(nodeKey(file, pid, field)));
    const parts = SHOWN_ON_LOAD.map((field) => {
      if (field !== "attrs") return get(field);
      const kept = parseAttrs(get("attrs")).filter(([ns, name]) => ns !== "" || (name !== NAME && !(node.tag === "img" && LIVE_ATTRS.includes(name))));
      return JSON.stringify(kept);
    });
    // A text that is not the whole of an element is not one the frame is told.
    if (node.kind === "text" && !texts.has(node.parent)) parts.push(node.parent, node.order, get("text"));
    nodes.set(pid, JSON.stringify(parts));
  }
  return { shell: shell.join("\n"), nodes };
}

/** Whether the room changed something of a frame between two shapes that the open frame cannot show; nodes added or removed it can. */
export function needsLoading(before: FrameShape, now: FrameShape): boolean {
  if (before.shell !== now.shell) return true;
  for (const [pid, shape] of now.nodes) {
    const was = before.nodes.get(pid);
    if (was !== undefined && was !== shape) return true;
  }
  return false;
}
