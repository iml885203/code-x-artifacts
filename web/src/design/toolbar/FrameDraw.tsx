import { useState, type PointerEvent } from "react";
import { useT } from "../../i18n";

/** How far a press must move before it is a drawn box and not a click. */
const DRAWN = 8;

interface Props {
  zoom: number;
  /** The size of the box in the canvas's pixels, or null for a click, which asks for the usual size. */
  onDraw: (size: { w: number; h: number } | null) => void;
}

interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * The surface over the canvas while the Frame tool is armed: a drag draws the
 * box of the new frame (shown with its size), a click asks for the usual size.
 * It only reports what was drawn; adding the frame is the structure's.
 */
export function FrameDraw({ zoom, onDraw }: Props) {
  const t = useT();
  const [start, setStart] = useState<{ x: number; y: number } | null>(null);
  const [box, setBox] = useState<Box | null>(null);

  const boxTo = (from: { x: number; y: number }, e: PointerEvent<HTMLDivElement>): Box => {
    const r = e.currentTarget.getBoundingClientRect();
    return { x: Math.min(from.x, e.clientX) - r.left, y: Math.min(from.y, e.clientY) - r.top, w: Math.abs(e.clientX - from.x), h: Math.abs(e.clientY - from.y) };
  };
  const finish = (e: PointerEvent<HTMLDivElement>) => {
    if (!start) return;
    const drawn = boxTo(start, e);
    setStart(null);
    setBox(null);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    const moved = drawn.w >= DRAWN || drawn.h >= DRAWN;
    onDraw(moved ? { w: Math.round(drawn.w / zoom), h: Math.round(drawn.h / zoom) } : null);
  };

  return (
    <div
      className="frame-draw"
      role="group"
      aria-label={t("toolbar.drawing")}
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        e.currentTarget.setPointerCapture(e.pointerId);
        setStart({ x: e.clientX, y: e.clientY });
      }}
      onPointerMove={(e) => {
        if (start) setBox(boxTo(start, e));
      }}
      onPointerUp={finish}
      onPointerCancel={() => {
        setStart(null);
        setBox(null);
      }}
    >
      {box && (
        <div className="frame-drawn" data-drawn style={{ left: box.x, top: box.y, width: box.w, height: box.h }}>
          <span className="mono">
            {Math.round(box.w / zoom)} × {Math.round(box.h / zoom)}
          </span>
        </div>
      )}
    </div>
  );
}
