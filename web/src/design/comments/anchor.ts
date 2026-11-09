import type { Rect } from "../controller";
import type { DesignFrame } from "../manifest";

/** Source identity and an intentional frame point in unscaled frame pixels. Replies inherit their thread's anchor. */
export interface DesignAnchor extends Record<string, unknown> {
  type: "design";
  frameId: string;
  pid?: string;
  name?: string;
  x: number;
  y: number;
}

const identity = (v: unknown): v is string => typeof v === "string" && v.length > 0 && v.length <= 256;
const coordinate = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 8192;
const clamp = (n: number, max: number) => Number.isFinite(n) ? Math.max(0, Math.min(max, n)) : 0;

export function readAnchor(raw: Record<string, unknown> | undefined): DesignAnchor | null {
  if (raw?.type !== "design" || !identity(raw.frameId) || !coordinate(raw.x) || !coordinate(raw.y) || (raw.pid !== undefined && !identity(raw.pid))) return null;
  return { type: "design", frameId: raw.frameId, x: raw.x, y: raw.y, ...(raw.pid === undefined ? {} : { pid: raw.pid }), ...(typeof raw.name === "string" && raw.name.length > 0 && raw.name.length <= 160 ? { name: raw.name } : {}) };
}

export function anchorAt(frame: DesignFrame, point: { x: number; y: number }, pid?: string, name = frame.name): DesignAnchor {
  return { type: "design", frameId: frame.id, ...(identity(pid) ? { pid } : {}), name: name.slice(0, 160), x: clamp(point.x, frame.w), y: clamp(point.y, frame.h) };
}

/** An element needs current geometry; its absence never turns it into a frame-position comment. */
export function resolveAnchor(anchor: DesignAnchor, frame: DesignFrame, rect?: Rect): { x: number; y: number; fallback: boolean } | null {
  if (anchor.frameId !== frame.id) return null;
  if (!anchor.pid) return { x: clamp(anchor.x, frame.w), y: clamp(anchor.y, frame.h), fallback: false };
  const valid = rect && [rect.x, rect.y, rect.w, rect.h].every(Number.isFinite) && rect.w > 0 && rect.h > 0;
  return valid
    ? { x: clamp(rect.x + rect.w, frame.w), y: clamp(rect.y, frame.h), fallback: false }
    : null;
}

export function anchorName(raw: Record<string, unknown> | undefined): string | null {
  const anchor = readAnchor(raw);
  return anchor ? anchor.name ?? anchor.frameId : null;
}
