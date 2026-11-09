import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import type { Artifact } from "../api";
import { artifact, renderIn } from "../test-support";
import { DesignCanvas } from "./Canvas";
import { DesignEdits, ROOM_MAP } from "./edits";
import { emptyFrameEntries, LEGACY_KEY } from "./pages/structure";
import { FILE, key, seed } from "./test-room";
import type { Live } from "./useRoom";

// A new version of a design while the canvas is open: someone in the room keeps
// the frames they see (the room already showed them the version), and a frame is
// loaded again only for what the room changed that an open frame cannot show.
const room = vi.hoisted(() => ({ edits: null as DesignEdits | null, live: null as Live | null }));
vi.mock("./useRoom", async (original) => ({
  ...(await original<typeof import("./useRoom")>()),
  useRoom: () => ({ edits: room.edits, live: room.live }),
}));

const manifest = { version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: FILE, w: 800, h: 600 }]] }] };
const design = artifact({ title: "Landing", author: "alice", kind: "design", canChange: true, canEdit: true, role: "author" });
const TITLE = "Hero, 800 by 600";

afterEach(() => {
  vi.unstubAllGlobals();
  room.edits = null;
  room.live = null;
});

function openRoom() {
  const doc = new Y.Doc();
  seed(doc);
  doc.transact(() => {
    const map = doc.getMap(ROOM_MAP);
    map.set(LEGACY_KEY, JSON.stringify(manifest));
    map.set(key("h1", "tag"), "h1");
    map.set(key("para", "tag"), "p");
  });
  room.edits = new DesignEdits(doc);
  return doc.getMap(ROOM_MAP);
}

/**
 * The canvas, with a way to tell it that a newer version exists, as the viewer
 * does after asking what changed. Each read of the design hands out a version
 * address and, with `live`, a live address that differs every time it is read,
 * as a fresh grant can.
 */
async function open({ live = true, older = null, account = "alice" }: { live?: boolean; older?: number | null; account?: string | null } = {}) {
  let version = 1;
  let reads = 0;
  let withLive = live;
  const fetchMock = vi.fn(() => {
    reads++;
    const data = { version, manifest, base: `/raw/x/${String(version)}.sig/`, files: "/files/art/g.sig/", ...(withLive ? { live: `/raw/x/live.${String(reads)}/` } : {}) };
    return Promise.resolve(new Response(JSON.stringify(data), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);
  let bump: (a: Artifact) => void = () => undefined;
  function Harness() {
    const [shown, setShown] = useState(design);
    bump = setShown;
    return <DesignCanvas artifact={shown} older={older} {...(account === null ? {} : { account })} />;
  }
  renderIn(<Harness />, "en", `/a/${design.id}`);
  await screen.findByRole("toolbar", { name: "Canvas" });
  const settle = async () => {
    await act(async () => {
      await Promise.resolve();
    });
  };
  const newVersion = async () => {
    version = 2;
    const before = fetchMock.mock.calls.length;
    act(() => {
      bump({ ...design, version: 2 });
    });
    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBe(before + 1);
    });
    await settle();
  };
  const rerender = () => {
    act(() => {
      bump({ ...design });
    });
  };
  const loseLive = () => {
    withLive = false;
  };
  return { newVersion, settle, rerender, loseLive, reads: () => fetchMock.mock.calls.length, asked: () => (fetchMock.mock.calls as unknown as [string][]).map(([u]) => u) };
}

function frameDouble() {
  const el = screen.getByTitle(TITLE);
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
  const answer = async (type: string, reply: object) => {
    await waitFor(() => {
      asked(type);
    });
    say({ pictor: "inspect", id: asked(type).id, type, ...reply });
  };
  say({ pictor: "inspect", type: "ready", pids: ["b", "box", "para", "h1"] });
  return { el, say, answer };
}

const item = (pid: string) => ({ pid, tag: "h1", rect: { x: 40, y: 50, w: 300, h: 80 }, attrs: [["data-pid", pid]], inline: [], computed: {} });

async function selectHeading(hero: ReturnType<typeof frameDouble>) {
  await userEvent.click(screen.getByRole("button", { name: "Properties" }));
  const row = document.querySelector('[data-row="hero/h1"] .orow-main');
  if (!(row instanceof HTMLElement)) throw new Error("no outline row");
  await userEvent.click(row);
  await hero.answer("report", { items: [item("h1")] });
  await waitFor(() => {
    expect(document.querySelector('[data-selection="h1"]')).not.toBeNull();
  });
}

/** A double-click in the middle of the frame, where the canvas has drawn it. */
function doubleClickFrame() {
  const world = document.querySelector(".canvas-world")?.getAttribute("style") ?? "";
  const [, x = "0", y = "0", z = "1"] = /translate\(([-\d.]+)px, ([-\d.]+)px\) scale\(([\d.]+)\)/.exec(world) ?? [];
  const box = document.querySelector('[data-frame="hero"]');
  if (!(box instanceof HTMLElement)) throw new Error("no frame box");
  const left = parseFloat(box.style.left) + 100;
  const top = parseFloat(box.style.top) + 100;
  const viewport = document.querySelector(".canvas-viewport");
  if (!viewport) throw new Error("no canvas");
  fireEvent.doubleClick(viewport, { button: 0, clientX: Number(x) + left * Number(z), clientY: Number(y) + top * Number(z) });
}

const textOf = (map: Y.Map<unknown>, pid: string) => (map.get(key(pid, "text")) as Y.Text).toJSON();

describe("a new version while the room is open", () => {
  it("keeps the frame and what a live editor selected when the room already holds the new version", async () => {
    openRoom();
    const { newVersion } = await open();
    const hero = frameDouble();
    expect(hero.el).toHaveAttribute("src", `/raw/x/live.1/${FILE}`);
    await selectHeading(hero);

    await newVersion();

    expect(screen.getByTitle(TITLE)).toBe(hero.el);
    expect(hero.el).toHaveAttribute("src", `/raw/x/live.1/${FILE}`);
    expect(document.querySelector('[data-selection="h1"]')).not.toBeNull();
  });

  it("keeps a text edit that started just before the version, and writes what was typed into the room", async () => {
    const map = openRoom();
    const { newVersion } = await open();
    const hero = frameDouble();
    doubleClickFrame();
    await hero.answer("chain", { pids: ["~t1", "para"] });
    await hero.answer("edit", { ok: true });

    await newVersion();
    hero.say({ pictor: "inspect", type: "edited", pid: "para", runs: [{ s: "Hello there", f: [] }] });

    expect(screen.getByTitle(TITLE)).toBe(hero.el);
    expect(textOf(map, "~t1")).toBe("Hello there");
  });

  it("does not load the frame again for what it shows as it changes: a style, a name in the outline, a text", async () => {
    const map = openRoom();
    const { settle } = await open();
    const hero = frameDouble();
    act(() => {
      map.set(key("box", "s|left"), "90px");
      map.set(key("box", "attrs"), JSON.stringify([["", "data-name", "Card"]]));
      (map.get(key("~t1", "text")) as Y.Text).insert(5, "!");
    });
    await settle();
    expect(screen.getByTitle(TITLE)).toBe(hero.el);
  });

  it("loads a frame again, from a freshly read live address, when the room changes what an open frame cannot show", async () => {
    const map = openRoom();
    const { settle, reads } = await open();
    const hero = frameDouble();
    const before = reads();
    act(() => {
      // A restore by someone else, or an agent's change to the frame's head: the styles of the page itself.
      map.set(`${FILE}|shell|pre`, "<head><style>h1{color:red}</style></head>");
    });
    await waitFor(() => {
      expect(screen.getByTitle(TITLE)).not.toBe(hero.el);
    });
    await settle();
    expect(reads()).toBe(before + 1);
    expect(screen.getByTitle(TITLE)).toHaveAttribute("src", `/raw/x/live.${String(before + 1)}/${FILE}`);
  });
});

describe("when the frames and the room do not arrive together", () => {
  it("loads a frame that was loaded before the room arrived once more, since the room may have changed it in between, and what it was being told is no error", async () => {
    const errors = vi.spyOn(console, "error");
    const { settle, rerender, reads } = await open();
    const hero = frameDouble();
    expect(hero.el).toHaveAttribute("src", `/raw/x/live.1/${FILE}`);
    openRoom();
    rerender();
    await waitFor(() => {
      expect(screen.getByTitle(TITLE)).not.toBe(hero.el);
    });
    await settle();
    const again = screen.getByTitle(TITLE);
    expect(again).toHaveAttribute("src", `/raw/x/live.${String(reads())}/${FILE}`);
    rerender();
    await settle();
    expect(screen.getByTitle(TITLE)).toBe(again);
    expect(errors).not.toHaveBeenCalled();
  });

  it("keeps the frame it has when the live address can no longer be read, rather than load it from nowhere", async () => {
    const map = openRoom();
    const { settle, reads, loseLive } = await open();
    const hero = frameDouble();
    loseLive();
    const before = reads();
    act(() => {
      map.set(`${FILE}|shell|pre`, "<head><style>h1{color:red}</style></head>");
    });
    await waitFor(() => {
      expect(reads()).toBe(before + 1);
    });
    await settle();
    expect(screen.getByTitle(TITLE)).toBe(hero.el);
    expect(hero.el).toHaveAttribute("src", `/raw/x/live.1/${FILE}`);
  });
});

describe("a frame added in the room", () => {
  it("is asked for and loaded at the live address, which is the only one that has it before a version does", async () => {
    const map = openRoom();
    const { asked } = await open();
    act(() => {
      map.set(LEGACY_KEY, JSON.stringify({ ...manifest, pages: [{ ...manifest.pages[0], rows: [[...(manifest.pages[0]?.rows[0] ?? []), { id: "fresh", name: "Fresh", file: "frames/fresh.html", w: 300, h: 200 }]] }] }));
      for (const [k, v] of emptyFrameEntries("frames/fresh.html")) map.set(k, v);
    });
    const fresh = await screen.findByTitle("Fresh, 300 by 200");
    expect(fresh).toHaveAttribute("src", "/raw/x/live.1/frames/fresh.html");
    expect(asked()).toContain("/raw/x/live.1/frames/fresh.html");
    expect(asked().some((u) => u.startsWith("/raw/x/1.sig/frames/fresh"))).toBe(false);
  });
});

describe("a new version for someone outside the room", () => {
  it("shows the new version to a viewer who is given no live address", async () => {
    const { newVersion } = await open({ live: false, account: null });
    const hero = frameDouble();
    expect(hero.el).toHaveAttribute("src", `/raw/x/1.sig/${FILE}`);
    await newVersion();
    expect(screen.getByTitle(TITLE)).toHaveAttribute("src", `/raw/x/2.sig/${FILE}`);
  });

  it("shows an older version from that version's address, not the live one", async () => {
    openRoom();
    await open({ older: 1 });
    expect(screen.getByTitle(TITLE)).toHaveAttribute("src", `/raw/x/1.sig/${FILE}`);
  });
});
