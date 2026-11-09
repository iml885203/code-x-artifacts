import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Awareness } from "y-protocols/awareness";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { artifact, renderIn } from "../test-support";
import { AvatarStack } from "../presence/AvatarStack";
import { useRoomPeople } from "../presence/roomPeople";
import { DesignCanvas } from "./Canvas";
import type { Live } from "./useRoom";

// The canvas with a room: what other people have selected is drawn over the frame under the name the server gave them.
const room = vi.hoisted(() => ({ live: null as Live | null }));
vi.mock("./useRoom", async (original) => ({
  ...(await original<typeof import("./useRoom")>()),
  useRoom: () => ({ edits: null, live: room.live }),
}));

const manifest = { version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] };
const reader = artifact({ title: "Landing", author: "alice", kind: "design", canChange: false, canEdit: false, role: "reader" });

afterEach(() => {
  vi.unstubAllGlobals();
});

function joinRoom(state: Live["state"] = "live") {
  const awareness = new Awareness(new Y.Doc());
  awareness.setLocalStateField("user", { name: "bob", color: "c1" });
  const live: Live = { awareness, self: awareness.clientID, state };
  room.live = live;
  return { awareness, live };
}

/** Another person's awareness state, as the server relays it (their name is the one the server wrote). */
const someoneElse = (awareness: Awareness, client: number, state: object | null) => {
  act(() => {
    const states = awareness.getStates() as Map<number, object>;
    if (state) states.set(client, state);
    else states.delete(client);
    awareness.emit("change", [{ added: [client], updated: [], removed: [] }, "net"]);
  });
};

/** The header's stack, which is where the canvas's room shows who is here. */
function HeaderStack() {
  return <AvatarStack people={useRoomPeople()} />;
}

async function open() {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(new Response(JSON.stringify({ version: 1, manifest, base: "/raw/x/1.2.sig/", files: "/files/art/g.sig/" }), { status: 200 }))),
  );
  renderIn(
    <>
      <HeaderStack />
      <DesignCanvas artifact={reader} older={null} account="bob" />
    </>,
    "en",
    `/a/${reader.id}`,
  );
  await screen.findByRole("toolbar", { name: "Canvas" });
  const el = screen.getByTitle("Hero, 800 by 600");
  if (!(el instanceof HTMLIFrameElement) || !el.contentWindow) throw new Error("no frame");
  const win = el.contentWindow;
  const sent = vi.spyOn(win, "postMessage").mockImplementation(() => undefined);
  const say = (data: unknown) => {
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data, source: win }));
    });
  };
  const asked = (type: string) => {
    const found = sent.mock.calls.map(([m]) => m as { id: number; type: string; pids?: string[] }).filter((m) => m.type === type).at(-1);
    if (!found) throw new Error(`the frame was never asked ${type}`);
    return found;
  };
  say({ pictor: "inspect", type: "ready" });
  return { say, asked };
}

const item = (pid: string, rect = { x: 40, y: 50, w: 300, h: 80 }) => ({ pid, tag: "h1", rect, attrs: [], inline: [], computed: {} });

describe("other people on the canvas", () => {
  it("draws a box with the person's name over what they selected and takes it away when they let go", async () => {
    const { awareness } = joinRoom();
    const frame = await open();
    someoneElse(awareness, 7, { user: { name: "alice", color: "c3" }, selection: { hero: ["title"] } });
    await waitFor(() => {
      expect(frame.asked("report").pids).toEqual(["title"]);
    });
    frame.say({ pictor: "inspect", id: frame.asked("report").id, type: "report", items: [item("title")] });
    const box = await screen.findByText("alice", { selector: ".peer-tag" });
    expect(box.closest("[data-peer]")).toHaveStyle({ left: "160px", top: "170px", width: "300px", height: "80px" });
    expect(box.closest("[data-peer]")).toHaveClass("ed-c3");

    someoneElse(awareness, 7, { user: { name: "alice", color: "c3" }, selection: null });
    await waitFor(() => {
      expect(screen.queryByText("alice", { selector: ".peer-tag" })).not.toBeInTheDocument();
    });
  });

  it("draws an agent's name over the elements it changed, marked as an agent's", async () => {
    const { awareness } = joinRoom();
    const frame = await open();
    someoneElse(awareness, 9, { user: { name: "alice 的 Claude Code", agent: true }, touched: { hero: ["title"] }, at: 1 });
    await waitFor(() => {
      expect(frame.asked("report").pids).toEqual(["title"]);
    });
    frame.say({ pictor: "inspect", id: frame.asked("report").id, type: "report", items: [item("title")] });
    const tag = await screen.findByText("alice 的 Claude Code", { selector: ".peer-tag" });
    expect(tag.closest("[data-peer]")).toHaveClass("peerbox-agent");
    expect(tag.closest("[data-peer]")).toHaveStyle({ left: "160px", top: "170px", width: "300px", height: "80px" });
  });

  it("draws nothing for an element the frame does not show, and nothing for someone with no name", async () => {
    const { awareness } = joinRoom();
    const frame = await open();
    someoneElse(awareness, 8, { selection: { hero: ["title"] } });
    someoneElse(awareness, 7, { user: { name: "alice", color: "c3" }, selection: { hero: ["gone"] } });
    await waitFor(() => {
      expect(frame.asked("report").pids).toEqual(["gone"]);
    });
    frame.say({ pictor: "inspect", id: frame.asked("report").id, type: "report", items: [] });
    await act(() => Promise.resolve());
    expect(document.querySelector("[data-peer]")).toBeNull();
  });

  it.each(["live", "reconnecting"] as const)("keeps the header roster while showing only connection exceptions (%s)", async (state) => {
    const { awareness } = joinRoom(state);
    await open();
    someoneElse(awareness, 7, { user: { name: "alice", color: "c3" } });
    const stack = await screen.findByRole("group", { name: "Viewing now" });
    expect(within(stack).getByText("alice")).toBeInTheDocument();
    expect(within(stack).getByText("bob (You)")).toBeInTheDocument();
    const presence = document.querySelector<HTMLElement>(".canvas-presence");
    if (!presence) throw new Error("no connection light");
    if (state === "live") expect(presence).not.toBeVisible();
    else expect(within(presence).getByText("Reconnecting…")).toBeVisible();
    someoneElse(awareness, 7, null);
    expect(within(stack).queryByText("alice")).toBeNull();
  });

  it("tells the others what I selected", async () => {
    const { awareness } = joinRoom();
    const frame = await open();
    const user = userEvent.setup();
    const viewport = document.querySelector(".canvas-viewport");
    if (!(viewport instanceof HTMLElement)) throw new Error("no canvas");
    await user.pointer({ keys: "[MouseLeft]", target: viewport, coords: { clientX: 300, clientY: 300 } });
    await waitFor(() => {
      frame.asked("hit");
    });
    frame.say({ pictor: "inspect", id: frame.asked("hit").id, type: "hit", pid: "title" });
    await waitFor(() => {
      frame.asked("report");
    });
    frame.say({ pictor: "inspect", id: frame.asked("report").id, type: "report", items: [item("title")] });
    await waitFor(() => {
      expect(awareness.getLocalState()?.selection).toEqual({ hero: ["title"] });
    });
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(awareness.getLocalState()?.selection).toBeNull();
    });
  });
});
