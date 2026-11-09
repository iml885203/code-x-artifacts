import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { describe, expect, it } from "vitest";
import { Tooltip } from "./Tooltip";

describe("shared interface hints", () => {
  it("describes a focused trigger through a portal and dismisses on Escape without changing its name", async () => {
    const { container } = render(<Tooltip content="Exact date"><button aria-label="Updated">clock</button></Tooltip>);
    const trigger = screen.getByRole("button", { name: "Updated" });
    expect(trigger).not.toHaveAttribute("title");
    await userEvent.tab();
    const tip = await screen.findByRole("tooltip");
    expect(tip).toHaveTextContent("Exact date");
    expect(trigger).toHaveAttribute("aria-describedby", tip.id);
    expect(container).not.toContainElement(tip);
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(trigger).not.toHaveAttribute("aria-describedby");
    expect(trigger).toHaveFocus();
  });

  it("delays a pointer hint and closes when the pointer leaves", async () => {
    render(<Tooltip content="Pin this artifact"><button>Pin</button></Tooltip>);
    const trigger = screen.getByRole("button");
    fireEvent.pointerMove(trigger, { pointerType: "mouse" });
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Pin this artifact");
    fireEvent.pointerLeave(trigger, { pointerType: "mouse", clientX: 0, clientY: 0 });
    fireEvent.pointerMove(document.body, { pointerType: "mouse", clientX: 500, clientY: 500 });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("lets a hinted dropdown trigger open its menu without a competing hint", async () => {
    render(<DropdownMenu.Root modal={false}><Tooltip content="Account details"><DropdownMenu.Trigger>Account</DropdownMenu.Trigger></Tooltip><DropdownMenu.Portal><DropdownMenu.Content><DropdownMenu.Item>Language</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Portal></DropdownMenu.Root>);
    await userEvent.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Account details");
    await userEvent.keyboard("{Enter}");
    expect(await screen.findByRole("menuitem", { name: "Language" })).toBeVisible();
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
