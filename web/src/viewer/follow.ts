import { useEffect, useEffectEvent } from "react";
import { ApiError, getChanges, SignedOutError, type Changes } from "../api";

/** How often an open viewer asks whether a newer version exists. */
export const pollEveryMs = 10_000;

/**
 * Keeps asking the server what changed since the version the viewer shows, for
 * as long as `active`: not while the tab is hidden (it asks at once when the tab
 * is shown again) and not once the artifact is gone or the session ended.
 * `onChanges` hears each answer; it decides what is news. One request is in
 * flight at a time, and the next is asked only after the last was answered.
 */
export function useFollowChanges(id: string, active: boolean, shown: () => number, onChanges: (c: Changes) => void) {
  const version = useEffectEvent(shown);
  const hear = useEffectEvent(onChanges);
  useEffect(() => {
    if (!active) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    let failing = false;
    let asking = false;
    let finished = false;
    const controller = new AbortController();

    const ask = async () => {
      timer = undefined;
      asking = true;
      try {
        const changes = await getChanges(id, version(), controller.signal);
        failing = false;
        if (!stopped) hear(changes);
      } catch (err) {
        if (stopped) return;
        // Gone or signed out: there is nothing more to learn by asking.
        if (err instanceof SignedOutError || (err instanceof ApiError && (err.status === 404 || err.status === 401))) {
          finished = true;
          return;
        }
        // A failed answer is tried again on the next beat; say so once per run of failures.
        if (!failing) console.error("ask what changed", err);
        failing = true;
      } finally {
        asking = false;
      }
      if (!stopped && !document.hidden) timer = setTimeout(() => void ask(), pollEveryMs);
    };
    const onVisibility = () => {
      if (document.hidden) {
        clearTimeout(timer);
        timer = undefined;
      } else if (timer === undefined && !asking && !finished) {
        void ask();
      }
    };

    document.addEventListener("visibilitychange", onVisibility);
    if (!document.hidden) timer = setTimeout(() => void ask(), pollEveryMs);
    return () => {
      stopped = true;
      controller.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [id, active]);
}
