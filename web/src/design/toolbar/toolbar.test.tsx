import { act, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { ShellBar } from "./ShellBar";
import { DesignEdits, ROOM_MAP } from "../edits";
import { emptyFrameEntries, LEGACY_KEY, PageStructure, useRoomDesign } from "../pages/structure";
import { FrameDraw } from "./FrameDraw";
import { PageMenu } from "./PageMenu";
import type { Tool } from "./tools";

async function pageActions() {
  const name = screen.getByRole("button", { name: /^Page: / }).getAttribute("aria-label")?.slice(6) ?? "";
  await userEvent.hover(screen.getByRole("menuitem", { name: `More actions for ${name}` }));
  await screen.findByRole("menuitem", { name: "Copy link to page" });
}

const frame = (id: string, w = 800, h = 600) => ({ id, name: id.toUpperCase(), file: `frames/${id}.html`, w, h });

/** A room holding Home (one frame) and About (one frame), in the form the first change migrates. */
function designRoom() {
  const doc = new Y.Doc();
  const map = doc.getMap(ROOM_MAP);
  const pages = [
    { id: "home", name: "Home", rows: [[frame("a")]] },
    { id: "about", name: "About", rows: [[frame("b")]] },
  ];
  doc.transact(() => {
    map.set(LEGACY_KEY, JSON.stringify({ version: 1, pages }));
    for (const p of pages) for (const f of p.rows.flat()) for (const [k, v] of emptyFrameEntries(f.file)) map.set(k, v);
  });
  const first = new DesignEdits(doc);
  new PageStructure(first).renamePage("home", "Home");
  first.destroy();
  const edits = new DesignEdits(doc);
  return { doc, edits, structure: new PageStructure(edits) };
}

/** The page menu on the room's pages, with the active page kept as the canvas keeps it. */
function pageMenu(options: { editable?: boolean } = {}) {
  const { editable = true } = options;
  const r = designRoom();
  function Harness() {
    const design = useRoomDesign(r.edits);
    const [active, setActive] = useState("home");
    if (!design) return null;
    return (
      <>
        <PageMenu pages={design.pages} active={design.pages.find((p) => p.id === active)?.id ?? "home"} onSelect={setActive} structure={editable ? r.structure : null} />
        <output data-testid="active">{active}</output>
        <output data-testid="names">{design.pages.map((p) => p.name).join(",")}</output>
      </>
    );
  }
  renderIn(<Harness />, "en", "/a/x");
  const names = () => screen.getByTestId("names").textContent;
  const open = async () => {
    await userEvent.click(screen.getByRole("button", { name: /^Page: / }));
  };
  return { ...r, names, open };
}

describe("the page menu", () => {
  it("switches to the page chosen, and names the page that is open", async () => {
    const { open } = pageMenu();
    expect(screen.getByRole("button", { name: "Page: Home" })).toBeInTheDocument();
    await open();
    await userEvent.click(screen.getByRole("menuitemradio", { name: "About" }));
    expect(screen.getByTestId("active")).toHaveTextContent("about");
    expect(screen.getByRole("button", { name: "Page: About" })).toBeInTheDocument();
  });

  it("opens with the keyboard, switches pages and cancels renaming without losing the keyboard", async () => {
    pageMenu();
    const trigger = screen.getByRole("button", { name: "Page: Home" });
    trigger.focus();
    await userEvent.keyboard("{Enter}{ArrowDown}{ArrowDown}{Enter}");
    expect(screen.getByRole("button", { name: "Page: About" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Page: About" }));
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename page" }));
    await userEvent.type(screen.getByRole("textbox"), " changed{Escape}");
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByRole("button", { name: "Page: About" })).toHaveFocus();
  });

  it("adds a page with its first frame, opens it, and Undo takes it away", async () => {
    const { open, names, structure, edits } = pageMenu();
    await open();
    await userEvent.click(screen.getByRole("menuitem", { name: "Add page" }));
    expect(names()).toBe("Home,About,Page 3");
    expect(screen.getByTestId("active")).toHaveTextContent(structure.design()?.pages[2]?.id ?? "?");
    act(() => {
      edits.undo();
    });
    expect(names()).toBe("Home,About");
  });

  it("renames the open page where its button is, and a name over the limit is refused with the limit named", async () => {
    const { open, names } = pageMenu();
    await open();
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename page" }));
    const box = screen.getByRole("textbox", { name: "Name of page Home" });
    await userEvent.clear(box);
    await userEvent.type(box, "Start{Enter}");
    expect(names()).toBe("Start,About");
    expect(screen.getByRole("button", { name: "Page: Start" })).toBeInTheDocument();
    await open();
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename page" }));
    const long = screen.getByRole("textbox", { name: "Name of page Start" });
    await userEvent.clear(long);
    await userEvent.type(long, "y".repeat(81));
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("alert")).toHaveTextContent("A name is at most 80 characters.");
    expect(names()).toBe("Start,About");
  });

  it("deleting a page asks once and says how many frames go; Cancel and Escape leave it, Delete removes it and Undo brings it back", async () => {
    const { open, names, edits } = pageMenu();
    await open();
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Delete page" }));
    const ask = screen.getByRole("alertdialog");
    expect(ask).toHaveTextContent('Delete page "Home" and its artboard?');
    await userEvent.click(within(ask).getByRole("button", { name: "Cancel" }));
    expect(names()).toBe("Home,About");
    await open();
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Delete page" }));
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    await open();
    await pageActions();
    await userEvent.click(screen.getByRole("menuitem", { name: "Delete page" }));
    await userEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    expect(names()).toBe("About");
    expect(screen.getByTestId("active")).toHaveTextContent("about");
    await open();
    await pageActions();
    expect(screen.getByRole("menuitem", { name: "Delete page" })).toHaveAttribute("aria-disabled", "true");
    await userEvent.keyboard("{Escape}");
    act(() => {
      edits.undo();
    });
    expect(names()).toBe("Home,About");
  });

  it("says the page limit when a page past it is asked for", async () => {
    const r = designRoom();
    for (let i = 0; i < 18; i++) r.structure.addPage({ page: (n) => `P${String(n)}`, frame: (n) => `F${String(n)}` });
    function Harness() {
      const design = useRoomDesign(r.edits);
      return design ? <PageMenu pages={design.pages} active="home" onSelect={() => undefined} structure={r.structure} /> : null;
    }
    renderIn(<Harness />, "en", "/a/x");
    await userEvent.click(screen.getByRole("button", { name: /^Page: / }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Add page" }));
    expect(screen.getByRole("alert")).toHaveTextContent("A design has at most 20 pages.");
  });

  it("gives someone who may not change the design only the switch", async () => {
    const { open } = pageMenu({ editable: false });
    await open();
    expect(screen.getAllByRole("menuitemradio").map((i) => i.textContent)).toEqual(["Home", "About"]);
    await pageActions();
    expect(screen.getByRole("menuitem", { name: "Copy link to page" })).toBeVisible();
    expect(screen.queryByRole("menuitem", { name: "Rename page" })).toBeNull();
  });
});

/** The bar with everything a person can have; what a reader has is `reader`. */
const pages = [
  { id: "home", name: "Home", rows: [[frame("a")]] },
  { id: "about", name: "About", rows: [[frame("b")]] },
];
function bar(over: Partial<React.ComponentProps<typeof ShellBar>> = {}) {
  const calls = { tool: vi.fn<(tool: Tool) => void>(), present: vi.fn(), panel: vi.fn(), step: vi.fn(), reset: vi.fn(), fit: vi.fn() };
  const r = designRoom();
  renderIn(
    <ShellBar
      ready
      tool="select"
      onTool={calls.tool}
      page={{ pages, active: "home", onSelect: () => undefined, structure: r.structure }}
      onPicture={() => undefined}
      frames
      onPresent={calls.present}
      panel={{ open: true, onToggle: calls.panel }}
      zoom={{ percent: 128, onStep: calls.step, onReset: calls.reset, onFit: calls.fit, onPreset: calls.reset }}
      {...over}
    />,
    "en",
    "/a/x",
  );
  const names = (name: string) => within(screen.getByRole("toolbar", { name })).getAllByRole("button").map((b) => b.getAttribute("aria-label"));
  return { calls, names };
}

describe("the floating bar", () => {
  it("lists Page, select, hand, text, frame and shape on the left and present, panel and zoom on the right for an editor", () => {
    const { names } = bar();
    expect(names("Canvas")).toEqual(["Page: Home", "Select", "Hand tool", "Text", "Artboard", "Drawing", "Shape"]);
    expect(names("View")).toEqual(["Present", "Properties", "Zoom 128%"]);
  });

  it("gives a reader navigation and inspection controls with nothing to insert", () => {
    const { names } = bar({ onPicture: null, frames: false, page: { pages, active: "home", onSelect: () => undefined, structure: null } });
    expect(names("Canvas")).toEqual(["Page: Home", "Select", "Hand tool"]);
    expect(names("View")).toEqual(["Present", "Properties", "Zoom 128%"]);
  });

  it("leaves the page menu out for a reader of a one-page design", () => {
    const { names } = bar({ onPicture: null, frames: false, page: { pages: pages.slice(0, 1), active: "home", onSelect: () => undefined, structure: null } });
    expect(names("Canvas")).toEqual(["Page: Home", "Select", "Hand tool"]);
  });

  it("arms a tool, and pressing the armed one puts it away", async () => {
    const { calls } = bar({ tool: "frame" });
    expect(screen.getByRole("button", { name: "Artboard" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(screen.getByRole("button", { name: "Text" }));
    expect(calls.tool).toHaveBeenLastCalledWith("text");
    await userEvent.click(screen.getByRole("button", { name: "Artboard" }));
    expect(calls.tool).toHaveBeenLastCalledWith("select");
  });

  it("arms a shape from the shape menu, which also holds the image", async () => {
    const { calls } = bar();
    await userEvent.click(screen.getByRole("button", { name: "Shape" }));
    expect(screen.getAllByRole("menuitem").map((i) => i.textContent.replace(/[A-Z]$/, ""))).toEqual(["Rectangle", "Oval", "ArrowShift+", "Line", "Image"]);
    await userEvent.click(screen.getByRole("menuitem", { name: "Oval" }));
    expect(calls.tool).toHaveBeenLastCalledWith("oval");
  });

  it("presents, toggles the panel and zooms from the right of the bar", async () => {
    const { calls } = bar();
    await userEvent.click(screen.getByRole("button", { name: "Present" }));
    expect(calls.present).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Properties" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(screen.getByRole("button", { name: "Properties" }));
    expect(calls.panel).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Zoom 128%" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Zoom in" }));
    expect(calls.step).toHaveBeenLastCalledWith(1);
    await userEvent.click(screen.getByRole("button", { name: "Zoom 128%" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Fit" }));
    expect(calls.fit).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Zoom 128%" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "100%" }));
    expect(calls.reset).toHaveBeenCalledTimes(1);
  });

  it("cannot present a page that has no frame to show", () => {
    bar({ onPresent: null });
    expect(screen.getByRole("button", { name: "Present" })).toBeDisabled();
  });

  it("speaks Chinese when the language is", () => {
    const r = designRoom();
    renderIn(
      <ShellBar
        ready
        tool="select"
        onTool={() => undefined}
        page={{ pages, active: "home", onSelect: () => undefined, structure: r.structure }}
        onPicture={() => undefined}
        frames
        onPresent={() => undefined}
        panel={{ open: false, onToggle: () => undefined }}
        zoom={{ percent: 100, onStep: () => undefined, onReset: () => undefined, onFit: () => undefined, onPreset: () => undefined }}
      />,
      "zh-Hant",
      "/a/x",
    );
    expect(screen.getByRole("button", { name: "簡報模式" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "頁面：Home" })).toBeInTheDocument();
  });
});

describe("drawing a frame", () => {
  const draw = (from: [number, number], to: [number, number], zoom = 0.5) => {
    const onDraw = vi.fn();
    renderIn(<FrameDraw zoom={zoom} onDraw={onDraw} />, "en", "/a/x");
    const surface = screen.getByLabelText("Draw an artboard");
    fireEvent.pointerDown(surface, { button: 0, clientX: from[0], clientY: from[1], pointerId: 1 });
    fireEvent.pointerMove(surface, { clientX: to[0], clientY: to[1], pointerId: 1 });
    fireEvent.pointerUp(surface, { button: 0, clientX: to[0], clientY: to[1], pointerId: 1 });
    return onDraw;
  };

  it.each([
    ["down and right", [100, 100], [300, 250]],
    ["up and left", [300, 250], [100, 100]],
  ] as const)("hands over the size that was dragged %s, in the canvas's pixels (the drag on screen divided by the zoom)", (_, from, to) => {
    expect(draw([...from], [...to])).toHaveBeenCalledWith({ w: 400, h: 300 });
  });

  it("hands over no size for a click, so the frame gets the usual one", () => {
    expect(draw([100, 100], [102, 101])).toHaveBeenCalledWith(null);
  });

  it("shows the box while it is dragged and nothing after", () => {
    const onDraw = vi.fn();
    renderIn(<FrameDraw zoom={1} onDraw={onDraw} />, "en", "/a/x");
    const surface = screen.getByLabelText("Draw an artboard");
    fireEvent.pointerDown(surface, { button: 0, clientX: 10, clientY: 10, pointerId: 1 });
    fireEvent.pointerMove(surface, { clientX: 110, clientY: 60, pointerId: 1 });
    expect(document.querySelector("[data-drawn]")).toHaveTextContent("100 × 50");
    fireEvent.pointerUp(surface, { button: 0, clientX: 110, clientY: 60, pointerId: 1 });
    expect(document.querySelector("[data-drawn]")).toBeNull();
  });
});

describe("a frame drawn on the current page", () => {
  it("is added to that page at the size given, as one step that Undo takes back", () => {
    const { structure, edits } = designRoom();
    const out = structure.addFrame("about", (n) => `Frame ${String(n)}`, undefined, { w: 400, h: 300 });
    expect(out.kind).toBe("changed");
    const added = structure.design()?.pages.find((p) => p.id === "about")?.rows.flat().find((f) => f.name === "Frame 3");
    expect(added).toMatchObject({ w: 400, h: 300 });
    expect(structure.design()?.pages.find((p) => p.id === "home")?.rows.flat()).toHaveLength(1);
    act(() => {
      edits.undo();
    });
    expect(structure.design()?.pages.find((p) => p.id === "about")?.rows.flat()).toHaveLength(1);
  });

  it("is refused, with nothing written, when a side is outside the limits", () => {
    const { structure, edits } = designRoom();
    expect(structure.addFrame("about", (n) => `Frame ${String(n)}`, undefined, { w: 9000, h: 300 })).toMatchObject({ kind: "refused", why: { rule: "side", field: "w" } });
    expect(edits.steps().undo).toBe(0);
  });
});

it("renames and deletes an inactive page without switching away, and Undo restores its artboard", async () => {
  const { open, names, edits, structure } = pageMenu();
  await open();
  await userEvent.hover(screen.getByRole("menuitem", { name: "More actions for About" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Rename page" }));
  const input = screen.getByRole("textbox", { name: "Name of page About" });
  await userEvent.clear(input);
  await userEvent.type(input, "Details{Enter}");
  expect(names()).toBe("Home,Details");
  expect(screen.getByTestId("active")).toHaveTextContent("home");
  const before = structure.design()?.pages[1];
  await open();
  await userEvent.hover(screen.getByRole("menuitem", { name: "More actions for Details" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Delete page" }));
  await userEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
  expect(names()).toBe("Home");
  expect(screen.getByTestId("active")).toHaveTextContent("home");
  act(() => { edits.undo(); });
  expect(structure.design()?.pages[1]).toEqual(before);
});


it.each(["Delete page", "Rename page"])("cancels %s when a peer removes its inactive target without changing Home", async (action) => {
  const { doc, edits, structure, open, names } = pageMenu();
  act(() => { structure.addPage({ page: () => "Pricing", frame: () => "Price" }); });
  const peerDoc = new Y.Doc();
  Y.applyUpdate(peerDoc, Y.encodeStateAsUpdate(doc));
  const peerEdits = new DesignEdits(peerDoc);
  const peer = new PageStructure(peerEdits);
  const before = structure.design()?.pages.find((page) => page.id === "home");
  const steps = edits.steps().undo;
  await open();
  await userEvent.hover(screen.getByRole("menuitem", { name: "More actions for About" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: action }));
  if (action === "Rename page") await userEvent.clear(screen.getByRole("textbox", { name: "Name of page About" }));
  if (action === "Rename page") await userEvent.type(screen.getByRole("textbox"), "New About");
  act(() => {
    expect(peer.deletePage("about").kind).toBe("changed");
    Y.applyUpdate(doc, Y.encodeStateAsUpdate(peerDoc));
  });
  expect(screen.queryByRole("alertdialog")).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(names()).toBe("Home,Pricing");
  expect(structure.design()?.pages.find((page) => page.id === "home")).toEqual(before);
  expect(screen.getByTestId("active")).toHaveTextContent("home");
  expect(edits.steps().undo).toBe(steps);
  peerEdits.destroy(); peerDoc.destroy();
});
