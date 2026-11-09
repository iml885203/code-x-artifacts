import { useEffect, useState } from "react";

/** How long to wait for the server to have a frame the room was just given, and how often to ask. */
const GIVE_UP = 8000;
const EVERY = 250;

/**
 * Which of the addresses the server answers for. A frame someone has just added
 * is served from the room, which the server has the moment the person's change
 * reaches it, a few milliseconds after the person's own screen shows it; a frame
 * loaded before then is an error page for good, so it is not loaded until the
 * server has it. An address that does not come in time is let through, so a
 * frame that cannot be drawn is the frame's own error.
 */
export function useServed(urls: string[]): Set<string> {
  const [served, setServed] = useState<Set<string>>(new Set());
  const key = urls.join("\n");
  useEffect(() => {
    const wanted = key === "" ? [] : key.split("\n");
    let live = true;
    const timers: number[] = [];
    for (const url of wanted) {
      const started = Date.now();
      const ask = () => {
        fetch(url, { cache: "no-store" }).then(
          (res) => {
            if (!live) return;
            if (res.ok || Date.now() - started > GIVE_UP) setServed((s) => new Set(s).add(url));
            else timers.push(window.setTimeout(ask, EVERY));
          },
          () => {
            if (!live) return;
            if (Date.now() - started > GIVE_UP) setServed((s) => new Set(s).add(url));
            else timers.push(window.setTimeout(ask, EVERY));
          },
        );
      };
      ask();
    }
    return () => {
      live = false;
      for (const timer of timers) window.clearTimeout(timer);
    };
  }, [key]);
  return served;
}
