import { useEffect, useRef, useState } from "react";
import type { Awareness } from "y-protocols/awareness";
import { liveTicket, type Artifact } from "../api";
import { colorOf } from "../editor/colors";
import { LiveProvider, openWebSocket, type Connection } from "../editor/provider";
import { readTokens } from "./tokens";
import { DesignEdits, SHOWN } from "./edits";
import { isFrameGone, type ChildOrder, type InlineChange, type FrameLinks, type StyleChange } from "./inspect";
import { elementOrder } from "./tree";

/** The live connection of a design's room, as the canvas shows it: who is here and whether the connection holds. */
export interface Live {
  awareness: Awareness;
  /** This browser's awareness id. */
  self: number;
  state: Connection;
}

/**
 * The design's room, for anyone who may see the newest version: the live
 * connection (read-only for someone who may not edit), and once the room's state
 * has arrived, the view of it. Both are null until then, and again while a room
 * that was thrown away is being rebuilt.
 */
export function useRoom(artifact: Artifact, account: string, enabled: boolean, onAccessChanged?: () => void): { edits: DesignEdits | null; live: Live | null } {
  const [room, setRoom] = useState<{ edits: DesignEdits; provider: LiveProvider } | null>(null);
  const [state, setState] = useState<Connection>("connecting");
  const accessChanged = useRef(onAccessChanged);
  useEffect(() => { accessChanged.current = onAccessChanged; });
  const id = artifact.id;
  const readOnly = !artifact.canEdit;
  useEffect(() => {
    if (!enabled) return;
    const provider = new LiveProvider({
      ticket: () => liveTicket(id),
      open: openWebSocket,
      user: { id: account, name: account, color: colorOf(account) },
      readOnly,
      onAccessChanged: () => { accessChanged.current?.(); },
      network: window,
    });
    let made: DesignEdits | null = null;
    const drop = () => {
      made?.destroy();
      made = null;
      setRoom(null);
    };
    const offChange = provider.onChange(() => {
      setState(provider.state);
      if (provider.synced && !made) {
        made = new DesignEdits(provider.doc);
        setRoom({ edits: made, provider });
      }
    });
    const offReset = provider.onReset(drop);
    provider.start();
    return () => {
      offChange();
      offReset();
      drop();
      provider.destroy();
    };
  }, [id, account, enabled, readOnly]);
  const live = room ? { awareness: room.provider.awareness, self: room.provider.doc.clientID, state } : null;
  return { edits: room?.edits ?? null, live };
}

/**
 * Keeps each frame showing what the room holds: everything when the frame says
 * it is ready (it begins as the file of the latest saved version, which may be
 * older than the room), then only what changed after each change to the room,
 * whoever made it. `shown` is called once the frames have done it, for whatever
 * depends on where things are.
 */
export function useFramesShowRoom(edits: DesignEdits | null, links: FrameLinks, frames: { id: string; file: string }[], shown: () => void) {
  const latest = useRef({ frames, shown });
  useEffect(() => {
    latest.current = { frames, shown };
  });
  useEffect(() => {
    if (!edits) return;
    const sent = new Map<string, { styles: Map<string, string>; inline: Map<string, string>; alive: Set<string>; order: Map<string, string> }>();
    const tokensSent = new Map<string, string>();
    const push = async (frameId: string): Promise<void> => {
      const frame = latest.current.frames.find((f) => f.id === frameId);
      if (!frame) return Promise.resolve();
      const tokens = readTokens(edits);
      const serialized = JSON.stringify(tokens);
      if (tokensSent.get(frameId) !== serialized) {
        await links.tokens(frameId, tokens);
        tokensSent.set(frameId, serialized);
      }
      const state = edits.frame(frame.file);
      const now = new Map<string, string>();
      for (const [pid, own] of state.styles) if (!state.inline.has(pid)) for (const prop of SHOWN) if (own.has(prop)) now.set(`${pid}\0${prop}`, own.get(prop) ?? "");
      // Hiding is a state of its own; what the frame is shown for it is the display it implies.
      for (const [pid, visibility] of state.hidden) {
        if (state.inline.has(pid)) continue;
        if (visibility === "1") now.set(`${pid}\0display`, "none");
        else if ((state.styles.get(pid)?.get("display") ?? "").trim() === "") now.set(`${pid}\0display`, "revert");
      }
      const before = sent.get(frameId);
      const inline: InlineChange[] = [];
      for (const [pid, source] of state.inline) if (before?.inline.get(pid) !== source) inline.push([pid, source]);
      for (const pid of before?.inline.keys() ?? []) if (!state.inline.has(pid)) inline.push([pid, null]);
      const changes: StyleChange[] = [];
      for (const [k, v] of now) if (before?.styles.get(k) !== v) changes.push([...(k.split("\0") as [string, string]), v]);
      for (const k of before?.styles.keys() ?? []) if (!now.has(k)) changes.push([...(k.split("\0") as [string, string]), null]);
      // What the frame keeps showing: the room's elements and the inline elements of their formatted texts.
      const shownEls = new Set([...state.alive, ...state.inText]);
      // A frame the room knows nothing of is not hidden whole.
      const aliveChanged = state.alive.size > 0 && (before?.alive.size !== shownEls.size || [...shownEls].some((p) => !before.alive.has(p)));
      // The parents whose element children are in another order than the frame was last told (all of them for a frame that has just said it is ready).
      const tree = edits.tree(frame.file);
      const order = new Map<string, string>();
      const reordered: ChildOrder[] = [];
      if (state.alive.size > 0) {
        for (const [parent, kids] of elementOrder(tree)) {
          order.set(parent, kids.join("\0"));
          if (before?.order.get(parent) !== kids.join("\0")) reordered.push([parent, kids]);
        }
      }
      if (changes.length === 0 && inline.length === 0 && !aliveChanged && reordered.length === 0) return Promise.resolve();
      sent.set(frameId, { styles: now, inline: state.inline, alive: aliveChanged ? shownEls : (before?.alive ?? shownEls), order });
      return links.apply(frameId, changes, aliveChanged ? [...shownEls] : undefined, reordered.length > 0 ? reordered : undefined, inline.length > 0 ? inline : undefined);
    };
    const pushAll = () => {
      Promise.all(latest.current.frames.map((f) => push(f.id))).then(
        () => {
          latest.current.shown();
        },
        (err: unknown) => {
          if (!isFrameGone(err)) console.error("show the room in its frames", err);
        },
      );
    };
    pushAll();
    const offChange = edits.onChange(pushAll);
    const offReady = links.onReady((frameId) => {
      sent.delete(frameId);
      tokensSent.delete(frameId);
      pushAll();
    });
    return () => {
      offChange();
      offReady();
    };
  }, [edits, links]);
}
