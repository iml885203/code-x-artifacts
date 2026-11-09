import { useRef, type PointerEvent as ReactPointerEvent } from "react";
import { useT } from "../i18n";
import type { Key } from "../i18n";
import type { Drag, Editor, Rect, Target } from "./controller";
import type { Handle } from "./geometry";
import { keyOf } from "./useSelection";

const LABELS: Record<Handle, Key> = { nw: "design.resizeNW", ne: "design.resizeNE", sw: "design.resizeSW", se: "design.resizeSE" };
const ALL: Handle[] = ["nw", "ne", "sw", "se"];

interface Props {
  /** The one selected element, once its frame has described it. */
  target: Target | undefined;
  /** Where its frame is on the canvas. */
  origin: { x: number; y: number } | undefined;
  zoom: number;
  preview: Record<string, Rect> | null;
  editor: Editor;
  onPreview: (rects: Record<string, Rect> | null) => void;
  onDone: () => void;
}

/**
 * The corners of the selected element that resize it. An element that is in the
 * flow has no place of its own to give up, so it has only the bottom right one.
 */
export function Handles({ target, origin, zoom, preview, editor, onPreview, onDone }: Props) {
  const t = useT();
  const held = useRef<{ drag: Drag; x: number; y: number } | null>(null);
  if (!target || !origin) return null;
  const box = preview?.[keyOf(target)] ?? target.report.rect;
  const positioned = target.report.computed.position === "absolute";

  const down = (handle: Handle) => (e: ReactPointerEvent<HTMLElement>) => {
    if (e.button !== 0) return;
    e.stopPropagation();
    e.currentTarget.setPointerCapture(e.pointerId);
    held.current = { drag: editor.resize(target, handle), x: e.clientX, y: e.clientY };
  };
  const move = (e: ReactPointerEvent<HTMLElement>) => {
    const h = held.current;
    if (h) onPreview(h.drag.update((e.clientX - h.x) / zoom, (e.clientY - h.y) / zoom, e.shiftKey));
  };
  const up = (e: ReactPointerEvent<HTMLElement>) => {
    const h = held.current;
    held.current = null;
    if (!h) return;
    if (e.type === "pointerup") h.drag.end();
    else h.drag.cancel();
    onPreview(null);
    onDone();
  };

  return (
    <>
      {ALL.filter((h) => positioned || h === "se").map((h) => (
        <button
          key={h}
          type="button"
          tabIndex={-1}
          className="handle"
          data-handle={h}
          aria-label={t(LABELS[h])}
          style={{ left: origin.x + box.x + (h.endsWith("e") ? box.w : 0), top: origin.y + box.y + (h.startsWith("s") ? box.h : 0), cursor: `${h}-resize` }}
          onPointerDown={down(h)}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={up}
        />
      ))}
    </>
  );
}
