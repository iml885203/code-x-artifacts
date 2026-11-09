// Headings and paragraphs: the block type of the caret's block.
import type { Node } from "prosemirror-model";
import type { Command } from "prosemirror-state";
import { schema } from "./schema";

function nodeType(name: string) {
  const type = schema.nodes[name];
  if (type === undefined) throw new Error(`the editor's schema has no node ${name}`);
  return type;
}

/** Headings and paragraphs: a block is one of them (lists, quotes and code have commands of their own). */
export function blockCommand(kind: "paragraph" | number): Command {
  const type = nodeType(kind === "paragraph" ? "paragraph" : "heading");
  return (state, dispatch) => {
    const { from, to } = state.selection;
    const blocks: { node: Node; pos: number }[] = [];
    state.doc.nodesBetween(from, to, (node, pos) => {
      if (node.isTextblock) blocks.push({ node, pos });
      return !node.isTextblock;
    });
    const applicable = blocks.some(({ node, pos }) => {
      if (node.type === type) return kind !== "paragraph" && node.attrs.level !== kind;
      const $pos = state.doc.resolve(pos);
      return $pos.parent.canReplaceWith($pos.index(), $pos.index() + 1, type);
    });
    if (!applicable) return false;
    // The block keeps its id: it is the same block, drawn another way.
    dispatch?.(
      state.tr
        .setBlockType(from, to, type, (node) => {
          const id: unknown = node.attrs.id;
          return { id, ...(kind === "paragraph" ? {} : { level: kind }) };
        })
        .scrollIntoView(),
    );
    return true;
  };
}
