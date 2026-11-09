// The two pages that lead into each other, the overview and the viewer, are
// one app: opening an artifact from the overview does not reload anything, so
// the thumbnail can grow into the viewer (a View Transition) and the overview
// is where it was left on the way back. Every other page, and every direct
// visit, is a normal navigation that the server answers with the shell.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { flushSync } from "react-dom";

export interface Router {
  /** The path being shown, with its query. */
  path: string;
  /** Show another page of the app, moving the shared artifact between the two views when the browser can. */
  go: (to: string) => void;
}

const Context = createContext<Router | null>(null);

const viewerPath = /^\/a\/([^/?#]+)/;

/** The artifact id a path names, or "". */
export function artifactIdOf(path: string): string {
  const m = viewerPath.exec(path);
  return m?.[1] ? decodeURIComponent(m[1]) : "";
}

/** Longest the new page may keep the transition waiting for the artifact's first paint. */
const FRAME_WAIT_MS = 300;

function thumbOf(id: string): Element | null {
  if (!id) return null;
  for (const card of document.querySelectorAll("[data-artifact-id]")) {
    if (card.getAttribute("data-artifact-id") === id) return card.querySelector("[data-thumb]");
  }
  return null;
}

/** Resolves when the viewer's iframe has loaded, or after FRAME_WAIT_MS: a transition that ends on a blank white box is worse than a short wait. */
function frameLoaded(): Promise<void> {
  const frame = document.querySelector<HTMLIFrameElement>("iframe[data-artifact-frame]");
  if (!frame) return Promise.resolve();
  return new Promise((resolve) => {
    const done = () => {
      window.clearTimeout(timer);
      frame.removeEventListener("load", done);
      resolve();
    };
    const timer = window.setTimeout(done, FRAME_WAIT_MS);
    frame.addEventListener("load", done);
  });
}

/** A transition nobody waits on: the browser rejects its promises too when it aborts it. */
function quiet(transition: ViewTransition): void {
  transition.ready.catch(() => undefined);
  transition.updateCallbackDone.catch(() => undefined);
  transition.finished.catch(() => undefined);
}

export function RouterProvider({ initial, children }: { initial: string; children: ReactNode }) {
  const [path, setPath] = useState(initial);
  const current = useRef(initial);
  const scrolls = useRef(new Map<string, number>());

  const show = useCallback((to: string, push: boolean) => {
    const from = current.current;
    scrolls.current.set(from, window.scrollY);
    // The artifact that travels is the one the viewer shows, whichever side it is on.
    const id = artifactIdOf(to) || artifactIdOf(from);
    const apply = async () => {
      if (push) history.pushState(null, "", to);
      current.current = to;
      flushSync(() => {
        setPath(to);
      });
      window.scrollTo(0, scrolls.current.get(to) ?? 0);
      thumbOf(id)?.setAttribute("data-vt", "artifact");
      await frameLoaded();
    };
    if (typeof document.startViewTransition !== "function") {
      void apply();
      return;
    }
    thumbOf(id)?.setAttribute("data-vt", "artifact");
    const transition = document.startViewTransition(apply);
    const clear = () => {
      for (const el of document.querySelectorAll("[data-vt]")) el.removeAttribute("data-vt");
    };
    transition.finished.then(clear, clear);
    // A browser aborts a transition when the page is hidden or replaced, and rejects these two as well; nobody waits on them.
    quiet(transition);
  }, []);

  useEffect(() => {
    const onPop = () => {
      show(location.pathname + location.search, false);
    };
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("popstate", onPop);
    };
  }, [show]);

  const router = useMemo<Router>(
    () => ({
      path,
      go: (to) => {
        if (to !== current.current) show(to, true);
      },
    }),
    [path, show],
  );
  return <Context value={router}>{children}</Context>;
}

export function useRouter(): Router {
  const r = useContext(Context);
  if (!r) throw new Error("useRouter outside RouterProvider");
  return r;
}

/** A link that stays in the app unless the person means a new tab or window. */
export function inApp(router: Router, to: string) {
  return (e: React.MouseEvent) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    router.go(to);
  };
}
