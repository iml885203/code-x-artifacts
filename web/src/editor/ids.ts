// Every block of a doc has an id (docs/doc-format.md) and the server's snapshot
// refuses a block without one, so the editor guarantees it: whatever an edit
// leaves without an id, or with one another block already has (a split copies
// the attributes), gets a new one in the same step.
import type { Node } from "prosemirror-model";
import { Plugin, PluginKey, type EditorState, type Transaction } from "prosemirror-state";

/** Ids are `b_` and eight characters: 1 to 64 of letters, digits, `-` and `_` is what the format allows. */
export function newBlockId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return "b_" + Array.from(bytes, (b) => "abcdefghijklmnopqrstuvwxyz0123456789"[b % 36]).join("");
}

/** The paragraph of a list item and of a table cell have the attribute and leave it empty: the model has no id for them. */
function wantsId(node: Node, parent: Node | null): boolean {
  const attrs = node.type.spec.attrs;
  if (attrs === undefined || !("id" in attrs) || parent === null) return false;
  return parent.type.name !== "list_item" && parent.type.name !== "table_cell";
}

/** The transaction that gives every block a unique id, or null when they all have one already. */
export function fixIds(state: EditorState, make: () => string): Transaction | null {
  const seen = new Set<string>();
  let tr: Transaction | null = null;
  state.doc.descendants((node, pos, parent) => {
    if (!wantsId(node, parent)) return true;
    const current: unknown = node.attrs.id;
    if (typeof current === "string" && current !== "" && !seen.has(current)) {
      seen.add(current);
      return true;
    }
    let id = make();
    while (seen.has(id)) id = make();
    seen.add(id);
    tr ??= state.tr;
    tr.setNodeMarkup(pos, undefined, { ...node.attrs, id });
    return true;
  });
  return tr;
}

export const idsKey = new PluginKey("pictor-block-ids");

export function idPlugin(make: () => string = newBlockId): Plugin {
  return new Plugin({
    key: idsKey,
    appendTransaction: (trs, _before, after) => (trs.some((tr) => tr.docChanged) ? fixIds(after, make) : null),
  });
}
