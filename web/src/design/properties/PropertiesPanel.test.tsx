import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../../test-support";
import type { Target } from "../controller";
import { DesignEdits, ROOM_MAP } from "../edits";
import type { ElementReport } from "../inspect";
import { FILE, key, seed } from "../test-room";
import { PropertiesPanel } from "./PropertiesPanel";
import { TokenSection } from "./TokenSection";

// What a frame computes for the element when the room has said nothing: a block
// with a class that sets its type, which is where the panel's values come from.
const BASE: Record<string, string> = {
  display: "block",
  position: "absolute",
  left: "10px",
  top: "20px",
  width: "30px",
  height: "40px",
  "padding-top": "4px",
  "padding-right": "4px",
  "padding-bottom": "4px",
  "padding-left": "4px",
  "margin-top": "0px",
  "margin-right": "0px",
  "margin-bottom": "0px",
  "margin-left": "0px",
  color: "rgb(17, 34, 51)",
  "background-color": "rgba(0, 0, 0, 0)",
  opacity: "1",
  "box-shadow": "none",
  "border-top-style": "none",
  "border-top-width": "0px",
  "border-top-color": "rgb(0, 0, 0)",
  "border-top-left-radius": "0px",
  "border-top-right-radius": "0px",
  "border-bottom-right-radius": "0px",
  "border-bottom-left-radius": "0px",
  "font-family": "Inter, sans-serif",
  "font-size": "16px",
  "font-weight": "400",
  "font-style": "normal",
  "text-align": "start",
  "line-height": "normal",
  "letter-spacing": "normal",
  "flex-grow": "0",
  gap: "normal",
  "flex-direction": "row",
  "align-items": "normal",
  "justify-content": "normal",
};

const PARTS: Record<string, string[]> = {
  padding: ["padding-top", "padding-right", "padding-bottom", "padding-left"],
  margin: ["margin-top", "margin-right", "margin-bottom", "margin-left"],
  "border-radius": ["border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"],
};

/** The frame, as far as the panel can tell: the computed values with what the room holds laid over them, shorthands first so a part overrides them. */
function reportOf(edits: DesignEdits): ElementReport {
  const own = [...(edits.frame(FILE).styles.get("box") ?? new Map<string, string>())].map(([p, v]): [string, string] => [p, v.trim()]);
  const computed = { ...BASE };
  for (const [prop, value] of own) for (const part of PARTS[prop] ?? []) computed[part] = value;
  for (const [prop, value] of own) if (!PARTS[prop] && value !== "") computed[prop] = value;
  return { pid: "box", tag: "div", rect: { x: 10, y: 20, w: 30, h: 40 }, attrs: [], inline: own, computed };
}

function setup() {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  function Harness() {
    const [, bump] = useState(0);
    useEffect(() => edits.onChange(() => { bump((n) => n + 1); }), []);
    const target: Target = { frameId: "hero", file: FILE, pid: "box", report: reportOf(edits) };
    return <PropertiesPanel edits={edits} targets={[target]} />;
  }
  const view = renderIn(<Harness />, "en", "/a/x");
  const entry = (prop: string) => doc.getMap(ROOM_MAP).get(key("box", `s|${prop}`));
  return { edits, doc, view, entry, user: userEvent.setup() };
}

const box = (label: string) => screen.getByRole("textbox", { name: label });
async function enter(user: ReturnType<typeof userEvent.setup>, label: string, text: string) {
  const field = box(label);
  await user.clear(field);
  await user.type(field, `${text}{Enter}`);
}

/** Every entry of the room as text, to see what a change touched. */
function snapshot(doc: Y.Doc) {
  return new Map([...doc.getMap(ROOM_MAP).entries()].map(([k, v]) => [k, String(v)]));
}

describe("the properties panel", () => {
  it("shows what the frame computed, including what comes from a class", () => {
    setup();
    expect(box("Width")).toHaveValue("30");
    expect(box("Font")).toHaveValue("Inter, sans-serif");
    expect(box("Size")).toHaveValue("16");
    expect(box("Text colour, hex")).toHaveValue("#112233");
    expect(screen.getByRole("button", { name: "Absolute" })).toHaveAttribute("aria-pressed", "true");
  });

  it("changing a font size to 44 writes font-size:44px and leaves every other declaration", async () => {
    const { doc, entry, user } = setup();
    const before = snapshot(doc);
    await enter(user, "Size", "44");
    expect(entry("font-size")).toBe("44px");
    const after = snapshot(doc);
    for (const [k, v] of before) expect(after.get(k)).toBe(v);
    expect([...after.keys()].filter((k) => !before.has(k))).toEqual([key("box", "s|font-size")]);
    expect(box("Size")).toHaveValue("44");
  });

  it.each([
    ["Width", "120", "width", "120px"],
    ["Height", "90.5", "height", "90.5px"],
    ["Left", "5", "left", " 5px"],
    ["Top", "-8", "top", "-8px"],
    ["Gap", "12", "gap", "12px"],
    ["Line height", "24", "line-height", "24px"],
    ["Letter spacing", "1.5", "letter-spacing", "1.5px"],
    ["Opacity (%)", "40", "opacity", "0.4"],
    ["Font", '"Helvetica Neue", Arial', "font-family", '"Helvetica Neue", Arial'],
  ])("%s %s writes %s:%s and reads back the same", async (label, typed, prop, written) => {
    const { entry, user, view } = setup();
    if (prop === "gap") await user.click(screen.getByRole("button", { name: "Row" }));
    await enter(user, label, typed);
    expect(entry(prop)).toBe(written);
    // Selecting the element again shows the same value.
    view.unmount();
    expect(entry(prop)).toBe(written);
  });

  it("reads a value back after reselecting: a new panel on the same room shows what was written", async () => {
    const { edits, entry, user, view } = setup();
    await enter(user, "Width", "120");
    await enter(user, "Size", "44");
    await user.click(screen.getByRole("button", { name: "Italic" }));
    view.unmount();
    function Again() {
      return <PropertiesPanel edits={edits} targets={[{ frameId: "hero", file: FILE, pid: "box", report: reportOf(edits) }]} />;
    }
    renderIn(<Again />, "en", "/a/x");
    expect(entry("width")).toBe("120px");
    expect(box("Width")).toHaveValue("120");
    expect(box("Size")).toHaveValue("44");
    expect(screen.getByRole("button", { name: "Italic" })).toHaveAttribute("aria-pressed", "true");
  });

  it("writes the size modes: Fill and Hug replace the pixels, Fixed puts them back", async () => {
    const { entry, user } = setup();
    const modes = within(screen.getByRole("group", { name: "Width mode" }));
    await user.click(modes.getByRole("button", { name: "Fill" }));
    expect(entry("width")).toBe("100%");
    await user.click(modes.getByRole("button", { name: "Hug" }));
    expect(entry("width")).toBe("fit-content");
    await user.click(modes.getByRole("button", { name: "Fixed" }));
    expect(entry("width")).toBe("30px");
  });

  it("grows with flex-grow:1 and takes it away", async () => {
    const { entry, user } = setup();
    await user.click(screen.getByRole("button", { name: "Grow" }));
    expect(entry("flex-grow")).toBe("1");
    await user.click(screen.getByRole("button", { name: "Grow" }));
    expect(entry("flex-grow")).toBe("");
  });

  it("switches Absolute to Relative and keeps the offset", async () => {
    const { entry, user } = setup();
    await user.click(screen.getByRole("button", { name: "Relative" }));
    expect(entry("position")).toBe("relative");
    expect(entry("left")).toBe(" 10px");
    expect(entry("top")).toBe("20px");
    expect(screen.getByRole("button", { name: "Relative" })).toHaveAttribute("aria-pressed", "true");
    expect(box("Left")).toHaveValue("10");
  });

  it("makes a container with a direction and sets align, justify and gap", async () => {
    const { entry, user } = setup();
    expect(screen.queryByRole("group", { name: "Align" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Column" }));
    expect(entry("display")).toBe("flex");
    expect(entry("flex-direction")).toBe("column");
  });

  it("writes padding for all sides, then each side, and the shorthand goes when the parts are written", async () => {
    const { entry, user } = setup();
    await enter(user, "Padding", "10");
    expect(entry("padding")).toBe("10px");
    await user.click(within(screen.getByRole("group", { name: "Padding" })).getByRole("button", { name: "Each side" }));
    await enter(user, "Padding, Right", "20");
    expect(entry("padding")).toBe("");
    expect(["top", "right", "bottom", "left"].map((s) => entry(`padding-${s}`))).toEqual(["10px", "20px", "10px", "10px"]);
    expect(box("Padding, Right")).toHaveValue("20");
  });

  it("writes the radius for all corners and for each", async () => {
    const { entry, user } = setup();
    await enter(user, "Radius", "8");
    expect(entry("border-radius")).toBe("8px");
    await user.click(within(screen.getByRole("group", { name: "Radius" })).getByRole("button", { name: "Each corner" }));
    await enter(user, "Radius, Bottom right", "0");
    expect(entry("border-bottom-right-radius")).toBe("0px");
    expect(entry("border-top-left-radius")).toBe("8px");
  });

  it("writes a fill with alpha as rgba() and a plain one as hex", async () => {
    const { entry, user } = setup();
    await enter(user, "Fill, hex", "#336699");
    expect(entry("background-color")).toBe("rgba(51, 102, 153, 0)");
    await enter(user, "Fill, Alpha (%)", "50");
    expect(entry("background-color")).toBe("rgba(51, 102, 153, 0.5)");
    await enter(user, "Fill, Alpha (%)", "100");
    expect(entry("background-color")).toBe("#336699");
    expect(box("Fill, hex")).toHaveValue("#336699");
  });

  it("keeps a committed fill when alpha is changed before the frame reports it back", async () => {
    const doc = new Y.Doc();
    seed(doc);
    const edits = new DesignEdits(doc);
    const report = reportOf(edits);
    report.computed["background-color"] = "rgb(204, 51, 51)";
    const target: Target = { frameId: "hero", file: FILE, pid: "box", report };
    renderIn(<PropertiesPanel edits={edits} targets={[target]} />, "en", "/a/x");
    const user = userEvent.setup();
    await enter(user, "Fill, hex", "#336699");
    await enter(user, "Fill, Alpha (%)", "50");
    expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("rgba(51, 102, 153, 0.5)");
    edits.undo();
    expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("#336699");
  });

  it("keeps a chosen token when a committed alpha field loses focus before the frame reports back", async () => {
    const doc = new Y.Doc();
    seed(doc);
    const edits = new DesignEdits(doc);
    edits.map.set("t|--accent", "#336699");
    const report = reportOf(edits);
    report.computed["background-color"] = "rgb(51, 102, 153)";
    const target: Target = { frameId: "hero", file: FILE, pid: "box", report };
    renderIn(<><PropertiesPanel edits={edits} targets={[target]} /><TokenSection edits={edits} /></>, "en", "/a/x");
    const user = userEvent.setup();
    await enter(user, "Fill, Alpha (%)", "50");
    await user.click(screen.getByRole("button", { name: "Fill picker" }));
    await user.click(screen.getByRole("button", { name: /--accent/ }));
    expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("var(--accent)");
    await user.click(box("--accent"));
    expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("var(--accent)");
  });

  it("writes a colour typed as rgba with its own alpha", async () => {
    const { entry, user } = setup();
    await enter(user, "Text colour, hex", "rgba(1, 2, 3, 0.25)");
    expect(entry("color")).toBe("rgba(1, 2, 3, 0.25)");
    expect(box("Text colour, Alpha (%)")).toHaveValue("25");
  });

  it("writes the border: a width brings a solid style, then the style and colour", async () => {
    const { entry, user } = setup();
    await enter(user, "Border width", "2");
    for (const side of ["top", "right", "bottom", "left"]) {
      expect(entry(`border-${side}-width`)).toBe("2px");
      expect(entry(`border-${side}-style`)).toBe("solid");
    }
    await user.selectOptions(screen.getByRole("combobox", { name: "Border style" }), "dashed");
    for (const side of ["top", "right", "bottom", "left"]) expect(entry(`border-${side}-style`)).toBe("dashed");
    await enter(user, "Border colour, hex", "#ff0000");
    for (const side of ["top", "right", "bottom", "left"]) expect(entry(`border-${side}-color`)).toBe("#ff0000");
  });

  it("adds a drop shadow, edits it and removes it", async () => {
    const { entry, user } = setup();
    await user.click(screen.getByRole("button", { name: "Add shadow" }));
    expect(entry("box-shadow")).toBe("0px 4px 8px rgba(0, 0, 0, 0.25)");
    await enter(user, "Shadow blur", "12");
    expect(entry("box-shadow")).toBe("0px 4px 12px rgba(0, 0, 0, 0.25)");
    await user.click(screen.getByRole("button", { name: "Remove shadow" }));
    expect(entry("box-shadow")).toBe("");
  });

  it("writes the text properties: weight, italic, align", async () => {
    const { entry, user } = setup();
    await user.selectOptions(screen.getByRole("combobox", { name: "Weight" }), "700");
    expect(entry("font-weight")).toBe("700");
    await user.click(screen.getByRole("button", { name: "Italic" }));
    expect(entry("font-style")).toBe("italic");
    await user.click(screen.getByRole("button", { name: "Center" }));
    expect(entry("text-align")).toBe("center");
  });

  it("changing one colour changes one entry of the room and nothing else", async () => {
    const { doc, user } = setup();
    const before = snapshot(doc);
    await enter(user, "Text colour, hex", "#ff8800");
    const after = snapshot(doc);
    const changed = [...after].filter(([k, v]) => before.get(k) !== v).map(([k]) => k);
    expect(changed).toEqual([key("box", "s|color")]);
    expect([...before.keys()].every((k) => after.has(k))).toBe(true);
  });

  it("refuses a value that is not a length and says so, writing nothing", async () => {
    const { doc, user } = setup();
    const before = snapshot(doc);
    await enter(user, "Width", "wide");
    expect(screen.getByRole("alert")).toHaveTextContent("That value isn't valid here.");
    expect(snapshot(doc)).toEqual(before);
    expect(box("Width")).toHaveValue("30");
    await enter(user, "Font", "Arial; color: red");
    expect(snapshot(doc)).toEqual(before);
  });

  it("is one step of the person's history each, and Undo takes it back", async () => {
    const { edits, entry, user } = setup();
    await enter(user, "Size", "44");
    await enter(user, "Text colour, hex", "#ff8800");
    expect(edits.steps().undo).toBe(2);
    edits.undo();
    expect(entry("color")).toBeUndefined();
    expect(entry("font-size")).toBe("44px");
    edits.undo();
    expect(entry("font-size")).toBeUndefined();
  });

  it("takes away a declaration the element had, and writes nothing for one it never had", async () => {
    const { edits, entry, user } = setup();
    // The frame's own declaration, as the room holds it from the file.
    act(() => {
      edits.map.set(key("box", "s|box-shadow"), "0px 1px 2px #000000");
    });
    expect(screen.getByRole("button", { name: "Remove shadow" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Remove shadow" }));
    expect(entry("box-shadow")).toBe("");
    expect(entry("flex-grow")).toBeUndefined();
  });
});
