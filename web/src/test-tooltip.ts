import { fireEvent, screen, waitFor } from "@testing-library/react";
import { expect } from "vitest";

/** Checks the mounted description rather than an implementation-only hint attribute. */
export async function expectHint(trigger: HTMLElement, text: string | RegExp) {
  fireEvent.focus(trigger);
  const tip = await screen.findByRole("tooltip");
  expect(tip).toHaveTextContent(text);
  expect(trigger).toHaveAttribute("aria-describedby", tip.id);
  expect(trigger).not.toHaveAttribute("title");
  fireEvent.blur(trigger);
  await waitFor(() => { expect(screen.queryByRole("tooltip")).toBeNull(); });
}
