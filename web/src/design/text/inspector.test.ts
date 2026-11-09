import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

// The script the server puts in every frame, run in a document of its own: here
// the part that lets a person type into an element and says what it became.
const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "../../../../internal/kind/design_inspector.js"), "utf8");

interface Said {
  pictor: string;
  id?: number;
  type: string;
  [k: string]: unknown;
}
const sent: Said[] = [];
const parent = { postMessage: (message: Said) => sent.push(message) };
const listeners: Record<string, ((e: unknown) => void)[]> = {};
const emit = (type: string, event: object) => {
  for (const listener of listeners[type] ?? []) listener(event);
};
let turn = 0;
const ask = (body: object) => {
  emit("message", { source: parent, data: { pictor: "inspect", id: ++turn, ...body } });
};
const edited = () => sent.filter((m) => m.type === "edited");

beforeAll(() => {
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const run = new Function("parent", "addEventListener", "scrollX", "scrollY", source) as (...args: unknown[]) => void;
  run(parent, (type: string, listener: (e: unknown) => void) => (listeners[type] ??= []).push(listener), 0, 0);
});

const execCommand = vi.fn(() => true);
const must = <T,>(value: T | null | undefined): T => {
  if (value === null || value === undefined) throw new Error("not there");
  return value;
};
const STYLE = "position:absolute;left:48px;top:48px";
beforeEach(() => {
  // A test may leave a person typing; the next one begins with nobody.
  ask({ type: "edit", pid: "nobody", html: "" });
  sent.length = 0;
  Object.defineProperty(document, "execCommand", { value: execCommand, configurable: true });
  execCommand.mockClear();
  const page = new DOMParser().parseFromString(`<h1 data-pid="e2" class="t" style="${STYLE}">Hello</h1><p data-pid="e3">Hi <b data-pid="e4">bold</b><br data-pid="e5">line2</p>`, "text/html");
  document.body.replaceChildren(...page.body.childNodes);
  document.body.setAttribute("data-pid", "e1");
});

const key = (el: Element, init: KeyboardEventInit & { keyCode?: number }) => {
  const e = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  el.dispatchEvent(e);
  return e;
};
const start = (pid: string, html: string) => {
  ask({ type: "edit", pid, html });
  return must(document.querySelector<HTMLElement>(`[data-pid=${pid}]`));
};

describe("typing into an element of the frame", () => {
  it("makes the element editable with the content the page says, and says it started; an element that is not there is not", () => {
    const el = start("e2", "Hello");
    expect(sent).toEqual([{ pictor: "inspect", id: turn, type: "edit", ok: true }]);
    expect(el.getAttribute("contenteditable")).toBe("true");
    key(el, { key: "Escape" });
    sent.length = 0;
    ask({ type: "edit", pid: "nobody", html: "x" });
    expect(sent).toEqual([{ pictor: "inspect", id: turn, type: "edit", ok: false }]);
  });

  it("ends with Enter, once, and says the content; the element has its attributes back exactly as they were, and no listener is left", () => {
    const el = start("e2", "Hello");
    // What the browser does to an element it edits: it respells the style.
    el.style.cssText = "position: absolute; left: 48px; top: 48px;";
    el.textContent = "你好";
    const before = sent.length;
    const e = key(el, { key: "Enter" });
    expect(e.defaultPrevented).toBe(true);
    expect(sent.slice(before)).toEqual([{ pictor: "inspect", type: "edited", pid: "e2", runs: [{ s: "你好", f: [] }] }]);
    expect(el.outerHTML).toBe(`<h1 data-pid="e2" class="t" style="${STYLE}">你好</h1>`);
    key(el, { key: "Enter" });
    el.dispatchEvent(new Event("blur"));
    key(el, { key: "Escape" });
    expect(edited()).toHaveLength(1);
  });

  it("does not end on the Enter that picks a word, while a composition is under way or after it, and ends on the next one", () => {
    const el = start("e2", "Hello");
    el.dispatchEvent(new CompositionEvent("compositionstart"));
    el.textContent = "ㄋㄧˇ";
    expect(key(el, { key: "Enter" }).defaultPrevented).toBe(false);
    el.dispatchEvent(new CompositionEvent("compositionend"));
    el.textContent = "你";
    el.dispatchEvent(new CompositionEvent("compositionstart"));
    el.dispatchEvent(new CompositionEvent("compositionend"));
    expect(key(el, { key: "Enter", isComposing: true }).defaultPrevented).toBe(false);
    expect(key(el, { key: "Enter", keyCode: 229 }).defaultPrevented).toBe(false);
    expect(edited()).toEqual([]);
    key(el, { key: "Enter" });
    expect(edited()).toEqual([{ pictor: "inspect", type: "edited", pid: "e2", runs: [{ s: "你", f: [] }] }]);
  });

  it("puts the content back on Escape and says nothing was kept", () => {
    const el = start("e2", "Hello");
    el.textContent = "changed";
    key(el, { key: "Escape" });
    expect(edited()).toEqual([{ pictor: "inspect", type: "edited", pid: "e2", runs: null }]);
    expect(el.textContent).toBe("Hello");
    expect(el.hasAttribute("contenteditable")).toBe(false);
  });

  it("ends when the element loses the keyboard, and a second edit after a first is its own, with one answer each", () => {
    const el = start("e2", "Hello");
    el.dispatchEvent(new Event("blur"));
    expect(edited()).toHaveLength(1);
    start("e2", "Hello");
    key(el, { key: "Escape" });
    expect(edited()).toHaveLength(2);
  });

  it("applies Cmd+B and Cmd+I to the selection, and leaves Shift+Enter to the browser, which makes a break", () => {
    const el = start("e2", "Hello");
    expect(key(el, { key: "b", metaKey: true }).defaultPrevented).toBe(true);
    key(el, { key: "I", ctrlKey: true });
    expect(execCommand.mock.calls).toEqual([["bold"], ["italic"]]);
    expect(key(el, { key: "Enter", shiftKey: true }).defaultPrevented).toBe(false);
    expect(edited()).toEqual([]);
  });

  it("keeps the elements that were there as their ids and the break, gives a bold the browser made an id, and drops what is not text", () => {
    const p = start("e3", 'Hi <b data-pid="e4">bold</b><br data-pid="e5">line2');
    must(p.querySelector("b")).textContent = "bolder";
    p.append(" ", Object.assign(document.createElement("i"), { textContent: "new" }));
    const stray = document.createElement("div");
    stray.append(Object.assign(document.createElement("span"), { textContent: "kept" }));
    p.append(stray, document.createElement("br")); // and the break a browser leaves at the end
    key(p, { key: "Enter" });
    const done = edited()[0] as unknown as { runs: { s?: string; f: { t: string; p: string }[] }[] };
    const fresh = done.runs.find((r) => r.s === "new")?.f[0]?.p ?? "";
    expect(fresh).toMatch(/^x[0-9a-z]+$/);
    expect(p.querySelector("i")?.getAttribute("data-pid")).toBe(fresh);
    expect(done.runs).toEqual([
      { s: "Hi ", f: [] },
      { s: "bolder", f: [{ t: "b", p: "e4" }] },
      { br: 1, n: false, f: [] },
      { s: "line2", f: [] },
      { s: " ", f: [] },
      { s: "new", f: [{ t: "i", p: fresh }] },
      { s: "kept", f: [] },
    ]);
  });

  it("says markup that is kept as written as it is, and a comment", () => {
    const p = start("e3", 'a<mark data-pid="e7">m</mark><!--c-->');
    key(p, { key: "Enter" });
    const done = edited()[0] as unknown as { runs: unknown[] };
    expect(done.runs).toEqual([{ s: "a", f: [] }, { raw: '<mark data-pid="e7">m</mark>', f: [] }, { raw: "<!--c-->", f: [] }]);
  });

  it("makes a pasted text its text", () => {
    const el = start("e2", "Hello");
    const paste = Object.assign(new Event("paste", { cancelable: true }), { clipboardData: { getData: () => "plain" } });
    el.dispatchEvent(paste);
    expect(paste.defaultPrevented).toBe(true);
    expect(execCommand.mock.calls).toEqual([["insertText", false, "plain"]]);
  });
});

describe("showing the content the room holds", () => {
  it("sets the content of an element, and leaves one that already shows it as it is", () => {
    const b = document.querySelector("[data-pid=e4]");
    ask({ type: "apply", styles: [], texts: [["e3", 'Hi <b data-pid="e4">bold</b><br data-pid="e5">line2']] });
    expect(document.querySelector("[data-pid=e4]")).toBe(b);
    ask({ type: "apply", styles: [], texts: [["e3", 'Hi <b data-pid="e4">bold!</b>']] });
    expect(document.querySelector("[data-pid=e3]")?.innerHTML).toBe('Hi <b data-pid="e4">bold!</b>');
  });

  it("does not touch an element a person is typing in, whatever the room says, and shows the room's after the edit", () => {
    const el = start("e2", "Hello");
    el.textContent = "typed";
    ask({ type: "apply", styles: [], texts: [["e2", "someone else"]] });
    expect(el.textContent).toBe("typed");
    key(el, { key: "Enter" });
    ask({ type: "apply", styles: [], texts: [["e2", "typed and someone else"]] });
    expect(el.textContent).toBe("typed and someone else");
  });

  it("refuses content that is not a list of an id and markup", () => {
    const before = document.body.innerHTML;
    sent.length = 0;
    for (const texts of ["x", [["e2"]], [[1, "x"]], [["e2", 3]]]) ask({ type: "apply", styles: [], texts });
    expect(document.body.innerHTML).toBe(before);
    expect(sent).toEqual([]);
  });
});

describe("what is under a point", () => {
  it("lists every element with an id from the nearest outwards", () => {
    document.elementFromPoint = () => document.querySelector("b");
    ask({ type: "chain", x: 1, y: 1 });
    expect(sent).toEqual([{ pictor: "inspect", id: turn, type: "chain", pids: ["e4", "e3", "e1"] }]);
  });
});
