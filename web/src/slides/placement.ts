/**
 * Where the deck's page says each editable string of the shown slide sits. Only the place is heard: a path and a
 * box as fractions of the frame, plus the type size, so the shell can put its own field over the text. The text a
 * person edits comes from the slide's JSON, never from the page, and a path the slide does not have is dropped.
 */
export interface Placed {
  path: string;
  x: number;
  y: number;
  w: number;
  h: number;
  /** Font size and line height as fractions of the frame's width. */
  size: number;
  line: number;
  weight: number;
  align: "left" | "center" | "right";
}

const maxFields = 400;
const path = /^(title|elements(\/[a-z0-9]+)+)$/;
const aligns = new Set(["left", "center", "right"]);

const within = (v: unknown, lo: number, hi: number): v is number => typeof v === "number" && Number.isFinite(v) && v >= lo && v <= hi;

function one(v: unknown): Placed | null {
  if (typeof v !== "object" || v === null) return null;
  const f = v as Record<string, unknown>;
  if (typeof f.path !== "string" || f.path.length > 160 || !path.test(f.path)) return null;
  if (!within(f.x, -0.5, 1.5) || !within(f.y, -0.5, 1.5) || !within(f.w, 0, 2) || !within(f.h, 0, 2)) return null;
  if (!within(f.size, 0, 0.5) || !within(f.line, 0, 1) || !within(f.weight, 1, 1000)) return null;
  const align = typeof f.align === "string" && aligns.has(f.align) ? (f.align as Placed["align"]) : "left";
  return { path: f.path, x: f.x, y: f.y, w: f.w, h: f.h, size: f.size, line: f.line, weight: f.weight, align };
}

/** The places a `fields` message from `frame` names for slide `number`, or null for any other message. */
export function placedFields(e: MessageEvent, frame: HTMLIFrameElement | null): { number: number; fields: Placed[] } | null {
  if (e.source === null || e.source !== frame?.contentWindow || typeof e.data !== "object" || e.data === null) return null;
  const m = e.data as { pictor?: unknown; number?: unknown; fields?: unknown };
  if (m.pictor !== "fields" || !within(m.number, 1, 1000) || !Number.isInteger(m.number) || !Array.isArray(m.fields)) return null;
  const fields = m.fields.slice(0, maxFields).map(one).filter((f): f is Placed => f !== null);
  return { number: m.number, fields };
}
