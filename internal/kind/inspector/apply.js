// What the room holds, shown in the frame: elements, attributes, order, text, styles, which elements are there.
import { create, reorder } from "./mount.js";
import { PID, find, setHtml } from "./dom.js";
import { editing } from "./edit.js";
import { setInline, setStyle, show } from "./style.js";
import { valid, validAttrs, validCreate, validOrder } from "./valid.js";

export const apply = (m) => {
  if (!valid(m) || !validOrder(m.order) || !validCreate(m.create) || !validAttrs(m.attrs)) return null;
  for (const c of m.create || []) create(c);
  for (const [pid, name, value] of m.attrs || []) {
    const el = find(pid);
    if (el && value === null) el.removeAttribute(name);
    else if (el) el.setAttribute(name, value);
  }
  for (const row of m.order || []) reorder(row);
  for (const [pid, html] of m.texts || []) {
    const el = find(pid);
    if (el && !(editing && editing.el === el)) setHtml(el, html);
  }
  for (const [pid, source] of m.inline || []) {
    const el = find(pid);
    if (el) setInline(el, source);
  }
  for (const [pid, prop, value] of m.styles) {
    const el = find(pid);
    if (el) setStyle(el, prop, value);
  }
  if (m.alive) {
    const alive = new Set(m.alive);
    for (const el of document.querySelectorAll("[" + PID + "]")) show(el, alive.has(el.getAttribute(PID)));
  }
  return { ok: true };
};
