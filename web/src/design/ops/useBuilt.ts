import { useEffect, useRef } from "react";
import type { DesignEdits } from "../edits";
import type { FrameLinks } from "../inspect";
import { mountBuilt } from "./built";

/** Has each frame build the elements the room has and its file does not (see mountBuilt). Call it before useFramesShowRoom. */
export function useBuiltElements(edits: DesignEdits | null, links: FrameLinks, frames: { id: string; file: string }[]): void {
  const latest = useRef(frames);
  useEffect(() => {
    latest.current = frames;
  });
  useEffect(() => {
    if (!edits) return;
    return mountBuilt(edits, links, () => latest.current);
  }, [edits, links]);
}
