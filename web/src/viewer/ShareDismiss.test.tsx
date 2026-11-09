import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "./Viewer";

const mine = artifact({ title: "Shared notes", author: "alice", canChange: true, canEdit: true });

afterEach(() => { vi.unstubAllGlobals(); });

function open(canChange = true, pending = false) {
  let saved = "private";
  let finish: (() => void) | undefined;
  const a = { ...mine, canChange };
  const writes: string[] = [];
  vi.stubGlobal("fetch", vi.fn((url: string, init?: RequestInit) => {
    if (init?.method === "PATCH") {
      const next = (JSON.parse(init.body as string) as { visibility: string }).visibility;
      writes.push(next);
      saved = next;
      const response = new Response(JSON.stringify({ ...a, visibility: next }));
      if (pending && writes.length === 1) return new Promise<Response>((resolve) => { finish = () => { resolve(response); }; });
      return Promise.resolve(response);
    }
    return Promise.resolve(new Response(JSON.stringify(url === `/api/artifacts/${a.id}` ? a : {})));
  }));
  renderIn(<Viewer boot={bootFor("en", { page: "viewer" })} artifact={a} />);
  return { saved: () => saved, writes, finish: () => finish?.() };
}

async function share(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /^Share$/ }));
  const dialog = screen.getByRole("dialog");
  // jsdom has no layout or native backdrop: give the dialog an interior and exterior.
  vi.spyOn(dialog, "getBoundingClientRect").mockReturnValue(new DOMRect(100, 100, 400, 500));
  return dialog;
}

async function clickAt(user: ReturnType<typeof userEvent.setup>, target: HTMLElement, x: number, y: number) {
  await user.pointer([{ target, coords: { clientX: x, clientY: y }, keys: "[MouseLeft>]" }, { keys: "[/MouseLeft]" }]);
}

it("dismisses from the backdrop without rolling back a pending save", async () => {
  const user = userEvent.setup();
  const server = open(true, true);
  const dialog = await share(user);
  await user.click(within(dialog).getByRole("radio", { name: /^Organisation/ }));
  await clickAt(user, dialog, 20, 20);
  await waitFor(() => { expect(screen.queryByRole("dialog")).not.toBeInTheDocument(); });
  expect(server.saved()).toBe("organisation");
  await act(async () => { server.finish(); await Promise.resolve(); });
  expect(within(screen.getByRole("banner")).getByRole("img", { name: "Organisation" })).toBeInTheDocument();
  const reopened = await share(user);
  expect(within(reopened).getByRole("radio", { name: /^Organisation/ })).toBeChecked();
});

it("keeps content, interior padding and inside-to-outside drags open, then allows Escape to close without undoing saved access", async () => {
  const user = userEvent.setup();
  const server = open();
  const dialog = await share(user);
  await user.click(within(dialog).getByRole("radio", { name: /^Organisation/ }));
  await clickAt(user, within(dialog).getByRole("heading", { level: 2 }), 150, 150);
  await clickAt(user, dialog, 105, 105);
  await user.pointer([
    { target: within(dialog).getByRole("textbox", { name: "Link" }), coords: { clientX: 150, clientY: 400 }, keys: "[MouseLeft>]" },
    { target: dialog, coords: { clientX: 20, clientY: 20 }, keys: "[/MouseLeft]" },
  ]);
  expect(dialog).toHaveAttribute("open");
  expect(server.saved()).toBe("organisation");
  fireEvent(dialog, new Event("cancel", { cancelable: true }));
  await waitFor(() => { expect(dialog).not.toHaveAttribute("open"); });
  expect(server.saved()).toBe("organisation");
});

it("lets a read-only viewer dismiss from the backdrop without changing permissions", async () => {
  const user = userEvent.setup();
  const server = open(false);
  const dialog = await share(user);
  await clickAt(user, dialog, 20, 20);
  await waitFor(() => { expect(dialog).not.toHaveAttribute("open"); });
  expect(server.writes).toEqual([]);
});
