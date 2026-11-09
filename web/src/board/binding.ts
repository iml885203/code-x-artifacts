// A board's scene and its room (docs/spikes/board.md, internal/boardroom): the
// shared map holds one entry per element field, "<id>|<field>" → the field's
// JSON. version, versionNonce and updated are each editor's own bookkeeping and
// never shared. It knows the editor only through Scene, so tests drive it with
// a fake.
import * as Y from "yjs";

export const BOARD_MAP = "board";
const LOCAL = new Set(["id", "version", "versionNonce", "updated"]);
const ORIGIN = "board-binding";

// Excalidraw's BOUND_TEXT_PADDING: a label sits this far inside its shape.
const PADDING = 5;

/** An Excalidraw element, as far as the binding reads it. */
export interface Element {
  readonly id: string;
  readonly version: number;
  readonly [field: string]: unknown;
}

/** The part of the editor's API the binding uses. */
export interface Scene {
  getSceneElementsIncludingDeleted(): readonly Element[];
  updateScene(update: { elements: readonly Element[]; captureUpdate: never }): void;
}

export interface Binding {
  /** The editor's change event: writes what this person changed into the room. */
  onChange(elements: readonly Element[]): void;
  destroy(): void;
}

/**
 * Binds scene to the board in doc. Remote changes reach the scene with
 * captureUpdate (Excalidraw's CaptureUpdateAction.NEVER), so they never enter
 * this person's Undo. A reader's binding never writes.
 */
export function bindBoard(doc: Y.Doc, scene: Scene, captureUpdate: never, readOnly: boolean): Binding {
  const map = doc.getMap<string>(BOARD_MAP);
  // The version of each element as last written or applied here, so a change
  // event only compares the elements that changed.
  const seen = new Map<string, number>();

  const write = (elements: readonly Element[]) => {
    const changes: [string, string][] = [];
    for (const el of elements) {
      if (seen.get(el.id) === el.version) continue;
      seen.set(el.id, el.version);
      for (const [field, value] of Object.entries(el)) {
        if (LOCAL.has(field) || value === undefined) continue;
        const json = JSON.stringify(value);
        if (map.get(`${el.id}|${field}`) !== json) changes.push([`${el.id}|${field}`, json]);
      }
    }
    if (changes.length === 0) return;
    doc.transact(() => {
      for (const [k, v] of changes) map.set(k, v);
    }, ORIGIN);
  };

  const apply = (ids: Set<string>) => {
    const current = new Map(scene.getSceneElementsIncludingDeleted().map((el) => [el.id, el]));
    const changed = new Map<string, Record<string, unknown>>([...ids].map((id) => [id, {}]));
    for (const [k, v] of map.entries()) {
      const [id, field] = splitKey(k);
      const fields = changed.get(id);
      if (fields) fields[field] = JSON.parse(v) as unknown;
    }
    for (const [id, fields] of changed) {
      if (!fields.type) continue;
      const before = current.get(id);
      current.set(id, {
        ...before,
        ...fields,
        id,
        version: (before?.version ?? 0) + 1,
        versionNonce: Math.floor(Math.random() * 2 ** 31),
        updated: Date.now(),
      });
    }
    const elements = placeLabels([...current.values()]);
    elements.sort((a, b) => compare(a.index, b.index));
    scene.updateScene({ elements, captureUpdate });
    // What the editor holds now is the baseline: whatever it changed while taking
    // the room's state in (an empty label someone has only begun is dropped as
    // deleted) is not this person's edit, and written back it would undo theirs.
    for (const el of scene.getSceneElementsIncludingDeleted()) seen.set(el.id, el.version);
  };

  const observer = (event: Y.YMapEvent<string>, txn: Y.Transaction) => {
    if (txn.origin === ORIGIN) return;
    const ids = new Set<string>();
    for (const k of event.keysChanged as Set<string>) ids.add(splitKey(k)[0]);
    apply(ids);
  };
  map.observe(observer);
  if (map.size > 0) apply(new Set([...map.keys()].map((k) => splitKey(k)[0])));

  return {
    onChange: (elements) => {
      if (!readOnly) write(elements);
    },
    destroy: () => {
      map.unobserve(observer);
    },
  };
}

function splitKey(k: string): [string, string] {
  const bar = k.indexOf("|");
  return [k.slice(0, bar), k.slice(bar + 1)];
}

function compare(a: unknown, b: unknown): number {
  const x = typeof a === "string" ? a : "";
  const y = typeof b === "string" ? b : "";
  return x < y ? -1 : x > y ? 1 : 0;
}

const num = (v: unknown) => (typeof v === "number" ? v : 0);

/**
 * Lays every label out from its shape, as the server does when it cuts a
 * version (boardfmt.LayoutLabels): two people's writes of a label's position
 * (one moved the shape, one retyped the label and so changed its width) would
 * otherwise leave it where neither put it.
 */
export function placeLabels(elements: Element[]): Element[] {
  const byId = new Map(elements.map((el) => [el.id, el]));
  return elements.map((el) => {
    const box = el.type === "text" && typeof el.containerId === "string" ? byId.get(el.containerId) : undefined;
    if (!box || box.isDeleted === true) return el;
    const [bx, by, bw, bh, w, h] = [num(box.x), num(box.y), num(box.width), num(box.height), num(el.width), num(el.height)];
    const inset = insetOf(box.type, bw, bh);
    let x = bx + (bw - w) / 2;
    let y = by + (bh - h) / 2;
    if (el.textAlign === "left") x = bx + inset.x;
    if (el.textAlign === "right") x = bx + bw - inset.x - w;
    if (el.verticalAlign === "top") y = by + inset.y;
    if (el.verticalAlign === "bottom") y = by + bh - inset.y - h;
    if (x === el.x && y === el.y) return el;
    return { ...el, x, y, version: el.version + 1, versionNonce: Math.floor(Math.random() * 2 ** 31) };
  });
}

// How far inside a shape its label's box starts (Excalidraw's getContainerCoords).
function insetOf(type: unknown, w: number, h: number): { x: number; y: number } {
  if (type === "ellipse") return { x: PADDING + (w / 2) * (1 - Math.SQRT1_2), y: PADDING + (h / 2) * (1 - Math.SQRT1_2) };
  if (type === "diamond") return { x: PADDING + w / 4, y: PADDING + h / 4 };
  return { x: PADDING, y: PADDING };
}
