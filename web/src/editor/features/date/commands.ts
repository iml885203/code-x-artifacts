import type { Node } from "prosemirror-model";
import { NodeSelection, Plugin, PluginKey, TextSelection, type Command, type EditorState, type Transaction } from "prosemirror-state";
import { absolutePositionToRelativePosition, relativePositionToAbsolutePosition, ySyncPluginKey, yUndoPluginKey } from "y-prosemirror";
import * as Y from "yjs";

type Mapping = Parameters<typeof absolutePositionToRelativePosition>[2];
interface Binding { doc: Y.Doc; type: Y.XmlFragment; mapping: Mapping }
export interface DatePicker {
  session: number;
  from: Y.RelativePosition;
  to: Y.RelativePosition;
  target: Y.XmlElement | null;
  initial: string;
}
export const datePickerKey = new PluginKey<DatePicker | null>("date-picker");
const bindingOf = (state: EditorState) => (ySyncPluginKey.getState(state) as { binding?: Binding } | undefined)?.binding;

export function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value) || value.startsWith("0000")) return false;
  const date = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(date.valueOf()) && date.toISOString().slice(0, 10) === value;
}

const selectedDate = (state: EditorState) => state.selection instanceof NodeSelection && state.selection.node.type.name === "date" ? state.selection.node : null;

/** Opening is a selection operation. Only submitting the picker writes to the room. */
export const insertDate: Command = (state, dispatch, view) => {
  const binding = bindingOf(state);
  const selected = selectedDate(state);
  if (!binding || (!selected && (!state.selection.$from.parent.inlineContent || state.selection.$from.parent.type.spec.code === true))) return false;
  if (!dispatch) return true;
  if (!view?.editable) return false;
  let target: Y.XmlElement | null = null;
  if (selected) {
    for (const [element, node] of binding.mapping) {
      if (node === selected && element instanceof Y.XmlElement) target = element;
    }
    if (target === null) return false;
  }
  const picker: DatePicker = {
    session: (datePickerKey.getState(state)?.session ?? 0) + 1,
    from: absolutePositionToRelativePosition(state.selection.from, binding.type, binding.mapping) as Y.RelativePosition,
    to: absolutePositionToRelativePosition(state.selection.to, binding.type, binding.mapping) as Y.RelativePosition,
    target,
    initial: typeof selected?.attrs.date === "string" ? selected.attrs.date : "",
  };
  dispatch(state.tr.setMeta(datePickerKey, picker));
  return true;
};

export const cancelDate: Command = (state, dispatch) => {
  if (!datePickerKey.getState(state)) return false;
  dispatch?.(state.tr.setMeta(datePickerKey, null));
  return true;
};

/** The shared element's identity prevents a deleted target's neighbour from being edited. */
export function chooseDate(value: string): Command {
  return (state, dispatch, view) => {
    const picker = datePickerKey.getState(state);
    const binding = bindingOf(state);
    if (!picker || !binding || !validDate(value)) return false;
    let from: number | null = null;
    let to: number | null = null;
    if (picker.target) {
      const alive = Y.createAbsolutePositionFromRelativePosition(Y.createRelativePositionFromTypeIndex(picker.target, 0), binding.doc);
      if (alive === null) return false;
      const node = binding.mapping.get(picker.target);
      state.doc.descendants((child: Node, pos: number) => {
        if (child === node) { from = pos; to = pos + child.nodeSize; }
      });
    } else {
      from = relativePositionToAbsolutePosition(binding.doc, binding.type, picker.from, binding.mapping);
      to = relativePositionToAbsolutePosition(binding.doc, binding.type, picker.to, binding.mapping);
    }
    const type = state.schema.nodes.date;
    if (from === null || to === null || type === undefined) return false;
    if (!dispatch) return true;
    if (!view?.editable) return false;
    const tr = picker.target
      ? state.tr.setNodeMarkup(from, type, { date: value }, [])
      : state.tr.replaceWith(from, to, type.create({ date: value }));
    tr.setSelection(TextSelection.create(tr.doc, from + 1)).setMeta(datePickerKey, null);
    const manager = yUndoPluginKey.getState(state)?.undoManager;
    manager?.stopCapturing();
    try { dispatch(tr.scrollIntoView()); }
    finally { manager?.stopCapturing(); }
    return true;
  };
}

export const datePicker = new Plugin<DatePicker | null>({
  key: datePickerKey,
  state: {
    init: () => null,
    apply: (tr, before) => {
      const next: unknown = tr.getMeta(datePickerKey);
      return next === undefined ? before : next as DatePicker | null;
    },
  },
  appendTransaction: (transactions, _before, after) => {
    if (!transactions.some((tr) => tr.docChanged)) return null;
    let clean: Transaction | null = null;
    after.doc.descendants((node, pos) => {
      if (node.type.name === "date" && node.marks.length > 0) clean = (clean ?? after.tr).removeMark(pos, pos + node.nodeSize);
    });
    return clean;
  },
  props: {
    handleKeyDown: (view, event) => {
      if (event.key !== "Enter" || event.ctrlKey || event.metaKey || event.altKey || event.shiftKey || event.isComposing || view.composing) return false;
      if (!view.editable || !selectedDate(view.state)) return false;
      return insertDate(view.state, view.dispatch, view);
    },
    handleClickOn: (view, _pos, node, nodePos, _event, direct) => {
      if (!direct || node.type.name !== "date" || !view.editable) return false;
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, nodePos)));
      return insertDate(view.state, view.dispatch, view);
    },
  },
});
