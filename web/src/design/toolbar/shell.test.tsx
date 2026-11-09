import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { renderIn } from "../../test-support";
import { useState } from "react";
import { ArtboardStrip } from "./ArtboardStrip";
import { ZoomMenu } from "./ZoomMenu";

it("offers zoom steps, fit and the full preset range through one keyboard menu", async () => {
  const user = userEvent.setup();
  renderIn(<ZoomMenu percent={75} onReset={vi.fn()} onFit={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: /75/ }));
  expect(screen.getByRole("menuitem", { name: /Zoom in/ })).toBeVisible();
  expect(screen.getByRole("menuitem", { name: /Zoom out/ })).toBeVisible();
  for (const value of [25, 50, 75, 100, 150, 200]) expect(screen.getByRole("menuitem", { name: `${String(value)}%` })).toBeVisible();
});

it("returns keyboard focus before a zoom menu choice changes the canvas", async () => {
  const user = userEvent.setup();
  let activeAtChoice: Element | null = null;
  const step = vi.fn(() => { activeAtChoice = document.activeElement; });
  renderIn(<ZoomMenu percent={75} onReset={vi.fn()} onFit={vi.fn()} onStep={step} />);
  const trigger = screen.getByRole("button", { name: /75/ });
  await user.click(trigger);
  await user.click(screen.getByRole("menuitem", { name: "Zoom in" }));
  expect(step).toHaveBeenCalledWith(1);
  expect(activeAtChoice).toBe(trigger);
});

const artboards = [
  { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 },
  { id: "mobile", name: "A very long name for a mobile checkout artboard", file: "frames/mobile.html", w: 390, h: 844 },
];

function ArtboardActions() {
  const [active, setActive] = useState("hero");
  const [focused, setFocused] = useState(false);
  return <>
    <button type="button">Before artboards</button>
    <ArtboardStrip frames={artboards} active={active} onActive={setActive} onExpand={() => undefined} onFocusWithin={setFocused} />
    <button type="button">After artboards</button>
    <output aria-label="Artboard focus">{focused ? "inside" : "outside"}</output>
  </>;
}

it("chooses an artboard by its full name and keeps keyboard focus in its actions until leaving", async () => {
  const user = userEvent.setup();
  renderIn(<ArtboardActions />);
  await user.tab();
  await user.tab();
  expect(screen.getByRole("button", { name: "Artboard: Hero" })).toHaveFocus();
  expect(screen.getByLabelText("Artboard focus")).toHaveTextContent("inside");
  await user.keyboard("{Enter}");
  expect(screen.getByRole("menuitemradio", { name: "Hero" })).toBeChecked();
  await user.keyboard("{End}{Enter}");
  expect(screen.getByRole("button", { name: "Artboard: A very long name for a mobile checkout artboard" })).toHaveFocus();
  expect(screen.getByRole("button", { name: "Expand A very long name for a mobile checkout artboard" })).toBeVisible();
  expect(screen.queryByRole("button", { name: /^Comment on/ })).toBeNull();
  expect(screen.queryByText(/390.*844/)).toBeNull();
  await user.tab();
  expect(screen.getByRole("button", { name: "Expand A very long name for a mobile checkout artboard" })).toHaveFocus();
  expect(screen.getByLabelText("Artboard focus")).toHaveTextContent("inside");
  await user.tab();
  expect(screen.getByRole("button", { name: "After artboards" })).toHaveFocus();
  expect(screen.getByLabelText("Artboard focus")).toHaveTextContent("outside");
});
