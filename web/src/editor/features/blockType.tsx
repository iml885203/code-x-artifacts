// The block type: a paragraph or a heading of a level, chosen in a select.
import { format } from "../../i18n";
import { blockCommand } from "../block";
import type { Feature } from "../feature";
import { itemAt } from "../lists";
import type { EditorState } from "prosemirror-state";

type Kind = "paragraph" | 1 | 2 | 3 | 4 | 5 | 6;
const kinds: Kind[] = ["paragraph", 1, 2, 3, 4, 5, 6];

/** The block the caret is in: a paragraph, or a heading's level; null when it is in something this editor does not change that way (a list item, a table). */
function blockOf(state: EditorState): Kind | null {
  if (itemAt(state) !== null) return null;
  const parent = state.selection.$from.parent;
  const level: unknown = parent.attrs.level;
  if (parent.type.name === "paragraph") return "paragraph";
  return parent.type.name === "heading" && typeof level === "number" && level >= 1 && level <= 6 ? (level as 1 | 2 | 3 | 4 | 5 | 6) : null;
}

export const blockType: Feature = {
  id: "block-type",
  tools: [
    {
      id: "block-type",
      group: "block",
      bubble: 5,
      render: (state, ctx, t) => {
        const block = blockOf(state);
        return (
          <select
            aria-label={t("editor.blockType")}
            value={block === null ? "" : String(block)}
            disabled={block === null}
            onChange={(e) => {
              const v = e.target.value;
              ctx.run(blockCommand(v === "paragraph" ? "paragraph" : Number(v)));
            }}
          >
            {block === null && <option value="" />}
            {kinds.map((k) => (
              <option key={k} value={String(k)}>
                {k === "paragraph" ? t("editor.paragraph") : format(t("editor.heading"), { level: String(k) })}
              </option>
            ))}
          </select>
        );
      },
    },
  ],
};
