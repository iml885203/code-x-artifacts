// The popover and bar of the link feature (features/link.tsx): the form that
// makes or changes the link at the caret, and the bar that shows the link the caret is in.
import { useState, type RefObject } from "react";
import type { EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import type { ToolContext } from "./feature";
import { linkAt, linkDraft, removeLink, setLink, type LinkRange } from "./link";
import { LinkBar, LinkForm } from "./LinkPopover";

/** `openLink` for the tools and Cmd+K, and the `overlay` to draw under the toolbar. */
export function useLinkEditing(view: RefObject<EditorView | null>, run: ToolContext["run"], state: EditorState, editable: boolean) {
  // The link being made or changed.
  const [form, setForm] = useState<LinkRange | null>(null);
  const openLink = () => {
    const v = view.current;
    const draft = v ? linkDraft(v.state) : null;
    if (draft !== null) setForm(draft);
  };
  const current = linkAt(state);
  const overlay = !editable ? null : form !== null ? (
    <LinkForm
      key={`${String(form.from)}-${String(form.to)}`}
      draft={form}
      onCancel={() => {
        setForm(null);
        view.current?.focus();
      }}
      onSave={(text, href) => {
        run(setLink(form, text, href));
        setForm(null);
      }}
    />
  ) : current !== null ? (
    <LinkBar
      link={current}
      onChange={() => {
        setForm(current);
      }}
      onRemove={() => {
        run(removeLink(current));
      }}
    />
  ) : null;
  return { openLink, overlay };
}
