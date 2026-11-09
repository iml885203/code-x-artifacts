// The viewport of the canvas: where the page is and how big, as the pure
// arithmetic of pan and zoom. The screen point (px, py) is measured from the
// top left corner of the viewport.

export interface View {
  /** Where the canvas origin is, in screen pixels. */
  x: number;
  y: number;
  /** Screen pixels per canvas pixel. */
  z: number;
}

/** The zoom the buttons and keys step through, 25% to 200%. */
export const STEPS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2] as const;
export const MIN_ZOOM = STEPS[0];
export const MAX_ZOOM = STEPS[STEPS.length - 1] ?? 2;
/** Fit may need less than the smallest step to show a big page. */
export const FIT_FLOOR = 0.05;

export const clampZoom = (z: number, floor: number = MIN_ZOOM): number => Math.min(MAX_ZOOM, Math.max(floor, z));

/** The view with zoom z that keeps the canvas point under the screen point (px, py) where it is. */
export function zoomAt(view: View, z: number, px: number, py: number, floor: number = MIN_ZOOM): View {
  const next = clampZoom(z, floor);
  const ratio = next / view.z;
  return { x: px - (px - view.x) * ratio, y: py - (py - view.y) * ratio, z: next };
}

/** The next step above or below z. */
export function stepZoom(z: number, direction: 1 | -1): number {
  const eps = 1e-6;
  if (direction > 0) return STEPS.find((s) => s > z + eps) ?? MAX_ZOOM;
  return [...STEPS].reverse().find((s) => s < z - eps) ?? MIN_ZOOM;
}

export function panBy(view: View, dx: number, dy: number): View {
  return { ...view, x: view.x + dx, y: view.y + dy };
}

/** The band along the top of the canvas that the floating bars and the frames' labels use: a fitted page starts below it. */
export const BAR_BAND = 80;

/** The view that shows a page of the given size whole and centred in a viewport, never larger than 100%, and never higher than `top` from the viewport's top. */
export function fit(width: number, height: number, vw: number, vh: number, top = 0): View {
  const z = Math.min(1, Math.max(FIT_FLOOR, Math.min(vw / width, (vh - top) / height)));
  return { z, x: (vw - width * z) / 2, y: Math.max(top, (vh - height * z) / 2) };
}
