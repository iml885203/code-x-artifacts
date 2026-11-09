import { flushSync } from "react-dom";

/** Changing the language or the theme crossfades the whole page (--dur-fast), and just changes it when the browser cannot or the person asked for less motion. */
export function crossfade(change: () => void): void {
  const run = () => { flushSync(change); };
  const reduced = typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (typeof document.startViewTransition !== "function" || reduced) {
    run();
    return;
  }
  const transition = document.startViewTransition(run);
  // An aborted transition rejects these with nobody to take them.
  transition.ready.catch(() => undefined);
  transition.finished.catch(() => undefined);
}
