"use strict";
// The only script a frame runs: it answers its parent, and nobody else, with what an element is,
// and, told to (apply, edit), shows what the room holds or lets a person type. Every answer carries
// the asker's number. What it does and does not do: internal/kind/AGENTS.md.
//
// It is built from the modules next to this file into ../design_inspector.js (make inspector):
// the built file is in every frame, so it has a size limit (TestTheInspectorStaysSmallBecauseItIsInEveryFrame)
// and the comments stay here.
import { showTokens } from "./tokens.js";
import { apply } from "./apply.js";
import { describe } from "./describe.js";
import { PID, find } from "./dom.js";
import { edit } from "./edit.js";
import { hidden } from "./style.js";

const answer = (m) => {
  if (m.type === "hit" && typeof m.x === "number" && typeof m.y === "number") {
    const under = document.elementFromPoint(m.x, m.y);
    const owner = under && under.closest("[" + PID + "]");
    return { pid: owner ? owner.getAttribute(PID) : null };
  }
  // Every element with an id from the one under a point outwards: the nearest one may be inside a text.
  if (m.type === "chain" && typeof m.x === "number" && typeof m.y === "number") {
    const pids = [];
    for (let e = document.elementFromPoint(m.x, m.y); e; e = e.parentElement) if (e.hasAttribute(PID)) pids.push(e.getAttribute(PID));
    return { pids };
  }
  if (m.type === "edit" && typeof m.pid === "string" && typeof m.html === "string") return edit(m);
  if (m.type === "report" && Array.isArray(m.pids)) {
    const items = [];
    for (const pid of m.pids) {
      const el = typeof pid === "string" ? find(pid) : null;
      if (el && !hidden.has(el)) items.push(describe(el));
    }
    return { items };
  }
  if (m.type === "tokens") return showTokens(m.tokens);
  if (m.type === "apply") return apply(m);
  return null;
};

addEventListener("message", (e) => {
  if (e.source !== parent) return;
  const m = e.data;
  if (!m || m.pictor !== "inspect" || typeof m.id !== "number") return;
  const reply = answer(m);
  if (reply) parent.postMessage({ pictor: "inspect", id: m.id, type: m.type, ...reply }, "*");
});

addEventListener("DOMContentLoaded", () => {
  // Says which elements it has, so the page knows which of the room's to build.
  const pids = Array.from(document.querySelectorAll("[" + PID + "]"), (el) => el.getAttribute(PID));
  parent.postMessage({ pictor: "inspect", type: "ready", pids }, "*");
});
