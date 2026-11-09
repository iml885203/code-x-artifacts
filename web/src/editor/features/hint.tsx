// The hint an empty doc shows: a line in its first block, gone at the first character.
import { Plugin } from "prosemirror-state";
import { Decoration, DecorationSet } from "prosemirror-view";
import type { Feature } from "../feature";

export const hint: Feature = {
  id: "hint",
  plugins: (hooks) => [
    new Plugin({
      props: {
        decorations: (state) => {
          const { doc } = state;
          // The view shows one tab: empty is a tab with one block in it and nothing in the block.
          const tab = doc.firstChild;
          const first = tab?.firstChild;
          const words = hooks.hint();
          if (doc.childCount !== 1 || tab?.childCount !== 1 || !first?.isTextblock || first.content.size > 0 || words === "") return null;
          // The tab opens at 0, so its first block starts at 1.
          return DecorationSet.create(doc, [Decoration.node(1, 1 + first.nodeSize, { class: "ed-empty", "data-hint": words })]);
        },
      },
    }),
  ],
};
