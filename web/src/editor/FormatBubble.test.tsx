import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { EditorView } from "prosemirror-view";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { catalogues, renderIn } from "../test-support";
import type { ToolContext } from "./feature";
import { FormatBubble } from "./FormatBubble";
import { para, person, room, select, text } from "./testRoom";

const openLink = vi.fn<ToolContext["openLink"]>();
const start = vi.fn<() => void>();

const item = (s: string) => ({ type: "list_item", attrs: { checked: null }, content: [para("", s)] });

/** A bubble for the editor on a room of `blocks`: `draw` shows it for the editor's state as it is now. */
function bubbleOn(blocks: unknown[], options: { editable?: boolean; comment?: boolean; anchored?: boolean; typing?: boolean } = {}) {
  const { editable = true, comment = false, anchored = true, typing = false } = options;
  openLink.mockClear();
  start.mockClear();
  const view: EditorView = person(room(...blocks), editable);
  // jsdom lays nothing out: the selection sits on one line at a fixed place.
  view.coordsAtPos = () => ({ left: 120, right: 120, top: 200, bottom: 220 });
  const scroller = document.createElement("div");
  document.body.append(scroller);
  const ctx: ToolContext = {
    run: (command) => {
      command(view.state, view.dispatch);
    },
    openLink,
    addImages: () => undefined,
  };
  const element = () => (
    <FormatBubble view={{ current: view }} scroller={{ current: scroller }} state={view.state} ctx={ctx} editable={editable} comment={comment ? { start, anchored } : null} typing={typing} />
  );
  const shown = renderIn(element());
  return {
    view,
    /** Draws the bubble again for what the editor holds now. */
    draw: () => {
      act(() => {
        shown.rerender(
          <I18nProvider strings={catalogues.en} lang="en">
            {element()}
          </I18nProvider>,
        );
      });
    },
  };
}

const bubble = () => screen.queryByRole("toolbar", { name: "Format" });

describe("the bubble over a selection", () => {
  it("shows the formatting tools for selected words, applies one to them, and hides when the selection collapses", async () => {
    const { view, draw } = bubbleOn([para("b_1", "make this loud")]);
    expect(bubble()).not.toBeInTheDocument();

    select(view, "make", 5, 9);
    draw();
    for (const name of ["Bold", "Italic", "Underline", "Link", "Bullet list", "Code"]) expect(screen.getByRole("button", { name })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Block type" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Bold" }));
    expect(view.dom.querySelector("strong")?.textContent).toBe("this");
    draw();
    expect(screen.getByRole("button", { name: "Bold" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Italic" })).toHaveAttribute("aria-pressed", "false");

    select(view, "make", 5);
    draw();
    expect(bubble()).not.toBeInTheDocument();
  });

  it("opens the link popover with Link and turns the block into a list with the list button", async () => {
    const { view, draw } = bubbleOn([para("b_1", "make this loud")]);
    select(view, "make", 0, 4);
    draw();
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(openLink).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Bullet list" }));
    expect(view.dom.querySelector("ul > li")?.textContent).toBe("make this loud");
  });

  it("shows what the selection is in: no marks inside code, and no block type inside a list", () => {
    const code = bubbleOn([para("b_1", "plain"), { type: "code_block", attrs: { id: "b_c", lang: null }, content: [text("let x")] }]);
    select(code.view, "let x", 0, 3);
    code.draw();
    expect(screen.getByRole("button", { name: "Bold" })).toBeDisabled();
    document.body.replaceChildren();

    const inList = bubbleOn([{ type: "bullet_list", attrs: { id: "b_l" }, content: [item("one two")] }]);
    select(inList.view, "one", 0, 3);
    inList.draw();
    expect(screen.getByRole("button", { name: "Bullet list" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("combobox", { name: "Block type" })).toBeDisabled();
  });

  it("is never there in a read-only doc, and while the person types", () => {
    const reader = bubbleOn([para("b_1", "make this loud")], { editable: false });
    select(reader.view, "make", 5, 9);
    reader.draw();
    expect(bubble()).not.toBeInTheDocument();
    document.body.replaceChildren();

    const typing = bubbleOn([para("b_1", "make this loud")], { typing: true });
    select(typing.view, "make", 5, 9);
    typing.draw();
    expect(bubble()).not.toBeInTheDocument();
  });

  it("holds Comment beside the formatting tools in one bubble, and only Comment for a reader who may comment", async () => {
    const writer = bubbleOn([para("b_1", "make this loud")], { comment: true });
    select(writer.view, "make", 5, 9);
    writer.draw();
    const tools = screen.getByRole("toolbar", { name: "Format" });
    expect(tools).toContainElement(screen.getByRole("button", { name: "Bold" }));
    expect(tools).toContainElement(screen.getByRole("button", { name: "Comment" }));
    await userEvent.click(screen.getByRole("button", { name: "Comment" }));
    expect(start).toHaveBeenCalledOnce();
    document.body.replaceChildren();

    const reader = bubbleOn([para("b_1", "make this loud")], { editable: false, comment: true });
    select(reader.view, "make", 5, 9);
    reader.draw();
    expect(screen.getByRole("button", { name: "Comment" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Bold" })).not.toBeInTheDocument();
    select(reader.view, "make", 5);
    reader.draw();
    expect(bubble()).not.toBeInTheDocument();
  });

  it("is not there for a reader whose selection cannot be commented on", () => {
    const reader = bubbleOn([para("b_1", "make this loud")], { editable: false, comment: true, anchored: false });
    select(reader.view, "make", 5, 9);
    reader.draw();
    expect(bubble()).not.toBeInTheDocument();
  });
});
