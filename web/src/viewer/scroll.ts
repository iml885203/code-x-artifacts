// Keeping the place in a page when a new version replaces the frame. The page
// in the frame (internal/web/bridge.js) tells the viewer where it is scrolled
// to and that it has loaded; the viewer, when it replaces the frame because
// someone published, sends the last position to the new page once it has
// loaded. The page is not trusted: only numbers it sends from its own frame are
// kept, and a position is only a place to scroll to.

interface Position {
  x: number;
  y: number;
}

const isPosition = (m: { x?: unknown; y?: unknown }): m is Position => typeof m.x === "number" && typeof m.y === "number" && Number.isFinite(m.x) && Number.isFinite(m.y);

export interface ScrollKeeper {
  /** The listener for the window's "message" events: it reads only what the artifact's own frame says. */
  hear: (e: MessageEvent) => void;
  /** The frame is about to be replaced by a newer version: put its successor back where this one is. */
  carryOver: () => void;
}

export function scrollKeeper(frame: () => Window | null): ScrollKeeper {
  let at: Position = { x: 0, y: 0 };
  let carry: Position | null = null;
  return {
    hear(e) {
      const target = frame();
      if (!target || e.source !== target || typeof e.data !== "object" || e.data === null) return;
      const m = e.data as { pictor?: unknown; x?: unknown; y?: unknown };
      if (m.pictor === "scroll" && isPosition(m)) {
        at = { x: m.x, y: m.y };
      } else if (m.pictor === "ready" && carry) {
        // An opaque origin has no name to address, so "*" is the only way to reach it.
        target.postMessage({ pictor: "scrollTo", ...carry }, "*");
        carry = null;
      }
    },
    carryOver() {
      carry = at;
    },
  };
}
