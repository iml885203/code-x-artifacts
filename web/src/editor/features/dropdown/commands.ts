import type { Node } from "prosemirror-model";
import { PluginKey, TextSelection, type Command, type EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { absolutePositionToRelativePosition, relativePositionToAbsolutePosition, ySyncPluginKey, yUndoPluginKey } from "y-prosemirror";
import * as Y from "yjs";
import { defaults, encodeConfig, readConfig, validConfig, type DropdownConfig } from "./config";
import { showPicker, type DropdownWords } from "./picker";

export const dropdownWordsKey = new PluginKey<{ words: () => DropdownWords } | null>("dropdown-words");

/** Plus and slash use the same command; probes neither resolve words nor open UI. */
export const openDropdown: Command = (state, dispatch, view) => {
  if (!dispatch) return captureAllowed(state);
  if (!view || view.isDestroyed || !view.editable) return false;
  const words = dropdownWordsKey.getState(state)?.words();
  return words !== undefined && insertDropdown(words)(state, dispatch, view);
};

type Mapping = Parameters<typeof absolutePositionToRelativePosition>[2];
interface Binding { doc: Y.Doc; type: Y.XmlFragment; mapping: Mapping }
const bindingOf = (state: EditorState): Binding | null => (ySyncPluginKey.getState(state) as { binding?: Binding } | undefined)?.binding ?? null;

function elementOf(binding: Binding, node: Node): Y.XmlElement | null {
  for (const [shared, mapped] of binding.mapping) {
    if (shared instanceof Y.XmlElement && mapped === node) return shared;
  }
  return null;
}

function positionOf(state: EditorState, binding: Binding, element: Y.XmlElement): number | null {
  if (element.parent === null) return null;
  const mapped = binding.mapping.get(element);
  let found: number | null = null;
  state.doc.descendants((node, pos) => {
    if (node === mapped) found = pos;
    return found === null;
  });
  return found;
}

function captureAllowed(state: EditorState): boolean {
  const { $from, $to } = state.selection;
  const type = state.schema.nodes.dropdown;
  return type !== undefined && state.selection instanceof TextSelection && $from.sameParent($to)
    && $from.parent.type.spec.code !== true && $from.parent.canReplaceWith($from.index(), $to.index(), type);
}

/** Availability probes never create a picker or touch the room. */
export function insertDropdown(words: DropdownWords): Command {
  return (state, dispatch, view) => {
    if (!captureAllowed(state)) return false;
    if (!dispatch) return true;
    if (!view || view.isDestroyed || !view.editable) return false;
    const binding = bindingOf(state);
    if (!binding) return false;
    const parent = elementOf(binding, state.selection.$from.parent);
    if (!parent) return false;
    const start = absolutePositionToRelativePosition(state.selection.from, binding.type, binding.mapping) as Y.RelativePosition;
    const end = absolutePositionToRelativePosition(state.selection.to, binding.type, binding.mapping) as Y.RelativePosition;
    showPicker(view, defaults(), words, (config) => {
      const current = bindingOf(view.state);
      if (current?.type !== binding.type || !validConfig(config)) return "missing";
      const parentPos = positionOf(view.state, current, parent);
      const from = relativePositionToAbsolutePosition(current.doc, current.type, start, current.mapping);
      const to = relativePositionToAbsolutePosition(current.doc, current.type, end, current.mapping);
      if (parentPos === null || from === null || to === null || from > to) return "missing";
      const $from = view.state.doc.resolve(from);
      const $to = view.state.doc.resolve(to);
      if (!$from.sameParent($to) || elementOf(current, $from.parent) !== parent) return "missing";
      const selection = TextSelection.create(view.state.doc, from, to);
      const selected = view.state.apply(view.state.tr.setSelection(selection));
      if (!captureAllowed(selected)) return "missing";
      const type = selected.schema.nodes.dropdown;
      if (!type) return "missing";
      saveBoundary(view, () => {
        view.dispatch(view.state.tr.setSelection(selection).replaceSelectionWith(type.create({ config: encodeConfig(config) }), false).scrollIntoView());
      });
      return null;
    });
    return true;
  };
}

/** Edits retain the shared element, so deleting or replacing a target never edits its neighbour. */
export function editDropdown(view: EditorView, pos: number, words: DropdownWords): boolean {
  if (view.isDestroyed || !view.editable) return false;
  const binding = bindingOf(view.state);
  const node = view.state.doc.nodeAt(pos);
  if (!binding || node?.type.name !== "dropdown") return false;
  const element = elementOf(binding, node);
  const initial = readConfig(node.attrs.config);
  if (!element || !initial) return false;
  showPicker(view, initial, words, (submitted) => {
    const current = bindingOf(view.state);
    if (current?.type !== binding.type || positionOf(view.state, current, element) === null) return "missing";
    const fresh = readConfig(element.getAttribute("config"));
    if (!fresh) return "missing";
    const optionsChanged = JSON.stringify(submitted.options) !== JSON.stringify(initial.options);
    let next: DropdownConfig;
    if (optionsChanged) {
      // An options editor cannot silently restore options another person changed while it was open.
      if (JSON.stringify(fresh.options) !== JSON.stringify(initial.options)) return "changed";
      next = submitted;
    } else {
      if (!fresh.options.includes(submitted.value)) return "changed";
      next = { options: fresh.options, value: submitted.value };
    }
    if (!validConfig(next)) return "invalid";
    const encoded = encodeConfig(next);
    if (encoded === element.getAttribute("config")) return null;
    saveBoundary(view, () => {
      current.doc.transact(() => { element.setAttribute("config", encoded); }, ySyncPluginKey);
    });
    return null;
  });
  return true;
}

function saveBoundary(view: EditorView, write: () => void): void {
  const history = yUndoPluginKey.getState(view.state) as { undoManager?: Y.UndoManager } | undefined;
  history?.undoManager?.stopCapturing();
  try { write(); } finally { history?.undoManager?.stopCapturing(); }
}
