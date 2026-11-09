import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NodeSelection } from "prosemirror-state";
import { describe, expect, it, vi } from "vitest";
import { renderIn } from "../test-support";
import type { ToolContext } from "./feature";
import { toggle } from "./format";
import { InsertRow } from "./InsertRow";
import { para, person, room, select, text } from "./testRoom";

const run = vi.fn<ToolContext["run"]>();
const openLink = vi.fn<ToolContext["openLink"]>();
const addImages = vi.fn<ToolContext["addImages"]>();

/** The insert row for the state of an editor whose caret is `inside` the first block that holds that text. */
async function show(blocks: unknown[], inside = "") {
  run.mockClear();
  openLink.mockClear();
  addImages.mockClear();
  const view = person(room(...blocks));
  if (inside !== "") select(view, inside, 1);
  renderIn(<InsertRow state={view.state} ctx={{ run, openLink, addImages }} />);
  await openMore();
  return view;
}

/** The tools that are not the row's own are in More. */
const openMore = () => userEvent.click(screen.getByRole("button", { name: "More" }));

const item = (s: string, checked: boolean | null = null) => ({ type: "list_item", attrs: { checked }, content: [para("", s)] });
const list = (type: string, ...items: ReturnType<typeof item>[]) => ({ type, attrs: { id: "b_l" }, content: items });
const cell = (s: string, align: string | null = null) => ({ type: "table_cell", attrs: { align }, content: [{ type: "paragraph", attrs: { id: null }, content: [text(s)] }] });
const table = (header: boolean, rows: number, columns: number, align: string | null = null) => ({
  type: "table",
  attrs: { id: "b_t", header },
  content: Array.from({ length: rows }, () => ({ type: "table_row", content: Array.from({ length: columns }, () => cell("x", align)) })),
});

describe("the insert row", () => {
  it("gives each button of the tools a command to run", async () => {
    await show([para("b_1", "word")], "word");
    for (const name of ["Strikethrough", "Highlight", "Numbered list", "To-do list", "Quote", "Code block", "Insert table", "Horizontal rule"]) {
      // Pressing a tool of the row itself closes More: it is opened again for the next one.
      if (screen.queryByRole("button", { name }) === null) await openMore();
      await userEvent.click(screen.getByRole("button", { name }));
    }
    expect(run).toHaveBeenCalledTimes(8);
    expect(openLink).not.toHaveBeenCalled();
  });

  it("runs the command of the button that was pressed", async () => {
    const view = await show([para("b_1", "make this loud")]);
    select(view, "make", 5, 9);
    run.mockImplementation((command) => command(view.state, view.dispatch));
    await userEvent.click(screen.getByRole("button", { name: "Strikethrough" }));
    expect(view.dom.querySelector("s")?.textContent).toBe("this");
  });

  it("shows the list, the quote and the mark the caret is in as pressed", async () => {
    await show([list("task_list", item("todo", false))], "todo");
    expect(screen.getByRole("button", { name: "To-do list" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Numbered list" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Quote" })).toHaveAttribute("aria-pressed", "false");
    document.body.replaceChildren();

    await show([{ type: "quote", attrs: { id: "b_q" }, content: [para("b_2", "said")] }], "said");
    expect(screen.getByRole("button", { name: "Quote" })).toHaveAttribute("aria-pressed", "true");
    document.body.replaceChildren();

    const view = person(room(para("b_1", "make this loud")));
    select(view, "make", 5, 9);
    toggle("strike")(view.state, view.dispatch);
    renderIn(<InsertRow state={view.state} ctx={{ run, openLink, addImages }} />);
    await openMore();
    expect(screen.getByRole("button", { name: "Strikethrough" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Highlight" })).toHaveAttribute("aria-pressed", "false");
  });

  it("only offers indent and outdent inside a list, and no text marks inside code", async () => {
    await show([para("b_1", "plain"), { type: "code_block", attrs: { id: "b_c", lang: null }, content: [text("let x")] }], "let x");
    expect(screen.getByRole("button", { name: "Indent" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Outdent" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Strikethrough" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Code block" })).toHaveAttribute("aria-pressed", "true");
  });

  it("offers indent and outdent in a list", async () => {
    await show([list("bullet_list", item("one"))], "one");
    expect(screen.getByRole("button", { name: "Indent" })).toBeEnabled();
  });

  it("keeps the tools of the bubble out of the row and its More menu", async () => {
    await show([para("b_1", "word")], "word");
    for (const name of ["Bold", "Italic", "Underline", "Link", "Bullet list", "Code"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Block type" })).not.toBeInTheDocument();
  });

  it("starts with nothing to undo or redo", async () => {
    await show([para("b_1", "word")], "word");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
  });

  it("shows the table tools only while the caret is in a table", async () => {
    await show([para("b_1", "outside"), table(true, 2, 2)], "outside");
    expect(screen.queryByRole("toolbar", { name: "Table tools" })).toBeNull();
  });

  it("offers each table tool by name, shows what the table has pressed, and stops where a table cannot go on", async () => {
    const view = await show([table(true, 1, 30, "right")], "x");
    run.mockImplementation((command) => command(view.state, view.dispatch));
    const tools = screen.getByRole("toolbar", { name: "Table tools" });
    expect(within(tools).getByRole("button", { name: "Header row" })).toHaveAttribute("aria-pressed", "true");
    expect(within(tools).getByRole("button", { name: "Align right" })).toHaveAttribute("aria-pressed", "true");
    expect(within(tools).getByRole("button", { name: "Align left" })).toHaveAttribute("aria-pressed", "false");
    expect(within(tools).getByRole("button", { name: "Delete row" })).toBeDisabled();
    expect(within(tools).getByRole("button", { name: "Column left" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Insert table" })).toBeDisabled();
    await userEvent.click(within(tools).getByRole("button", { name: "Delete table" }));
    expect(run).toHaveBeenCalledOnce();
    expect(view.state.doc.textContent).toBe("");
  });

  it("hands the files chosen with the Image button over, and shows no image tools while no picture is selected", async () => {
    await show([para("b_1", "word")], "word");
    expect(screen.queryByRole("toolbar", { name: "Image tools" })).toBeNull();
    const file = new File(["x"], "a.png", { type: "image/png" });
    await userEvent.upload(screen.getByTestId("image-picker"), file);
    expect(addImages).toHaveBeenCalledExactlyOnceWith([file]);
  });

  it("shows the selected picture's alt text and size, keeps the alt text when Enter is pressed or the field is left, and takes a size off by pressing it again", async () => {
    const view = await show([para("b_1", "before"), { type: "image", attrs: { id: "b_i", src: "/raw/x/a.png", alt: "Chart", size: "medium" } }]);
    let at = -1;
    view.state.doc.descendants((node, pos) => {
      if (node.type.name === "image") at = pos;
    });
    view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, at)));
    document.body.replaceChildren();
    renderIn(<InsertRow state={view.state} ctx={{ run, openLink, addImages }} />);
    run.mockImplementation((command) => command(view.state, view.dispatch));
    const tools = screen.getByRole("toolbar", { name: "Image tools" });
    expect(within(tools).getByLabelText("Alt text")).toHaveValue("Chart");
    expect(within(tools).getByRole("button", { name: "Medium" })).toHaveAttribute("aria-pressed", "true");
    expect(within(tools).getByRole("button", { name: "Small" })).toHaveAttribute("aria-pressed", "false");

    await userEvent.type(within(tools).getByLabelText("Alt text"), " 2024{Enter}");
    expect(view.state.doc.child(0).child(1).attrs.alt).toBe("Chart 2024");
    await userEvent.click(within(tools).getByRole("button", { name: "Extra large" }));
    expect(view.state.doc.child(0).child(1).attrs.size).toBe("xlarge");
    await userEvent.click(within(tools).getByRole("button", { name: "Delete image" }));
    expect(view.state.doc.textContent).toBe("before");
  });
});
