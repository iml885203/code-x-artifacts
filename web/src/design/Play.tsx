import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Close } from "../icons/chrome";
import { format, useT } from "../i18n";
import type { DesignEdits } from "./edits";
import { useTokenFrame } from "./useTokenFrame";
import type { DesignFrame } from "./manifest";

interface Props {
  frame: DesignFrame;
  /** Where the frame's file is served. */
  src: string;
  onExit: () => void;
  edits?: DesignEdits | null;
}

export type PlayMode = "fit" | "fill";

/** The scale a frame is played at in a window: all of it visible at most at its real size, or its width filling the window. */
export function playScale(mode: PlayMode, frame: Pick<DesignFrame, "w" | "h">, vw: number, vh: number): number {
  if (mode === "fill") return vw / frame.w;
  return Math.min(1, vw / frame.w, vh / frame.h);
}

/**
 * One frame at full size over the canvas. Fit shows the whole frame, at its
 * real size unless the window is smaller; Fill makes its width the window's and
 * scrolls the rest. Escape or the close button goes back to the canvas.
 */
export function Play({ frame, src, onExit, edits }: Props) {
  const t = useT();
  const tokenFrame = useTokenFrame(edits ?? null, frame.id);
  const [mode, setMode] = useState<PlayMode>("fit");
  const stage = useRef<HTMLDivElement>(null);
  const close = useRef<HTMLButtonElement>(null);
  const [size, setSize] = useState({ w: window.innerWidth, h: window.innerHeight });

  useLayoutEffect(() => {
    const el = stage.current;
    if (!el) return;
    const measure = () => {
      const r = el.getBoundingClientRect();
      if (r.width > 0 && r.height > 0) setSize({ w: r.width, h: r.height });
    };
    measure();
    const watch = new ResizeObserver(measure);
    watch.observe(el);
    return () => {
      watch.disconnect();
    };
  }, []);

  useEffect(() => {
    close.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      onExit();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, [onExit]);

  const scale = playScale(mode, frame, size.w, size.h);
  return (
    <div className="play" role="dialog" aria-label={format(t("design.playing"), { name: frame.name })}>
      <div className="play-bar">
        <strong>{frame.name}</strong>
        <span className="mono">
          {frame.w} × {frame.h}
        </span>
        <span className="spacer" />
        <div className="play-modes" role="group">
          {(["fit", "fill"] as const).map((m) => (
            <button
              key={m}
              type="button"
              aria-pressed={mode === m}
              onClick={() => {
                setMode(m);
              }}
            >
              {t(m === "fit" ? "design.playFit" : "design.playFill")}
            </button>
          ))}
        </div>
        <button ref={close} type="button" className="play-exit" aria-label={t("design.exitPlay")} title={t("design.exitPlay")} onClick={onExit}>
          <Close />
        </button>
      </div>
      <div className={`play-stage ${mode}`} ref={stage}>
        <div className="play-sizer" style={{ width: frame.w * scale, height: frame.h * scale }}>
          <iframe
            ref={tokenFrame}
            src={src}
            title={frame.name}
            sandbox="allow-scripts"
            style={{ width: frame.w, height: frame.h, transform: `scale(${String(scale)})` }}
          />
        </div>
      </div>
    </div>
  );
}
