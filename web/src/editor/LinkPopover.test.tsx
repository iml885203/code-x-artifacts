import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderIn } from "../test-support";
import { LinkBar, LinkForm } from "./LinkPopover";

const draft = { from: 1, to: 5, href: "", text: "docs" };

function form(over: Partial<typeof draft> = {}) {
  const onSave = vi.fn<(text: string, href: string) => void>();
  const onCancel = vi.fn<() => void>();
  renderIn(<LinkForm draft={{ ...draft, ...over }} onSave={onSave} onCancel={onCancel} />);
  return { onSave, onCancel };
}

describe("the link popover", () => {
  it.each(["javascript:alert(1)", "data:text/html,<script>1</script>", "example.com"])("refuses %s with the reason beside the field and saves nothing", async (address) => {
    const { onSave } = form();
    await userEvent.type(screen.getByLabelText("Address"), address);
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("alert")).toHaveTextContent(`Links must start with https:, http:, mailto: or #. ${address} does not.`);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("asks for an address when there is none, and clears the reason when the address is edited", async () => {
    form();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter an address.");
    await userEvent.type(screen.getByLabelText("Address"), "h");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("saves the text and the trimmed address, and Cancel saves nothing", async () => {
    const { onSave, onCancel } = form();
    await userEvent.type(screen.getByLabelText("Address"), "  https://example.com  ");
    await userEvent.clear(screen.getByLabelText("Text"));
    await userEvent.type(screen.getByLabelText("Text"), "the docs");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledExactlyOnceWith("the docs", "https://example.com");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onSave).toHaveBeenCalledOnce();
  });

  it("starts with the address of the link being changed", () => {
    form({ href: "https://old.example", text: "old" });
    expect(screen.getByLabelText("Address")).toHaveValue("https://old.example");
    expect(screen.getByLabelText("Text")).toHaveValue("old");
  });
});

describe("the link bar", () => {
  it("shows where the link goes, opens it in a new tab, and offers Change and Remove", async () => {
    const onChange = vi.fn<() => void>();
    const onRemove = vi.fn<() => void>();
    renderIn(<LinkBar link={{ ...draft, href: "https://example.com/x" }} onChange={onChange} onRemove={onRemove} />);
    expect(screen.getByText("https://example.com/x")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("href", "https://example.com/x");
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("rel", "noopener noreferrer nofollow");
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(onChange).toHaveBeenCalledOnce();
    expect(onRemove).toHaveBeenCalledOnce();
  });
});
