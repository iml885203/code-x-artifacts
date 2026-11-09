import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as Y from "yjs";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { renderIn } from "../test-support";
import { TabList } from "./TabList";
import { addTab, deleteTab, moveTab, renameTab, tabElement, tabsOf } from "./tabs";
import { para, tabbed } from "./testRoom";
import { useTabs } from "./useTabs";

/** The tab list over a real room: what a button does is an edit of the room, and the list is drawn from the room. */
function Harness({ doc, editable }: { doc: Y.Doc; editable: boolean }) {
  const tabs = useTabs(doc);
  const [chosen, setChosen] = useState("");
  const shown = tabs.find((x) => x.id === chosen)?.id ?? tabs[0]?.id ?? "";
  return (
    <TabList
      tabs={tabs}
      shown={shown}
      editable={editable}
      onShow={setChosen}
      onAdd={() => {
        const made = addTab(doc, "Untitled tab", shown);
        if (made !== null) setChosen(made);
      }}
      onRename={(id, title) => renameTab(doc, id, title)}
      onMove={(id, by) => {
        setChosen(id);
        moveTab(doc, id, by);
      }}
      onDelete={(id) => {
        const at = tabs.findIndex((x) => x.id === id);
        const next = tabs[at + 1] ?? tabs[at - 1];
        if (deleteTab(doc, id) && next) setChosen(next.id);
      }}
    />
  );
}

interface Spec {
  id: string;
  title: string;
}

function show(editable = true, lang: "en" | "zh-Hant" = "en", specs: Spec[] = [{ id: "t1", title: "One" }, { id: "t2", title: "Two" }, { id: "t3", title: "Three" }]) {
  const doc = tabbed(...specs.map((x) => ({ ...x, blocks: [para(`b_${x.id}`, x.title.toLowerCase())] })));
  renderIn(<Harness doc={doc} editable={editable} />, lang);
  return doc;
}

const names = () => within(screen.getByRole("navigation", { name: "Tabs" })).getAllByRole("listitem").map((li) => li.querySelector(".ed-tab-name")?.textContent ?? li.querySelector<HTMLInputElement>(".ed-tab-field")?.value ?? "");
const titles = (doc: Y.Doc) => tabsOf(doc).map((x) => x.title);

describe("the tab list", () => {
  it("names every tab and marks the one showing, and choosing another shows it", async () => {
    show();
    expect(names()).toEqual(["One", "Two", "Three"]);
    expect(screen.getByRole("button", { name: "One" })).toHaveAttribute("aria-current", "true");
    await userEvent.click(screen.getByRole("button", { name: "Three" }));
    expect(screen.getByRole("button", { name: "Three" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: "One" })).not.toHaveAttribute("aria-current");
  });

  it("calls a tab with no title untitled", () => {
    show(true, "en", [{ id: "t1", title: "" }]);
    expect(screen.getByRole("button", { name: "Untitled tab" })).toBeInTheDocument();
  });

  it("gives a reader the list to read from and none of the buttons that change it", async () => {
    show(false);
    await userEvent.click(screen.getByRole("button", { name: "Two" }));
    expect(screen.getByRole("button", { name: "Two" })).toHaveAttribute("aria-current", "true");
    for (const name of ["Add tab", "Rename tab", "Move tab up", "Move tab down", "Delete tab"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
  });

  it("shows the buttons for the tab showing only", () => {
    show();
    expect(screen.getAllByRole("button", { name: "Delete tab" })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Add tab" })).toBeInTheDocument();
  });

  it("speaks Chinese when the page does", () => {
    show(true, "zh-Hant");
    expect(screen.getByRole("navigation", { name: "分頁" })).toBeInTheDocument();
    for (const name of ["新增分頁", "重新命名分頁", "分頁上移", "分頁下移", "刪除分頁"]) expect(screen.getByRole("button", { name })).toBeInTheDocument();
  });
});

describe("adding a tab", () => {
  it("adds one after the tab showing and shows it", async () => {
    show();
    await userEvent.click(screen.getByRole("button", { name: "Add tab" }));
    expect(names()).toEqual(["One", "Untitled tab", "Two", "Three"]);
    expect(screen.getByRole("button", { name: "Untitled tab" })).toHaveAttribute("aria-current", "true");
  });
});

describe("renaming a tab", () => {
  it("opens a field with the name that Enter keeps and Escape does not", async () => {
    const doc = show();
    await userEvent.click(screen.getByRole("button", { name: "Rename tab" }));
    const field = screen.getByRole("textbox", { name: "Tab name" });
    expect(field).toHaveValue("One");
    await userEvent.clear(field);
    await userEvent.type(field, "Intro{Enter}");
    expect(titles(doc)).toEqual(["Intro", "Two", "Three"]);
    expect(screen.queryByRole("textbox", { name: "Tab name" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Rename tab" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Tab name" }), " and more{Escape}");
    expect(titles(doc)).toEqual(["Intro", "Two", "Three"]);
  });

  it("opens the field on a double click and keeps the old name for a blank one", async () => {
    const doc = show();
    await userEvent.dblClick(screen.getByRole("button", { name: "Two" }));
    const field = screen.getByRole("textbox", { name: "Tab name" });
    await userEvent.clear(field);
    await userEvent.type(field, "   {Enter}");
    expect(titles(doc)).toEqual(["One", "Two", "Three"]);
  });

  it("keeps what was typed when the field is left", async () => {
    const doc = show();
    await userEvent.click(screen.getByRole("button", { name: "Rename tab" }));
    const field = screen.getByRole("textbox", { name: "Tab name" });
    await userEvent.clear(field);
    await userEvent.type(field, "Left");
    await userEvent.click(document.body);
    expect(titles(doc)).toEqual(["Left", "Two", "Three"]);
  });
});

describe("moving a tab", () => {
  it("moves the tab showing up and down, and the buttons are off at the ends", async () => {
    const doc = show();
    expect(screen.getByRole("button", { name: "Move tab up" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Move tab down" }));
    expect(titles(doc)).toEqual(["Two", "One", "Three"]);
    await userEvent.click(screen.getByRole("button", { name: "Move tab down" }));
    expect(screen.getByRole("button", { name: "Move tab down" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Move tab up" }));
    expect(titles(doc)).toEqual(["Two", "One", "Three"]);
    expect(names()).toEqual(["Two", "One", "Three"]);
  });
});

describe("deleting a tab", () => {
  it("asks once, and Cancel keeps the tab", async () => {
    const doc = show();
    await userEvent.click(screen.getByRole("button", { name: "Delete tab" }));
    const dialog = screen.getByRole("alertdialog", { hidden: true });
    expect(within(dialog).getByText(/"One"/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel", hidden: true }));
    expect(titles(doc)).toEqual(["One", "Two", "Three"]);
    expect(names()).toEqual(["One", "Two", "Three"]);
  });

  it("deletes the tab with its text when confirmed, and shows the next one", async () => {
    const doc = show();
    await userEvent.click(screen.getByRole("button", { name: "Delete tab" }));
    await userEvent.click(within(screen.getByRole("alertdialog", { hidden: true })).getByRole("button", { name: "Delete tab", hidden: true }));
    expect(titles(doc)).toEqual(["Two", "Three"]);
    expect(tabElement(doc, "t1")).toBeNull();
    expect(screen.getByRole("button", { name: "Two" })).toHaveAttribute("aria-current", "true");
  });

  it("cannot delete the last tab", () => {
    show(true, "en", [{ id: "t1", title: "Only" }]);
    expect(screen.getByRole("button", { name: "Delete tab" })).toBeDisabled();
  });
});
