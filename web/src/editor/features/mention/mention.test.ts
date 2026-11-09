import { DOMParser as PMParser } from "prosemirror-model";
import { EditorState, TextSelection } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import { undo } from "y-prosemirror";
import * as Y from "yjs";
import { describe, expect, it, vi } from "vitest";
import type { Person, PeopleFound } from "../../../api";
import { join, para, person, reload, room, typeAt } from "../../testRoom";
import { schema } from "../../schema";
import { toggle } from "../../format";
import { holds, triggerAt } from "./trigger";

const anna: Person = { user: "anna.lee", name: "Anna Lee", canOpen: true };
const bob: Person = { user: "bob", name: "王小明 😀", canOpen: false };

/** An editor on the room whose people list is `directory`, matched by account or name as the server does. */
function editorWith(doc: Y.Doc, directory: Person[] = [anna, bob]) {
  const asked: string[] = [];
  const mentioned: Person[] = [];
  const view = person(doc, true, undefined, undefined, {
    mention: {
      people: (query) => {
        asked.push(query);
        const q = query.toLowerCase();
        return Promise.resolve({ people: directory.filter((p) => p.user.includes(q) || p.name.toLowerCase().includes(q)), complete: true });
      },
      mentioned: (p) => mentioned.push(p),
      words: () => ({ label: "People", none: "No one found", incomplete: "Some people may be missing" }),
    },
  });
  return { view, asked, mentioned };
}

const list = () => document.querySelector<HTMLElement>("[role=listbox]");
const rows = () => [...document.querySelectorAll("[role=option]")].map((r) => r.querySelector(".ed-mention-name")?.textContent);
const key = (view: EditorView, init: KeyboardEventInit) => view.dom.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init }));
const blocksOf = (view: EditorView) => (view.state.doc.toJSON() as { content: { content: unknown[] }[] }).content[0]?.content;
const mentions = (view: EditorView): Record<string, unknown>[] => {
  const out: Record<string, unknown>[] = [];
  view.state.doc.descendants((n) => {
    if (n.type.name === "mention") out.push({ ...n.attrs });
    return true;
  });
  return out;
};

describe("where a mention is being typed", () => {
  const at = (text: string, pos = text.length) => {
    const state = EditorState.create({ schema, doc: schema.node("doc", null, [schema.node("tab", { id: "t" }, [schema.node("paragraph", { id: "p" }, text === "" ? [] : [schema.text(text)])])]) });
    // The tab opens at 0 and the paragraph at 1, so its text starts at 2.
    return triggerAt(state.apply(state.tr.setSelection(TextSelection.create(state.doc, 2 + pos))));
  };

  it.each([
    ["@", { query: "" }],
    ["@an", { query: "an" }],
    ["ask @an", { query: "an" }],
    ["@Anna L", { query: "Anna L" }],
    ["@王小", { query: "王小" }],
    ["a @b @c", { query: "c" }],
  ])("%j is an @ with the query", (text, want) => {
    expect(at(text)).toMatchObject(want);
  });

  it.each([["a@b.co"], ["@ x"], ["hello"], [""], ["@" + "x".repeat(31)], ["@" + "word ".repeat(7)], ["@@"]])("%j is not", (text) => {
    expect(at(text)).toBeNull();
  });

  it("is not there when the caret is before the @ or a range is selected", () => {
    expect(at("@an", 0)).toBeNull();
  });
});

describe("typing @ in a doc", () => {
  it("lists the people, narrows as the person types, and picks with Enter into a mention that others see and a reload keeps", async () => {
    const doc = room(para("b_1", "Ask "));
    const other = room(para("b_1", ""));
    join(doc, other);
    const { view, mentioned } = editorWith(doc);
    const watcher = person(other);
    typeAt(view, 6, "@");
    await vi.waitFor(() => { expect(rows()).toEqual(["Anna Lee", "王小明 😀"]); });
    expect(list()).toHaveAccessibleName("People");
    typeAt(view, 7, "a");
    await vi.waitFor(() => { expect(rows()).toEqual(["Anna Lee"]); });
    key(view, { key: "Enter" });
    expect(list()).toBeNull();
    expect(mentions(view)).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    expect(view.state.doc.textContent).toBe("Ask  ");
    // The caret is after the space that follows the mention.
    expect(view.state.selection.from).toBe(view.state.doc.content.size - 2);
    expect(mentioned).toEqual([anna]);
    // The other editor in the room sees it, drawn as the name after an @ and nothing else.
    expect(mentions(watcher)).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    expect(watcher.dom.querySelector(".ed-mention")?.textContent).toBe("@Anna Lee");
    expect(mentions(person(reload(doc)))).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    // Undo takes the mention back out.
    undo(view.state);
    expect(mentions(view)).toEqual([]);
  });

  it("picks with the arrow keys and Tab, and with a click, and keeps the caret in the editor", async () => {
    const { view } = editorWith(room(para("b_1", "")));
    typeAt(view, 2, "@");
    await vi.waitFor(() => { expect(rows()).toHaveLength(2); });
    key(view, { key: "ArrowDown" });
    expect(document.querySelector("[aria-selected=true] .ed-mention-name")?.textContent).toBe("王小明 😀");
    key(view, { key: "ArrowDown" });
    expect(document.querySelector("[aria-selected=true] .ed-mention-name")?.textContent).toBe("Anna Lee");
    key(view, { key: "ArrowUp" });
    key(view, { key: "Tab" });
    expect(mentions(view)).toEqual([{ user: "bob", name: "王小明 😀" }]);
    typeAt(view, view.state.selection.from, "@");
    await vi.waitFor(() => { expect(rows()).toHaveLength(2); });
    const row = [...document.querySelectorAll<HTMLElement>("[role=option]")][0];
    const pressed = row?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
    expect(pressed, "a press on a row keeps the editor's focus").toBe(false);
    row?.click();
    expect(mentions(view).map((m) => m.user)).toEqual(["bob", "anna.lee"]);
  });

  it("leaves the @ as text on Escape and does not ask again until another @", async () => {
    const { view, asked } = editorWith(room(para("b_1", "")));
    typeAt(view, 2, "@");
    await vi.waitFor(() => { expect(list()).not.toBeNull(); });
    key(view, { key: "Escape" });
    expect(list()).toBeNull();
    typeAt(view, 3, "a");
    expect(list()).toBeNull();
    expect(asked).toEqual([""]);
    expect(view.state.doc.textContent).toBe("@a");
    typeAt(view, 4, " @");
    await vi.waitFor(() => { expect(list()).not.toBeNull(); });
    expect(mentions(view)).toEqual([]);
  });

  it("does not take Enter or the arrows that belong to an input method composing a name", async () => {
    const { view } = editorWith(room(para("b_1", "")));
    typeAt(view, 2, "@");
    await vi.waitFor(() => { expect(rows()).toHaveLength(2); });
    key(view, { key: "ArrowDown", isComposing: true });
    key(view, { key: "ArrowDown", keyCode: 229 });
    key(view, { key: "Tab", isComposing: true });
    expect(mentions(view)).toEqual([]);
    expect(document.querySelector("[aria-selected=true] .ed-mention-name")?.textContent).toBe("Anna Lee");
    key(view, { key: "Tab" });
    expect(mentions(view)).toHaveLength(1);
  });

  it("says no one was found for an @ that matches nobody, lets Enter go on, and closes for a sentence", async () => {
    const { view } = editorWith(room(para("b_1", "")), []);
    typeAt(view, 2, "@");
    await vi.waitFor(() => { expect(list()?.textContent).toBe("No one found"); });
    key(view, { key: "Enter" });
    expect(mentions(view)).toEqual([]);
    typeAt(view, 3, "x y");
    await vi.waitFor(() => { expect(list()).toBeNull(); });
  });

  it("warns when the directory is incomplete while keeping cached people selectable", async () => {
    const view = person(room(para("b_1", "")), true, undefined, undefined, {
      mention: { people: () => Promise.resolve({ people: [anna], complete: false }), mentioned: () => undefined,
        words: () => ({ label: "People", none: "No one found", incomplete: "Some people may be missing" }) },
    });
    typeAt(view, 2, "@an");
    await vi.waitFor(() => { expect(list()?.textContent).toContain("Some people may be missing"); });
    expect(rows()).toEqual(["Anna Lee"]);
    key(view, { key: "Enter" });
    expect(mentions(view)).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    view.destroy();
  });

  it("closes the list when the caret moves away or the editor is gone, and ignores an answer that comes too late", async () => {
    let answer: (people: PeopleFound) => void = () => undefined;
    const view = person(room(para("b_1", "")), true, undefined, undefined, {
      mention: { people: () => new Promise<PeopleFound>((resolve) => { answer = resolve; }), mentioned: () => undefined, words: () => ({ label: "People", none: "No one found", incomplete: "Some people may be missing" }) },
    });
    typeAt(view, 2, "@");
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2)));
    answer({ people: [anna], complete: true });
    await Promise.resolve();
    expect(list()).toBeNull();
    typeAt(view, 3, "x");
    typeAt(view, 4, " @");
    answer({ people: [anna], complete: true });
    await vi.waitFor(() => { expect(list()).not.toBeNull(); });
    view.destroy();
    expect(list()).toBeNull();
  });
});

describe("what a mention is in the document", () => {
  it("has no marks: a bold over it is on the words around it and still there after a reload", () => {
    const doc = room({ type: "paragraph", attrs: { id: "b_1" }, content: [{ type: "text", text: "Ask " }, { type: "mention", attrs: { user: "anna.lee", name: "Anna Lee" } }, { type: "text", text: " now" }] });
    const view = person(doc);
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 2, view.state.doc.content.size - 2)));
    toggle("bold")(view.state, view.dispatch);
    let marked = 0;
    view.state.doc.descendants((n) => {
      if (n.type.name === "mention") expect(n.marks).toEqual([]);
      else if (n.isText && n.marks.length > 0) marked++;
      return true;
    });
    expect(marked).toBe(2);
    const again = person(reload(doc));
    expect(mentions(again)).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    expect(again.dom.querySelectorAll("strong")).toHaveLength(2);
  });

  it("loses the marks a paste inside bold text would give it", () => {
    const view = person(room(para("b_1", "")));
    view.pasteHTML(`<p><strong>see <span class="ed-mention" data-user="anna.lee">@Anna Lee</span></strong></p>`, new Event("paste") as ClipboardEvent);
    expect(mentions(view)).toEqual([{ user: "anna.lee", name: "Anna Lee" }]);
    view.state.doc.descendants((n) => {
      if (n.type.name === "mention") expect(n.marks).toEqual([]);
      return true;
    });
    expect(view.dom.querySelector("strong")?.textContent).toBe("see ");
  });

  it("is drawn as text from the name it holds, so a hostile name is only words", () => {
    const evil = `<img src=x onerror=alert(1)><script>alert(2)</script>`;
    const view = person(room({ type: "paragraph", attrs: { id: "b_1" }, content: [{ type: "mention", attrs: { user: "anna.lee", name: evil } }] }));
    expect(view.dom.querySelector(".ed-mention")?.textContent).toBe(`@${evil}`);
    expect(view.dom.querySelectorAll("img, script, a, [onerror]")).toHaveLength(0);
  });

  it("is made from a pasted mention only when its account and name are ones a doc may hold", () => {
    const parse = (html: string) => {
      const dom = new DOMParser().parseFromString(html, "text/html").body;
      const paragraph = schema.nodes.paragraph;
      if (paragraph === undefined) throw new Error("the schema has no paragraph");
      return PMParser.fromSchema(schema).parse(dom, { topNode: paragraph.create() });
    };
    const kept = parse(`<span class="ed-mention" data-user="anna.lee">@Anna Lee</span>`);
    expect(kept.firstChild?.type.name).toBe("mention");
    expect(kept.firstChild?.attrs).toMatchObject({ user: "anna.lee", name: "Anna Lee" });
    for (const bad of [`data-user="a b"`, `data-user="javascript:x"`, `data-user=""`, `data-user="${"a".repeat(65)}"`]) {
      expect(parse(`<span class="ed-mention" ${bad}>@Anna</span>`).firstChild?.type.name, bad).toBe("text");
    }
    expect(parse(`<span class="ed-mention" data-user="anna">@${"n".repeat(101)}</span>`).firstChild?.type.name).toBe("text");
    expect(parse(`<span class="ed-mention" data-user="anna">@</span>`).firstChild?.type.name).toBe("text");
  });

  it("is found in the document by account", () => {
    const view = person(room({ type: "paragraph", attrs: { id: "b_1" }, content: [{ type: "mention", attrs: { user: "anna.lee", name: "Anna Lee" } }] }));
    expect(holds(view.state.doc, "anna.lee")).toBe(true);
    expect(holds(view.state.doc, "bob")).toBe(false);
    expect(blocksOf(view)).toHaveLength(1);
  });
});
