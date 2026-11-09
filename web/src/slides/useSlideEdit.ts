import { useEffect, useState } from "react";
import { ApiError } from "../api";
import { useT } from "../i18n";
import { useToast } from "../Toast";
import { getSlide, listSlides, replaceSlide, type SlideList } from "./api";
import { applyChange, textAt, type Change, type Slide } from "./fields";
import type { Deck } from "./useDeck";

/** The selected slide as read: its JSON and the version it was read at, the version an edit is saved on. */
interface Held {
  n: number;
  version: number;
  slide: Slide;
}

/** One saved change of this session: the slide before and after it. */
interface Step {
  n: number;
  before: Slide;
  after: Slide;
}

/** A change a newer version refused: kept so the person can put it on the newer slide. */
export interface Conflict {
  n: number;
  change: Change;
  /** The text the change replaced: it goes on the newer slide only where that text still is. Null for an insert. */
  from: string | null;
  /** The newer slide has no place for it. */
  gone: boolean;
}

/** What a save did: saved; refused by a newer version and kept as the conflict; or not sent, because nothing could be saved yet. */
export type Saved = "saved" | "kept" | "skipped";

export interface SlideEdit {
  /** The selected slide's JSON, or null while it is read. */
  slide: Slide | null;
  busy: boolean;
  /** Saves a change with the version the slide was read at. Any refusal other than a newer version rejects. */
  save: (change: Change) => Promise<Saved>;
  conflict: Conflict | null;
  applyConflict: () => Promise<void>;
  dropConflict: () => void;
  canUndo: boolean;
  canRedo: boolean;
  undo: () => Promise<void>;
  redo: () => Promise<void>;
}

const stale = (err: unknown) => err instanceof ApiError && err.status === 409;

/**
 * Editing what is on the selected slide. Every save is replace_slide with the whole slide and the version it was read
 * at; a newer version (409) shows the newer deck and keeps the change. Undo and redo walk the changes of this session,
 * and are offered only while the deck is at the version this session saved last: a change by anyone or anything else
 * (another person, an agent, the filmstrip) ends them.
 */
export function useSlideEdit(id: string, deck: Deck, active: boolean): SlideEdit {
  const t = useT();
  const toast = useToast();
  const n = deck.selected;
  const version = deck.list?.version ?? 0;
  const [held, setHeld] = useState<Held | null>(null);
  const [busy, setBusy] = useState(false);
  const [conflict, setConflict] = useState<Conflict | null>(null);
  const [past, setPast] = useState<readonly Step[]>([]);
  const [future, setFuture] = useState<readonly Step[]>([]);
  const [mine, setMine] = useState<number | null>(null);

  const current = held !== null && held.n === n && held.version >= version ? held : null;
  const walkable = !busy && mine === version;

  useEffect(() => {
    if (!active || version === 0 || current !== null) return;
    let live = true;
    getSlide(id, n).then(
      (read) => {
        if (live) setHeld({ n, version: read.version, slide: read.slide });
      },
      (err: unknown) => {
        console.error(`read slide ${String(n)} of the deck to edit it`, err);
      },
    );
    return () => {
      live = false;
    };
  }, [id, n, version, current, active]);

  async function put(at: number, slide: Slide, base: number): Promise<SlideList> {
    const saved = await replaceSlide(id, at, slide, base);
    setHeld({ n: at, version: saved.version, slide });
    setMine(saved.version);
    deck.adopt({ version: saved.version, slides: saved.slides });
    return saved;
  }

  /** Shows the newer deck after a refusal; if it cannot be read, says so, and what was kept stays kept. */
  async function newer(): Promise<void> {
    try {
      deck.adopt(await listSlides(id));
    } catch (err) {
      console.error("read the newer slides of the deck", err);
      toast({ message: t("deck.staleUnread"), error: true });
    }
  }

  async function guarded<T>(run: () => Promise<T>): Promise<T> {
    setBusy(true);
    try {
      return await run();
    } finally {
      setBusy(false);
    }
  }

  async function record(at: number, before: Slide, after: Slide, base: number): Promise<void> {
    await put(at, after, base);
    setPast((p) => [...p, { n: at, before, after }]);
    setFuture([]);
  }

  function save(change: Change): Promise<Saved> {
    const from = current;
    if (!from || busy) return Promise.resolve("skipped");
    const next = applyChange(from.slide, change);
    if (!next) return Promise.reject(new Error(`the change has no place on slide ${String(from.n)}`));
    const replaced = change.op === "text" ? textAt(from.slide, change.path) : null;
    return guarded(async (): Promise<Saved> => {
      try {
        await record(from.n, from.slide, next, from.version);
        return "saved";
      } catch (err) {
        if (!stale(err)) throw err;
        setConflict({ n: from.n, change, from: replaced, gone: false });
        await newer();
        return "kept";
      }
    });
  }

  function applyConflict(): Promise<void> {
    const c = conflict;
    if (!c || busy) return Promise.resolve();
    return guarded(async () => {
      const read = await getSlide(id, c.n).catch((err: unknown) => {
        console.error(`read slide ${String(c.n)} to apply the kept change`, err);
        return null;
      });
      const still = read !== null && (c.change.op !== "text" || textAt(read.slide, c.change.path) === c.from);
      const next = read && still ? applyChange(read.slide, c.change) : null;
      if (!read || !next) {
        setConflict({ ...c, gone: true });
        return;
      }
      try {
        await record(c.n, read.slide, next, read.version);
        setConflict(null);
      } catch (err) {
        if (!stale(err)) throw err;
        await newer();
      }
    });
  }

  async function travel(step: Step | undefined, slide: (s: Step) => Slide, done: () => void): Promise<void> {
    if (!step || !walkable) return;
    await guarded(async () => {
      try {
        await put(step.n, slide(step), version);
        deck.select(step.n);
        done();
      } catch (err) {
        if (!stale(err)) throw err;
        setPast([]);
        setFuture([]);
        toast({ message: t("deck.stale"), error: true });
        await newer();
      }
    });
  }

  const undo = () => {
    const step = past.at(-1);
    return travel(step, (s) => s.before, () => {
      setPast((p) => p.slice(0, -1));
      if (step) setFuture((f) => [...f, step]);
    });
  };
  const redo = () => {
    const step = future.at(-1);
    return travel(step, (s) => s.after, () => {
      setFuture((f) => f.slice(0, -1));
      if (step) setPast((p) => [...p, step]);
    });
  };

  return {
    slide: current?.slide ?? null,
    busy,
    save,
    conflict,
    applyConflict,
    dropConflict: () => { setConflict(null); },
    canUndo: walkable && past.length > 0,
    canRedo: walkable && future.length > 0,
    undo,
    redo,
  };
}
