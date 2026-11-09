// @vitest-environment node
import { describe, expect, it } from "vitest";
import { guestId, guestKey, knownGuestId, type GuestMemory } from "./guest";

const form = /^[A-Za-z0-9_-]{22}$/;

function store(initial: Record<string, string> = {}): Storage {
  const items = new Map(Object.entries(initial));
  return {
    get length() { return items.size; },
    clear: () => { items.clear(); },
    key: (i: number) => [...items.keys()][i] ?? null,
    getItem: (k: string) => items.get(k) ?? null,
    setItem: (k: string, v: string) => { items.set(k, v); },
    removeItem: (k: string) => { items.delete(k); },
  };
}

function blocked(): Storage {
  const no = () => { throw new DOMException("blocked", "SecurityError"); };
  return { length: 0, clear: no, key: no, getItem: no, setItem: no, removeItem: no };
}

describe("a browser's guest id", () => {
  it("is made up once at random and kept for every tab and reload of the browser profile", () => {
    const local = store();
    const memory: GuestMemory = {};
    const first = guestId([local, store()], memory);
    expect(first).toMatch(form);
    expect(local.getItem(guestKey)).toBe(first);
    expect(guestId([local, store()], {})).toBe(first); // another tab, or a reload
    expect(guestId([store(), store()], {})).not.toBe(first); // another browser profile
  });

  it("falls back to the tab's session when site storage is blocked, and to the open page when that is too", () => {
    const session = store();
    const inTab = guestId([blocked(), session], {});
    expect(session.getItem(guestKey)).toBe(inTab);
    expect(guestId([blocked(), session], {})).toBe(inTab);

    const memory: GuestMemory = {};
    const onPage = guestId([blocked(), blocked()], memory);
    expect(onPage).toMatch(form);
    expect(guestId([blocked(), blocked()], memory)).toBe(onPage);
    expect(guestId([null, null], {})).toMatch(form);
  });

  it("replaces a stored value that is not a guest id", () => {
    const local = store({ [guestKey]: "alice" });
    const id = guestId([local, store()], {});
    expect(id).not.toBe("alice");
    expect(local.getItem(guestKey)).toBe(id);
  });

  it("is only looked up, never made, for a signed-in page", () => {
    const local = store();
    expect(knownGuestId([local, store()], {})).toBeUndefined();
    expect(local.getItem(guestKey)).toBeNull();
    const id = guestId([local, store()], {});
    expect(knownGuestId([local, store()], {})).toBe(id);
  });
});
