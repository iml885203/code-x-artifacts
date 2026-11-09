import { useEffect, useRef, type RefObject } from "react";
import type { Editor, Target } from "./controller";
import type { DesignEdits } from "./edits";
import { opsKey } from "./ops/keys";
import type { Operations } from "./ops/useDesignOps";
import type { PageStructure } from "./pages/structure";
import { isArmed, type Tool } from "./toolbar/tools";
import type { Selected } from "./useSelection";

const typing = (target: EventTarget | null) =>
  target instanceof HTMLElement && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
/** Space presses a focused button, so it only holds the hand tool when something else has the keyboard. */
const pressable = (target: EventTarget | null) => target instanceof HTMLElement && target.tagName === "BUTTON";

/** What the keys read, as the canvas holds it now. */
export interface Held {
  edits: DesignEdits | null;
  editor: Editor | null;
  targets: () => Target[];
  selected: Selected[];
  ops: Operations | null;
  structure: PageStructure | null;
  tool: Tool;
}

/** What the keys ask the canvas to do. */
export interface KeyActions {
  armTool: (tool: Tool) => void;
  zoomStep: (direction: 1 | -1) => void;
  zoomTo: (z: number) => void;
  fit: () => void;
  setTool: (change: (now: Tool) => Tool) => void;
  clear: () => void;
  clearNotice: () => void;
  removeFrame?: () => boolean;
  holdSpace: (held: boolean) => void;
}

/**
 * The canvas's keys: Undo and Redo, Delete, the element operations, zoom (+, -, 0, 1),
 * the tools (V, H, F, and T, R, O from the operations), Esc and Space for the hand.
 * They are off while a frame is played and while something that types has the keyboard.
 */
export function useCanvasKeys(paused: boolean, held: RefObject<Held>, actions: KeyActions) {
  const ask = useRef(actions);
  useEffect(() => {
    ask.current = actions;
  });
  useEffect(() => {
    if (paused) return;
    const onKey = (e: KeyboardEvent) => {
      if (typing(e.target) || (e.target instanceof HTMLElement && e.target.closest('[role="menu"], [role="alertdialog"]'))) return;
      const { edits: room, editor: ed, targets: chosen, selected: picked, ops: acts, structure: pages, tool: armed } = held.current;
      const act = ask.current;
      if ((e.metaKey || e.ctrlKey) && !e.altKey && ["z", "Z", "y", "Y"].includes(e.key)) {
        if (!room || !ed) return;
        e.preventDefault();
        if (e.key.toLowerCase() === "y" || e.shiftKey) room.redo();
        else room.undo();
        return;
      }
      if (ed && !e.metaKey && !e.ctrlKey && (e.key === "Delete" || e.key === "Backspace") && act.removeFrame?.()) {
        e.preventDefault();
        return;
      }
      if (ed && !e.metaKey && !e.ctrlKey && (e.key === "Delete" || e.key === "Backspace") && picked.length > 0) {
        e.preventDefault();
        ed.remove(chosen());
        act.clear();
        return;
      }
      const asked = acts ? opsKey(e) : null;
      if (acts && asked) {
        e.preventDefault();
        if (asked.do === "tool") act.armTool(armed === asked.shape ? "select" : asked.shape);
        else if (asked.do === "stack") acts.stack(asked.how);
        else acts[asked.do]();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      switch (e.key) {
        case "+":
        case "=":
          act.zoomStep(1);
          break;
        case "-":
          act.zoomStep(-1);
          break;
        case "0":
          act.fit();
          break;
        case "1":
          act.zoomTo(1);
          break;
        case "h":
        case "H":
          act.setTool(() => "hand");
          break;
        case "v":
        case "V":
          act.setTool(() => "select");
          break;
        case "f":
        case "F":
          if (pages) act.armTool(armed === "frame" ? "select" : "frame");
          break;
        case "p":
        case "P":
          if (acts) act.armTool(armed === "pen" ? "select" : "pen");
          break;
        case "Escape":
          act.clear();
          act.setTool((now) => (isArmed(now) ? "select" : now));
          act.clearNotice();
          break;
        case " ":
          if (pressable(e.target)) break;
          e.preventDefault();
          act.holdSpace(true);
          break;
        default:
      }
    };
    const onKeyUp = (e: KeyboardEvent) => {
      if (e.key === " ") ask.current.holdSpace(false);
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("keyup", onKeyUp);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("keyup", onKeyUp);
    };
  }, [paused, held]);
}
