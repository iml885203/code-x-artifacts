import type { Shape, Stack } from "./nodes";

export type OpsKey =
  | { do: "duplicate" | "copy" | "paste" | "group" | "ungroup" }
  | { do: "stack"; how: Stack }
  | { do: "tool"; shape: Shape };

/**
 * What a key press asks of the elements, or null for a key that is not ours.
 * Command or Control with D, C, V, G (Shift for Ungroup), and the square
 * brackets for the stacking order (Shift for all the way); T, R and O alone pick
 * the insert tools. Letters are read by what they say and the brackets by the key
 * they are on, because Shift changes what a bracket says.
 */
export function opsKey(e: Pick<KeyboardEvent, "key" | "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">): OpsKey | null {
  if (e.altKey) return null;
  const mod = e.metaKey || e.ctrlKey;
  const letter = e.key.toLowerCase();
  if (mod) {
    if (e.code === "BracketRight") return { do: "stack", how: e.shiftKey ? "front" : "forward" };
    if (e.code === "BracketLeft") return { do: "stack", how: e.shiftKey ? "back" : "backward" };
    if (e.shiftKey) return letter === "g" ? { do: "ungroup" } : null;
    switch (letter) {
      case "d":
        return { do: "duplicate" };
      case "c":
        return { do: "copy" };
      case "v":
        return { do: "paste" };
      case "g":
        return { do: "group" };
      default:
        return null;
    }
  }
  if (e.shiftKey) return null;
  if (letter === "t") return { do: "tool", shape: "text" };
  if (letter === "r") return { do: "tool", shape: "rect" };
  if (letter === "o") return { do: "tool", shape: "oval" };
  return null;
}
