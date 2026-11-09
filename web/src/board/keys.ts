// The keys a board shares with a Design canvas (design/useCanvasKeys.ts), so the
// two canvases move the same way: + or = and - step the zoom through the same
// steps, 0 fits, 1 shows 100%. Excalidraw has 0 as its eraser and 1 as its
// selection tool; on a board they mean what they mean on a design. H, V and
// holding Space already agree, and stay Excalidraw's.
import { stepZoom } from "../design/view";

export type ZoomKey = "in" | "out" | "fit" | "actual";

interface KeyLike {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  target: EventTarget | null;
}

/** The zoom a key asks for, or null when the key is not one of them or is typed into a field. */
export function zoomKeyOf(e: KeyLike): ZoomKey | null {
  if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return null;
  switch (e.key) {
    case "+":
    case "=":
      return "in";
    case "-":
      return "out";
    case "0":
      return "fit";
    case "1":
      return "actual";
    default:
      return null;
  }
}

function typing(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement;
}

/** Excalidraw's view: a screen point is (scene point + scroll) × zoom. */
export interface BoardView {
  zoom: number;
  scrollX: number;
  scrollY: number;
  width: number;
  height: number;
}

/** The view at zoom z that keeps the board point at the middle of the screen where it is. */
export function zoomedAt(view: BoardView, z: number): Pick<BoardView, "zoom" | "scrollX" | "scrollY"> {
  const cx = view.width / 2 / view.zoom - view.scrollX;
  const cy = view.height / 2 / view.zoom - view.scrollY;
  return { zoom: z, scrollX: view.width / 2 / z - cx, scrollY: view.height / 2 / z - cy };
}

/** The zoom the key leads to from z; fit is the editor's own, so it has none here. */
export function nextZoom(key: Exclude<ZoomKey, "fit">, z: number): number {
  if (key === "actual") return 1;
  return stepZoom(z, key === "in" ? 1 : -1);
}
