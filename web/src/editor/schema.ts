import { validDate } from "./features/date/commands";
// The editor's schema is the one the Go bridge reads and writes
// (internal/docroom/interop/schema.mjs): the same nodes, attributes, content
// rules and marks. The shared file knows nothing about the page, so what is added
// here is only how each node is drawn and which clipboard markup is understood.
// A test holds the two together (schema.test.ts).
//
// Pasting needs nothing more: the view reads the clipboard with the parse rules
// below, so only the elements they name become nodes and marks, every other element
// gives up its tags and keeps its text, and <script> and <style> give up their
// contents too. No attribute is copied from the clipboard (a node's attributes come
// from the rule that matched), so event handlers, styles and `javascript:` addresses
// have nowhere to go. Headings, lists, bold, italic and links with an allowed address
// keep their structure; quotes and code have no rule yet, so they
// arrive as paragraphs of their text.
import { DOMParser as PMParser, Fragment, Schema, type MarkSpec, type Node, type NodeSpec } from "prosemirror-model";
import { schema as shared } from "../../../internal/docroom/interop/schema.mjs";

/** The links a doc may hold; the same list the server validates (docs/doc-format.md). */
export function safeHref(href: unknown): string | null {
  if (typeof href !== "string") return null;
  return /^(https?:|mailto:|#)/i.test(href.trim()) ? href.trim() : null;
}

/**
 * An image is drawn only from one of this site's uploads (`/files/{artifact}/{name}`), an `https:` address, or a small
 * inline picture; anything else is a placeholder. The same list the server validates (docs/doc-format.md).
 */
export function safeImageSrc(src: unknown): string | null {
  if (typeof src !== "string" || /[\s\p{Cc}]/u.test(src)) return null;
  if (/^\/files\/[A-Za-z0-9_-]+\/[^/?#]+$/.test(src)) return src;
  if (/^https:\/\/[^/?#]+/i.test(src)) return src;
  return src.length <= 256 << 10 && /^data:image\/(png|jpeg|gif|webp);base64,[A-Za-z0-9+/]*=*$/i.test(src) ? src : null;
}

const imageWidths: Record<string, string> = { small: "224", medium: "336", large: "504", xlarge: "672" };

/** The number an `<ol start>` begins at; anything that is not a small whole number is 1. */
function startOf(dom: HTMLElement): number {
  const n = Number(dom.getAttribute("start"));
  return Number.isInteger(n) && n > 1 && n < 1e9 ? n : 1;
}

const alignOf = (cell: HTMLElement): "left" | "center" | "right" | null => {
  const said = (cell.getAttribute("align") ?? styleOf(cell, "text-align")).trim().toLowerCase();
  return said === "left" || said === "center" || said === "right" ? said : null;
};

/** A cell's text as the one paragraph a cell holds: its blocks run together with a space between them, its marks stay. */
function cellOf(cell: HTMLElement, schema: Schema, align: string | null): Node {
  const paragraph = schema.nodes.paragraph;
  if (paragraph === undefined) throw new Error("the schema has no paragraph");
  const copy = cell.cloneNode(true) as HTMLElement;
  for (const block of copy.querySelectorAll("p, div, li, h1, h2, h3, h4, h5, h6, blockquote, pre, tr")) block.append(copy.ownerDocument.createTextNode(" "));
  const made = PMParser.fromSchema(schema).parse(copy, { topNode: paragraph.create() });
  const inline: Node[] = [];
  made.descendants((node) => {
    if (node.isInline) inline.push(node);
    return !node.isInline;
  });
  const last = inline.at(-1);
  if (last?.isText === true && (last.text ?? "").endsWith(" ")) {
    const trimmed = (last.text ?? "").trimEnd();
    inline.pop();
    if (trimmed !== "") inline.push(schema.text(trimmed, last.marks));
  }
  const out = schema.nodes.table_cell?.create({ align }, paragraph.create(null, Fragment.from(inline)));
  if (out === undefined) throw new Error("the schema has no table_cell");
  return out;
}

/** The rows of a pasted table, every row as wide as the widest and within the limits a doc may hold (docs/doc-format.md). A column keeps the alignment of the first cell that names one. */
function rowsFromTable(table: HTMLElement, schema: Schema): Fragment {
  const MAX_COLUMNS = 30;
  const MAX_CELLS = 2000;
  const rows = [...table.querySelectorAll("tr")].filter((tr) => tr.closest("table") === table);
  const grid = rows.map((tr) => {
    const cells: HTMLElement[] = [];
    for (const cell of tr.children) {
      if (!(cell instanceof HTMLElement) || (cell.localName !== "td" && cell.localName !== "th")) continue;
      const span = Math.max(1, Math.min(MAX_COLUMNS, Number.parseInt(cell.getAttribute("colspan") ?? "1", 10) || 1));
      for (let i = 0; i < span; i++) cells.push(cell);
    }
    return cells;
  });
  const width = Math.min(MAX_COLUMNS, Math.max(1, ...grid.map((r) => r.length)));
  const height = Math.max(1, Math.min(grid.length, Math.floor(MAX_CELLS / width)));
  const aligns: (string | null)[] = Array.from({ length: width }, (_, c) => grid.map((r) => r[c]).find((cell) => cell !== undefined && alignOf(cell) !== null)).map((cell) => (cell === undefined ? null : alignOf(cell)));
  const nodes: Node[] = [];
  for (let r = 0; r < height; r++) {
    const row = grid[r] ?? [];
    const cells = Array.from({ length: width }, (_, c) => {
      const cell = row[c];
      const align = aligns[c] ?? null;
      if (cell === undefined) return schema.nodes.table_cell?.createAndFill({ align });
      return cellOf(cell, schema, align);
    }).filter((n): n is Node => n !== undefined);
    const made = schema.nodes.table_row?.create(null, cells);
    if (made !== undefined) nodes.push(made);
  }
  return Fragment.from(nodes);
}

/** A table is headed when it has a `thead` or its first row is all `th`. */
const headed = (table: HTMLElement): boolean => {
  if (table.querySelector("thead") !== null) return true;
  const first = table.querySelector("tr");
  return first !== null && first.children.length > 0 && [...first.children].every((c) => c.localName === "th");
};

const clampLevel = (level: unknown): number => (typeof level === "number" && level >= 1 && level <= 6 ? Math.trunc(level) : 1);

// How each node is drawn. Attributes are only ever the ones written here: what a
// node holds never becomes markup, so a hostile room cannot put an attribute in the page.
export interface DropdownConfig { options: string[]; value: string }

/** The atom stores the complete configuration in one attribute, so peer edits cannot mix options and value. */
export function readDropdownConfig(raw: unknown): DropdownConfig | null {
  if (typeof raw !== "string") return null;
  try {
    const value: unknown = JSON.parse(raw);
    if (typeof value !== "object" || value === null || Array.isArray(value)) return null;
    const config = value as Record<string, unknown>;
    if (Object.keys(config).length !== 2 || !Array.isArray(config.options) || typeof config.value !== "string") return null;
    const options: unknown[] = config.options;
    const label = (option: unknown): option is string => typeof option === "string" && option !== "" && option === option.trim() && Array.from(option).length <= 100 && !/[\p{Cc}\u2028\u2029]/u.test(option);
    if (options.length < 1 || options.length > 50 || !options.every(label) || new Set(options).size !== options.length || !options.includes(config.value)) return null;
    return { options, value: config.value };
  } catch { return null; }
}

const draw: Record<string, Partial<NodeSpec>> = {
  tab: { toDOM: () => ["section", { class: "ed-tab" }, 0] },
  paragraph: { toDOM: () => ["p", 0], parseDOM: [{ tag: "p" }] },
  heading: {
    toDOM: (node) => [`h${String(clampLevel(node.attrs.level))}`, 0],
    parseDOM: [1, 2, 3, 4, 5, 6].map((level) => ({ tag: `h${String(level)}`, attrs: { level } })),
  },
  bullet_list: { toDOM: () => ["ul", 0], parseDOM: [{ tag: "ul" }] },
  numbered_list: {
    toDOM: (node) => (typeof node.attrs.start === "number" && node.attrs.start > 1 ? ["ol", { start: String(node.attrs.start) }, 0] : ["ol", 0]),
    parseDOM: [{ tag: "ol", getAttrs: (dom) => ({ start: startOf(dom) }) }],
  },
  task_list: { toDOM: () => ["ul", { class: "ed-tasks" }, 0] },
  // A to-do item holds a checkbox before its content; the box is what a click flips (view.ts).
  list_item: {
    parseDOM: [{ tag: "li" }],
    toDOM: (node) =>
      node.attrs.checked === null
        ? ["li", 0]
        : ["li", { class: node.attrs.checked === true ? "ed-task ed-done" : "ed-task" }, ["input", { type: "checkbox", class: "ed-check", contenteditable: "false", tabindex: "-1", ...(node.attrs.checked === true ? { checked: "" } : {}) }], ["div", 0]],
  },
  quote: { toDOM: () => ["blockquote", 0] },
  code_block: { toDOM: () => ["pre", ["code", 0]] },
  rule: { toDOM: () => ["hr"] },
  table: {
    toDOM: (node) => ["table", node.attrs.header === true ? { class: "ed-headed" } : {}, ["tbody", 0]],
    parseDOM: [{ tag: "table", getAttrs: (dom) => ({ header: headed(dom) }), getContent: (dom, schema) => rowsFromTable(dom as HTMLElement, schema) }],
  },
  table_row: { toDOM: () => ["tr", 0] },
  table_cell: { toDOM: (node) => (typeof node.attrs.align === "string" && /^(left|center|right)$/.test(node.attrs.align) ? ["td", { class: `ed-${node.attrs.align}` }, 0] : ["td", 0]) },
  image: {
    toDOM: (node) => {
      const src = safeImageSrc(node.attrs.src);
      const alt = typeof node.attrs.alt === "string" ? node.attrs.alt : "";
      const width = typeof node.attrs.size === "string" ? imageWidths[node.attrs.size] : undefined;
      return src === null ? ["span", { class: "ed-missing" }, alt] : ["img", { src, alt, ...(width === undefined ? {} : { width }) }];
    },
    parseDOM: [
      {
        tag: "img[src]",
        getAttrs: (dom) => {
          const src = safeImageSrc(dom.getAttribute("src"));
          const alt = dom.getAttribute("alt");
          return src === null ? false : { src, alt: alt === null || alt === "" ? null : alt };
        },
      },
    ],
  },
  pending: { toDOM: (node) => ["div", { class: "ed-pending", contenteditable: "false" }, typeof node.attrs.intent === "string" ? node.attrs.intent : ""] },
  hard_break: { toDOM: () => ["br"], parseDOM: [{ tag: "br" }] },
  // A person named in the text: the name after an @, as text. No link, no script: what the node holds never becomes markup.
  dropdown: {
    toDOM: (node) => {
      const config = readDropdownConfig(node.attrs.config);
      return ["span", { class: "ed-dropdown", "data-dropdown-config": config ? JSON.stringify(config) : "", contenteditable: "false" }, config?.value ?? ""];
    },
    parseDOM: [{ tag: "span[data-dropdown-config]", getAttrs: (dom) => {
      const config = readDropdownConfig(dom.getAttribute("data-dropdown-config"));
      return config ? { config: JSON.stringify(config) } : false;
    } }],
  },
  date: {
    toDOM: (node) => ["time", { class: "ed-date", datetime: typeof node.attrs.date === "string" ? node.attrs.date : "", contenteditable: "false" }, typeof node.attrs.date === "string" ? node.attrs.date : ""],
    parseDOM: [{ tag: "time[datetime]", getAttrs: (dom) => { const date = dom.getAttribute("datetime"); return validDate(date) ? { date } : false; } }],
  },
  mention: {
    toDOM: (node) => ["span", { class: "ed-mention", "data-user": typeof node.attrs.user === "string" ? node.attrs.user : "", contenteditable: "false" }, `@${typeof node.attrs.name === "string" ? node.attrs.name : ""}`],
    parseDOM: [{ tag: "span.ed-mention[data-user]", getAttrs: (dom) => mentionOf(dom) }],
  },
};

/** The account and the name of a pasted mention, only when both are what a doc may hold (docs/doc-format.md); otherwise the pasted element is plain text. */
function mentionOf(dom: HTMLElement): { user: string; name: string } | false {
  const user = dom.getAttribute("data-user") ?? "";
  const name = dom.textContent.replace(/^@/, "");
  const ok = /^[A-Za-z0-9._-]{1,64}$/.test(user) && name.trim() !== "" && Array.from(name).length <= 100 && !/\p{Cc}/u.test(name);
  return ok ? { user, name } : false;
}

/** Where a pasted element's `style` attribute is kept (view.ts renames it before parsing): the page's CSP refuses to apply an inline style, so `element.style` of a pasted element says nothing. */
export const pastedStyle = "data-pasted-style";

/** One property of the style a pasted element came with, lower-cased and trimmed; "" when it has none. */
const styleOf = (dom: HTMLElement, property: string): string => new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*([^;]+)`, "i").exec(dom.getAttribute(pastedStyle) ?? "")?.[1]?.trim().toLowerCase() ?? "";

/** Word and Google Docs say "bold" and "italic" in styles; Google Docs also wraps a whole copy in a `<b>` that says it is not bold. */
const styledBold = (value: string): boolean => /^(bold(er)?|[5-9]\d\d)$/i.test(value.trim());
const notBold = (dom: HTMLElement): boolean => /^(normal|[1-4]\d\d)$/.test(styleOf(dom, "font-weight"));

// A mark keeps its meaning when pasted; a link only when its address is on the list
// (an address that is not stays as plain text).
const drawMarks: Record<string, Partial<MarkSpec>> = {
  bold: {
    toDOM: () => ["strong", 0],
    parseDOM: [{ tag: "strong" }, { tag: "b", getAttrs: (dom) => !notBold(dom) && null }, { tag: "span", getAttrs: (dom) => styledBold(styleOf(dom, "font-weight")) && null }],
  },
  italic: {
    toDOM: () => ["em", 0],
    parseDOM: [{ tag: "em" }, { tag: "i", getAttrs: (dom) => styleOf(dom, "font-style") !== "normal" && null }, { tag: "span", getAttrs: (dom) => styleOf(dom, "font-style") === "italic" && null }],
  },
  underline: { toDOM: () => ["u", 0], parseDOM: [{ tag: "u" }] },
  strike: { toDOM: () => ["s", 0], parseDOM: [{ tag: "s" }, { tag: "del" }, { tag: "strike" }] },
  code: { toDOM: () => ["code", 0], parseDOM: [{ tag: "code" }] },
  highlight: { toDOM: () => ["mark", 0], parseDOM: [{ tag: "mark" }] },
  link: {
    parseDOM: [{ tag: "a[href]", getAttrs: (dom) => { const href = safeHref(dom.getAttribute("href")); return href === null ? false : { href }; } }],
    toDOM: (m) => {
      const href = safeHref(m.attrs.href);
      return href === null ? ["span", { class: "ed-link" }, 0] : ["a", { href, rel: "noopener noreferrer nofollow", target: "_blank" }, 0];
    },
  },
};

let nodes = shared.spec.nodes;
for (const [name, extra] of Object.entries(draw)) {
  const spec = nodes.get(name);
  if (!spec) throw new Error(`the shared schema has no node ${name}`);
  nodes = nodes.update(name, { ...spec, ...extra });
}
let marks = shared.spec.marks;
for (const [name, extra] of Object.entries(drawMarks)) {
  const spec = marks.get(name);
  if (!spec) throw new Error(`the shared schema has no mark ${name}`);
  marks = marks.update(name, { ...spec, ...extra });
}

export const schema = new Schema({ nodes, marks });
