// Relative times ("2 hours ago") for everything that shows when something
// happened, and the one shared clock that keeps them current. The wording is
// Intl's, in the page language; the exact local date and time stays available
// next to it (RelativeTime.tsx) because "2 hours ago" is not a timestamp.
import { useSyncExternalStore } from "react";

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const MONTH = 30 * DAY;
const YEAR = 365 * DAY;

/** How often the shared clock ticks, and so the finest step of the wording: under a minute it counts in tens of seconds. */
export const tickMs = 10 * SECOND;

export interface Span {
  /** Negative: in the past. Never positive, see relativeSpan. */
  value: number;
  unit: Intl.RelativeTimeFormatUnit;
}

/**
 * The span from `then` to `now` in the largest whole unit. A time after `now`
 * (a clock that runs behind the server's) counts as now rather than as "in 3
 * minutes": nothing a person reads here has not happened yet.
 */
export function relativeSpan(then: number, now: number): Span {
  const ago = Math.max(0, now - then);
  if (ago < tickMs) return { value: 0, unit: "second" };
  if (ago < MINUTE) return { value: -Math.floor(ago / tickMs) * 10, unit: "second" };
  if (ago < HOUR) return { value: -Math.floor(ago / MINUTE), unit: "minute" };
  if (ago < DAY) return { value: -Math.floor(ago / HOUR), unit: "hour" };
  if (ago < WEEK) return { value: -Math.floor(ago / DAY), unit: "day" };
  if (ago < MONTH) return { value: -Math.floor(ago / WEEK), unit: "week" };
  if (ago < YEAR) return { value: -Math.min(11, Math.floor(ago / MONTH)), unit: "month" };
  return { value: -Math.floor(ago / YEAR), unit: "year" };
}

const formats = new Map<string, Intl.RelativeTimeFormat>();

/** "2 hours ago", "yesterday", "now" in `lang`; the text of `iso` itself when it is not a time. */
export function relativeTime(iso: string, now: number, lang: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;
  let format = formats.get(lang);
  if (!format) {
    format = new Intl.RelativeTimeFormat(lang, { numeric: "auto" });
    formats.set(lang, format);
  }
  const { value, unit } = relativeSpan(then, now);
  return format.format(value, unit);
}

// One timer for every time on the page, however many there are. It runs only
// while something shows a time and the tab is visible; coming back to a tab
// that was hidden refreshes at once instead of waiting for the next tick.
let current = Date.now();
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | undefined;

function tick() {
  current = Date.now();
  for (const listener of listeners) listener();
}

function run() {
  if (timer === undefined && !document.hidden) timer = setInterval(tick, tickMs);
}

function pause() {
  clearInterval(timer);
  timer = undefined;
}

function onVisibility() {
  if (document.hidden) {
    pause();
    return;
  }
  tick();
  run();
}

function subscribe(listener: () => void): () => void {
  if (listeners.size === 0) {
    // Nothing kept the value current while no time was on the page.
    current = Date.now();
    document.addEventListener("visibilitychange", onVisibility);
    run();
  }
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      document.removeEventListener("visibilitychange", onVisibility);
      pause();
    }
  };
}

/** The current time in milliseconds, renewed by the shared clock. */
export function useNow(): number {
  return useSyncExternalStore(subscribe, () => current);
}
