import { useEffect, useState } from "react";
import type { DesignEdits } from "./edits";
import { FrameLinks, isFrameGone } from "./inspect";
import { readTokens } from "./tokens";

/** A standalone Play frame shares the theme without acquiring editor controls. */
export function useTokenFrame(edits: DesignEdits | null, frameId: string) {
  const [links] = useState(() => new FrameLinks());
  useEffect(() => {
    const hear = (event: MessageEvent) => { links.hear(event); };
    window.addEventListener("message", hear);
    return () => {
      window.removeEventListener("message", hear);
      links.close();
    };
  }, [links]);
  useEffect(() => {
    if (!edits) return;
    let current = true;
    const push = () => {
      links.tokens(frameId, readTokens(edits)).catch((err: unknown) => {
        if (current && !isFrameGone(err)) console.error("show shared tokens in Play", err);
      });
    };
    const off = edits.onChange(push);
    const offReady = links.onReady(push);
    push();
    return () => {
      current = false;
      off();
      offReady();
    };
  }, [edits, frameId, links]);
  return links.refFor(frameId);
}
