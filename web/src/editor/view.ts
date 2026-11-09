// The ProseMirror view of a live doc: the shared schema, the room's fragment
// as the document, the other people's carets, and per-person undo.
import { baseKeymap, chainCommands, exitCode } from "prosemirror-commands";
import { keymap } from "prosemirror-keymap";
import type { Node, NodeType } from "prosemirror-model";
import { EditorState, Plugin, Selection, TextSelection, type Command } from "prosemirror-state";
import { EditorView, type NodeViewConstructor } from "prosemirror-view";
import { yCursorPlugin, ySyncPlugin, yUndoPlugin } from "y-prosemirror";
import type { Awareness } from "y-protocols/awareness";
import type * as Y from "yjs";
import { agentMarks } from "./agents";
import { commentMarks } from "./comments/marks";
import { idPlugin } from "./ids";
import { paletteClass } from "./colors";
import type { Hooks } from "./feature";
import { features } from "./features";
import { fieldClipboardText } from "./fieldClipboard";
import { toggleTask } from "./format";
import { missingLabel } from "./image";
import { schema } from "./schema";
import { tabFragmentName, tabsOf } from "./tabs";


interface Person {
  name?: unknown;
  color?: unknown;
}

/** The caret of another person. It has no children: a caret the browser could enter would take the typing of whoever clicks next to it. The name is drawn from `data-name` by the stylesheet. */
function caret(user: Person): HTMLElement {
  const el = document.createElement("span");
  el.className = `ed-caret ${paletteClass(user.color)}`;
  el.setAttribute("aria-hidden", "true");
  el.dataset.name = typeof user.name === "string" ? user.name : "";
  return el;
}

function nodeType(name: string): NodeType {
  const type = schema.nodes[name];
  if (type === undefined) throw new Error(`the editor's schema has no node ${name}`);
  return type;
}

const hardBreak: Command = (state, dispatch) => {
  if (!state.selection.$from.parent.inlineContent) return false;
  dispatch?.(state.tr.replaceSelectionWith(nodeType("hard_break").create()).scrollIntoView());
  return true;
};

/** Select all selects the text of the tab the caret is in: replacing the tab itself would give it a new id. */
export const selectAllInTab: Command = (state, dispatch) => {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name !== "tab") continue;
    dispatch?.(state.tr.setSelection(TextSelection.between(state.doc.resolve($from.start(depth)), state.doc.resolve($from.end(depth)), -1)));
    return true;
  }
  return false;
};

/** When the room's content first arrives the selection can span a whole tab: it goes to the start of the text. */
const settleSelection = new Plugin({
  appendTransaction: (_trs, _before, after) => {
    const { $from, $to } = after.selection;
    return after.selection instanceof TextSelection && (!$from.parent.inlineContent || !$to.parent.inlineContent) ? after.tr.setSelection(Selection.atStart(after.doc)) : null;
  },
});

/**
 * What the clipboard's HTML is made ready for the parser: Word puts an empty `<o:p>` after nearly every paragraph,
 * which the parser takes for a paragraph of its own, and `style` and event-handler attributes are renamed because the page's CSP
 * refuses to run them (the browser logs an error for each one). Only this string changes: what is left still goes through the schema's rules.
 */
export const readyForParsing = (html: string): string => html.replace(/<\/?o:p\b[^>]*>/gi, "").replace(/<[a-z][^>]*>/gi, (tag) => tag.replace(/(\s)(style|on[a-z]+)(\s*=)/gi, "$1data-pasted-$2$3"));

export interface LiveView {
  view: EditorView;
  destroy(): void;
}

/** A to-do item's box is a real checkbox: a click flips the item, for the person who may edit; the browser's own flip never happens, so the document is the only state. */
const taskBoxes = new Plugin({
  props: {
    handleDOMEvents: {
      click: (view, event) => {
        const box = event.target;
        if (!(box instanceof HTMLInputElement) || !box.classList.contains("ed-check")) return false;
        event.preventDefault();
        const content = box.nextElementSibling;
        if (!view.editable || content === null) return true;
        const inside = view.state.doc.resolve(view.posAtDOM(content, 0));
        toggleTask(inside.before())(view.state, view.dispatch);
        return true;
      },
    },
  },
});

export function plugins(doc: Y.Doc, awareness: Awareness, editable: boolean, hooks: Hooks, tab: string, undo?: Y.UndoManager): Plugin[] {
  const fragment = doc.getXmlFragment(tabFragmentName(tab));
  // y-prosemirror is untyped JavaScript: each of its plugins is a Plugin.
  const shared: Plugin[] = [
    ySyncPlugin(fragment) as Plugin,
    yCursorPlugin(awareness, {
      cursorBuilder: (user: Person) => caret(user),
      selectionBuilder: (user: Person) => ({ class: `ed-selection ${paletteClass(user.color)}` }),
    }) as Plugin,
    agentMarks(awareness),
    commentMarks(hooks.comments),
    taskBoxes,
  ];
  const report = new Plugin({ view: () => ({ update: (view) => { hooks.onUpdate(view.state); } }) });
  if (!editable) return [...shared, report];
  return [
    ...shared,
    report,
    yUndoPlugin(undo ? { undoManager: undo } : {}) as Plugin,
    settleSelection,
    idPlugin(),
    ...features.flatMap((f) => f.plugins?.(hooks) ?? []),
    ...features.flatMap((f) => (f.keymap === undefined ? [] : [keymap(f.keymap(hooks))])),
    keymap({
      "Mod-a": selectAllInTab,
      "Shift-Enter": chainCommands(exitCode, hardBreak),
    }),
    keymap(baseKeymap),
  ];
}

export type { Hooks };

const nobody: Hooks = { onUpdate: () => undefined, onLink: () => undefined, onImages: () => undefined, say: (kind) => missingLabel[kind], comments: { threads: () => [], active: () => null, open: () => undefined, compose: () => undefined }, hint: () => "", mention: { people: () => Promise.resolve({ people: [], complete: true }), mentioned: () => undefined, words: () => ({ label: "", none: "", incomplete: "" }) } };

/**
 * Makes the view's selection the one the browser has. ProseMirror learns of a click or a key from the
 * `selectionchange` event, which is a task of its own: a remote edit that arrives first is applied
 * against the selection from before the click, and y-prosemirror puts that one back, so the person's
 * next keystrokes land wherever they were before.
 */
export function adoptBrowserSelection(view: EditorView): void {
  if (!view.hasFocus() || !(view.state.selection instanceof TextSelection)) return;
  const selection = (view.root as Document).getSelection();
  if (!selection) return;
  const { anchorNode, anchorOffset, focusNode, focusOffset } = selection;
  if (!anchorNode || !focusNode || !view.dom.contains(anchorNode) || !view.dom.contains(focusNode)) return;
  const anchor = view.posAtDOM(anchorNode, anchorOffset);
  const head = view.posAtDOM(focusNode, focusOffset);
  if (anchor === view.state.selection.anchor && head === view.state.selection.head) return;
  const { doc } = view.state;
  view.dispatch(view.state.tr.setSelection(TextSelection.between(doc.resolve(anchor), doc.resolve(head))));
}

/** Shows one tab of the room: the first one unless `tab` names another, with `undo` as its history when the caller keeps one (undoKeeper.ts). Each tab is a fragment of its own, so each has a view of its own. */
export function mount(parent: HTMLElement, doc: Y.Doc, awareness: Awareness, editable: boolean, label: string, given: Partial<Hooks> = {}, tab: string = tabsOf(doc)[0]?.id ?? "", undo?: Y.UndoManager): LiveView {
  const hooks = { ...nobody, ...given };
  // The browser's selection is adopted before every Yjs transaction that does not come from the view itself: those
  // run inside the view's own dispatch, where a dispatch of ours would be a second one inside it.
  const mounted: { view?: EditorView } = {};
  let dispatching = false;
  const beforeTransaction = () => {
    if (mounted.view && !dispatching) adoptBrowserSelection(mounted.view);
  };
  // Registered before the view exists, so it runs before y-prosemirror's own, which notes the selection.
  doc.on("beforeAllTransactions", beforeTransaction);
  const made: EditorView = new EditorView(parent, {
    state: EditorState.create({ schema, plugins: plugins(doc, awareness, editable, hooks, tab, undo) }),
    editable: () => editable,
    transformPastedHTML: readyForParsing,
    clipboardTextSerializer: fieldClipboardText,
    nodeViews: Object.assign({}, ...features.map((f) => f.nodeViews?.(hooks))) as Record<string, NodeViewConstructor>,
    dispatchTransaction(this: EditorView, tr) {
      dispatching = true;
      try {
        this.updateState(this.state.apply(tr));
      } finally {
        dispatching = false;
      }
    },
    attributes: { class: "ed-doc", role: "textbox", "aria-multiline": "true", "aria-label": label },
  });
  mounted.view = made;
  return {
    view: made,
    destroy: () => {
      doc.off("beforeAllTransactions", beforeTransaction);
      made.destroy();
      // y-prosemirror (1.3.7) hands a view its pending metadata on a timer, and does not know the view is gone:
      // without this the dispatch lands on a destroyed view and throws.
      made.dispatch = () => undefined;
    },
  };
}

/** The document the view holds, for tests. */
export function documentOf(view: EditorView): Node {
  return view.state.doc;
}
