import { act, createEvent, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import { DesignEdits, ROOM_MAP } from "../edits";
import type { Design } from "../manifest";
import { FramesPanel } from "./FramesPanel";
import { MAX_FRAMES, MAX_PAGES } from "./rules";
import { emptyFrameEntries, LEGACY_KEY, PageStructure, useRoomDesign } from "./structure";

const frame = (id: string, w = 800, h = 600) => ({ id, name: id.toUpperCase(), file: `frames/${id}.html`, w, h });
const legacy = (pages: unknown) => JSON.stringify({ version: 1, pages });
const HOME = [{ id: "home", name: "Start", rows: [[frame("a"), frame("b")], [frame("c")]] }, { id: "about", name: "About", rows: [[frame("d")]] }];

/** A room holding Home (a and b side by side, c below; a has a text) and About (d). It starts in the older form, one entry; the first change moves it to the entries per value, which the rename here does. */
function designRoom(pages: unknown = HOME) {
  const doc = new Y.Doc();
  const map = doc.getMap(ROOM_MAP);
  doc.transact(() => {
    map.set(LEGACY_KEY, legacy(pages));
    for (const p of pages as { rows: { file: string }[][] }[]) for (const f of p.rows.flat()) for (const [k, v] of emptyFrameEntries(f.file)) map.set(k, v);
    map.set("frames/a.html|n|t1|text", new Y.Text("Hello"));
  });
  const first = new DesignEdits(doc);
  new PageStructure(first).renamePage("home", "Home");
  first.destroy();
  // Everyone's history starts from here.
  const edits = new DesignEdits(doc);
  return { doc, map, edits, structure: new PageStructure(edits) };
}

const files = (map: Y.Map<unknown>) => [...new Set([...map.keys()].filter((k) => k.startsWith("frames/")).map((k) => k.slice(0, k.indexOf("|"))))].sort();
const shape = (d: Design | null, page = "home") => d?.pages.find((p) => p.id === page)?.rows.map((row) => row.map((f) => f.id)) ?? null;
const changed = (o: { kind: string }) => {
  expect(o.kind).toBe("changed");
};

/** Two people with the same room, each making changes in their own copy, then meeting. */
function twoPeople() {
  const base = designRoom();
  const copy = () => {
    const doc = new Y.Doc();
    Y.applyUpdate(doc, Y.encodeStateAsUpdate(base.doc));
    const edits = new DesignEdits(doc);
    return { doc, map: doc.getMap(ROOM_MAP), edits, structure: new PageStructure(edits) };
  };
  const alice = copy();
  const bob = copy();
  const meet = () => {
    Y.applyUpdate(alice.doc, Y.encodeStateAsUpdate(bob.doc));
    Y.applyUpdate(bob.doc, Y.encodeStateAsUpdate(alice.doc));
    expect(alice.structure.design()).toEqual(bob.structure.design());
    return alice.structure.design();
  };
  return { alice, bob, meet };
}

describe("a design's pages and frames in the room", () => {
  it("a room with the older single entry reads as that design, and the first change keeps all of it", () => {
    const doc = new Y.Doc();
    doc.getMap(ROOM_MAP).set(LEGACY_KEY, legacy(HOME));
    const structure = new PageStructure(new DesignEdits(doc));
    expect(shape(structure.design())).toEqual([["a", "b"], ["c"]]);
    changed(structure.renameFrame("c", "Footer"));
    expect(doc.getMap(ROOM_MAP).has(LEGACY_KEY)).toBe(false);
    expect(shape(structure.design())).toEqual([["a", "b"], ["c"]]);
    expect(structure.design()?.pages[0]?.rows[1]?.[0]).toMatchObject({ id: "c", name: "Footer", file: "frames/c.html", w: 800, h: 600 });
    expect(structure.design()?.pages.map((p) => p.name)).toEqual(["Start", "About"]);
  });

  it("a page is added with one empty frame, after the others, and Undo takes both away", () => {
    const { structure, edits, map } = designRoom();
    const out = structure.addPage({ page: (n) => `Page ${String(n)}`, frame: (n) => `Frame ${String(n)}` });
    changed(out);
    const added = structure.design()?.pages[2];
    expect(added).toMatchObject({ name: "Page 3", id: out.kind === "changed" ? out.page : "" });
    expect(added?.rows).toHaveLength(1);
    expect(added?.rows[0]?.[0]).toMatchObject({ name: "Frame 5", w: 1440, h: 900 });
    expect(files(map)).toHaveLength(5);
    expect(edits.steps().undo).toBe(1);
    edits.undo();
    expect(structure.design()?.pages.map((p) => p.id)).toEqual(["home", "about"]);
    expect(files(map)).toEqual(["frames/a.html", "frames/b.html", "frames/c.html", "frames/d.html"]);
    edits.redo();
    expect(structure.design()?.pages).toHaveLength(3);
  });

  it("deleting a page deletes its frames' files and Undo brings them back with their text", () => {
    const { structure, edits, map } = designRoom();
    changed(structure.deletePage("home"));
    expect(structure.design()?.pages.map((p) => p.id)).toEqual(["about"]);
    expect(files(map)).toEqual(["frames/d.html"]);
    edits.undo();
    expect(structure.design()?.pages.map((p) => p.id)).toEqual(["home", "about"]);
    expect(files(map)).toHaveLength(4);
    const text = map.get("frames/a.html|n|t1|text");
    expect(text instanceof Y.Text ? text.toJSON() : null).toBe("Hello");
  });

  it("deleting a frame takes its file and Undo restores it; a row left empty is not drawn", () => {
    const { structure, edits, map } = designRoom();
    changed(structure.deleteFrame("c"));
    expect(shape(structure.design())).toEqual([["a", "b"]]);
    expect(files(map)).not.toContain("frames/c.html");
    edits.undo();
    expect(shape(structure.design())).toEqual([["a", "b"], ["c"]]);
    expect(files(map)).toContain("frames/c.html");
  });

  it("each rename, resize and move is one step, writes no frame content, and Undo reverses it", () => {
    const { structure, edits, map } = designRoom();
    const before = structure.design();
    const content = files(map);
    changed(structure.renamePage("home", "Start"));
    changed(structure.renameFrame("a", "Hero"));
    changed(structure.resizeFrame("a", { w: 390, h: 844 }));
    changed(structure.moveFrame("b", { newRowBefore: 0 }));
    expect(edits.steps().undo).toBe(4);
    expect(files(map)).toEqual(content);
    expect(structure.design()?.pages[0]).toMatchObject({ name: "Start", rows: [[{ id: "b" }], [{ id: "a", name: "Hero", w: 390, h: 844 }], [{ id: "c" }]] });
    for (let i = 0; i < 4; i++) edits.undo();
    expect(structure.design()).toEqual(before);
  });

  it("refuses what the format does not allow, names the limit, and writes nothing", () => {
    const { structure, edits } = designRoom();
    for (const bad of [{ w: 8193, h: 10 }, { w: 10, h: 0 }, { w: 1.5, h: 10 }, { w: Number.NaN, h: 10 }, { w: -4, h: 10 }]) {
      expect(structure.resizeFrame("a", bad), JSON.stringify(bad)).toMatchObject({ kind: "refused", why: { rule: "side", min: 1, max: 8192 } });
    }
    expect(structure.resizeFrame("a", { w: 8192, h: 1 }).kind).toBe("changed");
    edits.undo();
    expect(structure.renamePage("home", "x".repeat(81))).toMatchObject({ kind: "refused", why: { rule: "name", limit: 80 } });
    expect(structure.renamePage("home", "é".repeat(80)).kind).toBe("changed");
    edits.undo();
    expect(structure.renamePage("home", "   ").kind).toBe("same");
    expect(structure.renamePage("home", "Home").kind).toBe("same");
    expect(structure.resizeFrame("a", { w: 800, h: 600 }).kind).toBe("same");
    expect(structure.deleteFrame("d")).toMatchObject({ kind: "refused", why: { rule: "lastFrame" } });
    expect(structure.deletePage("home").kind).toBe("changed");
    edits.undo();
    expect(edits.steps().undo).toBe(0);
    structure.deletePage("home");
    expect(structure.deletePage("about")).toMatchObject({ kind: "refused", why: { rule: "lastPage" } });
    expect(structure.renameFrame("zzz", "x")).toMatchObject({ kind: "refused", why: { rule: "missing" } });
  });

  it("refuses a twenty-first page and a sixty-first frame, naming the limit", () => {
    const pages = Array.from({ length: MAX_PAGES }, (_, i) => ({ id: `p${String(i)}`, name: `P${String(i)}`, rows: [[frame(`f${String(i)}`)]] }));
    const full = designRoom(pages);
    expect(full.structure.addPage({ page: () => "x", frame: () => "y" })).toMatchObject({ kind: "refused", why: { rule: "pages", limit: 20 } });
    const wide = designRoom([{ id: "p", name: "P", rows: [Array.from({ length: MAX_FRAMES }, (_, i) => frame(`f${String(i)}`))] }]);
    expect(wide.structure.addFrame("p", () => "more")).toMatchObject({ kind: "refused", why: { rule: "frames", limit: 60 } });
    expect(wide.structure.addPage({ page: () => "x", frame: () => "y" })).toMatchObject({ kind: "refused", why: { rule: "frames", limit: 60 } });
  });

  it("a frame dropped before another, at the end of a row or in a row of its own goes there, and a drop where it already is changes nothing", () => {
    const at = (id: string, drop: Parameters<PageStructure["moveFrame"]>[1]) => {
      const r = designRoom();
      const out = r.structure.moveFrame(id, drop);
      return { out, shape: shape(r.structure.design()), steps: r.edits.steps().undo };
    };
    expect(at("b", { before: "a" }).shape).toEqual([["b", "a"], ["c"]]);
    expect(at("a", { before: "c" }).shape).toEqual([["b"], ["a", "c"]]);
    expect(at("c", { endOf: 0 }).shape).toEqual([["a", "b", "c"]]);
    expect(at("a", { endOf: 1 }).shape).toEqual([["b"], ["c", "a"]]);
    expect(at("c", { newRowBefore: 0 }).shape).toEqual([["c"], ["a", "b"]]);
    expect(at("a", { newRowBefore: null }).shape).toEqual([["b"], ["c"], ["a"]]);
    expect(at("a", { newRowBefore: 1 }).shape).toEqual([["b"], ["a"], ["c"]]);
    for (const [id, drop] of [["a", { before: "a" }], ["a", { before: "b" }], ["b", { endOf: 0 }], ["c", { endOf: 1 }], ["c", { newRowBefore: null }], ["c", { newRowBefore: 1 }], ["a", { before: "d" }]] as const) {
      const r = at(id, drop);
      expect(r.out.kind, `${id} ${JSON.stringify(drop)}`).toBe("same");
      expect(r.steps).toBe(0);
    }
    expect(designRoom().structure.moveFrame("a", { endOf: 7 }).kind).toBe("refused");
  });

  it("a new frame never takes the file of a frame of the saved version, nor one the room still holds", () => {
    const r = designRoom();
    const saved = new PageStructure(r.edits, () => ["frames/frame-1.html"]);
    for (let i = 0; i < 20; i++) changed(saved.addFrame("home", (n) => `F${String(n)}`));
    const ids = saved.design()?.pages[0]?.rows.flat().map((f) => f.file) ?? [];
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).not.toContain("frames/frame-1.html");
  });
});

describe("two people restructuring at the same time", () => {
  it("both adding a frame keep theirs, each in its own row", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.addFrame("home", (n) => `Alice ${String(n)}`));
    changed(bob.structure.addFrame("home", (n) => `Bob ${String(n)}`));
    const d = meet();
    expect(d?.pages[0]?.rows.flat().map((f) => f.name.split(" ")[0])).toEqual(expect.arrayContaining(["Alice", "Bob"]));
    expect(d?.pages[0]?.rows.flat()).toHaveLength(5);
    expect(new Set(files(alice.map)).size).toBe(files(alice.map).length);
  });

  it("one renaming a frame while the other resizes and moves it keeps all three", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.renameFrame("a", "Banner"));
    changed(bob.structure.resizeFrame("a", { w: 390, h: 844 }));
    changed(bob.structure.moveFrame("a", { newRowBefore: null }));
    const d = meet();
    expect(shape(d)).toEqual([["b"], ["c"], ["a"]]);
    expect(d?.pages[0]?.rows[2]?.[0]).toMatchObject({ name: "Banner", w: 390, h: 844 });
  });

  it("moving different frames keeps both moves", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.moveFrame("a", { newRowBefore: null }));
    changed(bob.structure.moveFrame("c", { endOf: 0 }));
    expect(shape(meet())).toEqual([["b", "c"], ["a"]]);
  });

  it("deleting a page while the other moves one of its frames ends with the same design, without that page and what was done in it", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.deletePage("home"));
    changed(bob.structure.moveFrame("a", { newRowBefore: null }));
    changed(bob.structure.addFrame("home", (n) => `F${String(n)}`));
    const d = meet();
    expect(d?.pages.map((p) => p.id)).toEqual(["about"]);
  });

  it("deleting a frame while the other renames it deletes it", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.deleteFrame("c"));
    changed(bob.structure.renameFrame("c", "Footer"));
    expect(shape(meet())).toEqual([["a", "b"]]);
    changed(bob.structure.renameFrame("a", "Shared banner"));
    meet();
    alice.edits.undo();
    const restored = meet();
    expect(shape(restored)).toEqual([["a", "b"], ["c"]]);
    expect(restored?.pages[0]?.rows[0]?.[0]?.name).toBe("Shared banner");
    expect(files(alice.map)).toEqual(files(bob.map));
    expect(files(alice.map)).toContain("frames/c.html");
  });

  it("two frames deleted at once from a page that had two leave the page out of the design instead of breaking the room", () => {
    const { alice, bob, meet } = twoPeople();
    changed(alice.structure.moveFrame("c", { endOf: 0 }));
    meet();
    changed(alice.structure.deleteFrame("a"));
    changed(alice.structure.deleteFrame("b"));
    changed(bob.structure.deleteFrame("c"));
    const d = meet();
    expect(d?.pages.map((p) => p.id)).toEqual(["about"]);
  });
});

/** What the screen shows of the structure, with the real catalogue. */
function screenOf(options: { editable?: boolean; lang?: "en" | "zh-Hant" } = {}) {
  const { editable = true, lang = "en" } = options;
  const r = designRoom();
  const added = vi.fn();
  function Harness() {
    const design = useRoomDesign(r.edits);
    if (!design) return null;
    const page = design.pages[0];
    if (!page) return null;
    return <FramesPanel page={page} structure={editable ? r.structure : null} onAdded={added} />;
  }
  renderIn(<Harness />, lang, "/a/x");
  const item = (id: string) => {
    const el = document.querySelector(`[data-frame-item="${id}"]`);
    if (!(el instanceof HTMLElement)) throw new Error(`no frame ${id}`);
    return el;
  };
  const main = (id: string) => {
    const button = within(item(id)).getAllByRole("button")[0];
    if (!button) throw new Error(`no button for ${id}`);
    return button;
  };
  const rows = () => shape(r.structure.design());
  return { ...r, item, main, rows, added };
}

const find = (selector: string) => {
  const el = document.querySelector(selector);
  if (!(el instanceof HTMLElement)) throw new Error(`nothing matches ${selector}`);
  return el;
};
const transfer = () => ({ dataTransfer: { setData: vi.fn(), effectAllowed: "", dropEffect: "" } });
const dragAt = (type: "dragOver" | "drop", el: HTMLElement, clientY: number) => {
  const event = createEvent[type](el, transfer());
  Object.defineProperty(event, "clientY", { value: clientY });
  return fireEvent(el, event);
};
const boxAt = (el: HTMLElement) => {
  el.getBoundingClientRect = () => ({ top: 100, height: 20, bottom: 120, left: 0, right: 100, width: 100, x: 0, y: 100, toJSON: () => ({}) });
};

describe("what a person sees and does with the pages and frames", () => {
  it("a reader sees the frames and has no controls", () => {
    const { item } = screenOf({ editable: false });
    expect(within(item("a")).getByText("800 × 600")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Add/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Delete/ })).toBeNull();
    expect(item("a")).not.toHaveAttribute("draggable", "true");
  });

  it("adding a frame opens its size and tells the canvas; the width and height are set by Enter or by leaving the box, and Undo puts the size back", async () => {
    const { rows, added, main, structure, edits } = screenOf();
    await userEvent.click(screen.getByRole("button", { name: "Add artboard" }));
    expect(rows()?.length).toBe(3);
    expect(added).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("textbox", { name: "Width" })).toHaveValue("1440");
    await userEvent.click(main("a"));
    const w = screen.getByRole("textbox", { name: "Width" });
    await userEvent.clear(w);
    await userEvent.type(w, "1000{Enter}");
    expect(structure.design()?.pages[0]?.rows[0]?.[0]).toMatchObject({ w: 1000, h: 600 });
    const h = screen.getByRole("textbox", { name: "Height" });
    await userEvent.clear(h);
    await userEvent.type(h, "700");
    await userEvent.tab();
    expect(structure.design()?.pages[0]?.rows[0]?.[0]).toMatchObject({ w: 1000, h: 700 });
    act(() => {
      edits.undo();
    });
    expect(structure.design()?.pages[0]?.rows[0]?.[0]).toMatchObject({ w: 1000, h: 600 });
  });

  it("a size over the limit is refused with the limit named, a good size clears it, and a preset sets both sides", async () => {
    const { main, edits, structure } = screenOf();
    await userEvent.click(main("a"));
    const w = screen.getByRole("textbox", { name: "Width" });
    for (const bad of ["9000", "0", "abc"]) {
      await userEvent.clear(w);
      await userEvent.type(w, `${bad}{Enter}`);
      expect(screen.getByRole("alert")).toHaveTextContent("Width must be a whole number of pixels from 1 to 8192.");
    }
    expect(edits.steps().undo).toBe(0);
    await userEvent.clear(w);
    await userEvent.type(w, "8192{Enter}");
    expect(screen.queryByRole("alert")).toBeNull();
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Size preset" }), "Phone 390 × 844");
    expect(structure.design()?.pages[0]?.rows[0]?.[0]).toMatchObject({ w: 390, h: 844 });
  });

  it("frames are renamed by double-click or F2, deleted (not the last of a page), and Alt+Down moves one within its row", async () => {
    const { structure, main, rows } = screenOf();
    await userEvent.dblClick(main("a"));
    const box = screen.getByRole("textbox", { name: "Name of artboard A" });
    await userEvent.clear(box);
    await userEvent.type(box, "Hero{Enter}");
    expect(structure.design()?.pages[0]?.rows[0]?.[0]).toMatchObject({ id: "a", name: "Hero", file: "frames/a.html" });
    main("a").focus();
    await userEvent.keyboard("{F2}");
    expect(screen.getByRole("textbox", { name: "Name of artboard Hero" })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    main("a").focus();
    await userEvent.keyboard("{Alt>}{ArrowDown}{/Alt}");
    expect(rows()).toEqual([["b", "a"], ["c"]]);
    await userEvent.click(screen.getByRole("button", { name: "Delete artboard C" }));
    expect(rows()).toEqual([["b", "a"]]);
    expect(screen.queryByRole("button", { name: "Delete artboard B" })).not.toBeNull();
  });

  it("dragging a frame onto the top or bottom half of another, onto a row, or into the gap between rows moves it there, one step each", () => {
    const { rows, item, edits } = screenOf();
    const drag = (id: string) => fireEvent.dragStart(item(id), transfer());
    drag("b");
    boxAt(item("a"));
    dragAt("drop", item("a"), 102);
    expect(rows()).toEqual([["b", "a"], ["c"]]);
    drag("b");
    boxAt(item("a"));
    dragAt("drop", item("a"), 118);
    expect(rows()).toEqual([["a", "b"], ["c"]]);
    expect(edits.steps().undo).toBe(2);
    drag("c");
    dragAt("drop", find('[data-row-index="0"]'), 0);
    expect(rows()).toEqual([["a", "b", "c"]]);
    drag("a");
    expect(screen.getAllByText("New row").length).toBeGreaterThan(0);
    dragAt("drop", find('[data-newrow="end"]'), 0);
    expect(rows()).toEqual([["b", "c"], ["a"]]);
  });

  it("speaks Chinese when the language is", () => {
    screenOf({ lang: "zh-Hant" });
    expect(screen.getByRole("button", { name: "新增畫板" })).toBeInTheDocument();
  });
});
