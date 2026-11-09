import { act, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { artifact, renderIn } from "../../test-support";
import { DesignCanvas } from "../Canvas";
import { DesignEdits, ROOM_MAP } from "../edits";
import { emptyFrameEntries, LEGACY_KEY, PageStructure } from "../pages/structure";
import type { Live } from "../useRoom";

// Where the outline's place is while the room is not open yet: the canvas must not move when it opens.
const room = vi.hoisted(() => ({ edits: null as DesignEdits | null, live: null as Live | null }));
vi.mock("../useRoom", async (original) => ({
  ...(await original<typeof import("../useRoom")>()),
  useRoom: () => ({ edits: room.edits, live: room.live }),
}));

const manifest = { version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "a", name: "A", file: "frames/a.html", w: 800, h: 600 }]] }] };
const design = artifact({ title: "Landing", author: "alice", kind: "design", canChange: true, canEdit: true, role: "author" });

afterEach(() => {
  vi.unstubAllGlobals();
  room.edits = null;
  room.live = null;
});

function serve() {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ version: 1, manifest, base: "/raw/x/1.2.sig/", files: "/files/art/g.sig/" }), { status: 200 }))));
}
describe("the explicitly toggled panel", () => {
  it("starts closed while the design and room load, and opens only on request", async () => {
    serve();
    renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
    expect(document.querySelector(".side-panel")).toBeNull();
    await screen.findByRole("toolbar", { name: "Canvas" });
    const toggle = screen.getByRole("button", { name: "Properties" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  it.each([1, 2])("puts Layers with %s artboards in Properties without exposing deletion", async (count) => {
    serve();
    const doc = new Y.Doc();
    doc.transact(() => {
      doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
      for (const [key, value] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(key, value);
    });
    room.edits = new DesignEdits(doc);
    if (count === 2) new PageStructure(room.edits).addFrame("home", () => "B");
    renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
    await screen.findByRole("toolbar", { name: "Canvas" });
    expect(screen.queryByRole("navigation", { name: "Layers" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Properties" }));
    expect(screen.getByRole("navigation", { name: "Layers" }).closest(".side-panel")).not.toBeNull();
    const panel = document.querySelector(".side-panel");
    if (!(panel instanceof HTMLElement)) throw new Error("missing Properties panel");
    expect(within(panel).getByRole("button", { name: "Add artboard" })).toBeVisible();
    expect(within(panel).getByRole("button", { name: "A" })).toBeVisible();
    expect(within(panel).queryByRole("button", { name: /delete/i })).toBeNull();
    expect(within(panel).queryByText("A page keeps at least one artboard.")).toBeNull();
  });
});

describe("the Frame tool on the canvas", () => {
  it("arms Pencil with P only for an editor and puts it away with Escape", async () => {
    serve();
    const doc = new Y.Doc();
    doc.transact(() => { doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest)); for (const [k,v] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(k,v); });
    room.edits = new DesignEdits(doc);
    const {unmount} = renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
    await screen.findByRole("button", {name:"Drawing"});
    await userEvent.keyboard("p");
    expect(screen.getByRole("button", {name:"Drawing"})).toHaveAttribute("aria-pressed","true");
    expect(screen.getByRole("group", {name:"Draw a pencil stroke"})).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("group", {name:"Draw a pencil stroke"})).toBeNull();
    expect(screen.getByRole("button", {name:"Drawing"})).toHaveAttribute("aria-pressed","false");
    unmount();
    renderIn(<DesignCanvas artifact={{...design,canEdit:false}} older={null} account="alice" />, "en", `/a/${design.id}`);
    await screen.findByRole("toolbar",{name:"Canvas"});
    await userEvent.keyboard("p");expect(screen.queryByRole("button",{name:"Drawing"})).toBeNull();
  });

  it("adds a frame to the page that is open at the size dragged, and puts the tool away", async () => {
    serve();
    const doc = new Y.Doc();
    doc.transact(() => {
      doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
      for (const [k, v] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(k, v);
    });
    room.edits = new DesignEdits(doc);
    const user = userEvent.setup();
    renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
    await screen.findByRole("toolbar", { name: "Canvas" });
    await user.keyboard("1");
    await user.click(screen.getByRole("button", { name: "Artboard" }));
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    const surface = screen.getByLabelText("Draw an artboard");
    fireEvent.pointerDown(surface, { button: 0, clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.pointerMove(surface, { clientX: 500, clientY: 400, pointerId: 1 });
    fireEvent.pointerUp(surface, { button: 0, clientX: 500, clientY: 400, pointerId: 1 });
    const frames = new PageStructure(room.edits).design()?.pages[0]?.rows.flat() ?? [];
    expect(frames).toHaveLength(2);
    expect(frames[1]).toMatchObject({ name: "Artboard 2", w: 400, h: 300 });
    expect(screen.queryByLabelText("Draw an artboard")).toBeNull();
    expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute("aria-pressed", "true");
  });
});


it("selects a manually added frame rather than its elements, deletes it and restores its room entries with Undo", async () => {
  serve();
  const doc = new Y.Doc();
  doc.transact(() => {
    doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
    for (const [k, v] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(k, v);
  });
  const edits = new DesignEdits(doc);
  room.edits = edits;
  const user = userEvent.setup();
  renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
  await screen.findByRole("toolbar", { name: "Canvas" });
  await user.click(screen.getByRole("button", { name: "Properties" }));
  await user.click(screen.getByRole("button", { name: "Add artboard" }));
  const shape = new PageStructure(edits);
  const added = shape.design()?.pages[0]?.rows.flat().find((f) => f.name === "Artboard 2");
  expect(added).toBeDefined();
  if (!added) throw new Error("no added frame");
  const before = [...edits.map.entries()].sort();
  await user.click(screen.getByRole("button", { name: "Select artboard Artboard 2" }));
  expect(screen.getByRole("button", { name: "Select artboard Artboard 2" })).toHaveAttribute("aria-pressed", "true");
  expect(document.querySelector(`[data-frame="${added.id}"]`)).toHaveAttribute("data-frame-selected");
  expect(document.querySelector(".selbox")).toBeNull();
  await user.keyboard("{Delete}");
  expect(shape.design()?.pages[0]?.rows.flat().map((f) => f.id)).toEqual(["a"]);
  expect([...edits.map.keys()].some((k) => k.startsWith(added.file + "|"))).toBe(false);
  await user.keyboard("{Meta>}z{/Meta}");
  expect([...edits.map.entries()].sort()).toEqual(before);
  expect(shape.design()?.pages[0]?.rows.flat().map((f) => f.id)).toEqual(["a", added.id]);
  await user.click(screen.getByRole("button", { name: "Select artboard Artboard 2" }));
  const row = screen.getByRole("button", { name: "Artboard 2" });
  await user.click(row);
  expect(row).toHaveAttribute("aria-pressed", "true");
  const width = screen.getByRole("textbox", { name: "Width" });
  await user.click(width);
  await user.keyboard("{Delete}");
  expect(shape.design()?.pages[0]?.rows.flat()).toHaveLength(2);
  // A structural update received from a peer invalidates selection before the next key.
  act(() => { shape.deleteFrame(added.id); });
  expect(document.querySelector("[data-frame-selected]")).toBeNull();
  const afterRemote = [...edits.map.entries()].sort();
  await user.keyboard("{Escape}{Delete}");
  expect([...edits.map.entries()].sort()).toEqual(afterRemote);
  await user.click(screen.getByRole("button", { name: "Select artboard A" }));
  await user.click(screen.getByRole("button", { name: "Page: Home" }));
  await user.click(screen.getByRole("menuitem", { name: "Add page" }));
  expect(document.querySelector("[data-frame-selected]")).toBeNull();
  const afterPage = [...edits.map.entries()].sort();
  await user.keyboard("{Delete}");
  expect([...edits.map.entries()].sort()).toEqual(afterPage);
});


it("explains the last-frame constraint and refuses Delete without changing the room", async () => {
  serve();
  const doc = new Y.Doc();
  doc.transact(() => {
    doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
    for (const [k, v] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(k, v);
  });
  room.edits = new DesignEdits(doc);
  const before = [...room.edits.map.entries()];
  renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
  const label = await screen.findByRole("button", { name: "Select artboard A" });
  await userEvent.click(label);
  await userEvent.keyboard("{Delete}");
  expect(screen.getByRole("status")).toHaveTextContent("A page keeps at least one artboard.");
  expect([...room.edits.map.entries()]).toEqual(before);
  expect(room.edits.steps().undo).toBe(0);
  await userEvent.keyboard("{Escape}");
  expect(label).toHaveAttribute("aria-pressed", "false");
});

it.each([null, 2])("offers no frame mutation to a reader or a historical viewer (version %s)", async (older) => {
  serve();
  const doc = new Y.Doc();
  doc.transact(() => {
    doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
    for (const [k, v] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(k, v);
  });
  room.edits = new DesignEdits(doc);
  const before = [...room.edits.map.entries()];
  renderIn(<DesignCanvas artifact={{ ...design, canEdit: older !== null }} older={older} account="alice" />, "en", `/a/${design.id}`);
  await screen.findByRole("toolbar", { name: "Canvas" });
  expect(screen.queryByRole("button", { name: /^Select artboard/ })).toBeNull();
  expect(screen.queryByRole("button", { name: /^Delete artboard/ })).toBeNull();
  expect(screen.queryByRole("button", { name: "Add artboard" })).toBeNull();
  await userEvent.keyboard("{Delete}{Meta>}z{/Meta}");
  expect([...room.edits.map.entries()]).toEqual(before);
});

it("keeps Properties closed until explicitly toggled and closing it preserves artboard selection", async () => {
  serve();
  const doc = new Y.Doc();
  doc.transact(() => {
    doc.getMap(ROOM_MAP).set(LEGACY_KEY, JSON.stringify(manifest));
    for (const [key, value] of emptyFrameEntries("frames/a.html")) doc.getMap(ROOM_MAP).set(key, value);
  });
  room.edits = new DesignEdits(doc);
  const user = userEvent.setup();
  renderIn(<DesignCanvas artifact={design} older={null} account="alice" />, "en", `/a/${design.id}`);
  const label = await screen.findByRole("button", { name: "Select artboard A" });
  expect(screen.getByRole("button", { name: "Properties" })).toHaveAttribute("aria-pressed", "false");
  expect(screen.queryByRole("region", { name: "Properties" })).toBeNull();
  await user.click(label);
  expect(label).toHaveAttribute("aria-pressed", "true");
  expect(screen.queryByRole("textbox", { name: "Width" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Properties" }));
  expect(screen.getByRole("textbox", { name: "Width" })).toHaveValue("800");
  await user.click(screen.getByRole("button", { name: "Close properties" }));
  expect(label).toHaveAttribute("aria-pressed", "true");
  expect(screen.queryByRole("textbox", { name: "Width" })).toBeNull();
});
