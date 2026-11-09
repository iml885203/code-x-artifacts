import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { FrameLinks } from "../inspect";
import type { DesignEdits } from "../edits";
import type { DesignFrame, Placed } from "../manifest";
import { readAnchor, resolveAnchor } from "./anchor";
import type { CommentTarget, DesignComments } from "./types";

export interface CommentPin {
  id: string;
  frameId: string;
  name: string;
  number: number;
  x: number;
  y: number;
  fallback: boolean;
}

/** Report only the current page; old responses cannot move pins after page/version/source changes. */
export function useCommentPins(threads: DesignComments["threads"], frames: Placed[], links: FrameLinks, source?: { edits: DesignEdits | null; frames: DesignFrame[] }) {
  const [pins, setPins] = useState<CommentPin[]>([]);
  const [targets, setTargets] = useState<Record<string, CommentTarget>>({});
  const current = useRef({ threads, frames, source });
  const turn = useRef(0);
  useEffect(() => { current.current = { threads, frames, source }; });
  const refresh = useCallback(() => {
    const mine = ++turn.current;
    const { threads: all, frames: placed, source: room } = current.current;
    const nextTargets = sourceTargetsOf(all, room);
    void Promise.all(placed.map(async (at) => {
      const wanted = all.flatMap((thread, index) => {
        const anchor = readAnchor(thread.anchor);
        return anchor?.frameId === at.frame.id && nextTargets[thread.id] !== "removed" ? [{ id: thread.id, number: index + 1, anchor }] : [];
      });
      const pids = [...new Set(wanted.flatMap(({ anchor }) => anchor.pid ? [anchor.pid] : []))];
      const reports = links.has(at.frame.id) !== null && pids.length > 0 ? await links.report(at.frame.id, pids).catch(() => []) : [];
      return wanted.flatMap(({ id, number, anchor }) => {
        const rect = reports.find((r) => r.pid === anchor.pid)?.rect;
        const point = resolveAnchor(anchor, at.frame, rect);
        if (point) nextTargets[id] = "available";
        return point ? [{ ...point, id, number, frameId: at.frame.id, name: anchor.name ?? at.frame.name, x: at.x + point.x, y: at.y + point.y }] : [];
      });
    })).then((next) => { if (mine === turn.current) { setPins(next.flat()); setTargets({ ...nextTargets }); } });
  }, [links]);
  const key = JSON.stringify([threads, frames, source?.frames]);
  useEffect(() => {
    refresh();
    return () => { turn.current += 1; };
  }, [key, source?.edits, refresh]);
  useEffect(() => links.onReady(refresh), [links, refresh]);
  useEffect(() => source?.edits?.onChange(refresh), [source?.edits, refresh]);
  const subscribe = useCallback((notify: () => void) => source?.edits?.onChange(notify) ?? (() => undefined), [source?.edits]);
  const snapshot = () => JSON.stringify(sourceTargetsOf(threads, source));
  const identities = JSON.parse(useSyncExternalStore(subscribe, snapshot)) as Record<string, CommentTarget>;
  return {
    pins: pins.filter((pin) => identities[pin.id] !== "removed"),
    refresh,
    targets: Object.fromEntries(Object.entries(identities).map(([id, state]) => [id, state === "removed" ? state : targets[id] === "available" ? "available" : "unavailable"])),
  };
}

/** Only a populated room can confirm deletion, never inspector geometry/readiness. */
function sourceTargetsOf(threads: DesignComments["threads"], room?: { edits: DesignEdits | null; frames: DesignFrame[] }): Record<string, CommentTarget> {
  const targets: Record<string, CommentTarget> = {};
  for (const thread of threads) {
    const anchor = readAnchor(thread.anchor);
    if (!anchor) continue;
    const frame = room?.frames.find((f) => f.id === anchor.frameId);
    const state = frame && room?.edits?.frame(frame.file);
    // An inline element of formatted text is part of that text, not an element of the room, and is there while the text is.
    const removed = room?.edits && (!frame || state && state.root !== "" && anchor.pid && !state.alive.has(anchor.pid) && !state.inText.has(anchor.pid));
    targets[thread.id] = removed ? "removed" : "unavailable";
  }
  return targets;
}
