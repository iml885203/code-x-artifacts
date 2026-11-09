import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "../api";
import { format, useT } from "../i18n";
import { useToast } from "../Toast";
import { deleteSlide, getSlide, insertSlide, listSlides, moveSlide, type SlideList } from "./api";
import { clampSlide } from "./order";

/**
 * A change to the deck's structure, by slide number. A delete carries the version the person was asked about,
 * so a deck that changed while they were asked is refused (409) rather than losing whatever slide now has the number.
 */
export type DeckEdit =
  | { op: "add"; after: number }
  | { op: "duplicate"; n: number }
  | { op: "move"; n: number; to: number }
  | { op: "delete"; n: number; base: number };

export interface Deck {
  /** The slides the server last answered with; null until the first answer. */
  list: SlideList | null;
  /** The slides could not be read; `retry` reads them again. */
  failed: boolean;
  retry: () => void;
  selected: number;
  /** Selects a slide, kept inside the deck. */
  select: (n: number) => void;
  busy: boolean;
  /**
   * Saves an edit, one at a time: another asked for meanwhile is not sent and the person is told to wait. A stale
   * version (409) shows the newer deck and says so; any other refusal rejects.
   */
  edit: (e: DeckEdit) => Promise<void>;
  /** Shows a list another change saved or found (a slide's content edited in place), keeping the selection. */
  adopt: (fresh: SlideList) => void;
}

function isList(v: unknown): v is SlideList {
  const l = v as Partial<SlideList> | null;
  return typeof l?.version === "number" && Array.isArray(l.slides);
}

async function readList(id: string): Promise<SlideList> {
  const list = await listSlides(id);
  if (!isList(list)) throw new Error(`the slide list of ${id} is not a list`);
  return list;
}

/** What an edit sends, and the slide to select once it is saved. */
async function send(id: string, e: DeckEdit, base: number, newTitle: string): Promise<{ saved: SlideList; next: number }> {
  switch (e.op) {
    case "add":
      return { saved: await insertSlide(id, e.after, { layout: "title", title: newTitle }, base), next: e.after + 1 };
    case "duplicate": {
      const read = await getSlide(id, e.n);
      return { saved: await insertSlide(id, e.n, read.slide, base), next: e.n + 1 };
    }
    case "move":
      return { saved: await moveSlide(id, e.n, e.to, base), next: e.to };
    case "delete":
      return { saved: await deleteSlide(id, e.n, e.base), next: e.n };
  }
}

/**
 * The deck a filmstrip shows and edits. `version` is the artifact's version as the viewer holds it: when it moves
 * past the list (someone published), the list is read again. `onSaved` hears the version an edit saved, or the newer
 * one a refusal revealed, so the viewer shows that deck.
 */
export function useDeck(id: string, version: number, start: number, onSaved: (version: number) => void): Deck {
  const t = useT();
  const toast = useToast();
  const [list, setList] = useState<SlideList | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [selected, setSelected] = useState(start);
  const [busy, setBusy] = useState(false);
  // Read when an edit is asked for, which can come twice before a render shows the first as busy.
  const sending = useRef(false);
  const known = useRef(0);
  const count = list?.slides.length;

  useEffect(() => {
    if (known.current >= version) return;
    let current = true;
    readList(id).then(
      (fresh) => {
        if (!current) return;
        known.current = fresh.version;
        setList(fresh);
        setFailed(false);
        setSelected((n) => clampSlide(n, fresh.slides.length));
      },
      (err: unknown) => {
        console.error("read the slides of the deck", err);
        if (current) setFailed(true);
      },
    );
    return () => {
      current = false;
    };
  }, [id, version, attempt]);

  const select = useCallback((n: number) => {
    setSelected(count === undefined ? Math.max(1, n) : clampSlide(n, count));
  }, [count]);

  function show(fresh: SlideList, next: number) {
    known.current = fresh.version;
    setList(fresh);
    setSelected(clampSlide(next, fresh.slides.length));
    onSaved(fresh.version);
  }

  async function stale() {
    try {
      show(await readList(id), selected);
      toast({ message: t("deck.stale"), error: true });
    } catch (err) {
      console.error("read the newer slides of the deck", err);
      toast({ message: t("deck.staleUnread"), error: true });
    }
  }

  async function edit(e: DeckEdit): Promise<void> {
    if (!list) return;
    if (sending.current) {
      toast({ message: t("deck.busy") });
      return;
    }
    sending.current = true;
    setBusy(true);
    try {
      const { saved, next } = await send(id, e, list.version, t("deck.newTitle"));
      show({ version: saved.version, slides: saved.slides }, next);
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 409)) throw err;
      await stale();
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  function adopt(fresh: SlideList) {
    known.current = Math.max(known.current, fresh.version);
    setList((l) => (l && l.version > fresh.version ? l : fresh));
    setSelected((n) => clampSlide(n, fresh.slides.length));
    onSaved(fresh.version);
  }

  return { list, failed, retry: () => { setAttempt((n) => n + 1); }, selected, select, busy, edit, adopt };
}

/** What to tell the person when an edit was refused for a reason other than a newer version. */
export function refusal(t: ReturnType<typeof useT>, err: unknown): string {
  return err instanceof ApiError && err.reason !== "" ? format(t("deck.failed"), { reason: err.reason }) : t("deck.failedPlain");
}
