// The arithmetic of moving and resizing an element on the canvas. Distances are
// in the frame's own pixels (the pointer's distance on screen divided by the
// zoom), which is why a drag of 30 by 15 at 50% moves an element 60 by 30.

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

export type Handle = "nw" | "ne" | "sw" | "se";

/** The owned declarations to write, as the room holds them. */
export type Style = Partial<Record<"left" | "top" | "width" | "height", string>>;

const px = (n: number) => `${String(Math.round(n))}px`;

/** Where an element starts a gesture: its computed place and size, or null when its place is not in pixels. */
export function startOf(computed: Record<string, string>, rect: { x: number; y: number; w: number; h: number }): Box | null {
  const left = Number.parseFloat(computed.left ?? "");
  const top = Number.parseFloat(computed.top ?? "");
  if (Number.isNaN(left) || Number.isNaN(top)) return null;
  const width = Number.parseFloat(computed.width ?? "");
  const height = Number.parseFloat(computed.height ?? "");
  return { left, top, width: Number.isNaN(width) ? rect.w : width, height: Number.isNaN(height) ? rect.h : height };
}

/** The declarations that move an element by a distance; none when it rounds to no distance. */
export function moved(from: Box, dx: number, dy: number): Style {
  const out: Style = {};
  if (Math.round(dx) !== 0) out.left = px(from.left + dx);
  if (Math.round(dy) !== 0) out.top = px(from.top + dy);
  return out;
}

/**
 * The declarations that resize an element by pulling a corner by a distance:
 * the opposite corner stays, no side gets smaller than a pixel, and with
 * `keepRatio` the bigger change leads and the other follows. An element in the
 * flow has no place of its own to move, so only its size is written.
 */
export function resized(from: Box, handle: Handle, dx: number, dy: number, keepRatio: boolean, positioned: boolean): Style {
  const west = handle.endsWith("w");
  const north = handle.startsWith("n");
  let width = Math.max(1, from.width + (west ? -dx : dx));
  let height = Math.max(1, from.height + (north ? -dy : dy));
  if (keepRatio) {
    const scale = Math.max(width / from.width, height / from.height);
    width = Math.max(1, from.width * scale);
    height = Math.max(1, from.height * scale);
  }
  width = Math.round(width);
  height = Math.round(height);
  return {
    ...(positioned && west ? { left: px(from.left + from.width - width) } : {}),
    ...(positioned && north ? { top: px(from.top + from.height - height) } : {}),
    width: px(width),
    height: px(height),
  };
}
