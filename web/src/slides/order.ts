/**
 * Where a dragged slide lands. `from` is its number; `gap` is the place it is
 * dropped, the number of the slide it goes before (count + 1 for the end). The
 * answer is the number the slide has after the move, as move_slide takes it, or
 * null when it stays where it is.
 */
export function moveTarget(from: number, gap: number, count: number): number | null {
  if (from < 1 || from > count || gap < 1 || gap > count + 1) return null;
  const to = gap > from ? gap - 1 : gap;
  return to === from ? null : to;
}

/** The gap a drop over slide `over` makes: before it when the pointer is on its first half, else after it. */
export function gapAt(over: number, pointer: number, start: number, size: number): number {
  return pointer < start + size / 2 ? over : over + 1;
}

/** Which slide to select after an edit, from the one selected and the count the deck now has. */
export function clampSlide(n: number, count: number): number {
  return Math.min(Math.max(1, n), Math.max(1, count));
}
