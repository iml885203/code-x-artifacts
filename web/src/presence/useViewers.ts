import { useEffect, useState } from "react";
import { ApiError, call, SignedOutError } from "../api";
import { guestId, knownGuestId } from "./guest";
import type { Person } from "./people";

/** How often an open page tells the server it is still here, until the server says its own pace (`CodeXArtifacts-Heartbeat-Ms`); it forgets a person after three beats. */
export const heartbeatEveryMs = 20_000;

interface Entry {
  id?: string;
  name: string;
  agent: boolean;
  guest?: boolean;
  self: boolean;
}

/** What a beat says about this browser: signed out, the guest it is listed as; signed in, the guest it was, whose place the account takes. */
function beatBody(signedIn: boolean): { guest: string } | undefined {
  if (!signedIn) return { guest: guestId() };
  const was = knownGuestId();
  return was === undefined ? undefined : { guest: was };
}

/**
 * Tells the server this page has the artifact open every `heartbeatEveryMs`, and
 * hears who else does in the same answer. Nothing is sent while the tab is hidden
 * (the first thing it does when shown is a beat), and it stops for good once the
 * artifact is gone or the session ended. Nothing is sent while the browser is offline
 * either; the first beat after it is back is at once. A signed-out page is listed as its browser's guest and
 * hears only itself back.
 */
export function useViewers(id: string, signedIn = true): Person[] {
  const [people, setPeople] = useState<Person[]>([]);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    let finished = false;
    let beating = false;
    let every = heartbeatEveryMs;
    const controller = new AbortController();

    const beat = async () => {
      timer = undefined;
      // A browser that is offline has nobody to tell, and the failed request would be an error in the console:
      // the beat waits for "online" (resume below) instead.
      if (!navigator.onLine) return;
      beating = true;
      let retryIn: number | undefined;
      try {
        const res = await call("POST", `/api/artifacts/${encodeURIComponent(id)}/viewers`, beatBody(signedIn), controller.signal);
        const pace = Number(res.headers.get("CodeXArtifacts-Heartbeat-Ms"));
        if (pace > 0) every = pace;
        const list = (await res.json()) as Entry[] | null;
        if (!stopped) setPeople((Array.isArray(list) ? list : []).map((e) => ({ id: e.id ?? e.name, name: e.name, agent: e.agent, guest: e.guest === true, self: e.self })));
      } catch (err) {
        if (stopped) return;
        // 403 is a page whose session ended without a guest id to be listed under: signed out like 401.
        if (err instanceof SignedOutError || (err instanceof ApiError && (err.status === 404 || err.status === 401 || err.status === 403))) {
          finished = true;
          setPeople([]);
          return;
        }
        if (err instanceof ApiError && err.status === 429) {
          // The server lists as many guests as it may: not listed for now, and asked again when it said.
          setPeople([]);
          retryIn = err.retryAfter > 0 ? err.retryAfter * 1000 : undefined;
          return;
        }
        console.error("tell the server this page is open", err);
      } finally {
        beating = false;
        if (!stopped && !finished && !document.hidden) timer = setTimeout(() => void beat(), retryIn ?? every);
      }
    };
    const onVisibility = () => {
      if (document.hidden) {
        clearTimeout(timer);
        timer = undefined;
      } else if (timer === undefined && !beating && !finished) {
        void beat();
      }
    };

    const onOnline = () => {
      if (!document.hidden && timer === undefined && !beating && !finished) void beat();
    };

    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("online", onOnline);
    if (!document.hidden) void beat();
    return () => {
      stopped = true;
      controller.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", onOnline);
    };
  }, [id, signedIn]);
  return people;
}
