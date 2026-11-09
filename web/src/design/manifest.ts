// design.json as the canvas reads it. The server has validated it before it
// was ever saved, so this only turns the JSON into what the canvas draws and
// refuses what is not shaped like one rather than guessing.

export interface DesignFrame {
  id: string;
  name: string;
  /** The path of the frame's HTML among the artifact's files. */
  file: string;
  w: number;
  h: number;
}

export interface DesignPage {
  id: string;
  name: string;
  rows: DesignFrame[][];
}

export interface Design {
  pages: DesignPage[];
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

function readFrame(v: unknown): DesignFrame | null {
  if (!isRecord(v)) return null;
  const { id, name, file, w, h } = v;
  if (typeof id !== "string" || typeof name !== "string" || typeof file !== "string" || typeof w !== "number" || typeof h !== "number") return null;
  return { id, name, file, w, h };
}

function readPage(v: unknown): DesignPage | null {
  if (!isRecord(v) || typeof v.id !== "string" || typeof v.name !== "string" || !Array.isArray(v.rows)) return null;
  const rows: DesignFrame[][] = [];
  for (const row of v.rows as unknown[]) {
    if (!Array.isArray(row)) return null;
    const frames = (row as unknown[]).map(readFrame);
    if (frames.some((f) => f === null)) return null;
    rows.push(frames as DesignFrame[]);
  }
  return { id: v.id, name: v.name, rows };
}

/** The design in a design.json, or null when it is not one. */
export function readDesign(raw: unknown): Design | null {
  if (!isRecord(raw) || !Array.isArray(raw.pages)) return null;
  const pages = (raw.pages as unknown[]).map(readPage);
  if (pages.length === 0 || pages.some((p) => p === null)) return null;
  return { pages: pages as DesignPage[] };
}

/** Where a frame stands on the canvas, in canvas pixels from its top left corner. */
export interface Placed {
  frame: DesignFrame;
  x: number;
  y: number;
}

export interface Layout {
  frames: Placed[];
  /** The size of everything on the page, margins included. */
  width: number;
  height: number;
}

/** The space between frames of a row, and between rows (room for the names above the frames). */
export const GAP_X = 96;
export const GAP_Y = 168;
/** The margin around the page. */
export const MARGIN = 120;

/** Rows from top to bottom, the frames of a row side by side from the left, tops aligned. */
export function layoutPage(page: DesignPage): Layout {
  const frames: Placed[] = [];
  let y = MARGIN;
  let width = 0;
  for (const row of page.rows) {
    let x = MARGIN;
    let tallest = 0;
    for (const frame of row) {
      frames.push({ frame, x, y });
      x += frame.w + GAP_X;
      tallest = Math.max(tallest, frame.h);
    }
    width = Math.max(width, x - GAP_X);
    y += tallest + GAP_Y;
  }
  return { frames, width: width + MARGIN, height: y - GAP_Y + MARGIN };
}
