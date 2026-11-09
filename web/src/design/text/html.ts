// The one place the browser turns an element's formatted text into markup:
// internal/designroom's reading of a formatted text (richNodes), in the browser.
// The frame shows it (text editing) and builds elements from it (ops/built.ts).
// internal/designroom/testdata/cases holds both sides to the same markup.
import { FORMAT, type DeltaOp } from "./units";

const KEPT = new Set(["b", "strong", "i", "em", "u", "a", "span", "code"]);
const NAME = /^[A-Za-z_][\w:.-]*$/;

export const escapeText = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
export const escapeAttr = (s: string) => s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

export type Row = [string, string, string];

/** An attribute list: as the room keeps it in a string, or already read. */
function rowsOf(value: unknown): Row[] {
  let rows: unknown = value;
  if (typeof value === "string") {
    try {
      rows = JSON.parse(value);
    } catch {
      return [];
    }
  }
  return Array.isArray(rows) ? rows.filter((r): r is Row => Array.isArray(r) && r.length === 3 && r.every((c) => typeof c === "string")) : [];
}

/** The attributes of an element as markup: no event handler, no name that is not one. */
export const attributes = (rows: Row[]) =>
  rows
    .map(([ns, key, value]) => [ns === "" ? key : `${ns}:${key}`, value] as const)
    .filter(([name]) => NAME.test(name) && !name.toLowerCase().startsWith("on"))
    .map(([name, value]) => ` ${name}="${escapeAttr(value)}"`)
    .join("");

/** What inline markup of text keeps: attributes with no namespace (a namespace belongs to SVG, which text does not hold). */
const unspaced = (rows: Row[]) => rows.filter(([ns]) => ns === "");

/** A link to the outside opens in a new tab that knows nothing of the page, as a published one does. */
function withLinkRules(tag: string, rows: Row[]): Row[] {
  const href = rows.find(([ns, key]) => ns === "" && key === "href")?.[2] ?? "";
  if (tag !== "a" || !/^(https:|mailto:)/i.test(href.trim())) return rows;
  const set = (list: Row[], key: string, value: string): Row[] => (list.some(([ns, k]) => ns === "" && k === key) ? list.map((r) => (r[0] === "" && r[1] === key ? ["", key, value] : r)) : [...list, ["", key, value]]);
  return set(set(rows, "target", "_blank"), "rel", "noopener noreferrer");
}

interface Open {
  key: string;
  tag: string;
}

/** The formats a run is inside, outermost first (by depth, then id), as designroom orders them. */
function openedBy(attributesOfRun: Record<string, unknown> | undefined): { key: string; pid: string; tag: string; depth: number; rows: Row[] }[] {
  const list: { key: string; pid: string; tag: string; depth: number; rows: Row[] }[] = [];
  for (const [key, value] of Object.entries(attributesOfRun ?? {})) {
    if (!key.startsWith(FORMAT) || typeof value !== "string") continue;
    try {
      const f = JSON.parse(value) as { t?: unknown; d?: unknown; a?: unknown };
      if (typeof f.t === "string" && KEPT.has(f.t)) list.push({ key, pid: key.slice(FORMAT.length), tag: f.t, depth: typeof f.d === "number" ? f.d : 0, rows: withLinkRules(f.t, unspaced(rowsOf(f.a))) });
    } catch {
      // a format that is not JSON says nothing about the markup
    }
  }
  return list.sort((a, b) => a.depth - b.depth || (a.pid < b.pid ? -1 : a.pid > b.pid ? 1 : 0));
}

/** The markup of a formatted text: each format opened where its stretch starts and closed where it ends. */
export function htmlOfDelta(delta: DeltaOp[]): string {
  let out = "";
  const stack: Open[] = [];
  for (const op of delta) {
    const want = openedBy(op.attributes);
    let keep = 0;
    while (keep < stack.length && keep < want.length && stack[keep]?.key === want[keep]?.key) keep++;
    while (stack.length > keep) out += `</${stack.pop()?.tag ?? ""}>`;
    for (const f of want.slice(keep)) {
      out += `<${f.tag}${attributes(f.rows)}>`;
      stack.push({ key: f.key, tag: f.tag });
    }
    if (typeof op.insert === "string") out += escapeText(op.insert);
    else if (typeof op.insert.br === "string") out += `<br${attributes(unspaced(rowsOf(op.insert.br)))}>`;
    else if (typeof op.insert.raw === "string") out += op.insert.raw;
  }
  while (stack.length > 0) out += `</${stack.pop()?.tag ?? ""}>`;
  return out;
}

/** The markup of an element that holds plain text. */
export const htmlOfText = escapeText;
