// Undo and Redo: the person's own edits only (y-prosemirror's undo manager, whose plugin view.ts installs).
import type { EditorState } from "prosemirror-state";
import { redo, undo, yUndoPluginKey } from "y-prosemirror";
import { Redo, Undo } from "../../icons/history";
import { button, type Feature } from "../feature";

const manager = (state: EditorState) => yUndoPluginKey.getState(state)?.undoManager;
const canUndo = (state: EditorState) => (manager(state)?.undoStack.length ?? 0) > 0;
const canRedo = (state: EditorState) => (manager(state)?.redoStack.length ?? 0) > 0;

export const history: Feature = {
  id: "history",
  tools: [
    button({ id: "undo", group: "history", label: "editor.undo", face: <Undo />, disabled: (state) => !canUndo(state), run: (ctx) => { ctx.run(undo); } }),
    button({ id: "redo", group: "history", label: "editor.redo", face: <Redo />, disabled: (state) => !canRedo(state), run: (ctx) => { ctx.run(redo); } }),
  ],
  keymap: () => ({
    "Mod-z": undo,
    "Mod-y": redo,
    "Shift-Mod-z": redo,
  }),
};
