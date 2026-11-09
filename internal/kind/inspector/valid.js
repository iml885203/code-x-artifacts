// What an `apply` may say: anything else is refused whole, and nothing of it is done.
import { OWNED } from "./style.js";

const text = (v) => typeof v === "string";

// create: [{parent, before, ns, html}], markup for under parent, before the element `before` (null: the end).
export const validCreate = (c) =>
  c === undefined || (Array.isArray(c) && c.every((x) => x && text(x.parent) && (x.before === null || text(x.before)) && text(x.html) && (x.ns === undefined || text(x.ns))));

// attrs: [[pid, "alt" | "src", value or null]].
export const validAttrs = (a) =>
  a === undefined || (Array.isArray(a) && a.every((x) => Array.isArray(x) && text(x[0]) && ["alt", "src"].includes(x[1]) && (x[2] === null || text(x[2]))));

export const valid = (m) =>
  Array.isArray(m.styles) &&
  m.styles.every((s) => Array.isArray(s) && text(s[0]) && OWNED.includes(s[1]) && (s[2] === null || text(s[2]))) &&
  (m.inline === undefined || (Array.isArray(m.inline) && m.inline.every((s) => Array.isArray(s) && text(s[0]) && (s[1] === null || text(s[1]))))) &&
  (m.alive === undefined || (Array.isArray(m.alive) && m.alive.every(text))) &&
  (m.texts === undefined || (Array.isArray(m.texts) && m.texts.every((t) => Array.isArray(t) && text(t[0]) && text(t[1]))));

// order: [[parent pid, [child pids in order]]], elements only.
export const validOrder = (o) =>
  o === undefined || (Array.isArray(o) && o.every((r) => Array.isArray(r) && text(r[0]) && Array.isArray(r[1]) && r[1].every(text)));
