import type { Command, EditorState } from "prosemirror-state";
import { yUndoPluginKey } from "y-prosemirror";
import type { Translate } from "../i18n";
import { features } from "./features";

const order = ["bullet-list", "checklist", "code-block", "date", "divider", "dropdown", "heading-1", "heading-2", "heading-3", "numbered-list", "quote"];
const rank = (id: string) => { const at = order.indexOf(id); return at < 0 ? order.length : at; };
export const insertions = features.flatMap((feature) => feature.insertions ?? []).sort((a, b) => rank(a.id) - rank(b.id));

export function matchingInsertions(state: EditorState, query: string, t: Translate) {
  const find = query.trim().toLocaleLowerCase();
  return insertions.filter((item) => item.command(state) && [t(item.label), item.id, ...(item.keywords ?? [])].some((word) => word.toLocaleLowerCase().includes(find)));
}

/** A discovery action is one undo step, separate from the typing before and after it. */
export const insertionCommand = (command: Command): Command => (state, dispatch, view) => {
  if (!dispatch) return command(state);
  const manager = yUndoPluginKey.getState(state)?.undoManager;
  manager?.stopCapturing();
  try { return command(state, dispatch, view); }
  finally { manager?.stopCapturing(); }
};
