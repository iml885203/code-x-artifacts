import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderIn } from "../../test-support";
import { ColorPicker } from "./ColorPicker";
import type { Rgba } from "./css";

describe("shared color picker", () => {
  it("searches tokens, preserves a selected binding and dismisses back to its actual swatch", async () => {
    let source = "var(--forest)";
    const user = userEvent.setup();
    renderIn(<ColorPicker label="Fill" color={{r:56,g:95,b:75,a:1}} source={source} tokens={{"--forest":"#385f4b", "--paper":"#f6ebd8", "--space":"16px"}} onColor={() => undefined} onToken={v => { source=v; }} />);
    const trigger = screen.getByRole("button", {name:"Fill picker"});
    await user.click(trigger);
    const picker = within(screen.getByRole("dialog", {name:"Fill"}));
    expect(picker.getByRole("button", {name:/--forest/})).toHaveAttribute("aria-pressed", "true");
    await user.type(picker.getByRole("searchbox"), "paper");
    expect(picker.queryByRole("button", {name:/--forest/})).toBeNull();
    await user.click(picker.getByRole("button", {name:/--paper/}));
    expect(source).toBe("var(--paper)");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(trigger).toHaveFocus();
  });
  it("provides keyboard solid color controls and distinct no fill and reset intents", async () => {
    let current: Rgba = {r:255,g:0,b:0,a:1};
    let noFill = false, reset = false;
    const user = userEvent.setup();
    renderIn(<ColorPicker label="Fill" color={current} tokens={{}} onColor={change => { current=change(current); }} onToken={() => undefined} onNoFill={() => {noFill=true;}} onReset={() => {reset=true;}} />);
    await user.click(screen.getByRole("button", {name:"Fill picker"}));
    await user.click(screen.getByRole("tab", {name:"Custom"}));
    const area = screen.getByRole("slider", {name:/Saturation/});
    fireEvent.keyDown(area, {key:"ArrowDown"});
    expect(current.r).toBeLessThan(255);
    const alpha = screen.getByRole("slider", {name:"Alpha (%)"});
    fireEvent.change(alpha, {target:{value:"25"}});
    fireEvent.keyUp(alpha, {key:"ArrowLeft"});
    expect(current.a).toBe(0.25);
    await user.click(screen.getByRole("button", {name:"No fill"}));
    expect(noFill).toBe(true);
    expect(reset).toBe(false);
    await user.click(screen.getByRole("button", {name:"Reset"}));
    expect(reset).toBe(true);
  });
});
