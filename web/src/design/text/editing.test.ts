// @vitest-environment node
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { DesignEdits } from "../edits";
import { FILE, key, seed } from "../test-room";
import { htmlOfContent, contentOf } from "./content";
import { TextEditing, type TextLinks } from "./editing";
import type { Run } from "./units";

// The frame's side is the inspector (inspector.test.ts); here it is a stand-in that
// is told what the person ended up with, as the real one tells the page.
function frame() {
  let heard: (frameId: string, pid: string, runs: unknown) => void = () => undefined;
  const calls = { edit: [] as { pid: string; html: string }[], release: 0 };
  const links: TextLinks = {
    chain: () => Promise.resolve(["inner", "para"]),
    edit: (_f, pid, html) => {
      calls.edit.push({ pid, html });
      return Promise.resolve(true);
    },
    release: () => {
      calls.release++;
    },
    onEdited: (fn) => {
      heard = fn;
      return () => undefined;
    },
  };
  // A break is a 1 on the wire.
  const says = (pid: string, runs: Run[] | null) => {
    heard("hero", pid, runs?.map((r) => (r.br ? { ...r, br: 1 } : r)) ?? null);
  };
  return { links, calls, says, saysRaw: (pid: string, what: unknown) => { heard("hero", pid, what); } };
}

const bold = (pid: string) => JSON.stringify({ t: "b", d: 0, a: [["", "data-pid", pid]] });

/** An element whose text is formatted: `Hello <b>bold</b><br>line2`. */
function formatted(doc: Y.Doc, pid: string, parent = "b") {
  const map = doc.getMap("design");
  doc.transact(() => {
    map.set(key(pid, "kind"), "el");
    map.set(key(pid, "tag"), "div");
    map.set(key(pid, "parent"), parent);
    map.set(key(pid, "order"), "W");
    const t = new Y.Text();
    map.set(key(pid, "rich"), t);
    t.applyDelta([{ insert: "Hello " }, { insert: "bold", attributes: { "f:e3": bold("e3") } }, { insert: { br: '[["","data-pid","e4"]]' } }, { insert: "line2" }]);
  });
}

function setup() {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  const f = frame();
  const editing = new TextEditing(edits, f.links);
  const text = (pid: string) => {
    const c = contentOf(edits.map, FILE, edits.tree(FILE), pid);
    return c ? htmlOfContent(c) : null;
  };
  return { doc, edits, editing, text, ...f };
}

const said = (s: string): Run[] => [{ s, f: [] }];

describe("typing into the text of an element in place", () => {
  it("starts from what the room holds, writes the difference as one step, and Undo puts the old text back", async () => {
    const { editing, edits, says, text, calls } = setup();
    expect(await editing.beginAt("hero", FILE, 5, 5)).toBe("started");
    expect(calls.edit).toEqual([{ pid: "para", html: "Hello" }]);
    expect(editing.editing).toEqual({ frameId: "hero", pid: "para" });
    says("para", said("你好"));
    expect(text("para")).toBe("你好");
    expect(editing.editing).toBeNull();
    expect(calls.release).toBe(1);
    expect(edits.steps().undo).toBe(1);
    edits.undo();
    expect(text("para")).toBe("Hello");
  });

  it("writes nothing when the frame ends with what the room had, or when the edit is cancelled", async () => {
    const { editing, edits, says, text } = setup();
    await editing.begin("hero", FILE, "para");
    says("para", said("Hello"));
    await editing.begin("hero", FILE, "para");
    says("para", null);
    expect(text("para")).toBe("Hello");
    expect(edits.steps().undo).toBe(0);
  });

  it("keeps what someone else wrote to the same text while the person typed, and the typed characters", async () => {
    const { editing, says, text, doc } = setup();
    await editing.begin("hero", FILE, "para");
    // Someone puts a word at the end and another at the start, in the room, as the person types in the frame.
    const t = doc.getMap("design").get(key("~t1", "text")) as Y.Text;
    t.insert(5, " there");
    t.insert(0, "## ");
    says("para", said("Hello brave")); // the person added " brave"
    // Both are kept; the typed word goes after the other person's at the same place.
    expect(text("para")).toBe("## Hello there brave");
  });

  it("merges a person's Unicode replacement with a replicated agent edit and Undo leaves the agent's edit", async () => {
    const { editing, says, text, doc, edits } = setup();
    const agent = new Y.Doc();
    Y.applyUpdate(agent, Y.encodeStateAsUpdate(doc));
    const relay = (update: Uint8Array) => { Y.applyUpdate(doc, update, "agent"); };
    agent.on("update", relay);
    await editing.begin("hero", FILE, "para");
    const remote = agent.getMap("design").get(key("~t1", "text")) as Y.Text;
    remote.insert(0, "AGENT ");
    expect(editing.editing).toEqual({ frameId: "hero", pid: "para" });
    says("para", said("你好👍🏽👩‍💻𠮷𪚥"));
    expect(text("para")).toBe("AGENT 你好👍🏽👩‍💻𠮷𪚥");
    const reloaded = new Y.Doc();
    Y.applyUpdate(reloaded, Y.encodeStateAsUpdate(doc));
    const loaded = new DesignEdits(reloaded);
    const restored = contentOf(loaded.map, FILE, loaded.tree(FILE), "para");
    if (!restored) throw new Error("reloaded room lost the edited element");
    expect(htmlOfContent(restored)).toBe(text("para"));
    edits.undo();
    expect(text("para")).toBe("AGENT Hello");
    editing.destroy();
    edits.destroy();
    loaded.destroy();
    agent.off("update", relay);
    agent.destroy();
    reloaded.destroy();
    doc.destroy();
  });

  it("does not delete what someone inserted inside the stretch the person replaced", async () => {
    const { editing, says, text, doc } = setup();
    await editing.begin("hero", FILE, "para");
    (doc.getMap("design").get(key("~t1", "text")) as Y.Text).insert(5, "!");
    says("para", said("Help")); // "lo" replaced by "p"
    expect(text("para")).toBe("Help!");
  });

  it("keeps the bold word and the break when the bold word is changed, and nothing else of the element is written", async () => {
    const { editing, says, text, doc, edits } = setup();
    formatted(doc, "rich");
    const others = () => JSON.stringify([...edits.map.entries()].filter(([k]) => !k.startsWith(key("rich", "rich"))).map(([k, v]) => [k, v instanceof Y.Text ? v.toJSON() : v]));
    const before = others();
    await editing.begin("hero", FILE, "rich");
    says("rich", [
      { s: "Hello ", f: [] },
      { s: "bolder", f: [{ t: "b", p: "e3" }] },
      { br: true, f: [] },
      { s: "line2", f: [] },
    ]);
    expect(text("rich")).toBe('Hello <b data-pid="e3">bolder</b><br data-pid="e4">line2');
    expect(others()).toBe(before);
  });

  it("turns plain text that is made bold into formatted text, in the same step, and Undo makes it plain again", async () => {
    const { editing, says, text, edits, doc } = setup();
    await editing.begin("hero", FILE, "para");
    says("para", [
      { s: "He", f: [{ t: "b", p: "n1" }] },
      { s: "llo", f: [] },
    ]);
    expect(text("para")).toBe('<b data-pid="n1">He</b>llo');
    const keys = [...edits.map.keys()].filter((k) => k.includes("|~t1|"));
    expect(keys).toEqual([]);
    expect(doc.getMap("design").get(key("para", "rich"))).toBeInstanceOf(Y.Text);
    expect(edits.steps().undo).toBe(1);
    edits.undo();
    expect(text("para")).toBe("Hello");
    expect(doc.getMap("design").has(key("para", "rich"))).toBe(false);
  });

  it("gives an empty element its first text, plain or with a break", async () => {
    const { editing, says, text } = setup();
    await editing.begin("hero", FILE, "h1");
    says("h1", said("Title"));
    expect(text("h1")).toBe("Title");
    await editing.begin("hero", FILE, "h1");
    says("h1", [{ s: "Title", f: [] }, { br: true, f: [] }, { s: "more", f: [] }]);
    expect(text("h1")).toMatch(/^Title<br data-pid="x[0-9a-z]+">more$/);
  });

  it("is not text when the element holds elements, is the body, or is not there", async () => {
    const { editing } = setup();
    expect(await editing.begin("hero", FILE, "box")).toBe("not text");
    expect(await editing.begin("hero", FILE, "b")).toBe("not text");
    expect(await editing.begin("hero", FILE, "nobody")).toBe("not text");
  });

  it("edits one element at a time and says so", async () => {
    const { editing, says } = setup();
    expect(await editing.begin("hero", FILE, "para")).toBe("started");
    expect(await editing.begin("hero", FILE, "h1")).toBe("busy");
    says("para", null);
    expect(await editing.begin("hero", FILE, "h1")).toBe("started");
  });

  it("writes into the element as it is now when its text was replaced meanwhile, and nothing when it was deleted", async () => {
    const { editing, says, text, doc, edits } = setup();
    await editing.begin("hero", FILE, "para");
    // Someone writes the paragraph again as formatted text.
    doc.transact(() => {
      const map = doc.getMap("design");
      for (const k of [...map.keys()]) if (k.includes("|~t1|")) map.delete(k);
      const t = new Y.Text();
      map.set(key("para", "rich"), t);
      t.applyDelta([{ insert: "Hello " }, { insert: "all", attributes: { "f:e8": bold("e8") } }]);
    });
    says("para", [{ s: "Hello ", f: [] }, { s: "all!", f: [{ t: "b", p: "e8" }] }]);
    expect(text("para")).toBe('Hello <b data-pid="e8">all!</b>');
    await editing.begin("hero", FILE, "h1");
    edits.apply([{ file: FILE, pid: "h1", remove: true }]);
    says("h1", said("late"));
    expect(text("h1")).toBeNull();
  });

  it("does not write what the frame says when that is not text", async () => {
    const { editing, saysRaw, text } = setup();
    await editing.begin("hero", FILE, "para");
    saysRaw("para", "nonsense");
    expect(text("para")).toBe("Hello");
    expect(editing.editing).toBeNull();
  });
});
