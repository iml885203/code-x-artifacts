import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../../i18n";
import type { DesignEdits } from "../edits";
import { isFrameGone, type FrameLinks } from "../inspect";
import { textsOf } from "./content";
import { TextEditing } from "./editing";
import "./text.css";

interface Options {
  edits: DesignEdits | null;
  /** Whether the person may change the design: only then do they get to type into it. */
  editable: boolean;
  links: FrameLinks;
  frames: { id: string; file: string }[];
  /** Called once the frames have changed what they show, for whatever depends on where things are. */
  shown: () => void;
}

/**
 * Keeps each frame showing the text the room holds (the frame's file is only where
 * the room began, and other people type), and for someone who may edit, lets them
 * double-click text and type into it.
 */
export function useTextEditing({ edits, editable, links, frames, shown }: Options) {
  const t = useT();
  const editing = useMemo(() => (edits && editable ? new TextEditing(edits, links) : null), [edits, editable, links]);
  const [active, setActive] = useState<{ frameId: string; pid: string } | null>(null);
  const [notice, setNotice] = useState("");
  const latest = useRef({ frames, shown });
  useEffect(() => {
    latest.current = { frames, shown };
  });

  useEffect(() => {
    if (!edits) return;
    const sent = new Map<string, Map<string, string>>();
    // What a frame is told is the text that differs from what it was last told; a frame that has just said it is ready is told all.
    const push = async (frameId: string): Promise<boolean> => {
      const frame = latest.current.frames.find((f) => f.id === frameId);
      if (!frame) return false;
      const now = textsOf(edits.map, frame.file, edits.tree(frame.file));
      const before = sent.get(frameId);
      const changes = [...now].filter(([pid, html]) => before?.get(pid) !== html);
      sent.set(frameId, now);
      if (changes.length === 0) return false;
      await links.showTexts(frameId, changes);
      return true;
    };
    const pushAll = () => {
      Promise.all(latest.current.frames.map((f) => push(f.id))).then(
        (told) => {
          if (told.some(Boolean)) latest.current.shown();
        },
        (err: unknown) => {
          if (!isFrameGone(err)) console.error("show the text of the room in its frames", err);
        },
      );
    };
    pushAll();
    const offChange = edits.onChange(pushAll);
    const offReady = links.onReady((frameId) => {
      sent.delete(frameId);
      pushAll();
    });
    // The frame ignored what it was told about the element while the person typed in it: tell it again.
    const offEnded = editing?.onEnded((frameId, pid) => {
      sent.get(frameId)?.delete(pid);
      setActive(null);
      pushAll();
    });
    return () => {
      offChange();
      offReady();
      offEnded?.();
    };
  }, [edits, links, editing]);

  useEffect(
    () => () => {
      editing?.destroy();
    },
    [editing],
  );

  /** Starts typing into the text at a point of a frame (the frame's own pixels). */
  const begin = useCallback(
    async (frameId: string, file: string, x: number, y: number) => {
      if (!editing) return;
      setNotice("");
      const result = await editing.beginAt(frameId, file, x, y).catch((err: unknown) => {
        if (!isFrameGone(err)) console.error("start typing into a frame", err);
        return "gone" as const;
      });
      if (result === "not text") setNotice(t("textedit.cannot"));
      else if (result === "started") setActive(editing.editing);
    },
    [editing, t],
  );

  return { active, notice, begin: editing ? begin : null };
}
