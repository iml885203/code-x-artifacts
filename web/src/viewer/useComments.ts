import { useCallback, useEffect, useRef, useState } from "react";
import { listComments, type Comment } from "../api";

export type CommentLoad = { status: "loading" } | { status: "failed" } | { status: "ready"; list: Comment[] };

/** The artifact's comments, held by the viewer because two things draw them: the panel, and the words marked in the doc editor. */
export interface Comments {
  load: CommentLoad;
  /** Reads the comments again; the list it read, or null when it could not. A list already shown stays when the read fails. */
  refresh: () => Promise<Comment[] | null>;
  /** A comment this person just sent. */
  add: (saved: Comment) => void;
  remove: (id: string) => void;
}

/**
 * Reads the comments while `marked` is true (the doc editor is open and draws them) and again whenever how many there are
 * changes, so a comment someone else makes shows up as a mark. The panel reads them itself each time it opens.
 */
export function useComments(id: string, marked: boolean, count: number): Comments {
  const [load, setLoad] = useState<CommentLoad>({ status: "loading" });
  // How many comments the list on screen holds, and which read is the latest: an older one that answers late is dropped.
  const held = useRef(-1);
  const latest = useRef(0);
  const shown = useRef(false);

  const refresh = useCallback(async () => {
    const mine = ++latest.current;
    try {
      const list = await listComments(id);
      if (mine === latest.current) {
        held.current = list.filter((c) => !c.deleted).length;
        shown.current = true;
        setLoad({ status: "ready", list });
      }
      return list;
    } catch (err) {
      console.error("list comments", err);
      if (mine === latest.current && !shown.current) setLoad({ status: "failed" });
      return null;
    }
  }, [id]);

  useEffect(() => {
    if (marked && held.current !== count) void refresh();
  }, [marked, count, refresh]);

  const add = useCallback((saved: Comment) => {
    held.current += 1;
    setLoad((was) => (was.status === "ready" ? { status: "ready", list: [...was.list, saved] } : was));
  }, []);

  const remove = useCallback((id: string) => {
    // A read started before this deletion must not bring the removed body back.
    latest.current += 1;
    setLoad((was) => {
      if (was.status !== "ready") return was;
      const scrubbed = was.list.map((c) => c.id === id ? { ...c, body: "", deleted: true, canDelete: false, unread: false } : c);
      const list = scrubbed.filter((c) => !c.deleted || !c.parentId && scrubbed.some((r) => r.parentId === c.id && !r.deleted));
      held.current = list.filter((c) => !c.deleted).length;
      return { status: "ready", list };
    });
  }, []);

  return { load, refresh, add, remove };
}
