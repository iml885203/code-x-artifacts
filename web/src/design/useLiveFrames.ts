import { useCallback, useEffect, useRef, useState } from "react";
import type { DesignEdits } from "./edits";
import { needsLoading, shapeOf, type FrameShape } from "./frameShape";

interface Source {
  /** What the frame's iframe is keyed by: a new key loads the frame again. */
  key: string;
  src: string;
}

interface Loaded {
  generation: number;
  src: string;
  /** It was loaded before the room arrived, so what the room changed in between is not known to be in it. */
  beforeRoom: boolean;
}

/**
 * Where each frame of someone in the room is loaded from, and when it is loaded
 * again. The frames come from the live address (the room as it is now) and stay
 * loaded while versions are saved, so what a person is typing or has selected
 * survives a version the room already showed them. A frame is loaded again,
 * from a freshly read live address, only when the room changes something of it
 * the open frame cannot show (frameShape.ts), and once when the room arrives
 * after the frame was loaded. `live` is "" for someone who is not in the room:
 * they get no sources.
 */
export function useLiveFrames(edits: DesignEdits | null, live: string, files: string[], readLive: () => Promise<string>): ((file: string) => Source) | null {
  const [loaded, setLoaded] = useState<ReadonlyMap<string, Loaded>>(new Map());
  const shapes = useRef(new Map<string, FrameShape>());
  const latest = useRef({ files, readLive, loaded });
  useEffect(() => {
    latest.current = { files, readLive, loaded };
  });
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const shown = files.join("\n");
  const inRoom = edits !== null;

  // A frame keeps the address it was first loaded from; one that leaves the page is loaded afresh when it comes back.
  const kept = keptFor(loaded, shown, live, inRoom);
  if (kept !== loaded) setLoaded(kept);
  useEffect(() => {
    const now = new Set(shown === "" ? [] : shown.split("\n"));
    for (const file of [...shapes.current.keys()]) if (!now.has(file)) shapes.current.delete(file);
  }, [shown]);

  useEffect(() => {
    if (!edits || live === "") return;
    const check = () => {
      const stale: string[] = [];
      for (const file of latest.current.files) {
        const shape = shapeOf(edits.map, file);
        const before = shapes.current.get(file);
        shapes.current.set(file, shape);
        if (before ? needsLoading(before, shape) : latest.current.loaded.get(file)?.beforeRoom === true) stale.push(file);
      }
      if (stale.length === 0) return;
      latest.current.readLive().then(
        (address) => {
          // Without a live address (the person just lost the design) the frame stays as it is; the canvas hears of the change itself.
          if (!mounted.current || address === "") return;
          setLoaded((prev) => {
            const next = new Map(prev);
            for (const file of stale) next.set(file, { generation: (prev.get(file)?.generation ?? 0) + 1, src: address + file, beforeRoom: false });
            return next;
          });
        },
        (err: unknown) => {
          console.error("read the address of the design's live frames", err);
        },
      );
    };
    check();
    return edits.onChange(check);
  }, [edits, live, shown]);

  const source = useCallback(
    (file: string): Source => {
      const at = kept.get(file);
      return { key: `${file}#${String(at?.generation ?? 0)}`, src: at?.src ?? live + file };
    },
    [kept, live],
  );
  return live === "" ? null : source;
}

/** The frames shown, each with the address it was first loaded from: `loaded` itself when nothing came or went. */
function keptFor(loaded: ReadonlyMap<string, Loaded>, shown: string, live: string, inRoom: boolean): ReadonlyMap<string, Loaded> {
  if (live === "") return loaded;
  const now = new Set(shown === "" ? [] : shown.split("\n"));
  const missing = [...now].filter((file) => !loaded.has(file));
  if (missing.length === 0 && [...loaded.keys()].every((file) => now.has(file))) return loaded;
  const next = new Map([...loaded].filter(([file]) => now.has(file)));
  for (const file of missing) next.set(file, { generation: 0, src: live + file, beforeRoom: !inRoom });
  return next;
}
