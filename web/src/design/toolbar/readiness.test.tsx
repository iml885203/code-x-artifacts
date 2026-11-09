import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it } from "vitest";
import { renderIn } from "../../test-support";
import { ShellBar } from "./ShellBar";
import type { Tool } from "./tools";

const tools = ["Text", "Artboard", "Drawing", "Shape"];

function open(initialReady: boolean) {
  let setReady: (value: boolean) => void = () => undefined;
  function Harness() {
    const [ready, changeReady] = useState(initialReady);
    const [tool, setTool] = useState<Tool>("select");
    const [picture, setPicture] = useState("");
    setReady = changeReady;
    return <>
      <ShellBar ready={ready} tool={tool} onTool={setTool} frames onPicture={(file) => { setPicture(file.name); }}
        page={{ pages: [{ id: "home", name: "Home", rows: [] }], active: "home", onSelect: () => undefined, structure: null }}
        onPresent={null} panel={{ open: false, onToggle: () => undefined }}
        zoom={{ percent: 100, onStep: () => undefined, onReset: () => undefined, onFit: () => undefined, onPreset: () => undefined }} />
      <output aria-label="Chosen picture">{picture}</output>
    </>;
  }
  renderIn(<Harness />, "en", "/a/x");
  return (ready: boolean) => { act(() => { setReady(ready); }); };
}

it("keeps editor tools in place but unavailable until ready, including the picture chooser", async () => {
  const ready = open(false);
  const pending = tools.map((name) => screen.getByRole("button", { name }));
  for (const button of pending) expect(button).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Text" }));
  expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute("aria-pressed", "true");
  const chooser = screen.getByLabelText("Choose a picture to put in an artboard.");
  expect(chooser).toBeDisabled();
  fireEvent.change(chooser, { target: { files: [new File(["picture"], "picture.png", { type: "image/png" })] } });
  expect(screen.getByLabelText("Chosen picture")).toBeEmptyDOMElement();

  ready(true);
  for (const [index, name] of tools.entries()) {
    expect(screen.getByRole("button", { name })).toBe(pending[index]);
    expect(screen.getByRole("button", { name })).toBeEnabled();
  }
  await userEvent.click(screen.getByRole("button", { name: "Text" }));
  expect(screen.getByRole("button", { name: "Text" })).toHaveAttribute("aria-pressed", "true");
  fireEvent.change(chooser, { target: { files: [new File(["picture"], "picture.png", { type: "image/png" })] } });
  expect(screen.getByLabelText("Chosen picture")).toHaveTextContent("picture.png");
});

it.each([["Shape", "Rectangle"], ["Drawing", "Pencil"]])("disables an already open %s menu when readiness resets", async (menu, item) => {
  const ready = open(true);
  await userEvent.click(screen.getByRole("button", { name: menu }));
  const action = screen.getByRole("menuitem", { name: new RegExp(`^${item}`) });
  ready(false);
  expect(action).toHaveAttribute("data-disabled");
  await userEvent.click(action);
  expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute("aria-pressed", "true");
  for (const name of tools) expect(screen.getByRole("button", { name })).toBeDisabled();
});
