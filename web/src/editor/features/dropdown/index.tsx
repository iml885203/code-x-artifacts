import { NodeSelection, Plugin, type Transaction } from "prosemirror-state";
import type { Node } from "prosemirror-model";
import type { Translate } from "../../../i18n";
import { ChevronDown } from "../../../icons/editor";
import type { Feature, Hooks } from "../../feature";
import { dropdownWordsKey, editDropdown, openDropdown } from "./commands";
import { readConfig } from "./config";
import { closePicker, type DropdownWords } from "./picker";

export type { DropdownWords } from "./picker";
export { editDropdown, insertDropdown, openDropdown } from "./commands";

/** The page supplies fresh words when a field is opened, including after a language switch. */
export interface DropdownHooks {
  dropdown?: { words: () => DropdownWords };
}

export function dropdownWords(t: Translate): DropdownWords {
  return {
    label: t("editor.dropdown"), options: t("editor.dropdown.options"), value: t("editor.dropdown.value"),
    add: t("editor.dropdown.add"), remove: t("editor.dropdown.remove"), invalid: t("editor.dropdown.invalid"),
    changed: t("editor.dropdown.changed"), missing: t("editor.dropdown.missing"),
    save: t("editor.linkSave"), cancel: t("editor.linkCancel"),
  };
}

export const dropdown: Feature = {
  id: "dropdown",
  insertions: [{ id: "dropdown", label: "editor.dropdown", command: openDropdown, face: <ChevronDown /> }],
  plugins: (hooks: Hooks & DropdownHooks) => [new Plugin<{ words: () => DropdownWords } | null>({
    key: dropdownWordsKey,
    state: { init: () => hooks.dropdown ?? null, apply: (_tr, words) => words },
    view: (view) => ({ destroy: () => { closePicker(view); } }),
    // Room serialization keeps marks on text only; the atom must agree before a reload.
    appendTransaction: (transactions, _before, after) => {
      if (!transactions.some((tr) => tr.docChanged)) return null;
      let clean: Transaction | null = null;
      after.doc.descendants((node, pos) => {
        if (node.type.name === "dropdown" && node.marks.length > 0) clean = (clean ?? after.tr).removeMark(pos, pos + node.nodeSize);
      });
      return clean;
    },
    props: {
      handleKeyDown: (view, event) => {
        if (!view.editable || event.key !== "Enter" || event.shiftKey || event.ctrlKey || event.altKey || event.metaKey || event.isComposing || view.composing) return false;
        const { selection } = view.state;
        if (!(selection instanceof NodeSelection) || selection.node.type.name !== "dropdown") return false;
        const words = dropdownWordsKey.getState(view.state)?.words();
        return words !== undefined && editDropdown(view, selection.from, words);
      },
    },
  })],
  nodeViews: () => ({
    dropdown: (initial, view, getPos) => {
      const dom = view.dom.ownerDocument.createElement("span");
      dom.className = "ed-dropdown";
      dom.contentEditable = "false";
      const label = view.dom.ownerDocument.createElement("span");
      const trigger = view.dom.ownerDocument.createElement("button");
      trigger.type = "button";
      trigger.className = "quiet ed-dropdown-trigger";
      trigger.setAttribute("aria-haspopup", "dialog");
      const draw = (node: Node) => {
        const text = readConfig(node.attrs.config)?.value ?? "";
        label.textContent = text;
        trigger.textContent = text;
        trigger.hidden = !view.editable;
        label.hidden = view.editable;
      };
      trigger.addEventListener("mousedown", (event) => { event.preventDefault(); });
      trigger.addEventListener("click", () => {
        const pos = getPos();
        const words = dropdownWordsKey.getState(view.state)?.words();
        if (pos !== undefined && words) editDropdown(view, pos, words);
      });
      dom.append(label, trigger);
      draw(initial);
      return {
        dom,
        update: (node) => { if (node.type !== initial.type) return false; draw(node); return true; },
        stopEvent: (event) => event.target instanceof globalThis.Node && trigger.contains(event.target),
        ignoreMutation: () => true,
      };
    },
  }),
};
