import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, SignedOutError } from "../api";
import { FakeRoom, settle } from "./fakeRoom";
import { LiveProvider, type Connection } from "./provider";

let providers: LiveProvider[] = [];
let room: FakeRoom;

beforeEach(() => {
  room = new FakeRoom();
});
afterEach(() => {
  for (const p of providers) p.destroy();
  providers = [];
  vi.useRealTimers();
});

function connect(options: { readOnly?: boolean; ticket?: () => Promise<never> | ReturnType<FakeRoom["ticket"]>; network?: EventTarget & { onLine?: boolean } } = {}): LiveProvider {
  const provider = new LiveProvider({
    ticket: options.ticket ?? room.ticket,
    open: room.open,
    user: { id: "alice", name: "alice", color: "c1" },
    readOnly: options.readOnly ?? false,
    ...(options.network ? { network: options.network } : {}),
  });
  providers.push(provider);
  provider.start();
  return provider;
}

const type = (p: LiveProvider, text: string) => { p.doc.getText("t").insert(0, text); };

describe("the live provider", () => {
  it("connects with a ticket, becomes live when the room's state has arrived, and sends the first connection no epoch", async () => {
    const p = connect();
    expect(p.state).toBe("connecting");
    expect(p.synced).toBe(false);
    await settle();
    expect(p.state).toBe("live");
    expect(p.synced).toBe(true);
    expect(room.urls[0]).toMatch(/^ws:\/\/[^/]+\/ws\/room\?ticket=t1$/);
  });

  it("takes what the room holds and sends what it types, and Saved comes back only once the room has stored it", async () => {
    room.doc.getText("t").insert(0, "from the room");
    const p = connect();
    await settle();
    expect(p.doc.getText("t").toJSON()).toBe("from the room");
    expect(p.saved).toBe(true);

    type(p, "typed ");
    expect(p.saved).toBe(false);
    await settle();
    expect(room.doc.getText("t").toJSON()).toBe("typed from the room");
    expect(p.saved).toBe(true);
  });

  it("does not call a change saved while the room has not answered it", async () => {
    const p = connect();
    await settle();
    room.silent = true;
    type(p, "x");
    await settle();
    expect(p.saved).toBe(false);
    expect(room.doc.getText("t").toJSON()).toBe("");
  });

  it("shows the others in awareness and forgets them when the connection goes", async () => {
    const a = new LiveProvider({ ticket: room.ticket, open: room.open, user: { id: "alice", name: "alice", color: "c1" }, readOnly: false });
    const b = new LiveProvider({ ticket: room.ticket, open: room.open, user: { id: "bob", name: "bob", color: "c2" }, readOnly: false });
    providers.push(a, b);
    a.start();
    b.start();
    await settle();
    const names = (p: LiveProvider) => Array.from(p.awareness.getStates().values(), (s) => (s.user as { name: string }).name).sort();
    expect(names(a)).toEqual(["alice", "bob"]);
    expect(names(b)).toEqual(["alice", "bob"]);

    a.awareness.setLocalStateField("cursor", { anchor: 3 });
    await settle();
    expect(b.awareness.getStates().get(a.doc.clientID)).toMatchObject({ cursor: { anchor: 3 } });

    b.destroy();
    providers = [a];
    room.sockets[1]?.close();
    // The room knows the connection ended; here the browser's side is what is under test.
    room.live[0]?.end(1006, "");
    await settle();
    expect(names(a)).toEqual(["alice"]);
  });

  it("reconnects with a new ticket after the connection drops, and nothing typed offline is lost", async () => {
    vi.useFakeTimers();
    const p = connect();
    await vi.advanceTimersByTimeAsync(10);
    expect(p.state).toBe("live");

    room.refuse = 1006;
    room.closeAll(1006);
    await vi.advanceTimersByTimeAsync(10);
    expect(p.state).toBe("reconnecting");
    type(p, "offline edit ");
    room.doc.getText("t").insert(0, "room edit ");

    room.refuse = null;
    await vi.advanceTimersByTimeAsync(20_000);
    expect(p.state).toBe("live");
    expect(room.tickets).toBeGreaterThan(1);
    const merged = room.doc.getText("t").toJSON();
    expect(merged).toContain("offline edit ");
    expect(merged).toContain("room edit ");
    expect(p.doc.getText("t").toJSON()).toBe(merged);
    expect(p.saved).toBe(true);
  });

  it("backs off between attempts, doubling the wait", async () => {
    vi.useFakeTimers();
    room.refuse = 1006;
    const p = connect();
    await vi.advanceTimersByTimeAsync(10);
    expect(room.urls).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(490);
    expect(room.urls).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(999);
    expect(room.urls).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(room.urls).toHaveLength(3);
    expect(p.state).toBe("reconnecting");
  });

  it("sends the epoch it has on every reconnection, and throws its copy away when told the room was rebuilt", async () => {
    vi.useFakeTimers();
    const p = connect();
    await vi.advanceTimersByTimeAsync(10);
    type(p, "old history");
    await vi.advanceTimersByTimeAsync(10);
    const resets = vi.fn();
    p.onReset(resets);
    const before = p.doc;

    room.closeAll(1006);
    await vi.advanceTimersByTimeAsync(600);
    expect(room.urls.at(-1)).toContain("epoch=e1");

    room.epoch = "e2";
    room.doc.getText("t").delete(0, room.doc.getText("t").length);
    room.closeAll(4001, "e2");
    await vi.advanceTimersByTimeAsync(10);
    expect(p.synced).toBe(true);
    expect(resets).toHaveBeenCalledTimes(1);
    expect(p.doc).not.toBe(before);
    expect(room.urls.at(-1)).not.toContain("epoch=");
    expect(p.state).toBe("live");
  });

  it("starts from the room's state again when a limit was hit", async () => {
    const p = connect();
    await settle();
    const resets = vi.fn();
    p.onReset(resets);
    room.closeAll(4002, "update size (bytes): 70000 is over the limit of 65536");
    await settle();
    expect(resets).toHaveBeenCalledTimes(1);
    expect(p.state).toBe("live");
  });

  it("asks the page to reread who it is when access changed, and takes a new ticket at once", async () => {
    const onAccessChanged = vi.fn();
    const p = new LiveProvider({ ticket: room.ticket, open: room.open, user: { id: "a", name: "a", color: "c0" }, readOnly: false, onAccessChanged });
    providers.push(p);
    p.start();
    await settle();
    room.closeAll(4403);
    await settle();
    expect(onAccessChanged).toHaveBeenCalledTimes(1);
    expect(room.tickets).toBe(2);
    expect(p.state).toBe("live");
  });

  it.each([
    ["signed out", () => Promise.reject(new SignedOutError("the session ended"))],
    ["not allowed", () => Promise.reject(new ApiError("POST: 403", 403))],
    ["gone", () => Promise.reject(new ApiError("POST: 404", 404))],
  ])("stops, offline, when the ticket says the person is %s", async (_why, ticket) => {
    const onDenied = vi.fn();
    const p = new LiveProvider({ ticket, open: room.open, user: { id: "a", name: "a", color: "c0" }, readOnly: false, onDenied });
    providers.push(p);
    p.start();
    await settle();
    expect(p.state).toBe("offline");
    expect(onDenied).toHaveBeenCalledTimes(1);
    expect(room.urls).toHaveLength(0);
  });

  it("keeps trying when the ticket fails for another reason", async () => {
    vi.useFakeTimers();
    let calls = 0;
    const p = connect({
      ticket: () => (++calls < 3 ? Promise.reject(new ApiError("POST: 500", 500)) : room.ticket()),
    });
    await vi.advanceTimersByTimeAsync(10);
    expect(p.state).toBe("reconnecting");
    await vi.advanceTimersByTimeAsync(5_000);
    expect(p.state).toBe("live");
  });

  it("goes offline when the browser does and reconnects at once when it is back", async () => {
    const network = Object.assign(new EventTarget(), { onLine: true });
    const p = connect({ network });
    await settle();
    const states: Connection[] = [];
    p.onChange(() => states.push(p.state));

    network.onLine = false;
    network.dispatchEvent(new Event("offline"));
    expect(p.state).toBe("offline");
    expect(room.live).toHaveLength(0);
    type(p, "while offline");
    expect(p.saved).toBe(false);

    network.onLine = true;
    network.dispatchEvent(new Event("online"));
    await settle();
    expect(p.state).toBe("live");
    expect(room.doc.getText("t").toJSON()).toBe("while offline");
    expect(p.saved).toBe(true);
    expect(states).toContain("offline");
  });

  it("a reader receives edits and sends none", async () => {
    room.readOnly = true;
    const p = connect({ readOnly: true });
    await settle();
    room.doc.getText("t").insert(0, "live");
    await settle();
    expect(p.doc.getText("t").toJSON()).toBe("live");
    type(p, "nope");
    await settle();
    expect(room.doc.getText("t").toJSON()).toBe("live");
    expect(p.saved).toBe(true);
  });
});
