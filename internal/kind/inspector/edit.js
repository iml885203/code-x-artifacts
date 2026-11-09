// Typing into an element in the frame: one session at a time, and what it became when it ends.
import { PID, find, setHtml } from "./dom.js";

// The element being typed in, not touched by `texts` meanwhile (exported as a live binding).
export let editing = null;

// An element's content as runs: text, breaks and markup kept as written, each with the inline
// elements it is inside; markup the browser adds while editing is dropped, its content kept.
const KEEP = ["b", "strong", "i", "em", "u", "a", "span", "code"];
const MADE = ["b", "strong", "i", "em", "u"];
const RAW = ["s", "small", "mark", "sub", "sup", "abbr", "cite", "q", "kbd", "samp", "var", "time", "del", "ins", "data", "dfn", "bdi", "bdo", "wbr", "ruby", "rt", "rp"];
const runs = (node, f, out) => {
  for (const c of node.childNodes) {
    const tag = c.localName;
    if (c.nodeType === 3) out.push({ s: c.data, f });
    else if (c.nodeType === 8) out.push({ raw: "<!--" + c.data + "-->", f });
    else if (tag === "br") out.push({ br: 1, n: !c.hasAttribute(PID), f });
    else if (RAW.includes(tag)) out.push({ raw: c.outerHTML, f });
    else if (KEEP.includes(tag) && (c.hasAttribute(PID) || MADE.includes(tag))) {
      if (!c.hasAttribute(PID)) c.setAttribute(PID, "x" + Math.random().toString(36).slice(2, 10));
      runs(c, [...f, { t: tag, p: c.getAttribute(PID) }], out);
    } else if (c.nodeType === 1) runs(c, f, out);
  }
  return out;
};

// Done: the element gets its attributes back exactly (the browser rewrites style), the listeners go, the page is told.
const end = (keep) => {
  const s = editing;
  if (!s) return;
  editing = null;
  s.stop.abort();
  let out = null;
  if (keep) {
    out = runs(s.el, [], []);
    // A break the browser keeps at the end of an empty line is not the person's.
    if (out.length && out[out.length - 1].n) out.pop();
  } else setHtml(s.el, s.html);
  for (const a of Array.from(s.el.attributes)) s.el.removeAttribute(a.name);
  for (const [k, v] of s.attrs) s.el.setAttribute(k, v);
  parent.postMessage({ pictor: "inspect", type: "edited", pid: s.pid, runs: out }, "*");
};

export const edit = (m) => {
  end(true);
  const el = find(m.pid);
  if (!el) return { ok: false };
  setHtml(el, m.html);
  const s = (editing = { el, pid: m.pid, html: el.innerHTML, attrs: Array.from(el.attributes, (a) => [a.name, a.value]), stop: new AbortController(), composing: false });
  const on = (type, fn) => el.addEventListener(type, fn, { signal: s.stop.signal });
  el.setAttribute("contenteditable", "true");
  on("compositionstart", () => (s.composing = true));
  on("compositionend", () => (s.composing = false));
  on("blur", () => end(true));
  // What is pasted is its text.
  on("paste", (e) => {
    e.preventDefault();
    document.execCommand("insertText", false, e.clipboardData.getData("text/plain"));
  });
  on("keydown", (e) => {
    // Enter that picks a word is the input method's, not the person's.
    if (s.composing || e.isComposing || e.keyCode === 229) return;
    const key = e.key.toLowerCase();
    if (e.key === "Enter" && !e.shiftKey) end(true);
    else if (e.key === "Escape") end(false);
    else if ((e.metaKey || e.ctrlKey) && (key === "b" || key === "i")) document.execCommand(key === "b" ? "bold" : "italic");
    else return;
    e.preventDefault();
  });
  el.focus();
  getSelection().selectAllChildren(el);
  return { ok: true };
};
