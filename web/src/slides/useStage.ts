import { useEffect, useRef, useState, type RefObject } from "react";
import type { Zoom } from "./DeckBar";

const CANVAS = 1920;
const GUTTER = 24;
/** Above and below a fitted slide: room for the floating bar over the top of the stage, the same below so it stays centred. */
const GUTTER_Y = 72;
/** The width the stage assumes before it has been measured (and where nothing is laid out). */
const UNMEASURED = 960;

export interface Stage {
  size: { width: number; height: number };
  /** The slide's width as a percentage of its 1920 px canvas. */
  percent: number;
  /** Puts the stage's frame full screen; where the browser cannot, opens the full page in a new tab. */
  present: () => void;
}

/** The width of a 16:9 slide that fits in a box, clear of the floating bar. */
function fitWidth(box: { width: number; height: number }): number {
  if (box.width <= 0 || box.height <= 0) return UNMEASURED;
  return Math.max(160, Math.floor(Math.min(box.width - 2 * GUTTER, ((box.height - 2 * GUTTER_Y) * 16) / 9)));
}

/** A slide number the deck's page reported from the stage's frame, or null for any other message. */
function reported(e: MessageEvent, frame: HTMLIFrameElement | null): number | null {
  if (e.source === null || e.source !== frame?.contentWindow || typeof e.data !== "object" || !e.data) return null;
  const m = e.data as { pictor?: unknown; number?: unknown };
  return m.pictor === "slide" && typeof m.number === "number" && Number.isInteger(m.number) && m.number >= 1 ? m.number : null;
}

/**
 * The stage's frame and the shell agree on the slide by message: the page says which slide it shows (its own keys
 * move it too), the shell asks it to show the selected one. Only numbers from the stage's own frame are heard.
 */
export function useStage(area: RefObject<HTMLDivElement | null>, frame: RefObject<HTMLIFrameElement | null>, selected: number, select: (n: number) => void, zoom: Zoom): Stage {
  const [box, setBox] = useState({ width: 0, height: 0 });
  // What the frame said it shows, and which page said it: a page that has just loaded is told the selection.
  const showing = useRef<{ from: MessageEventSource; n: number } | null>(null);
  const wanted = useRef(selected);
  // The slide the frame was last asked to show, until it says it shows it.
  const asked = useRef<number | null>(null);

  useEffect(() => {
    const el = area.current;
    if (!el) return;
    const observer = new ResizeObserver(() => {
      setBox({ width: el.clientWidth, height: el.clientHeight });
    });
    observer.observe(el);
    return () => {
      observer.disconnect();
    };
  }, [area]);

  useEffect(() => {
    const hear = (e: MessageEvent) => {
      const n = reported(e, frame.current);
      if (n === null || e.source === null) return;
      const loaded = showing.current?.from !== e.source;
      showing.current = { from: e.source, n };
      if (loaded && n !== wanted.current) {
        asked.current = wanted.current;
        e.source.postMessage({ pictor: "slides", show: wanted.current }, { targetOrigin: "*" });
        return;
      }
      // An answer to an earlier ask, overtaken by a newer one, must not take the selection back.
      if (!loaded && asked.current !== null && n !== asked.current) return;
      asked.current = null;
      select(n);
    };
    window.addEventListener("message", hear);
    return () => {
      window.removeEventListener("message", hear);
    };
  }, [frame, select]);

  useEffect(() => {
    wanted.current = selected;
    if (showing.current?.n === selected) return;
    asked.current = selected;
    frame.current?.contentWindow?.postMessage({ pictor: "slides", show: selected }, "*");
  }, [frame, selected]);

  useEffect(() => {
    const changed = () => {
      const f = frame.current;
      f?.contentWindow?.postMessage({ pictor: "slides", present: document.fullscreenElement === f }, "*");
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
    };
  }, [frame]);

  function present() {
    const f = frame.current;
    if (!f) return;
    if (!document.fullscreenEnabled || typeof f.requestFullscreen !== "function") {
      const id = /^\/raw\/([^/?]+)/.exec(new URL(f.src, location.href).pathname)?.[1];
      if (id) window.open(`/a/${id}?view=full&slide=${String(selected)}`, "_blank", "noopener");
      return;
    }
    f.requestFullscreen().then(
      () => {
        f.focus();
      },
      (err: unknown) => {
        console.error("present the deck full screen", err);
      },
    );
  }

  const width = zoom === "fit" ? fitWidth(box) : Math.round((CANVAS * zoom) / 100);
  return { size: { width, height: Math.round((width * 9) / 16) }, percent: Math.round((width / CANVAS) * 100), present };
}
