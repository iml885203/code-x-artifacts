import type { Node, Slice } from "prosemirror-model";
import { validDate } from "./features/date/commands";
import { readDropdownConfig } from "./schema";

function leafText(node: Node): string {
  if (node.type.name === "date") return validDate(node.attrs.date) ? node.attrs.date : "";
  if (node.type.name === "dropdown") return readDropdownConfig(node.attrs.config)?.value ?? "";
  return node.type.spec.leafText?.(node) ?? "";
}

/** Keep ProseMirror's clipboard separators and other leaf text while including validated field values. */
export function fieldClipboardText(slice: Slice): string {
  return slice.content.textBetween(0, slice.content.size, "\n\n", leafText);
}
