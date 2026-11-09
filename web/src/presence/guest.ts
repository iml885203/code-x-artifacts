/** Where a browser keeps the id it is listed under as a guest while signed out. */
export const guestKey = "pictor-guest";

// The server accepts 16 to 64 of these characters; the browser always makes 22 (128 random bits).
const guestForm = /^[A-Za-z0-9_-]{16,64}$/;

/** The guest id of a page whose site storage is blocked entirely: it lasts as long as the page. */
export interface GuestMemory {
  id?: string;
}

type Store = Pick<Storage, "getItem" | "setItem">;

const pageMemory: GuestMemory = {};

/** The two places a guest id may be kept, best first; a store that is absent or throws is null. */
function browserStores(): (Store | null)[] {
  const open = (get: () => Storage): Store | null => {
    try {
      return get();
    } catch {
      return null;
    }
  };
  return [open(() => window.localStorage), open(() => window.sessionStorage)];
}

function read(s: Store | null): string | undefined {
  try {
    const v = s?.getItem(guestKey);
    return v && guestForm.test(v) ? v : undefined;
  } catch {
    return undefined;
  }
}

function keep(s: Store | null, id: string): boolean {
  if (s === null) return false;
  try {
    s.setItem(guestKey, id);
    return true;
  } catch {
    return false;
  }
}

function randomId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** The guest id this browser already has, if any: a signed-in page sends it so its account takes that guest's place. */
export function knownGuestId(stores = browserStores(), memory = pageMemory): string | undefined {
  for (const s of stores) {
    const id = read(s);
    if (id !== undefined) return id;
  }
  return memory.id;
}

/**
 * The id this browser is listed under as a guest while signed out: random, made up the first time and nothing
 * about the browser itself. It is kept in site storage so every tab and reload of the browser profile is the same
 * guest; where that is blocked, in the tab's session; where both are, on the open page only.
 */
export function guestId(stores = browserStores(), memory = pageMemory): string {
  const known = knownGuestId(stores, memory);
  if (known !== undefined) return known;
  const fresh = randomId();
  if (!stores.some((s) => keep(s, fresh))) memory.id = fresh;
  return fresh;
}
