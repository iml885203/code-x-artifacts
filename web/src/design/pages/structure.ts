// The pages and frames in the design's room (docs/design-room.md). They are not
// one entry but one entry per value, so that people restructuring at the same
// time merge instead of replacing each other:
//
//   s|page|<id>|order   the page exists; its place among the pages
//   s|page|<id>|name
//   s|row|<id>|page     the row exists; the page it is on
//   s|row|<id>|order    its place among the page's rows
//   s|frame|<id>|file   the frame exists; its file
//   s|frame|<id>|name, |w, |h
//   s|frame|<id>|row    the row it is in
//   s|frame|<id>|order  its place in the row
//
// Orders are the fractional keys of the node tree. design.json is the projection
// of these entries (internal/designroom/structure.go does the same): a row with
// no frame left is not drawn, and what has lost the thing it hangs on is not part
// of the design. Each change writes only the values it changes, with the frames'
// content, in one step of this person's history, so Undo brings a deleted page
// back with every frame's content, and what someone else did meanwhile stays.
import { useCallback, useMemo, useSyncExternalStore } from "react";
import type * as Y from "yjs";
import type { DesignEdits } from "../edits";
import { readDesign, type Design } from "../manifest";
import { between } from "../tree";
import { cleanName, numbered, MAX_FRAMES, MAX_PAGES, PRESETS, refused, same, sideProblem, type Outcome } from "./rules";

const PAGE = "s|page|";
const ROW = "s|row|";
const FRAME = "s|frame|";
/** The older form: the whole design.json as one entry. A room that still has it is read as it says and moved to the entries below by the first change. */
export const LEGACY_KEY = "manifest";

interface SFrame {
  id: string;
  name: string;
  file: string;
  w: number;
  h: number;
  row: string;
  order: string;
}
interface SRow {
  id: string;
  order: string;
  frames: SFrame[];
}
interface SPage {
  id: string;
  name: string;
  order: string;
  rows: SRow[];
}
interface Struct {
  pages: SPage[];
  /** Frames that exist but hang on a row or page that is gone. */
  orphans: SFrame[];
  /** The ids and files in use anywhere in the room, so a new one is never one of them. */
  ids: Set<string>;
  files: Set<string>;
  legacy: boolean;
}

const str = (v: unknown) => (typeof v === "string" ? v : "");
const byOrder = <T extends { id: string; order: string }>(list: T[]) => list.sort((a, b) => (a.order === b.order ? (a.id < b.id ? -1 : 1) : a.order < b.order ? -1 : 1));

/** Frames with nothing in the room, as the server projects them: one without content cannot be drawn. */
function readEntries(map: Y.Map<unknown>): Struct | null {
  const pages = new Map<string, { name: string; order: string; live: boolean }>();
  const rows = new Map<string, { page: string; order: string; live: boolean }>();
  const frames = new Map<string, { file: string; name: string; w: string; h: string; row: string; order: string; live: boolean }>();
  const ids = new Set<string>();
  const files = new Set<string>();
  const seen = { any: false };
  map.forEach((value, key) => {
    if (key.startsWith("frames/")) files.add(key.slice(0, key.indexOf("|")));
    const kind = key.startsWith(PAGE) ? PAGE : key.startsWith(ROW) ? ROW : key.startsWith(FRAME) ? FRAME : "";
    if (!kind) return;
    const rest = key.slice(kind.length);
    const bar = rest.indexOf("|");
    if (bar < 0) return;
    seen.any = true;
    const id = rest.slice(0, bar);
    const field = rest.slice(bar + 1);
    ids.add(id);
    const s = str(value);
    if (kind === PAGE) {
      const p = pages.get(id) ?? { name: "", order: "", live: false };
      pages.set(id, p);
      if (field === "order") Object.assign(p, { order: s, live: true });
      else if (field === "name") p.name = s;
    } else if (kind === ROW) {
      const r = rows.get(id) ?? { page: "", order: "", live: false };
      rows.set(id, r);
      if (field === "page") Object.assign(r, { page: s, live: true });
      else if (field === "order") r.order = s;
    } else {
      const f = frames.get(id) ?? { file: "", name: "", w: "", h: "", row: "", order: "", live: false };
      frames.set(id, f);
      if (field === "file") Object.assign(f, { file: s, live: true });
      else if (field === "name") f.name = s;
      else if (field === "w") f.w = s;
      else if (field === "h") f.h = s;
      else if (field === "row") f.row = s;
      else if (field === "order") f.order = s;
    }
  });
  if (!seen.any) return null;
  const out: Struct = { pages: [], orphans: [], ids, files, legacy: false };
  const rowsOf = new Map<string, SRow[]>();
  for (const [id, r] of rows) {
    if (!r.live || !pages.get(r.page)?.live) continue;
    const row: SRow = { id, order: r.order, frames: [] };
    rowsOf.set(r.page, [...(rowsOf.get(r.page) ?? []), row]);
  }
  const rowById = new Map([...rowsOf.values()].flat().map((r) => [r.id, r]));
  for (const [id, f] of frames) {
    if (!f.live) continue;
    const w = Number(f.w);
    const h = Number(f.h);
    const frame: SFrame = { id, name: f.name, file: f.file, w, h, row: f.row, order: f.order };
    if (f.name === "" || f.w === "" || f.h === "" || Number.isNaN(w) || Number.isNaN(h)) continue; // still being written, or not a frame
    const row = rowById.get(f.row);
    if (row) row.frames.push(frame);
    else out.orphans.push(frame);
  }
  for (const [id, p] of pages) {
    if (!p.live) continue;
    out.pages.push({ id, name: p.name, order: p.order, rows: byOrder(rowsOf.get(id) ?? []).map((r) => ({ ...r, frames: byOrder(r.frames) })) });
  }
  byOrder(out.pages);
  return out;
}

/** The structure the older single entry holds, with row ids and orders made up. */
function readLegacy(map: Y.Map<unknown>): Struct | null {
  const text = map.get(LEGACY_KEY);
  if (typeof text !== "string") return null;
  let design: Design | null;
  try {
    design = readDesign(JSON.parse(text));
  } catch {
    return null;
  }
  if (!design) return null;
  const ids = new Set<string>();
  const files = new Set<string>();
  const pageOrders = starts(design.pages.length);
  const pages = design.pages.map((p, i): SPage => {
    ids.add(p.id);
    const rowOrders = starts(p.rows.length);
    return {
      id: p.id,
      name: p.name,
      order: pageOrders[i] ?? "",
      rows: p.rows.map((row, r) => {
        const frameOrders = starts(row.length);
        const rid = `${p.id}.r${String(r + 1)}`;
        return { id: rid, order: rowOrders[r] ?? "", frames: row.map((f, k) => (ids.add(f.id), files.add(f.file), { ...f, row: rid, order: frameOrders[k] ?? "" })) };
      }),
    };
  });
  return { pages, orphans: [], ids, files, legacy: true };
}

/** n increasing keys, the first children of a node: designroom's startKeys. */
function starts(n: number): string[] {
  const keys: string[] = [];
  let prev = "";
  for (let i = 0; i < n; i++) keys.push((prev = between(prev, "")));
  return keys;
}

function structOf(map: Y.Map<unknown>): Struct | null {
  return readEntries(map) ?? readLegacy(map);
}

/** The design the structure projects to: pages and rows by order, a row without frames and a page without rows not drawn. */
function designOf(s: Struct): Design {
  const pages = s.pages
    .map((p) => ({ id: p.id, name: p.name, rows: p.rows.filter((r) => r.frames.length > 0).map((r) => r.frames.map((f) => ({ id: f.id, name: f.name, file: f.file, w: f.w, h: f.h }))) }))
    .filter((p) => p.rows.length > 0);
  return { pages };
}

/** The design the room holds, null when it holds none. */
export function designIn(map: Y.Map<unknown>): Design | null {
  const s = structOf(map);
  if (!s) return null;
  const d = designOf(s);
  return d.pages.length > 0 ? d : null;
}

/** The entries of an empty frame: what internal/designroom writes for a document with nothing in its body (its test reads these back as that frame). */
export function emptyFrameEntries(file: string): [string, string][] {
  const entries: Record<string, string> = {
    "shell|docAfter": "",
    "shell|docBefore": "",
    "shell|htmlAttrs": "[]",
    "shell|post": "",
    "shell|pre": '<head><meta charset="utf-8"/></head>',
    "shell|root": "e1",
    "n|e1|attrs": '[["","\\u0000pid",""]]',
    "n|e1|kind": "el",
    "n|e1|ns": "",
    "n|e1|order": "",
    "n|e1|parent": "",
    "n|e1|rest": "",
    "n|e1|tag": "body",
  };
  return Object.entries(entries).map(([k, v]) => [`${file}|${k}`, v]);
}

const deleteWith = (map: Y.Map<unknown>, prefix: string) => {
  for (const key of [...map.keys()]) if (key.startsWith(prefix)) map.delete(key);
};

function putFrame(map: Y.Map<unknown>, f: SFrame) {
  map.set(`${FRAME}${f.id}|file`, f.file);
  map.set(`${FRAME}${f.id}|name`, f.name);
  map.set(`${FRAME}${f.id}|w`, String(f.w));
  map.set(`${FRAME}${f.id}|h`, String(f.h));
  map.set(`${FRAME}${f.id}|row`, f.row);
  map.set(`${FRAME}${f.id}|order`, f.order);
}

const putRow = (map: Y.Map<unknown>, id: string, page: string, order: string) => {
  map.set(`${ROW}${id}|page`, page);
  map.set(`${ROW}${id}|order`, order);
};

const putPage = (map: Y.Map<unknown>, id: string, name: string, order: string) => {
  map.set(`${PAGE}${id}|order`, order);
  map.set(`${PAGE}${id}|name`, name);
};

/** Moves a room that has the whole design.json in one entry to the entries per value. */
function migrate(map: Y.Map<unknown>, s: Struct) {
  for (const p of s.pages) {
    putPage(map, p.id, p.name, p.order);
    for (const r of p.rows) {
      putRow(map, r.id, p.id, r.order);
      for (const f of r.frames) putFrame(map, f);
    }
  }
  map.delete(LEGACY_KEY);
  s.legacy = false;
}

/** A short id nobody else is making at the same moment: two people adding a frame at once must not pick the same one. */
function freshId(prefix: string, s: Struct): string {
  for (;;) {
    const bytes = crypto.getRandomValues(new Uint8Array(4));
    const id = `${prefix}-${Array.from(bytes, (b) => b.toString(36).padStart(2, "0")).join("").slice(0, 6)}`;
    if (!s.ids.has(id) && !s.files.has(`frames/${id}.html`)) {
      s.ids.add(id);
      s.files.add(`frames/${id}.html`);
      return id;
    }
  }
}

const framesOf = (s: Struct) => s.pages.flatMap((p) => p.rows.flatMap((r) => r.frames));
const liveFrames = (p: SPage) => p.rows.flatMap((r) => r.frames);
const countFrames = (s: Struct) => designOf(s).pages.reduce((n, p) => n + p.rows.flat().length, 0);
const countPages = (s: Struct) => designOf(s).pages.length;

function findFrame(s: Struct, id: string) {
  for (const page of s.pages) for (const row of page.rows) for (const frame of row.frames) if (frame.id === id) return { page, row, frame };
  return null;
}

/** Where a dragged frame goes: before another frame of the same page, at the end of a row, or in a row of its own before another row (null: after the last). Rows are numbered as drawn. */
export type Drop = { before: string } | { endOf: number } | { newRowBefore: number | null };

/** How the screen names what it adds: the nth page, the nth frame. */
export interface Names {
  page: (n: number) => string;
  frame: (n: number) => string;
}

export class PageStructure {
  private readonly edits: DesignEdits;
  private readonly saved: () => Iterable<string>;

  /** `saved` is the files of the newest saved version, whose names a new frame must not take. */
  constructor(edits: DesignEdits, saved: () => Iterable<string> = () => []) {
    this.edits = edits;
    this.saved = saved;
  }

  design(): Design | null {
    return designIn(this.edits.map);
  }

  /** Runs one change as one step of this person's history, on the structure as the room holds it now. */
  private change(make: (map: Y.Map<unknown>, s: Struct) => Outcome): Outcome {
    let outcome: Outcome = refused({ rule: "missing" });
    this.edits.transact((map) => {
      const s = structOf(map);
      if (!s) return;
      for (const file of this.saved()) s.files.add(file);
      outcome = make(map, s);
    });
    return outcome;
  }

  addPage(names: Names): Outcome {
    return this.change((map, s) => {
      if (countPages(s) >= MAX_PAGES) return refused({ rule: "pages", limit: MAX_PAGES });
      if (countFrames(s) >= MAX_FRAMES) return refused({ rule: "frames", limit: MAX_FRAMES });
      if (s.legacy) migrate(map, s);
      const page = freshId("page", s);
      const frame = freshId("frame", s);
      const row = freshId("row", s);
      const last = s.pages.at(-1)?.order ?? "";
      const size = PRESETS[0];
      putPage(map, page, numbered(names.page, s.pages.map((p) => p.name), s.pages.length + 1), between(last, ""));
      putRow(map, row, page, between("", ""));
      this.addFrameEntries(map, s, { id: frame, name: numbered(names.frame, framesOf(s).map((f) => f.name), countFrames(s) + 1), file: `frames/${frame}.html`, w: size.w, h: size.h, row, order: between("", "") });
      return { kind: "changed", page, frame };
    });
  }

  private addFrameEntries(map: Y.Map<unknown>, s: Struct, f: SFrame) {
    putFrame(map, f);
    for (const [k, v] of emptyFrameEntries(f.file)) map.set(k, v);
    s.files.add(f.file);
  }

  renamePage(id: string, raw: string): Outcome {
    return this.change((map, s) => {
      const page = s.pages.find((p) => p.id === id);
      if (!page) return refused({ rule: "missing" });
      const name = cleanName(raw);
      if (typeof name !== "string") return refused(name);
      if (name === "" || name === page.name) return same;
      if (s.legacy) migrate(map, s);
      map.set(`${PAGE}${id}|name`, name);
      return { kind: "changed", page: id };
    });
  }

  /** The page goes with its rows, its frames and the frames' content; the last page cannot. */
  deletePage(id: string): Outcome {
    return this.change((map, s) => {
      const page = s.pages.find((p) => p.id === id);
      if (!page) return refused({ rule: "missing" });
      if (countPages(s) === 1 && designOf(s).pages[0]?.id === id) return refused({ rule: "lastPage" });
      if (s.legacy) migrate(map, s);
      for (const frame of liveFrames(page)) {
        deleteWith(map, `${FRAME}${frame.id}|`);
        deleteWith(map, `${frame.file}|`);
      }
      for (const row of page.rows) deleteWith(map, `${ROW}${row.id}|`);
      deleteWith(map, `${PAGE}${id}|`);
      return { kind: "changed" };
    });
  }

  /** An empty frame in a row of its own at the bottom of the page, of `size`, else the size of `like`, else the desktop preset. */
  addFrame(pageId: string, name: (n: number) => string, like?: string, size?: { w: number; h: number }): Outcome {
    return this.change((map, s) => {
      const page = s.pages.find((p) => p.id === pageId);
      if (!page) return refused({ rule: "missing" });
      if (countFrames(s) >= MAX_FRAMES) return refused({ rule: "frames", limit: MAX_FRAMES });
      for (const field of ["w", "h"] as const) {
        const why = size && sideProblem(field, size[field]);
        if (why) return refused(why);
      }
      if (s.legacy) migrate(map, s);
      const model = like ? findFrame(s, like)?.frame : undefined;
      const id = freshId("frame", s);
      const row = freshId("row", s);
      const last = page.rows.at(-1)?.order ?? "";
      putRow(map, row, pageId, between(last, ""));
      this.addFrameEntries(map, s, { id, name: numbered(name, framesOf(s).map((f) => f.name), countFrames(s) + 1), file: `frames/${id}.html`, w: size?.w ?? model?.w ?? PRESETS[0].w, h: size?.h ?? model?.h ?? PRESETS[0].h, row, order: between("", "") });
      return { kind: "changed", page: pageId, frame: id };
    });
  }

  renameFrame(id: string, raw: string): Outcome {
    return this.change((map, s) => {
      const found = findFrame(s, id);
      if (!found) return refused({ rule: "missing" });
      const name = cleanName(raw);
      if (typeof name !== "string") return refused(name);
      if (name === "" || name === found.frame.name) return same;
      if (s.legacy) migrate(map, s);
      map.set(`${FRAME}${id}|name`, name);
      return { kind: "changed", frame: id };
    });
  }

  /** The frame goes with its content; the last frame of a page cannot. */
  deleteFrame(id: string): Outcome {
    return this.change((map, s) => {
      const found = findFrame(s, id);
      if (!found) return refused({ rule: "missing" });
      if (liveFrames(found.page).length === 1) return refused({ rule: "lastFrame" });
      if (s.legacy) migrate(map, s);
      deleteWith(map, `${FRAME}${id}|`);
      deleteWith(map, `${found.frame.file}|`);
      return { kind: "changed" };
    });
  }

  /** Whole pixels from 1 to 8192; anything else is refused with the limit. */
  resizeFrame(id: string, size: { w: number; h: number }): Outcome {
    return this.change((map, s) => {
      const found = findFrame(s, id);
      if (!found) return refused({ rule: "missing" });
      for (const field of ["w", "h"] as const) {
        const why = sideProblem(field, size[field]);
        if (why) return refused(why);
      }
      if (size.w === found.frame.w && size.h === found.frame.h) return same;
      if (s.legacy) migrate(map, s);
      if (size.w !== found.frame.w) map.set(`${FRAME}${id}|w`, String(size.w));
      if (size.h !== found.frame.h) map.set(`${FRAME}${id}|h`, String(size.h));
      return { kind: "changed", frame: id };
    });
  }

  /**
   * Puts a frame somewhere else on its page: another place in its row, another
   * row, or a row of its own; it writes the frame's row and order and nothing
   * else, so a rename or resize someone makes at the same time stays. A drop that
   * leaves everything where it was is no change.
   */
  moveFrame(id: string, drop: Drop): Outcome {
    return this.change((map, s) => {
      const found = findFrame(s, id);
      if (!found) return refused({ rule: "missing" });
      const { page, row: own, frame } = found;
      const drawn = page.rows.filter((r) => r.frames.length > 0);
      const alone = own.frames.length === 1;
      let target: { row: string; order: string } | null = null;
      let newRow: { order: string } | null = null;
      if ("before" in drop) {
        const at = findFrame(s, drop.before);
        if (at?.page !== page || drop.before === id) return same;
        const others = at.row.frames.filter((f) => f.id !== id);
        const i = others.findIndex((f) => f.id === drop.before);
        if (at.row === own && own.frames[own.frames.findIndex((f) => f.id === id) + 1]?.id === drop.before) return same;
        target = { row: at.row.id, order: between(others[i - 1]?.order ?? "", at.frame.order) };
      } else if ("endOf" in drop) {
        const row = drawn[drop.endOf];
        if (!row) return refused({ rule: "missing" });
        const others = row.frames.filter((f) => f.id !== id);
        if (row === own && own.frames.at(-1)?.id === id) return same;
        target = { row: row.id, order: between(others.at(-1)?.order ?? "", "") };
      } else {
        const index = drop.newRowBefore;
        if (index !== null && !drawn[index]) return refused({ rule: "missing" });
        const ownAt = drawn.indexOf(own);
        if (alone && (index === ownAt || (index === null ? ownAt === drawn.length - 1 : index === ownAt + 1))) return same;
        const others = drawn.filter((r) => !(alone && r === own));
        const next = index === null ? undefined : drawn[index];
        const nextAt = next ? others.indexOf(next) : others.length;
        newRow = { order: between(others[nextAt - 1]?.order ?? "", others[nextAt]?.order ?? "") };
      }
      if (s.legacy) migrate(map, s);
      if (newRow) {
        const row = freshId("row", s);
        putRow(map, row, page.id, newRow.order);
        target = { row, order: between("", "") };
      }
      if (!target) return same;
      if (target.row !== frame.row) map.set(`${FRAME}${id}|row`, target.row);
      map.set(`${FRAME}${id}|order`, target.order);
      return { kind: "changed", frame: id };
    });
  }
}

/** The design the room holds, following it as anyone changes it; null until there is a room, and while it holds none. */
export function useRoomDesign(edits: DesignEdits | null): Design | null {
  const subscribe = useCallback((changed: () => void) => (edits ? edits.onChange(changed) : () => undefined), [edits]);
  // The design as text, which is the same text until the design is another one.
  const text = useSyncExternalStore(subscribe, () => JSON.stringify(edits ? designIn(edits.map) : null));
  return useMemo(() => JSON.parse(text) as Design | null, [text]);
}
