import { redo, undo } from "y-prosemirror";
import { describe, expect, it } from "vitest";
import { checkAddress, linkAt, linkDraft, removeLink, setLink } from "./link";
import { outline, para, person, reload, room, select } from "./testRoom";
import type { EditorView } from "prosemirror-view";

const runs = (view: EditorView): string => {
  const out: string[] = [];
  view.state.doc.firstChild?.descendants((node) => {
    if (!node.isTextblock) return true;
    node.forEach((run) => {
      const link = run.marks.find((m) => m.type.name === "link");
      out.push(link === undefined ? (run.text ?? "") : `[${String(link.attrs.href)}:${run.text ?? ""}]`);
    });
    return false;
  });
  return out.join("");
};
const apply = (view: EditorView, command: (s: EditorView["state"], d: EditorView["dispatch"]) => boolean) => command(view.state, view.dispatch);

describe("the address of a link", () => {
  it.each([
    ["https://example.com/a?b=1", { href: "https://example.com/a?b=1" }],
    ["  http://example.com  ", { href: "http://example.com" }],
    ["mailto:a@b.c", { href: "mailto:a@b.c" }],
    ["#part", { href: "#part" }],
    ["javascript:alert(1)", { refused: "scheme", address: "javascript:alert(1)" }],
    ["data:text/html,<script>1</script>", { refused: "scheme", address: "data:text/html,<script>1</script>" }],
    ["example.com", { refused: "scheme", address: "example.com" }],
    ["   ", { refused: "empty" }],
  ])("%j is %j", (input, want) => {
    expect(checkAddress(input)).toEqual(want);
  });
});

describe("making and removing a link", () => {
  it("turns the selected text into a link, undoes and redoes it, and keeps it on reload", () => {
    const doc = room(para("b_1", "see the docs now"));
    const view = person(doc);
    select(view, "see the docs now", 8, 12);
    expect(linkDraft(view.state)).toMatchObject({ text: "docs", href: "" });
    expect(apply(view, setLink({ from: view.state.selection.from, to: view.state.selection.to }, "docs", "https://example.com"))).toBe(true);
    expect(runs(view)).toBe("see the [https://example.com:docs] now");
    undo(view.state);
    expect(runs(view)).toBe("see the docs now");
    redo(view.state);
    expect(runs(view)).toBe("see the [https://example.com:docs] now");
    expect(runs(person(reload(doc)))).toBe("see the [https://example.com:docs] now");
  });

  it("refuses an address that is not on the list and changes nothing", () => {
    const view = person(room(para("b_1", "click here")));
    select(view, "click here", 6, 10);
    const before = JSON.stringify(view.state.doc.toJSON());
    expect(apply(view, setLink({ from: view.state.selection.from, to: view.state.selection.to }, "here", "javascript:alert(1)"))).toBe(false);
    expect(JSON.stringify(view.state.doc.toJSON())).toBe(before);
  });

  it("changes the text and the address of a link together, and removes it leaving the text", () => {
    const view = person(room(para("b_1", "go to the old site now")));
    select(view, "go to the old site now", 10, 18);
    apply(view, setLink({ from: view.state.selection.from, to: view.state.selection.to }, "old site", "https://old.example"));
    select(view, "go to the old site now", 12);
    const found = linkAt(view.state);
    expect(found).toMatchObject({ href: "https://old.example", text: "old site" });
    apply(view, setLink(found ?? { from: 0, to: 0 }, "new site", "https://new.example"));
    expect(runs(view)).toBe("go to the [https://new.example:new site] now");
    const next = linkAt(view.state) ?? { from: 0, to: 0 };
    apply(view, removeLink(next));
    expect(runs(view)).toBe("go to the new site now");
    expect(outline(view)).toBe("p(go to the new site now)");
  });

  it("makes the address the text when there is no text, and finds no link where there is none", () => {
    const view = person(room(para("b_1", "x")));
    select(view, "x", 1);
    expect(linkAt(view.state)).toBeNull();
    apply(view, setLink({ from: view.state.selection.from, to: view.state.selection.to }, "", "mailto:a@b.c"));
    expect(runs(view)).toBe("x[mailto:a@b.c:mailto:a@b.c]");
  });
});
