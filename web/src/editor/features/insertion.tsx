import { Plugin, TextSelection } from "prosemirror-state";
import { blockCommand } from "../block";
import { insertRule, toggleCodeBlock, toggleQuote } from "../format";
import { toggleList } from "../lists";
import type { Feature } from "../feature";
import { Bullets, Numbers, Tasks } from "../../icons/lists";
import { CodeBlockIcon, QuoteIcon, RuleIcon } from "../../icons/blocks";

export const insertion: Feature = {
  id: "insertion",
  insertions: [
    { id: "bullet-list", label: "editor.bulletList", command: toggleList("bullet_list"), face: <Bullets />, keywords: ["bulleted", "ul"] },
    { id: "checklist", label: "editor.taskList", command: toggleList("task_list"), face: <Tasks />, keywords: ["checklist", "todo"] },
    { id: "code-block", label: "editor.codeBlock", command: toggleCodeBlock, face: <CodeBlockIcon />, keywords: ["codeblock"] },
    { id: "divider", label: "editor.rule", command: insertRule, face: <RuleIcon />, keywords: ["divider", "hr"] },
    { id: "heading-1", label: "editor.heading1", command: blockCommand(1), face: <span>H1</span>, keywords: ["h1"] },
    { id: "heading-2", label: "editor.heading2", command: blockCommand(2), face: <span>H2</span>, keywords: ["h2"] },
    { id: "heading-3", label: "editor.heading3", command: blockCommand(3), face: <span>H3</span>, keywords: ["h3"] },
    { id: "numbered-list", label: "editor.numberedList", command: toggleList("numbered_list"), face: <Numbers />, keywords: ["numberedlist", "ol"] },
    { id: "quote", label: "editor.quote", command: toggleQuote, face: <QuoteIcon />, keywords: ["blockquote"] },
  ],
  plugins: (hooks) => [new Plugin({
    props: {
      handleTextInput: (view, from, to, value) => {
        if (value !== "/" || !hooks.onInsert || !view.editable || view.composing) return false;
        const { state } = view;
        const $from = state.doc.resolve(from);
        const $to = state.doc.resolve(to);
        if (!$from.sameParent($to) || !$from.parent.inlineContent || $from.parent.type.name === "code_block") return false;
        const selection = new TextSelection($from, $to);
        const same = selection.eq(state.selection);
        const marks = [...(same ? (state.storedMarks ?? $from.marks()) : $from.marks()), ...($from.nodeAfter?.marks ?? [])];
        if (marks.some((mark) => mark.type.name === "link" || mark.type.name === "code")) return false;
        const before = selection.$from.parent.textBetween(0, selection.$from.parentOffset);
        if (before !== "" && !/\s$/.test(before) && selection.$from.nodeBefore?.type.name !== "hard_break") return false;
        // DOM input ranges can arrive before selectionchange updates the editor's selection.
        const tr = state.tr.setSelection(selection);
        if (same && state.storedMarks) tr.setStoredMarks(state.storedMarks);
        view.dispatch(tr);
        hooks.onInsert();
        return true;
      },
    },
  })],
};
