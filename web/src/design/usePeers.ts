import { useCallback, useEffect, useRef, useState } from "react";
import type { Live } from "./useRoom";
import { isFrameGone, type FrameLinks } from "./inspect";
import { peersOf, type Peer } from "./presence";
import { useAgentMarks } from "./useAgentMarks";

/** Another person's selected element, where it is on the canvas's frame. */
export interface PeerBox {
  key: string;
  name: string;
  color: unknown;
  frameId: string;
  /** What an agent changed, not what a person selected: the box fades. */
  agent: boolean;
  rect: { x: number; y: number; w: number; h: number };
}

/** The other people in the room and what they have selected, and what agents changed a moment ago, kept up to date with awareness. */
export function usePeers(live: Live | null): Peer[] {
  const [peers, setPeers] = useState<Peer[]>([]);
  const awareness = live?.awareness;
  const self = live?.self;
  useEffect(() => {
    if (!awareness || self === undefined) return;
    const read = () => {
      setPeers(peersOf(awareness, self));
    };
    read();
    awareness.on("change", read);
    return () => {
      awareness.off("change", read);
    };
  }, [awareness, self]);
  const marks = useAgentMarks(live);
  return awareness ? [...peers, ...marks] : [];
}

/**
 * Where each other person's selected elements are, asked of the frames. Again
 * whenever someone's selection changes and whenever `refresh` is called (the
 * elements moved). A selected element a frame no longer has, or has hidden, has
 * no box. Only the frames on the page shown are asked.
 */
export function usePeerBoxes(links: FrameLinks, peers: Peer[], frameIds: string[]): { boxes: PeerBox[]; refresh: () => void } {
  const [boxes, setBoxes] = useState<PeerBox[]>([]);
  const latest = useRef({ peers, frameIds });
  const turn = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    latest.current = { peers, frameIds };
  });

  const refresh = useCallback(() => {
    const mine = ++turn.current;
    const { peers: all, frameIds: shown } = latest.current;
    const wanted = new Map<string, Set<string>>();
    for (const p of all) for (const s of p.selection) if (shown.includes(s.frameId)) (wanted.get(s.frameId) ?? wanted.set(s.frameId, new Set()).get(s.frameId))?.add(s.pid);
    Promise.all(
      [...wanted].map(async ([frameId, pids]) => {
        const items = await links.report(frameId, [...pids]);
        return [frameId, new Map(items.map((r) => [r.pid, r.rect]))] as const;
      }),
    ).then(
      (found) => {
        if (mine !== turn.current || !mounted.current) return;
        const rects = new Map(found);
        const next: PeerBox[] = [];
        for (const p of all)
          for (const s of p.selection) {
            const rect = rects.get(s.frameId)?.get(s.pid);
            if (rect) next.push({ key: `${String(p.client)}/${String(p.at ?? "")}/${s.frameId}/${s.pid}`, name: p.name, color: p.color, frameId: s.frameId, agent: p.agent, rect });
          }
        setBoxes(next);
      },
      (err: unknown) => {
        if (mounted.current && !isFrameGone(err)) console.error("ask a frame where the others' selections are", err);
      },
    );
  }, [links]);

  const key = JSON.stringify([peers.map((p) => [p.client, p.at, p.selection]), frameIds]);
  useEffect(() => {
    refresh();
  }, [key, refresh]);
  return { boxes, refresh };
}
