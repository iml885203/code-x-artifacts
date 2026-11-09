// What the editor writes into a design's room and reads back (docs/design-room.md):
// the entries of the shared map `design` that the server's internal/designroom
// keeps, spelled the same way, written straight into the browser's copy of the
// room so that everyone sees them and the server saves them. Undo is Yjs's
// UndoManager restricted to this person's own changes: a change of someone else's
// to the same entry is never undone by it.
import * as Y from "yjs";
import { cascadeImportant, cascadeSource, coveredSlot, needsCascade } from "./styleCascade";
import list from "./properties/owned.json";
import { htmlOfDelta } from "./text/html";
import type { DeltaOp } from "./text/units";
import { parseAttrs, placement, readTree, type FrameTree } from "./tree";

/** The shared map of a design's room: designroom.Map. */
export const ROOM_MAP = "design";

/** The style properties the editor owns: designroom's ownedProps (properties/owned.json, which a Go test holds equal to it). */
export const OWNED: readonly string[] = list.owned;

/** What a frame is told to show: the owned properties and display, which the outline writes to hide an element. */
export const SHOWN: readonly string[] = [...OWNED, ...list.shown];

/** A display value without its !important, lower case. */
const displayText = (v: string) => v.replace(/!important\s*$/i, "").trim().toLowerCase();

const nodeKey = (file: string, pid: string, field: string) => `${file}|n|${pid}|${field}`;
const styleField = "s|";

export interface FrameState {
  /** The id of the frame's body. */
  root: string;
  /** The elements the room holds. */
  alive: Set<string>;
  /**
   * The elements inside the formatted text of an element the room holds (its
   * formats and the inline markup it keeps as written): parts of that text, not
   * elements of their own, yet the frame has them and must keep showing them.
   */
  inText: Set<string>;
  /** Each element's owned style, as the room writes it (with the spacing the declaration had). */
  styles: Map<string, Map<string, string>>;
  /** Authoritative cascade projection for nodes with covering shorthand or explicit overrides. */
  inline: Map<string, string>;
  /** The elements the editor hides ("1") or shows although their own markup hides them ("0"). */
  hidden: Map<string, string>;
}

/** What the room holds of one frame. An entry of an element that is gone (someone deleted it as another wrote to it) is no element. */
export function frameState(map: Y.Map<unknown>, file: string): FrameState {
  const prefix = `${file}|n|`;
  const alive = new Set<string>();
  const found = new Map<string, Map<string, string>>();
  const overrides = new Map<string, Map<string, string>>();
  const visibility = new Map<string, string>();
  map.forEach((value, key) => {
    if (!key.startsWith(prefix)) return;
    const rest = key.slice(prefix.length);
    const bar = rest.indexOf("|");
    if (bar < 0) return;
    const pid = rest.slice(0, bar);
    const field = rest.slice(bar + 1);
    if (field === "kind") {
      if (value === "el") alive.add(pid);
    } else if (field === "hidden") {
      if (typeof value === "string" && value !== "") visibility.set(pid, value);
    } else if (field.startsWith("override|") && typeof value === "string") {
      let own = overrides.get(pid);
      if (!own) overrides.set(pid, (own = new Map<string, string>()));
      own.set(field.slice("override|".length), value);
    } else if (field.startsWith(styleField) && typeof value === "string") {
      let own = found.get(pid);
      if (!own) found.set(pid, (own = new Map<string, string>()));
      own.set(field.slice(styleField.length), value);
    }
  });
  const styles = new Map<string, Map<string, string>>();
  const inline = new Map<string, string>();
  for (const pid of alive) {
    const base = found.get(pid) ?? new Map<string, string>();
    const extra = overrides.get(pid) ?? new Map<string, string>();
    const combined = new Map([...base, ...extra]);
    if (combined.size) styles.set(pid, combined);
    const raw = map.get(nodeKey(file, pid, "rest"));
    const rest = typeof raw === "string" ? raw : "";
    if (extra.size || needsCascade(rest)) {
      let source = cascadeSource(rest, base, extra);
      const hidden = visibility.get(pid);
      if (hidden === "1") source += ";display:none !important";
      else if (hidden === "0" && !combined.get("display")) source += ";display:revert";
      inline.set(pid, source);
    }
  }
  const root = map.get(`${file}|shell|root`);
  const hidden = new Map([...visibility].filter(([pid]) => alive.has(pid)));
  return { root: typeof root === "string" ? root : "", alive, inText: pidsInText(map, file, alive), styles, inline, hidden };
}

/**
 * The ids in the markup the formatted texts of the elements build, read from
 * that markup rather than from each way a text holds an element, so that
 * whatever the text can hold the frame keeps showing.
 */
function pidsInText(map: Y.Map<unknown>, file: string, elements: Set<string>): Set<string> {
  const out = new Set<string>();
  for (const pid of elements) {
    const rich = map.get(nodeKey(file, pid, "rich"));
    if (!(rich instanceof Y.Text)) continue;
    for (const m of htmlOfDelta(rich.toDelta() as DeltaOp[]).matchAll(/data-pid="([^"]*)"/g)) out.add(m[1] ?? "");
  }
  return out;
}

export type Change = { file: string; pid: string; style: Record<string, string | undefined> } | { file: string; pid: string; remove: true };

/** Preserve authored spacing and inherit priority only when the incoming value has no explicit priority. */
function written(old: string | undefined, value: string, priority?: string): string {
  if (value === "") return value;
  const lead = /^\s*/.exec(old ?? "")?.[0] ?? "";
  const important = priority ?? /\s*!important\s*$/i.exec(old ?? "")?.[0] ?? "";
  return lead + value + (/!important\s*$/i.test(value) ? "" : important);
}

/** The ids of an element and everything inside it. */
function subtree(map: Y.Map<unknown>, file: string, pid: string): Set<string> {
  const children = new Map<string, string[]>();
  const prefix = `${file}|n|`;
  map.forEach((value, key) => {
    if (!key.startsWith(prefix) || !key.endsWith("|parent") || typeof value !== "string") return;
    const id = key.slice(prefix.length, -"|parent".length);
    children.set(value, [...(children.get(value) ?? []), id]);
  });
  const gone = new Set<string>();
  const pending = [pid];
  for (let next = pending.pop(); next !== undefined; next = pending.pop()) {
    if (gone.has(next)) continue;
    gone.add(next);
    pending.push(...(children.get(next) ?? []));
  }
  return gone;
}

export class DesignEdits {
  readonly map: Y.Map<unknown>;
  private readonly doc: Y.Doc;
  private readonly undoer: Y.UndoManager;
  private readonly origin = {};
  private readonly watchers = new Set<() => void>();
  private readonly observe = () => {
    for (const watch of this.watchers) watch();
  };

  constructor(doc: Y.Doc) {
    this.doc = doc;
    this.map = doc.getMap(ROOM_MAP);
    // Every action is its own step: nothing is merged for being quick.
    this.undoer = new Y.UndoManager(this.map, { trackedOrigins: new Set([this.origin]), captureTimeout: 0 });
    this.map.observeDeep(this.observe);
  }

  frame(file: string): FrameState {
    return frameState(this.map, file);
  }

  /**
   * Makes the changes, in any number of frames, as one step of this person's
   * history. False, with nothing written, when nothing would change: a drag that
   * ended where it began is not a step.
   */
  apply(changes: Change[]): boolean {
    const states = new Map<string, FrameState>();
    let wrote = false;
    this.doc.transact(() => {
      for (const change of changes) {
        const { file } = change;
        let state = states.get(file);
        if (!state) states.set(file, (state = this.frame(file)));
        if (change.pid === state.root || !state.alive.has(change.pid)) continue;
        if ("remove" in change) {
          for (const pid of subtree(this.map, file, change.pid)) {
            const prefix = nodeKey(file, pid, "");
            for (const key of [...this.map.keys()]) {
              if (!key.startsWith(prefix)) continue;
              this.map.delete(key);
              wrote = true;
            }
          }
          continue;
        }
        for (const prop of SHOWN) {
          const value = change.style[prop];
          if (value === undefined) continue;
          const override = nodeKey(file, change.pid, "override|" + prop);
          const raw = this.map.get(nodeKey(file, change.pid, "rest"));
          const useOverride = this.map.has(override) || coveredSlot(typeof raw === "string" ? raw : "", prop);
          const key = useOverride ? override : nodeKey(file, change.pid, styleField + prop);
          const old = this.map.get(key);
          const effective = state.styles.get(change.pid)?.get(prop);
          const source = state.inline.get(change.pid);
          const priority = source && cascadeImportant(source, prop) ? " !important" : undefined;
          const next = written(effective, value, priority);
          // Taking away what was never written changes nothing.
          if (old === next || (old === undefined && next === "")) continue;
          this.map.set(key, next);
          wrote = true;
        }
      }
    }, this.origin);
    return wrote;
  }

  /**
   * Writes the room in one step of this person's history, for what is kept
   * beside the editor's own entries (the manifest and the frames, pages/room.ts).
   */
  transact(write: (map: Y.Map<unknown>) => void): void {
    this.doc.transact(() => {
      write(this.map);
    }, this.origin);
  }

  /** The tree of a frame as the room holds it, for the outline. */
  tree(file: string): FrameTree {
    return readTree(this.map, file);
  }

  /** Gives an element a name (its `data-name`; the empty name takes it away) and changes nothing else about it. One step. */
  rename(file: string, pid: string, name: string): boolean {
    const tree = this.tree(file);
    const node = tree.nodes.get(pid);
    const key = nodeKey(file, pid, "attrs");
    const value = name.trim();
    if (node?.kind !== "el" || pid === tree.root || node.name === value) return false;
    const rows = parseAttrs(this.map.get(key)).filter(([ns, k]) => !(ns === "" && k === "data-name"));
    if (value !== "") rows.push(["", "data-name", value]);
    this.doc.transact(() => {
      this.map.set(key, JSON.stringify(rows));
    }, this.origin);
    return true;
  }

  /**
   * Hides an element or shows it again. Hiding is a state of its own (`hidden`),
   * not a value of `display`, so the display the properties panel wrote comes back
   * when the element does. In the file a hidden element says display:none and stays
   * there. An element whose own markup says display:none, and an older room's
   * `display` entry that says none, are shown by the state "0" (display:revert)
   * unless a display of the editor's own does it. One step.
   */
  setHidden(file: string, pid: string, hidden: boolean): boolean {
    const tree = this.tree(file);
    const node = tree.nodes.get(pid);
    if (node?.kind !== "el" || pid === tree.root || node.hidden === hidden) return false;
    const stateKey = nodeKey(file, pid, "hidden");
    const displayKey = nodeKey(file, pid, styleField + "display");
    const display = this.map.get(displayKey);
    const own = typeof display === "string" ? displayText(display) : "";
    const legacy = own === "none" || own === "revert";
    this.doc.transact(() => {
      if (hidden) {
        this.map.set(stateKey, "1");
        return;
      }
      if (legacy) this.map.delete(displayKey);
      if (node.authoredHidden && (legacy || own === "")) this.map.set(stateKey, "0");
      else this.map.delete(stateKey);
    }, this.origin);
    return true;
  }

  /**
   * Puts an element under `parent`, right before `before` (the end when null),
   * writing its parent and order and nothing else, so what anyone does to its
   * text or style stays. False, with nothing written, for what is refused
   * (the body, into itself or something inside it) and for a move that leaves it
   * where it is. One step.
   */
  move(file: string, pid: string, parent: string, before: string | null): boolean {
    const tree = this.tree(file);
    const at = placement(tree, pid, parent, before);
    const node = tree.nodes.get(pid);
    if (!at || !node) return false;
    const siblings = tree.nodes.get(parent)?.children ?? [];
    const next = siblings.slice(siblings.indexOf(pid) + 1).find((c) => tree.nodes.get(c)?.kind === "el") ?? null;
    if (node.parent === parent && next === before) return false;
    this.doc.transact(() => {
      this.map.set(nodeKey(file, pid, "parent"), at.parent);
      this.map.set(nodeKey(file, pid, "order"), at.order);
    }, this.origin);
    return true;
  }

  undo(): boolean {
    return this.undoer.undo() !== null;
  }

  redo(): boolean {
    return this.undoer.redo() !== null;
  }

  steps(): { undo: number; redo: number } {
    return { undo: this.undoer.undoStack.length, redo: this.undoer.redoStack.length };
  }

  /** Called after every change to the room, whoever made it. Returns the unsubscribe. */
  onChange(fn: () => void): () => void {
    this.watchers.add(fn);
    return () => this.watchers.delete(fn);
  }

  destroy(): void {
    this.map.unobserveDeep(this.observe);
    this.undoer.destroy();
    this.watchers.clear();
  }
}
