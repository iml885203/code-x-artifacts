// The theme a person chose: follow the system (the default), or pin light or
// dark. The choice is remembered per browser and applied to <html> before the
// first paint by /static/theme.js, which holds the same rule in ES5 (the page
// is rendered before the app's module runs, so the app cannot do it).
import { useSyncExternalStore } from "react";

export type ThemeChoice = "system" | "light" | "dark";
export const themeChoices: readonly ThemeChoice[] = ["system", "light", "dark"];
export const themeKey = "pictor-theme";

type Reader = Pick<Storage, "getItem">;
type Writer = Pick<Storage, "setItem" | "removeItem">;

/** Storage can be absent, blocked or throwing (private windows, blocked site data): that is "no choice yet", never an error. */
export function browserStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function readChoice(storage: Reader | null): ThemeChoice {
  try {
    const v = storage?.getItem(themeKey);
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
}

export function saveChoice(storage: Writer | null, choice: ThemeChoice): void {
  try {
    if (choice === "system") storage?.removeItem(themeKey);
    else storage?.setItem(themeKey, choice);
  } catch {
    // A blocked store only means the choice lasts until the page closes.
  }
}

/** Which theme the page shows: the pinned one, else the system's. */
export function resolveTheme(choice: ThemeChoice, systemPrefersDark: boolean): "light" | "dark" {
  return choice === "system" ? (systemPrefersDark ? "dark" : "light") : choice;
}

/** Following the system is "no attribute": the stylesheet's own default does it. */
export function applyChoice(root: HTMLElement, choice: ThemeChoice): void {
  if (choice === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", choice);
}

let current: ThemeChoice | null = null;
const listeners = new Set<() => void>();

function snapshot(): ThemeChoice {
  current ??= readChoice(browserStorage());
  return current;
}

export function setThemeChoice(choice: ThemeChoice): void {
  current = choice;
  saveChoice(browserStorage(), choice);
  applyChoice(document.documentElement, choice);
  listeners.forEach((l) => { l(); });
}

/** Test seam: forget what this page remembers, so the next read asks storage again. */
export function forgetThemeChoice(): void {
  current = null;
  listeners.forEach((l) => { l(); });
}

export function useThemeChoice(): [ThemeChoice, (choice: ThemeChoice) => void] {
  const choice = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    snapshot,
    () => "system" as const,
  );
  return [choice, setThemeChoice];
}
