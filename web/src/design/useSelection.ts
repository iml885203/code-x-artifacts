import { useCallback, useRef, useState } from "react";
import { isFrameGone, type ElementReport, type FrameLinks } from "./inspect";

export interface Selected {
  frameId: string;
  pid: string;
}

const same = (a: Selected, b: Selected) => a.frameId === b.frameId && a.pid === b.pid;
export const keyOf = (s: Selected) => `${s.frameId}/${s.pid}`;

/**
 * What is selected on the canvas and what the frames said about it. A click is
 * a question to the frame under the pointer (what is here?) and then another
 * (what is it?); the answers arrive later, and an answer to a click that a newer
 * click has overtaken is dropped. With `isElement` (the room says what a frame's
 * elements are) a click selects the nearest element out from the point that the
 * room holds, not an inline element of formatted text, which is part of its text
 * and cannot be changed as an element of its own.
 */
export function useSelection(links: FrameLinks, isElement?: (frameId: string, pid: string) => boolean) {
  const [selectedFrame, setFrame] = useState<string | null>(null);
  const [selected, setShown] = useState<Selected[]>([]);
  const current = useRef<Selected[]>([]);
  const setSelected = useCallback((next: Selected[]) => {
    current.current = next;
    setShown(next);
  }, []);
  const [reports, setReports] = useState<Record<string, ElementReport>>({});
  const latest = useRef(0);

  const clear = useCallback(() => {
    latest.current++;
    setFrame(null);
    setSelected([]);
    setReports({});
  }, [setSelected]);

  const chooseFrame = useCallback((id: string) => {
    clear();
    setFrame(id);
  }, [clear]);

  const refresh = useCallback(
    async (items: Selected[], turn: number) => {
      const next: Record<string, ElementReport> = {};
      const frames = [...new Set(items.map((s) => s.frameId))];
      await Promise.all(
        frames.map(async (frameId) => {
          const pids = items.filter((s) => s.frameId === frameId).map((s) => s.pid);
          for (const r of await links.report(frameId, pids)) next[keyOf({ frameId, pid: r.pid })] = r;
        }),
      );
      if (turn !== latest.current) return;
      setReports(next);
      // An element the frame no longer has (someone deleted it) is no longer selected.
      const kept = items.filter((s) => keyOf(s) in next);
      if (kept.length !== items.length) setSelected(kept);
    },
    [links, setSelected],
  );

  /** Asks the frames about what is selected again, for when the elements changed (moved, resized, undone, deleted by someone). */
  const rerun = useCallback(async () => {
    if (current.current.length === 0) return;
    try {
      await refresh(current.current, latest.current);
    } catch (err) {
      if (!isFrameGone(err)) console.error("ask a frame about the selected elements", err);
    }
  }, [refresh]);

  /** Selects an element by what it is (the outline's rows); with `additive` it joins the selection or leaves it. */
  const choose = useCallback(
    async (hit: Selected, additive: boolean, current: Selected[]) => {
      setFrame(null);
      const turn = ++latest.current;
      const next = !additive ? [hit] : current.some((s) => same(s, hit)) ? current.filter((s) => !same(s, hit)) : [...current, hit];
      setSelected(next);
      if (next.length === 0) {
        setReports({});
        return;
      }
      try {
        await refresh(next, turn);
      } catch (err) {
        if (!isFrameGone(err)) console.error("ask a frame about the selected elements", err);
      }
    },
    [refresh, setSelected],
  );

  /** A click at a point of a frame (in the frame's own pixels); with `additive` the element joins the selection or leaves it. */
  const pick = useCallback(
    async (frameId: string, x: number, y: number, additive: boolean, current: Selected[]) => {
      setFrame(null);
      const turn = ++latest.current;
      let pid: string | null;
      try {
        pid = isElement ? ((await links.chain(frameId, x, y)).find((p) => isElement(frameId, p)) ?? null) : await links.hit(frameId, x, y);
      } catch (err) {
        if (!isFrameGone(err)) console.error("ask a frame what is under the pointer", err);
        return;
      }
      if (turn !== latest.current) return;
      if (pid === null) {
        if (!additive) clear();
        return;
      }
      await choose({ frameId, pid }, additive, current);
    },
    [links, isElement, clear, choose],
  );

  return { selected, selectedFrame, chooseFrame, reports, pick, choose, clear, rerun };
}
