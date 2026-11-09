// Lists: bullets, numbers and to-dos, and moving an item in or out.
import { chainCommands } from "prosemirror-commands";
import type { Command } from "prosemirror-state";
import { Bullets, Indent, Numbers, Outdent, Tasks } from "../../icons/lists";
import { button, type Feature } from "../feature";
import { backspaceItem, indentItem, itemAt, listAt, outdentItem, splitItem, toggleList, type ListKind } from "../lists";

const kinds: { list: ListKind; label: "editor.bulletList" | "editor.numberedList" | "editor.taskList"; face: React.JSX.Element }[] = [
  { list: "bullet_list", label: "editor.bulletList", face: <Bullets /> },
  { list: "numbered_list", label: "editor.numberedList", face: <Numbers /> },
  { list: "task_list", label: "editor.taskList", face: <Tasks /> },
];

// Tab stays in the editor while the caret is in a list, even where the item cannot move.
const inItem: Command = (state) => itemAt(state) !== null;

export const lists: Feature = {
  id: "lists",
  tools: [
    ...kinds.map((k) =>
      button({ id: k.list, group: k.list === "task_list" ? "insert" : "lists", ...(k.list === "bullet_list" ? { bubble: 6 } : {}), label: k.label, face: k.face, pressed: (state) => listAt(state) === k.list, run: (ctx) => { ctx.run(toggleList(k.list)); } }),
    ),
    button({ id: "outdent", group: "lists", label: "editor.outdent", face: <Outdent />, disabled: (state) => listAt(state) === null, run: (ctx) => { ctx.run(outdentItem); } }),
    button({ id: "indent", group: "lists", label: "editor.indent", face: <Indent />, disabled: (state) => listAt(state) === null, run: (ctx) => { ctx.run(indentItem); } }),
  ],
  keymap: () => ({
    "Shift-Mod-7": toggleList("numbered_list"),
    "Shift-Mod-8": toggleList("bullet_list"),
    "Shift-Mod-9": toggleList("task_list"),
    Tab: chainCommands(indentItem, inItem),
    "Shift-Tab": chainCommands(outdentItem, inItem),
    Enter: splitItem,
    Backspace: backspaceItem,
  }),
};
