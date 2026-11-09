// Where the bubble over a selection goes: above the start of the selection, kept inside the editor's scrolling area.
export interface Box {
  width: number;
  height: number;
}

export interface Placement {
  left: number;
  top: number;
}

/** The gap between the selection and the bubble. */
export const gap = 8;

/**
 * `selection` is the selection's first line (`x` its middle, `top` its top) and `bottom` the bottom of its last line, in the
 * scrolling area's own coordinates (scroll included). The bubble is centred above the first line and pushed back inside
 * `area` on every side, so it never hangs off or is cut by an edge. With no room above in view it goes below the
 * selection instead.
 */
export function place(selection: { x: number; top: number; bottom: number }, bubble: Box, area: Box & { scrollTop: number; viewHeight: number }): Placement {
  const clamp = (n: number, low: number, high: number) => Math.max(low, Math.min(n, Math.max(low, high)));
  const above = selection.top - gap - bubble.height;
  const top = above >= area.scrollTop ? above : selection.bottom + gap;
  return { left: clamp(selection.x - bubble.width / 2, 0, area.width - bubble.width), top: Math.max(0, top) };
}

/** Whether the bubble shows: words are selected and the person is not typing (a key since the selection last changed). */
export function shows(selected: boolean, typing: boolean): boolean {
  return selected && !typing;
}
