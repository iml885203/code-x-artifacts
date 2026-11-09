import { act, createEvent, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../test-support";
import { DesignEdits, ROOM_MAP } from "./edits";
import { Outline } from "./Outline";
import { FILE, key, seed } from "./test-room";
import type { Selected } from "./useSelection";

const frames = [{ id: "hero", name: "Hero", file: FILE }];

function setup(editable = true) {
  const doc = new Y.Doc();
  seed(doc);
  const map = doc.getMap(ROOM_MAP);
  map.set(key("box", "attrs"), JSON.stringify([["", "id", "box"], ["", "data-pid", "box"]]));
  map.set(key("para", "tag"), "p");
  map.set(key("h1", "tag"), "h1");
  map.set(key("box", "tag"), "div");
  map.set(key("b", "tag"), "body");
  const edits = new DesignEdits(doc);
  const onSelect = vi.fn();
  function Harness({ initial }: { initial: Selected[] }) {
    const [selected] = useState(initial);
    return <Outline edits={edits} frames={frames} selected={selected} editable={editable} onSelect={onSelect} />;
  }
  renderIn(<Harness initial={[]} />, "en", "/a/x");
  const rows = () => screen.getAllByRole("treeitem").map((r) => r.getAttribute("data-row")?.replace("hero/", ""));
  const row = (pid: string) => {
    const el = document.querySelector(`[data-row="hero/${pid}"]`);
    if (!(el instanceof HTMLElement)) throw new Error(`no row ${pid}`);
    return el;
  };
  const main = (pid: string) => {
    const el = row(pid).querySelector(".orow-main");
    if (!(el instanceof HTMLElement)) throw new Error(`no name on row ${pid}`);
    return el;
  };
  return { edits, map, onSelect, rows, row, main };
}

const attrsOf = (map: Y.Map<unknown>, pid: string) => JSON.parse(map.get(key(pid, "attrs")) as string) as string[][];
/** A drag event over a row at a height (jsdom has no DragEvent, so the position is put on the event). */
const dragAt = (type: "dragOver" | "drop", el: HTMLElement, clientY: number) => {
  const event = createEvent[type](el, transfer());
  Object.defineProperty(event, "clientY", { value: clientY });
  return fireEvent(el, event);
};
const transfer = () => ({ dataTransfer: { setData: vi.fn(), effectAllowed: "", dropEffect: "" } });

describe("the outline", () => {
  it("lists the frame's elements as a tree, with a name or the tag and the start of the text", () => {
    const { rows, row } = setup();
    expect(rows()).toEqual(["b", "box", "para", "h1"]);
    expect(screen.getByRole("tree")).toBeInTheDocument();
    const para = row("para");
    expect(para).toHaveAttribute("aria-level", "3");
    expect(within(para).getByText("<p>")).toBeInTheDocument();
    expect(within(para).getByText("Hello")).toBeInTheDocument();
  });

  it("selects an element when its row is clicked, with Shift to add", async () => {
    const { onSelect, main } = setup();
    const user = userEvent.setup();
    await user.click(main("h1"));
    expect(onSelect).toHaveBeenLastCalledWith({ frameId: "hero", pid: "h1" }, false);
    await user.keyboard("{Shift>}");
    await user.click(main("box"));
    await user.keyboard("{/Shift}");
    expect(onSelect).toHaveBeenLastCalledWith({ frameId: "hero", pid: "box" }, true);
  });

  it("renames on double-click and changes only the data-name; Escape leaves the name; an empty name takes it away", async () => {
    const { map, row, main } = setup();
    const user = userEvent.setup();
    const before = attrsOf(map, "box");
    await user.dblClick(main("box"));
    const input = screen.getByRole("textbox", { name: "Name of <div>" });
    await user.type(input, "Card{Enter}");
    expect(attrsOf(map, "box")).toEqual([...before, ["", "data-name", "Card"]]);
    expect(within(row("box")).getByText("Card")).toBeInTheDocument();

    await user.dblClick(main("box"));
    await user.type(screen.getByRole("textbox"), "Other{Escape}");
    expect(attrsOf(map, "box")).toEqual([...before, ["", "data-name", "Card"]]);

    await user.dblClick(main("box"));
    await user.clear(screen.getByRole("textbox"));
    await user.keyboard("{Enter}");
    expect(attrsOf(map, "box")).toEqual(before);
  });

  it("renames from the keyboard with F2", async () => {
    const { main } = setup();
    const user = userEvent.setup();
    main("h1").focus();
    await user.keyboard("{F2}");
    expect(screen.getByRole("textbox", { name: "Name of <h1>" })).toHaveFocus();
  });

  it("hides with the eye, as a state of its own and keeping the element in the room, and shows again", async () => {
    const { map, row, edits } = setup();
    const user = userEvent.setup();
    await user.click(within(row("h1")).getByRole("button", { name: "Hide <h1>" }));
    expect(map.get(key("h1", "hidden"))).toBe("1");
    expect(map.get(key("h1", "kind"))).toBe("el");
    expect(edits.frame(FILE).alive.has("h1")).toBe(true);
    expect(row("h1")).toHaveClass("is-hidden");
    await user.click(within(row("h1")).getByRole("button", { name: "Show <h1>" }));
    expect(map.has(key("h1", "hidden"))).toBe(false);
    expect(row("h1")).not.toHaveClass("is-hidden");
  });

  it("reorders by dragging a row before another, and Undo puts it back", () => {
    const { rows, row, edits } = setup();
    const grab = row("h1");
    fireEvent.dragStart(grab, transfer());
    const box = row("box");
    box.getBoundingClientRect = () => ({ top: 0, height: 100, bottom: 100, left: 0, right: 0, width: 0, x: 0, y: 0, toJSON: () => ({}) });
    expect(dragAt("dragOver", box, 5)).toBe(false); // accepted: the drop is allowed
    expect(box).toHaveClass("drop-before");
    dragAt("drop", box, 5);
    expect(rows()).toEqual(["b", "h1", "box", "para"]);
    act(() => {
      edits.undo();
    });
    expect(rows()).toEqual(["b", "box", "para", "h1"]);
  });

  it("makes an element a child by dropping it on the middle of another", () => {
    const { rows, row } = setup();
    fireEvent.dragStart(row("h1"), transfer());
    dragAt("drop", row("box"), 0);
    expect(rows()).toEqual(["b", "box", "para", "h1"]);
    expect(row("h1")).toHaveAttribute("aria-level", "3");
  });

  it("refuses to drop an element into itself or into something inside it, and changes nothing", () => {
    const { rows, row, edits } = setup();
    fireEvent.dragStart(row("box"), transfer());
    expect(fireEvent.dragOver(row("para"), transfer())).toBe(true); // not accepted
    expect(fireEvent.dragOver(row("box"), transfer())).toBe(true);
    fireEvent.drop(row("para"), transfer());
    fireEvent.drop(row("box"), transfer());
    expect(rows()).toEqual(["b", "box", "para", "h1"]);
    expect(edits.steps().undo).toBe(0);
  });

  it("moves with Alt+Arrow from the keyboard", async () => {
    const { rows, main } = setup();
    const user = userEvent.setup();
    main("h1").focus();
    await user.keyboard("{Alt>}{ArrowUp}{/Alt}");
    expect(rows()).toEqual(["b", "h1", "box", "para"]);
    await user.keyboard("{Alt>}{ArrowDown}{/Alt}");
    expect(rows()).toEqual(["b", "box", "para", "h1"]);
  });

  it("shows a reader the tree and lets them select, with no eye, no rename and nothing to drag", async () => {
    const { row, onSelect, main } = setup(false);
    const user = userEvent.setup();
    expect(screen.queryByRole("button", { name: /^Hide/ })).not.toBeInTheDocument();
    expect(row("h1")).not.toHaveAttribute("draggable", "true");
    await user.dblClick(main("h1"));
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(onSelect).toHaveBeenCalled();
  });

  it("follows what other people do to the room", () => {
    const { rows, map } = setup();
    act(() => {
      map.set(key("h1", "order"), "A");
    });
    expect(rows()).toEqual(["b", "h1", "box", "para"]);
  });
});
