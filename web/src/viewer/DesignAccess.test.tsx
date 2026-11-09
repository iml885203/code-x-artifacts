import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { FakeRoom } from "../editor/fakeRoom";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "./Viewer";
import { ROOM_MAP } from "../design/edits";
import { emptyFrameEntries, LEGACY_KEY } from "../design/pages/structure";

const manifest = { version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] };

afterEach(() => { vi.unstubAllGlobals(); });

it("removes Design authoring controls when editing permission changes on the connected room", async () => {
  const room = new FakeRoom();
  room.doc.transact(() => {
    room.doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
    for (const [key, value] of emptyFrameEntries("frames/hero.html")) room.doc.getMap(ROOM_MAP).set(key, value);
  });
  class RoomSocket extends EventTarget {
    binaryType = "arraybuffer";
    private readonly end: ReturnType<FakeRoom["open"]>;
    constructor(url: string) {
      super();
      this.end = room.open(url);
      this.end.onopen = () => this.dispatchEvent(new Event("open"));
      this.end.onmessage = (e) => this.dispatchEvent(new MessageEvent("message", { data: e.data }));
      this.end.onclose = (e) => this.dispatchEvent(Object.assign(new Event("close"), { code: e.code, reason: e.reason }));
    }
    send(data: Uint8Array) { this.end.send(data); }
    close() { this.end.close(); }
  }
  vi.stubGlobal("WebSocket", RoomSocket);
  let current = artifact({ kind: "design", title: "Shared design", author: "alice", role: "editor", canEdit: true, canChange: false, visibility: "organisation" });
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.endsWith("/live")) return Promise.resolve(new Response(JSON.stringify({ ticket: "t", epoch: "e1", path: "/ws/room" })));
    if (url.includes("/design")) return Promise.resolve(new Response(JSON.stringify({ version: 1, manifest, base: "/raw/x/1.2.sig/", files: "/files/art/g.sig/" })));
    if (url.endsWith("/comments")) return Promise.resolve(new Response(JSON.stringify([])));
    return Promise.resolve(new Response(JSON.stringify(current)));
  }));
  const view = renderIn(<Viewer artifact={current} boot={bootFor("en", { page: "viewer", user: "bob" })} />, "en", `/a/${current.id}`);
  try {
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Drawing" }));
    expect(await screen.findByRole("menuitem", { name: /^Pencil/ })).toBeVisible();
    expect(screen.getByRole("menuitem", { name: /^PenShift\+P$/ })).toBeVisible();
    await user.keyboard("{Escape}");
    await user.keyboard("p");
    expect(screen.getByRole("group", { name: "Draw a pencil stroke" })).toBeInTheDocument();
    current = { ...current, role: "reader", canEdit: false };
    room.readOnly = true;
    act(() => { room.closeAll(4403); });
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Drawing" })).not.toBeInTheDocument(); });
    for (const name of ["Text", "Artboard", "Shape"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Draw a pencil stroke" })).not.toBeInTheDocument();
    await user.keyboard("p");
    expect(screen.queryByRole("group", { name: "Draw a pencil stroke" })).not.toBeInTheDocument();
    await user.keyboard("{Shift>}p{/Shift}");
    expect(screen.queryByRole("group", { name: "Draw a connected path" })).not.toBeInTheDocument();
  } finally {
    view.unmount();
    room.awareness.destroy();
    room.doc.destroy();
  }
});
