import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { applyChoice, readChoice, resolveTheme, saveChoice, themeKey, type ThemeChoice } from "./theme";

const blocked = {
  getItem: () => {
    throw new DOMException("blocked", "SecurityError");
  },
  setItem: () => {
    throw new DOMException("blocked", "SecurityError");
  },
  removeItem: () => {
    throw new DOMException("blocked", "SecurityError");
  },
};

function memory(initial?: string) {
  const data = new Map<string, string>(initial === undefined ? [] : [[themeKey, initial]]);
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
    removeItem: (k: string) => void data.delete(k),
  };
}

describe("which theme the page shows", () => {
  it.each<[ThemeChoice, boolean, "light" | "dark"]>([
    ["system", true, "dark"],
    ["system", false, "light"],
    ["light", true, "light"],
    ["dark", false, "dark"],
  ])("with the choice %s and a dark system (%s) is %s", (choice, dark, shown) => {
    expect(resolveTheme(choice, dark)).toBe(shown);
  });

  it.each([
    [undefined, "system"],
    ["light", "light"],
    ["dark", "dark"],
    ["sepia", "system"],
    ["", "system"],
  ])("reads the stored %s as %s", (stored, choice) => {
    expect(readChoice(memory(stored))).toBe(choice);
  });

  it("follows the system when storage is blocked or missing, and a choice made then does not throw", () => {
    expect(readChoice(blocked)).toBe("system");
    expect(readChoice(null)).toBe("system");
    expect(() => { saveChoice(blocked, "dark"); }).not.toThrow();
    expect(() => { saveChoice(null, "dark"); }).not.toThrow();
  });

  it("remembers a pinned theme, and forgets it when following the system again", () => {
    const store = memory();
    saveChoice(store, "dark");
    expect(readChoice(store)).toBe("dark");
    saveChoice(store, "system");
    expect(store.data.size).toBe(0);
  });
});

// The script the shell loads in <head> is what puts the theme on the page before the first paint, so it
// has to give the same answer as the rule the app uses.
describe("the theme on the page before the app runs", () => {
  const script = readFileSync(resolve(process.cwd(), "../internal/web/static/theme.js"), "utf8");
  const root = document.documentElement;
  afterEach(() => {
    root.removeAttribute("data-theme");
    localStorage.clear();
  });

  function runScript(): string | null {
    root.removeAttribute("data-theme");
    const el = document.createElement("script");
    el.textContent = script;
    document.head.append(el);
    el.remove();
    return root.getAttribute("data-theme");
  }

  it.each(["light", "dark", "sepia", undefined])("matches the app for a stored %s", (stored) => {
    localStorage.clear();
    if (stored !== undefined) localStorage.setItem(themeKey, stored);
    const fromScript = runScript();
    applyChoice(root, readChoice(localStorage));
    expect(fromScript).toBe(root.getAttribute("data-theme"));
  });

  it("applies a pinned theme to <html> and sets nothing when following the system", () => {
    localStorage.setItem(themeKey, "dark");
    expect(runScript()).toBe("dark");
    localStorage.clear();
    expect(runScript()).toBeNull();
  });

  it("does not throw when storage is blocked", () => {
    const real = Object.getOwnPropertyDescriptor(window, "localStorage");
    Object.defineProperty(window, "localStorage", { configurable: true, get: () => { throw new DOMException("blocked", "SecurityError"); } });
    try {
      expect(runScript()).toBeNull();
    } finally {
      if (real) Object.defineProperty(window, "localStorage", real);
    }
  });
});
