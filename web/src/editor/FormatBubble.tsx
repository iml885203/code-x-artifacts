// The bubble over the words selected: the formatting tools (for people who may edit) and Comment (for people who may comment),
// in one place so the two never sit on top of each other. It hides when the selection collapses and while the person types.
import { Fragment, useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { TextSelection, type EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { useT } from "../i18n";
import { place, shows, type Placement } from "./bubblePlace";
import type { ToolContext } from "./feature";
import { features } from "./features";
import "./bubble.css";

const tools = features
  .flatMap((f) => f.tools ?? [])
  .filter((tool) => tool.bubble !== undefined)
  .sort((a, b) => (a.bubble ?? 0) - (b.bubble ?? 0));

/** A key pressed in the editor hides the bubble until the selection changes: it must not sit in front of what is being typed. */
export function useTyping(state: EditorState) {
  const selectionKey = `${String(state.selection.anchor)}-${String(state.selection.head)}`;
  const [typedAt, setTypedAt] = useState<string | null>(null);
  const onKeyDown = (e: KeyboardEvent) => {
    if (!e.metaKey && !e.ctrlKey && !e.altKey && e.key.length === 1) setTypedAt(selectionKey);
  };
  return { typing: typedAt === selectionKey, onKeyDown };
}

interface Props {
  view: RefObject<EditorView | null>;
  /** The editor's scrolling area: the bubble is placed in its coordinates and kept inside it. */
  scroller: RefObject<HTMLElement | null>;
  /** What the selection is in; the tools draw themselves from it. */
  state: EditorState;
  ctx: ToolContext;
  /** The person may change the doc: the formatting tools show. */
  editable: boolean;
  /** The person may comment: Comment shows, and `anchored` says whether the selection can be commented on. */
  comment: { start: () => void; anchored: boolean } | null;
  typing: boolean;
}

export function FormatBubble({ view, scroller, state, ctx, editable, comment, typing }: Props) {
  const t = useT();
  const bubble = useRef<HTMLDivElement>(null);
  const [at, setAt] = useState<Placement | null>(null);
  const { from, to, empty } = state.selection;
  const selected = state.selection instanceof TextSelection && !empty;
  const visible = selected && shows(true, typing) && (editable || (comment?.anchored ?? false));

  useLayoutEffect(() => {
    const v = view.current;
    const area = scroller.current;
    const box = bubble.current;
    if (!visible || !v || !area || !box || to > v.state.doc.content.size) {
      setAt(null);
      return;
    }
    const first = v.coordsAtPos(from);
    const last = v.coordsAtPos(to);
    const frame = area.getBoundingClientRect();
    const oneLine = Math.abs(first.top - last.top) < 4;
    const x = (oneLine ? (first.left + last.left) / 2 : first.left) - frame.left + area.scrollLeft;
    const top = first.top - frame.top + area.scrollTop;
    const bottom = last.bottom - frame.top + area.scrollTop;
    setAt(place({ x, top, bottom }, { width: box.offsetWidth, height: box.offsetHeight }, { width: area.clientWidth, height: area.scrollHeight, scrollTop: area.scrollTop, viewHeight: area.clientHeight }));
  }, [view, scroller, visible, from, to, state]);

  if (!visible) return null;
  // A press on the bubble must not take the selection away from the words; a select needs the press to open.
  const keepSelection = (e: React.MouseEvent) => {
    if (!(e.target instanceof HTMLSelectElement) && !(e.target instanceof HTMLOptionElement)) e.preventDefault();
  };
  return (
    <div
      ref={bubble}
      className="ed-bubble"
      role="toolbar"
      aria-label={t("editor.bubble")}
      style={at === null ? { visibility: "hidden" } : { left: at.left, top: at.top }}
      onMouseDown={keepSelection}
    >
      {editable &&
        tools.map((tool) => (
          <Fragment key={tool.id}>{tool.render(state, ctx, t)}</Fragment>
        ))}
      {editable && comment && <span className="ed-bubble-sep" aria-hidden="true" />}
      {comment && (
        <button type="button" className="quiet ed-bubble-comment" disabled={!comment.anchored} onClick={comment.start}>
          {t("anchors.comment")}
        </button>
      )}
    </div>
  );
}
