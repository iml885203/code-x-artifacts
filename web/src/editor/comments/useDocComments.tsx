// The comments of a doc, as the editor shows them: the words that have one are marked (marks.ts), Comment turns the
// selection into the place a new comment points at (the button in the status bar, and the one in the bubble over the selection,
// FormatBubble.tsx), and the editor tells the page whether each thread's words are still there. The
// threads themselves live in the viewer's panel; this is only the editor's side of it.
import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import type { EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import type * as Y from "yjs";
import { useT } from "../../i18n";
import { anchorOfSelection, readAnchor, standing, type Standing } from "./anchor";
import { redraw, type Marking, type Thread } from "./marks";
import "./comments.css";

/** What the page around the editor gives it to work with, when the viewer may comment. */
export interface DocComments {
  /** The comments that start a thread, with the anchor each was written with. */
  threads: { id: string; anchor: Record<string, unknown> | undefined }[];
  /** The thread open in the panel. */
  active: string | null;
  /** A mark was clicked. */
  onOpen: (id: string) => void;
  /** The person chose Comment with words selected: the place a new comment would point at. */
  onCompose: (anchor: Record<string, unknown>) => void;
  /** Whether each thread's words are still in the doc. */
  onStandings: (standings: Record<string, Standing>) => void;
}

/** The threads that point into a doc. */
export function docThreads(comments: DocComments | undefined): Thread[] {
  return (comments?.threads ?? []).flatMap(({ id, anchor }) => {
    const read = readAnchor(anchor);
    return read === null ? [] : [{ id, anchor: read }];
  });
}

export function useDocComments(view: RefObject<EditorView | null>, doc: Y.Doc, synced: boolean, state: EditorState, comments: DocComments | undefined) {
  const t = useT();
  // The latest comments, so the plugin made with the view reads the current ones.
  const latest = useRef(comments);
  useEffect(() => {
    latest.current = comments;
  });
  const compose = useRef<() => void>(() => undefined);
  const [marking] = useState<Marking>(() => ({
    threads: () => docThreads(latest.current),
    active: () => latest.current?.active ?? null,
    open: (id) => {
      latest.current?.onOpen(id);
    },
    compose: () => {
      compose.current();
    },
  }));

  const threads = comments?.threads;
  const active = comments?.active;
  useEffect(() => {
    const v = view.current;
    if (v) v.dispatch(v.state.tr.setMeta(redraw, true));
  }, [view, threads, active]);

  // Whether the words are there is the room's answer, for every tab, so it is asked of the room and not of the tab on screen.
  const told = useRef("");
  useEffect(() => {
    if (!synced || threads === undefined) return;
    const report = () => {
      const now: Record<string, Standing> = {};
      for (const { id, anchor } of docThreads(latest.current)) now[id] = standing(doc, anchor);
      const key = JSON.stringify(now);
      if (key === told.current) return;
      told.current = key;
      latest.current?.onStandings(now);
    };
    report();
    doc.on("update", report);
    return () => {
      doc.off("update", report);
    };
  }, [doc, synced, threads]);

  const anchored = comments !== undefined && anchorOfSelection(state) !== null;
  const start = () => {
    const v = view.current;
    const anchor = v ? anchorOfSelection(v.state) : null;
    if (anchor !== null && latest.current) latest.current.onCompose({ ...anchor });
  };
  useEffect(() => {
    compose.current = start;
  });

  const label = t("anchors.commentOnSelection");
  const mouseDownKeepsSelection = (e: { preventDefault: () => void }) => {
    e.preventDefault();
  };
  const button: ReactNode =
    comments === undefined ? null : (
      <button type="button" className="quiet ed-comment-button" aria-label={label} disabled={!anchored} onMouseDown={mouseDownKeepsSelection} onClick={start}>
        {t("anchors.comment")}
      </button>
    );
  return { marking, button, start, anchored };
}
