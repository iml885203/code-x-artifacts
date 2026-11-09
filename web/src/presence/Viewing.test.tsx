import { useState } from "react";
import { act, configure, getConfig, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Awareness } from "y-protocols/awareness";
import * as Y from "yjs";
import { renderIn } from "../test-support";
import { publishRoomPeople } from "./roomPeople";
import { RoomPresence } from "./RoomPresence";
import { Viewing } from "./Viewing";
import { guestKey } from "./guest";
import { heartbeatEveryMs } from "./useViewers";

// The header's stack as a page meets it: it tells the server it is here on a timer, and shows
// whoever the server says is.

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const original = getConfig();
const tick = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms));
const hide = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};

function server(answer: () => Response) {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      calls.push(`${init?.method ?? "GET"} ${url}`);
      return Promise.resolve(answer());
    }),
  );
  return calls;
}

beforeEach(() => {
  configure({ asyncWrapper: (callback) => callback() as ReturnType<typeof original.asyncWrapper> });
  vi.useFakeTimers();
});
afterEach(() => {
  configure({ asyncWrapper: (callback) => original.asyncWrapper(callback) });
  vi.useRealTimers();
  vi.unstubAllGlobals();
  hide(false);
});

const entries = [
  { id: "alice", name: "Alice", agent: false, self: true },
  { id: "bob", name: "Bob", agent: false, self: false },
];

describe("who is viewing, at the top right of a page", () => {
  it("beats on opening and every twenty seconds, and shows who the server names", async () => {
    const calls = server(() => json(entries));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    expect(calls).toEqual(["POST /api/artifacts/page1/viewers"]);
    expect(screen.getAllByRole("button").map((b) => b.querySelector(".visually-hidden")?.textContent)).toEqual(["Alice (You)", "Bob"]);
    await tick(heartbeatEveryMs - 1);
    expect(calls).toHaveLength(1);
    await tick(1);
    expect(calls).toHaveLength(2);
  });

  it("sends nothing while the browser is offline, logs nothing, and beats at once when it is back", async () => {
    const calls = server(() => json(entries));
    const logged = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const online = vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    renderIn(<Viewing id="page1" />);
    await tick(heartbeatEveryMs * 3);
    expect(calls).toEqual([]);
    expect(logged).not.toHaveBeenCalled();
    online.mockReturnValue(true);
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    await tick(0);
    expect(calls).toEqual(["POST /api/artifacts/page1/viewers"]);
    logged.mockRestore();
    online.mockRestore();
  });

  it("sends nothing while the tab is hidden and beats at once when it is shown", async () => {
    const calls = server(() => json(entries));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    hide(true);
    await tick(heartbeatEveryMs * 3);
    expect(calls).toHaveLength(1);
    hide(false);
    await tick(0);
    expect(calls).toHaveLength(2);
  });

  it("stops for good when the artifact is gone and shows nobody", async () => {
    let gone = false;
    const calls = server(() => (gone ? json({ error: "not found" }, 404) : json(entries)));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    gone = true;
    await tick(heartbeatEveryMs);
    expect(screen.queryByRole("group")).toBeNull();
    await tick(heartbeatEveryMs * 5);
    expect(calls).toHaveLength(2);
  });

  it("shows a person who is in the live room and in the server's list once", async () => {
    server(() => json(entries));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    let leave: () => void = () => undefined;
    act(() => {
      leave = publishRoomPeople([{ id: "bob", name: "Bob", agent: false, self: false, color: "c2" }, { id: "dana", name: "Dana", agent: false, self: false }]);
    });
    expect(screen.getAllByRole("button").map((b) => b.querySelector(".visually-hidden")?.textContent)).toEqual(["Alice (You)", "Bob", "Dana"]);
    act(leave);
    expect(screen.getAllByRole("button")).toHaveLength(2);
  });

  it("beats as the browser's guest when signed out, with the same id from every tab, and shows itself as the guest", async () => {
    localStorage.removeItem(guestKey);
    const bodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: RequestInit) => {
        bodies.push(JSON.parse(init?.body as string));
        return Promise.resolve(json([{ id: "guest:1", name: "Swift Otter", agent: false, guest: true, self: true }]));
      }),
    );
    renderIn(<><Viewing id="page1" signedIn={false} /><Viewing id="page1" signedIn={false} /></>);
    await tick(0);
    const id = localStorage.getItem(guestKey);
    expect(id).toMatch(/^[A-Za-z0-9_-]{22}$/);
    expect(bodies).toEqual([{ guest: id }, { guest: id }]);
    expect(screen.getAllByRole("button", { name: "Swift Otter (Guest, You)" })).toHaveLength(2);
  });

  it("beats at the pace the server names from its very first answer on", async () => {
    const calls = server(() => new Response(JSON.stringify(entries), { status: 200, headers: { "CodeXArtifacts-Heartbeat-Ms": "1000" } }));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    await tick(999);
    expect(calls).toHaveLength(1);
    await tick(1);
    expect(calls).toHaveLength(2);
  });

  it("waits quietly for as long as the server says when its guest list is full, and shows itself once it is let in", async () => {
    let full = true;
    const calls = server(() =>
      full
        ? new Response(JSON.stringify({ error: "this page already lists as many guests as it can" }), { status: 429, headers: { "Retry-After": "60" } })
        : json([{ id: "guest:1", name: "Swift Otter", agent: false, guest: true, self: true }]),
    );
    const logged = vi.spyOn(console, "error").mockImplementation(() => undefined);
    renderIn(<Viewing id="page1" signedIn={false} />);
    await tick(0);
    expect(calls).toHaveLength(1);
    expect(screen.queryByRole("group")).toBeNull();
    await tick(60_000 - 1);
    expect(calls).toHaveLength(1);
    full = false;
    await tick(1);
    expect(calls).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Swift Otter (Guest, You)" })).toBeInTheDocument();
    expect(logged).not.toHaveBeenCalled();
    logged.mockRestore();
  });

  it("stops for good when a page whose session ended is told it is signed out", async () => {
    const calls = server(() => json({ error: "sign in, or send guest" }, 403));
    const logged = vi.spyOn(console, "error").mockImplementation(() => undefined);
    renderIn(<Viewing id="page1" />);
    await tick(heartbeatEveryMs * 3);
    expect(calls).toHaveLength(1);
    expect(logged).not.toHaveBeenCalled();
    logged.mockRestore();
  });

  it("reads the page even when the browser blocks site storage, as a guest of this session", async () => {
    const blocked = vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const calls = server(() => json([{ id: "guest:1", name: "Swift Otter", agent: false, guest: true, self: true }]));
    renderIn(<Viewing id="page1" signedIn={false} />);
    await tick(0);
    expect(calls).toEqual(["POST /api/artifacts/page1/viewers"]);
    expect(sessionStorage.getItem(guestKey)).toMatch(/^[A-Za-z0-9_-]{22}$/);
    expect(screen.getByRole("button", { name: "Swift Otter (Guest, You)" })).toBeInTheDocument();
    blocked.mockRestore();
    sessionStorage.removeItem(guestKey);
  });

  it("sends the guest id its browser had once signed in, so the account takes the guest's place, and none it never had", async () => {
    localStorage.removeItem(guestKey);
    const bodies: (string | undefined)[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: RequestInit) => {
        bodies.push(init?.body as string | undefined);
        return Promise.resolve(json(entries));
      }),
    );
    const first = renderIn(<Viewing id="page1" />);
    await tick(0);
    expect(bodies).toEqual([undefined]);
    expect(localStorage.getItem(guestKey)).toBeNull();
    first.unmount();
    localStorage.setItem(guestKey, "abcdefghijklmnopqrstuv");
    renderIn(<Viewing id="page1" />);
    await tick(0);
    expect(bodies[1]).toBe(JSON.stringify({ guest: "abcdefghijklmnopqrstuv" }));
    localStorage.removeItem(guestKey);
  });

  it("keeps a guest apart from a live room's person of the same name, and the room's people once", async () => {
    server(() => json([...entries, { id: "guest:1", name: "Bob", agent: false, guest: true, self: false }]));
    renderIn(<Viewing id="page1" />);
    await tick(0);
    let leave: () => void = () => undefined;
    act(() => {
      leave = publishRoomPeople([{ id: "bob", name: "Bob", agent: false, self: false, color: "c2" }, { id: "alice", name: "alice", agent: false, self: true }]);
    });
    expect(screen.getAllByRole("button").map((b) => b.querySelector(".visually-hidden")?.textContent)).toEqual(["Alice (You)", "Bob", "Bob (Guest)"]);
    act(leave);
  });

  it("lists a live room's people from its awareness, for Docs and Design alike, and drops them when the room goes", async () => {
    server(() => json(entries));
    const awareness = new Awareness(new Y.Doc());
    awareness.setLocalStateField("user", { id: "alice", name: "Alice", color: "c3" });
    let closeRoom: () => void = () => undefined;
    function Page() {
      const [open, setOpen] = useState(true);
      closeRoom = () => {
        setOpen(false);
      };
      return (
        <>
          <Viewing id="page1" />
          {open && <RoomPresence awareness={awareness} self={awareness.clientID} state="live" />}
        </>
      );
    }
    renderIn(<Page />);
    await tick(0);
    const names = () => screen.getAllByRole("button").map((b) => b.querySelector(".visually-hidden")?.textContent);
    expect(names()).toEqual(["Alice (You)", "Bob"]);
    expect(screen.getByText("Live")).toBeInTheDocument();
    act(() => {
      (awareness.getStates() as Map<number, object>).set(9, { user: { id: "alice/Agent Smith", name: "Agent Smith", agent: true } });
      awareness.emit("change", [{ added: [9], updated: [], removed: [] }, "net"]);
    });
    expect(names()).toEqual(["Alice (You)", "Agent Smith (AI agent)", "Bob"]);
    expect(screen.getByText("AI")).toBeInTheDocument();
    act(() => {
      closeRoom();
    });
    expect(names()).toEqual(["Alice (You)", "Bob"]);
  });
});
