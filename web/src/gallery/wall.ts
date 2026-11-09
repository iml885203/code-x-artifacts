// The motion of the wall of cards when the filter or the search changes: the
// ones that stay slide to their new place (FLIP), the ones that go fade out
// first, the new ones fade in. Everything starts from where the card already
// is, and a browser without the Web Animations API (or a person who asked for
// less motion) gets the same wall without the movement.

const EASE = "cubic-bezier(0.16, 1, 0.3, 1)";
const FAST = 120;
const NORMAL = 200;
/** Cards beyond this many arrive together: a long stagger is waiting. */
const STAGGER_CARDS = 8;
const STAGGER_MS = 20;

export interface Place {
  x: number;
  y: number;
}
export type Places = Map<string, Place>;

export function canAnimate(): boolean {
  return typeof Element.prototype.animate === "function";
}

function reduced(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function cards(grid: HTMLElement): HTMLElement[] {
  return [...grid.querySelectorAll<HTMLElement>("[data-artifact-id]")];
}

/** Where every card is now, relative to the wall. */
export function placesOf(grid: HTMLElement): Places {
  const origin = grid.getBoundingClientRect();
  const places: Places = new Map();
  for (const el of cards(grid)) {
    const r = el.getBoundingClientRect();
    places.set(el.dataset.artifactId ?? "", { x: r.left - origin.left, y: r.top - origin.top });
  }
  return places;
}

/** Fades out the cards that the next filter drops. Resolves when they are gone from view, so the list can change. */
export async function fadeOut(grid: HTMLElement, staying: ReadonlySet<string>): Promise<void> {
  if (!canAnimate()) return;
  const leaving = cards(grid).filter((el) => !staying.has(el.dataset.artifactId ?? ""));
  await Promise.all(
    leaving.map(
      (el) => el.animate([{ opacity: 1 }, { opacity: 0 }], { duration: FAST, easing: "cubic-bezier(0.7, 0, 0.84, 0)", fill: "forwards" }).finished,
    ),
  ).catch(() => undefined); // an interrupted fade just means the list changes sooner
}

/**
 * Plays the arrival after the list changed: `before` is where the cards stood
 * (null when nothing stood there yet, the first time the list appears).
 */
export function arrive(grid: HTMLElement, before: Places | null) {
  if (!canAnimate()) return;
  const calm = reduced();
  const now = placesOf(grid);
  let entering = 0;
  for (const el of cards(grid)) {
    const id = el.dataset.artifactId ?? "";
    const was = before?.get(id);
    const here = now.get(id);
    if (was && here) {
      const dx = was.x - here.x;
      const dy = was.y - here.y;
      if (!calm && (dx !== 0 || dy !== 0)) {
        el.animate([{ transform: `translate(${String(dx)}px, ${String(dy)}px)` }, { transform: "none" }], { duration: NORMAL, easing: EASE });
      }
    } else {
      el.animate([{ opacity: 0 }, { opacity: 1 }], {
        duration: calm ? FAST : NORMAL,
        easing: EASE,
        delay: calm ? 0 : Math.min(entering, STAGGER_CARDS) * STAGGER_MS,
        fill: "backwards",
      });
      entering++;
    }
  }
}
