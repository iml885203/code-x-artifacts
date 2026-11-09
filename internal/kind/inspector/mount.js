// Making the elements the room has and the file lacks, and putting the children of a parent in the room's order.
import { PID, find } from "./dom.js";

export const create = ({ parent: parentPid, before, ns, html }) => {
  const holder = find(parentPid);
  const next = before === null ? null : find(before);
  const doc = new DOMParser().parseFromString("<!doctype html><body>" + (ns ? "<svg>" + html : html), "text/html");
  const made = Array.from((ns ? doc.body.firstChild : doc.body).childNodes).filter((n) => n.nodeType === 1 && !find(n.getAttribute(PID)));
  if (holder) for (const n of made) holder.insertBefore(document.importNode(n, true), next && next.parentNode === holder ? next : null);
};

export const reorder = ([parentPid, kids]) => {
  const holder = find(parentPid);
  const els = kids.map(find).filter(Boolean);
  if (!holder || els.some((el) => el.contains(holder))) return;
  const moving = new Set(els);
  const now = Array.from(holder.children).filter((el) => moving.has(el));
  if (now.length === els.length && now.every((el, i) => el === els[i])) return;
  let before = now.length ? now[now.length - 1].nextSibling : null;
  for (let i = els.length - 1; i >= 0; i--) {
    holder.insertBefore(els[i], before);
    before = els[i];
  }
};
