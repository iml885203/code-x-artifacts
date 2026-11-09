/**
 * The strings of one slide, by their place in it: the path the deck's page marks on each editable string
 * (`data-edit`, such as `elements/1/rows/0/2`). Reading and changing a slide by path is all the editor does to
 * the slide's JSON; every change returns a new slide and leaves the one it was given as it was.
 */
export type Slide = Record<string, unknown>;

/** The element types the insert tool adds; cards and stats are the slide's shapes. */
export type Insertable = "text" | "bullets" | "table" | "cards" | "stats";
export const insertables: readonly Insertable[] = ["text", "bullets", "table", "cards", "stats"];

/** The most elements a slide holds (the format's limit). */
export const maxElements = 8;

/** A change a person makes to a slide: text typed at a place, or an element added at the end. */
export type Change = { op: "text"; path: string; text: string } | { op: "insert"; kind: Insertable; element: Slide };

/** The words a new element starts with, in the person's language. */
export interface Words {
  text: string;
  point: string;
  column: (n: number) => string;
  card: (n: number) => string;
  cardText: string;
  statValue: string;
  statLabel: string;
}

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

function index(step: string, length: number): number | null {
  if (!/^\d+$/.test(step)) return null;
  const i = Number(step);
  return i < length ? i : null;
}

/** A leaf's text: a string, a stat's number, or a table cell written as {"text", "tone"}. */
function leaf(at: unknown): string | null {
  if (typeof at === "string") return at;
  if (typeof at === "number") return String(at);
  if (isObject(at) && ("text" in at || "tone" in at)) return typeof at.text === "string" ? at.text : at.text === undefined ? "" : null;
  return null;
}

/** The text at a place in the slide, or null when the slide has no string there. A step given as a string is its own title. */
export function textAt(slide: unknown, path: string): string | null {
  let at = slide;
  const steps = path.split("/");
  for (const [i, step] of steps.entries()) {
    if (Array.isArray(at)) {
      const n = index(step, at.length);
      if (n === null) return null;
      at = at[n];
    } else if (typeof at === "string" && step === "title" && i === steps.length - 1) {
      return at;
    } else if (isObject(at) && Object.hasOwn(at, step)) {
      at = at[step];
    } else {
      return null;
    }
  }
  return leaf(at);
}

/** The places inside an element of each type that the deck's page marks as editable (kind's slides_elements.go). */
const placesOf: Record<string, RegExp> = {
  text: /^text$/,
  bullets: /^items\/\d+$/,
  numbered: /^items\/\d+$/,
  table: /^(columns\/\d+|rows\/\d+\/\d+)$/,
  stats: /^items\/\d+\/(value|label)$/,
  quote: /^(text|by)$/,
  steps: /^items\/\d+\/(title|text)$/,
  cards: /^items\/\d+\/(title|text|items\/\d+)$/,
  progress: /^items\/\d+\/label$/,
};

function editableIn(element: unknown, rest: string): boolean {
  if (!isObject(element) || typeof element.type !== "string") return false;
  if (element.type === "columns") {
    const m = /^columns\/(\d+)\/(title|elements\/(\d+)\/(.+))$/.exec(rest);
    if (!m) return false;
    if (m[2] === "title") return true;
    const col = Array.isArray(element.columns) ? (element.columns[Number(m[1])] as unknown) : undefined;
    const inner = isObject(col) && Array.isArray(col.elements) ? (col.elements[Number(m[3])] as unknown) : undefined;
    return isObject(inner) && inner.type !== "columns" && editableIn(inner, m[4] ?? "");
  }
  return placesOf[element.type]?.test(rest) ?? false;
}

/**
 * Whether `path` is a place the deck's page marks as editable on this slide, judged by the element's type, and holds
 * a string there. A page that names any other place (an element's type, a code block, a language) is not heard.
 */
export function editable(slide: Slide, path: string): boolean {
  if (textAt(slide, path) === null) return false;
  if (path === "title") return true;
  const m = /^elements\/(\d+)\/(.+)$/.exec(path);
  const elements = elementsOf(slide);
  return m !== null && editableIn(elements[Number(m[1])], m[2] ?? "");
}

function put(at: unknown, steps: readonly string[], text: string): unknown {
  if (steps.length === 0) {
    if (typeof at === "string" || typeof at === "number") return text;
    return isObject(at) && leaf(at) !== null ? { ...at, text } : undefined;
  }
  const [step = "", ...rest] = steps;
  if (Array.isArray(at)) {
    const n = index(step, at.length);
    if (n === null) return undefined;
    const v = put(at[n], rest, text);
    return v === undefined ? undefined : at.map((x: unknown, i) => (i === n ? v : x));
  }
  if (typeof at === "string" && step === "title" && rest.length === 0) return text;
  if (!isObject(at) || !Object.hasOwn(at, step)) return undefined;
  const v = put(at[step], rest, text);
  return v === undefined ? undefined : { ...at, [step]: v };
}

/** The slide with `text` at `path`, or null when the slide has no string there. */
export function withText(slide: Slide, path: string, text: string): Slide | null {
  if (path === "") return null;
  const out = put(slide, path.split("/"), text);
  return isObject(out) ? out : null;
}

/** A text element's text keeps its line breaks (Shift+Enter); every other string is one line. */
export function multiline(slide: Slide, path: string): boolean {
  const m = /^(.*)\/text$/.exec(path);
  if (!m?.[1]) return false;
  let at: unknown = slide;
  for (const step of m[1].split("/")) at = Array.isArray(at) ? at[Number(step)] : isObject(at) ? at[step] : undefined;
  return isObject(at) && at.type === "text";
}

const elementsOf = (slide: Slide): unknown[] => (Array.isArray(slide.elements) ? slide.elements : []);

/** Whether the slide has room for one more element. */
export const canInsert = (slide: Slide): boolean => elementsOf(slide).length < maxElements;

/** A new element of a kind, with valid content to type over. */
export function newElement(kind: Insertable, w: Words): Slide {
  switch (kind) {
    case "text":
      return { type: "text", text: w.text };
    case "bullets":
      return { type: "bullets", items: [w.point] };
    case "table":
      return { type: "table", columns: [w.column(1), w.column(2)], rows: [["", ""], ["", ""]] };
    case "cards":
      return { type: "cards", items: [{ title: w.card(1), text: w.cardText }, { title: w.card(2), text: w.cardText }] };
    case "stats":
      return { type: "stats", items: [{ value: w.statValue, label: w.statLabel }] };
  }
}

/** The first place to type in a new element of a kind that is element `at` of the slide. */
export function firstPlace(kind: Insertable, at: number): string {
  const inside = { text: "text", bullets: "items/0", table: "columns/0", cards: "items/0/title", stats: "items/0/value" }[kind];
  return `elements/${String(at)}/${inside}`;
}

/** The slide after a change, or null when the change has no place in it (the string is gone, or the slide is full). */
export function applyChange(slide: Slide, change: Change): Slide | null {
  if (change.op === "text") return withText(slide, change.path, change.text);
  if (!canInsert(slide)) return null;
  return { ...slide, elements: [...elementsOf(slide), change.element] };
}
