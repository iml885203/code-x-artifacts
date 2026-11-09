import { useEffect, useRef } from "react";
import type { Design } from "../manifest";
import type { View } from "../view";
import { readAnchor } from "./anchor";
import type { DesignComments } from "./types";
import type { CommentPin } from "./useCommentPins";

/** The panel can reveal a thread on another page, without changing the source selection. */
export function useCommentFocus(comments: DesignComments | undefined, design: Design | null, pins: CommentPin[], onPage: (id: string) => void, setView: (change: (view: View) => View) => void, size: () => { w: number; h: number }) {
  const revealed = useRef<string | null>(null);
  const active = comments?.active ?? null;
  const thread = comments?.threads.find((item) => item.id === active);
  const anchor = readAnchor(thread?.anchor);
  const targetPage = design?.pages.find((page) => page.rows.flat().some((frame) => frame.id === anchor?.frameId))?.id;
  useEffect(() => {
    if (active && targetPage) onPage(targetPage);
  }, [active, targetPage, onPage]);
  const pin = pins.find((item) => item.id === active);
  useEffect(() => {
    if (!active) { revealed.current = null; return; }
    if (!pin || revealed.current === active) return;
    revealed.current = active;
    const { w, h } = size();
    setView((view) => ({ ...view, x: w / 2 - pin.x * view.z, y: h / 2 - pin.y * view.z }));
  }, [active, pin, setView, size]);
}
