import { GAP_Y } from "./manifest";

/** Screen sizes in px: a name line, a 44px target with the label's padding, and two targets side by side. */
export const NAME_LINE = 26;
export const ROW_ONE_LINE = 50;
export const ROW_TWO_LINES = 70;
export const TWO_TARGETS = 88;
/** Past this label width the name, size and actions share one line (the label's container query uses the same number). */
export const ONE_LINE_FROM = 240;

export interface LabelLayout {
  /** "row": each frame's label carries Comment and Play. "dock": one strip carries Comment and Play for the active frame. */
  actions: "row" | "dock";
  /** "above": the name sits in the row gap above its frame. "inside": the gap is too short, it sits on the frame's top edge. */
  name: "above" | "inside";
}

/**
 * Where names and frame actions live at a zoom, for the frames of a page (their widths in
 * world px). A label sits in the row gap, which is GAP_Y x zoom on screen whatever the
 * frames' heights, and is as wide as its frame is on screen. Actions stay in the labels
 * only while every label can hold two 44px targets and the gap holds its height; otherwise
 * Comment and Play are not squeezed into a frame or a gap: they move to one strip in
 * interface space.
 */
export function labelLayout(zoom: number, widths: readonly number[]): LabelLayout {
  const gap = GAP_Y * zoom;
  const fits = (w: number) => {
    const screen = w * zoom;
    return screen >= TWO_TARGETS && gap >= (screen > ONE_LINE_FROM ? ROW_ONE_LINE : ROW_TWO_LINES);
  };
  return { actions: widths.every(fits) ? "row" : "dock", name: gap >= NAME_LINE ? "above" : "inside" };
}
