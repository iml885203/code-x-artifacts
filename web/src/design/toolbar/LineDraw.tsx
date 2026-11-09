import { useRef, useState, type PointerEvent } from "react";
import { useT } from "../../i18n";
import type { PenDrawProps, PenFrame } from "../pen/PenDraw";
import { addLine, type LinePoint } from "./lines";

export function LineDraw({ arrow, ...props }: PenDrawProps & { arrow: boolean }) {
  if (!props.active || !props.editable || !props.edits) return null;
  return <Drawing {...props} arrow={arrow} />;
}

function Drawing({ frames, edits, view, onCreated, arrow }: PenDrawProps & { arrow: boolean }) {
  const t = useT();
  const held = useRef<{ frame: PenFrame; from: LinePoint; pointer: number } | null>(null);
  const [preview, setPreview] = useState<{ from: LinePoint; to: LinePoint } | null>(null);
  const point = (event: PointerEvent<HTMLElement>) => {
    const box = event.currentTarget.getBoundingClientRect();
    return { x: (event.clientX - box.left - view.x) / view.z, y: (event.clientY - box.top - view.y) / view.z };
  };
  const cancel = () => { held.current = null; setPreview(null); };
  return <div className="pen-draw" role="group" aria-label={t(arrow ? "toolbar.arrow" : "toolbar.line")} onPointerDown={(event) => {
    if (event.button !== 0) return;
    const from = point(event);
    const frame = frames.find(({ frame, x, y }) => from.x >= x && from.y >= y && from.x <= x + frame.w && from.y <= y + frame.h);
    if (!frame) return;
    held.current = { frame, from, pointer: event.pointerId };
    event.currentTarget.setPointerCapture(event.pointerId);
  }} onPointerMove={(event) => {
    if (held.current?.pointer === event.pointerId) setPreview({ from: held.current.from, to: point(event) });
  }} onPointerUp={(event) => {
    const gesture = held.current;
    if (gesture?.pointer !== event.pointerId) return;
    const end = point(event);
    const to = { x: Math.max(gesture.frame.x, Math.min(gesture.frame.x + gesture.frame.frame.w, end.x)), y: Math.max(gesture.frame.y, Math.min(gesture.frame.y + gesture.frame.frame.h, end.y)) };
    cancel();
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    if (!edits || !frames.some(({ frame }) => frame.id === gesture.frame.frame.id)) return;
    const { frame, x, y } = gesture.frame;
    const ref = addLine(edits, frame.file, { x: gesture.from.x - x, y: gesture.from.y - y }, { x: to.x - x, y: to.y - y }, arrow, t(arrow ? "toolbar.arrow" : "toolbar.line"));
    if (ref) onCreated?.(ref);
  }} onPointerCancel={cancel} onLostPointerCapture={cancel}>
    {preview && <svg className="line-preview" aria-hidden="true"><line x1={view.x + preview.from.x * view.z} y1={view.y + preview.from.y * view.z} x2={view.x + preview.to.x * view.z} y2={view.y + preview.to.y * view.z} /></svg>}
  </div>;
}
