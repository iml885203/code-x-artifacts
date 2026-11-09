// Text formatting: the marks, the quote and the code block.
import { toggle, inCode, markActive, quoteDepth, toggleCodeBlock, toggleQuote, type MarkName } from "../format";
import { CodeBlockIcon, QuoteIcon } from "../../icons/blocks";
import type { Key } from "../../i18n";
import { button, type Feature, type Tool } from "../feature";
import type { ReactNode } from "react";

const marks: { mark: MarkName; label: Key; face: ReactNode; bubble?: number }[] = [
  { mark: "bold", label: "editor.bold", face: <b>B</b>, bubble: 1 },
  { mark: "italic", label: "editor.italic", face: <i>I</i>, bubble: 2 },
  { mark: "underline", label: "editor.underline", face: <u>U</u>, bubble: 3 },
  { mark: "strike", label: "editor.strike", face: <s>S</s> },
  { mark: "code", label: "editor.code", face: <code>{"<>"}</code>, bubble: 7 },
  { mark: "highlight", label: "editor.highlight", face: <mark>H</mark> },
];

const markTools: Tool[] = marks.map((m) =>
  button({
    id: m.mark,
    group: "marks",
    label: m.label,
    face: m.face,
    ...(m.bubble === undefined ? {} : { bubble: m.bubble }),
    pressed: (state) => markActive(state, m.mark),
    // Inside code there are no text marks.
    disabled: inCode,
    run: (ctx) => {
      ctx.run(toggle(m.mark));
    },
  }),
);

export const format: Feature = {
  id: "format",
  tools: [
    ...markTools,
    button({ id: "quote", group: "blocks", label: "editor.quote", face: <QuoteIcon />, pressed: (state) => quoteDepth(state) !== null, run: (ctx) => { ctx.run(toggleQuote); } }),
    button({ id: "code-block", group: "blocks", label: "editor.codeBlock", face: <CodeBlockIcon />, pressed: inCode, run: (ctx) => { ctx.run(toggleCodeBlock); } }),
  ],
  keymap: () => ({
    "Mod-b": toggle("bold"),
    "Mod-i": toggle("italic"),
    "Mod-u": toggle("underline"),
    "Shift-Mod-x": toggle("strike"),
    "Mod-e": toggle("code"),
    "Shift-Mod-h": toggle("highlight"),
    "Mod-Alt-q": toggleQuote,
    "Mod-Alt-c": toggleCodeBlock,
  }),
};
