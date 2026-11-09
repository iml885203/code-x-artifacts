import type { Shape } from "../ops/nodes";

/** What a press on the canvas does: pick, pan, draw a frame, or place a new element. */
export type Tool = "select" | "hand" | "frame" | "pen" | "pencil" | "line" | "arrow" | Shape;

export const isShape = (tool: Tool): tool is Shape => tool !== "select" && tool !== "hand" && tool !== "frame" && tool !== "pen" && tool !== "pencil" && tool !== "line" && tool !== "arrow";

/** Whether the tool waits for the next press to put something on the canvas (Esc puts it away). */
export const isArmed = (tool: Tool): boolean => tool !== "select" && tool !== "hand";
