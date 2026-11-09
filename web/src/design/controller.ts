// What a gesture on the canvas does: it shows the element moving in its frame
// while the pointer is down, and when the pointer is released writes the result
// to the room as a single step of the person's history. Nothing reaches the room
// before that, so a drag is one change however many times the pointer moved.
import { DesignEdits, type Change } from "./edits";
import { moved, resized, startOf, type Box, type Handle, type Style } from "./geometry";
import { isFrameGone, type ElementReport, type FrameLinks, type StyleChange } from "./inspect";
import { keyOf } from "./useSelection";

export interface Target {
  frameId: string;
  file: string;
  pid: string;
  report: ElementReport;
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** A gesture in progress. Distances are in the frame's pixels, from where the pointer was pressed. */
export interface Drag {
  /** Shows the elements as they would be; returns where each one's box is then, by the selection's key. */
  update(dx: number, dy: number, keepRatio?: boolean): Record<string, Rect>;
  /** Writes what was shown to the room as one step. False, and nothing written, when it ends where it began. */
  end(): boolean;
  /** Shows what the room holds again. */
  cancel(): void;
}

export type Shown = Pick<FrameLinks, "apply">;

const PROPS = ["left", "top", "width", "height"] as const;
const absolute = (t: Target) => t.report.computed.position === "absolute";

export class Editor {
  constructor(
    private readonly edits: DesignEdits,
    private readonly links: Shown,
  ) {}

  /** A drag of the targets that can be dragged (positioned absolutely, in pixels); `still` are the ones that cannot. */
  move(targets: Target[]): { drag: Drag | null; still: Target[] } {
    const starts = new Map<Target, Box>();
    const still: Target[] = [];
    for (const t of targets) {
      const start = absolute(t) ? startOf(t.report.computed, t.report.rect) : null;
      if (start) starts.set(t, start);
      else still.push(t);
    }
    if (starts.size === 0) return { drag: null, still };
    return { drag: this.gesture([...starts], (start, dx, dy) => moved(start, dx, dy)), still };
  }

  /** A resize of one element by one of its corners. */
  resize(target: Target, handle: Handle): Drag {
    const start = startOf(target.report.computed, target.report.rect) ?? { left: target.report.rect.x, top: target.report.rect.y, width: target.report.rect.w, height: target.report.rect.h };
    return this.gesture([[target, start]], (from, dx, dy, keep) => resized(from, handle, dx, dy, keep, absolute(target)));
  }

  /** Deletes the targets, with what is inside them, as one step. */
  remove(targets: Target[]): boolean {
    return this.edits.apply(targets.map((t): Change => ({ file: t.file, pid: t.pid, remove: true })));
  }

  private gesture(items: [Target, Box][], style: (start: Box, dx: number, dy: number, keepRatio: boolean) => Style): Drag {
    let latest: [Target, Style][] = [];
    const show = (styles: Map<Target, Record<string, string | null>>) => {
      const byFrame = new Map<string, StyleChange[]>();
      for (const [t, props] of styles) {
        const list = byFrame.get(t.frameId) ?? [];
        for (const [prop, value] of Object.entries(props)) list.push([t.pid, prop, value]);
        byFrame.set(t.frameId, list);
      }
      for (const [frameId, list] of byFrame) {
        this.links.apply(frameId, list).catch((err: unknown) => {
          if (!isFrameGone(err)) console.error("show an element while it is dragged", err);
        });
      }
    };
    /** What the room holds of each property the gesture has touched, to show again. */
    const room = () => new Map(items.map(([t]) => [t, Object.fromEntries(PROPS.map((p) => [p, this.edits.frame(t.file).styles.get(t.pid)?.get(p) ?? null]))]));
    return {
      update: (dx, dy, keepRatio = false) => {
        latest = items.map(([t, start]) => [t, style(start, dx, dy, keepRatio)]);
        const rects: Record<string, Rect> = {};
        const shown = new Map<Target, Record<string, string | null>>();
        items.forEach(([t, start], i) => {
          const s = latest[i]?.[1] ?? {};
          // Every property this kind of gesture changes is shown each time, so one that returns to where it began is shown at where it began.
          shown.set(t, { left: `${String(start.left)}px`, top: `${String(start.top)}px`, ...s });
          const at = (prop: keyof Style, from: number) => (s[prop] === undefined ? 0 : Number.parseFloat(s[prop]) - from);
          rects[keyOf(t)] = {
            x: t.report.rect.x + at("left", start.left),
            y: t.report.rect.y + at("top", start.top),
            w: t.report.rect.w + at("width", start.width),
            h: t.report.rect.h + at("height", start.height),
          };
        });
        show(shown);
        return rects;
      },
      end: () => {
        const wrote = this.edits.apply(latest.map(([t, s]): Change => ({ file: t.file, pid: t.pid, style: s })));
        show(room());
        return wrote;
      },
      cancel: () => {
        show(room());
      },
    };
  }
}
