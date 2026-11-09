// The words that have a comment, drawn in the editor for everyone who opens the doc, and the click on them that opens the thread.
// What the marks are drawn from comes from the page around the editor (Hooks.comments), so the plugin holds no comments of its own.
import { Plugin, PluginKey } from "prosemirror-state";
import { Decoration, DecorationSet } from "prosemirror-view";
import { rangeOf, type DocAnchor } from "./anchor";

/** A thread whose place in the doc is known. */
export interface Thread {
  id: string;
  anchor: DocAnchor;
}

/** What the plugin asks of the page around the editor. */
export interface Marking {
  threads: () => Thread[];
  /** The thread open in the panel: its words are drawn stronger. */
  active: () => string | null;
  open: (id: string) => void;
  /** The shortcut to comment on the selection was pressed. */
  compose: () => void;
}

/** A transaction with this meta draws the marks again (the comments or the open thread changed, not the document). */
export const redraw = new PluginKey("comment-marks");

export const markClass = "ed-commented";
export const activeClass = "ed-commented-active";

export function commentMarks(marking: Marking): Plugin {
  return new Plugin({
    key: redraw,
    props: {
      decorations: (state) => {
        const active = marking.active();
        const marks: Decoration[] = [];
        for (const { id, anchor } of marking.threads()) {
          const range = rangeOf(state, anchor);
          if (range !== null) marks.push(Decoration.inline(range.from, range.to, { class: id === active ? `${markClass} ${activeClass}` : markClass, "data-comment": id }));
        }
        return DecorationSet.create(state.doc, marks);
      },
      handleKeyDown: (_view, event) => {
        // Mod-Alt-M: the key code, because Alt changes the character on some keyboards.
        if (event.code !== "KeyM" || !event.altKey || !(event.metaKey || event.ctrlKey)) return false;
        marking.compose();
        return true;
      },
      handleDOMEvents: {
        click: (_view, event) => {
          const hit = event.target instanceof Element ? event.target.closest("[data-comment]") : null;
          const id = hit?.getAttribute("data-comment");
          if (id) marking.open(id);
          return false;
        },
      },
    },
  });
}
