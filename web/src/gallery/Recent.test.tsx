import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { Gallery } from "./Gallery";

const older = artifact({ title: "Opened yesterday", author: "bob", openedAt: "2026-10-19T09:00:00Z", canChange: true, canEdit: true, role: "author" });
const newer = artifact({ title: "Opened today", author: "carol", openedAt: "2026-10-20T11:00:00Z" });
const never = artifact({ title: "Never opened", author: "alice", updatedAt: "2026-10-20T11:59:00Z" });

function serve(list: Artifact[]) {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify(list), { status: 200 }))));
}

const titles = () => screen.queryAllByRole("listitem").map((li) => within(li).getByRole("heading").textContent);
const menuOf = async (user: ReturnType<typeof userEvent.setup>, title: string) => {
  const item = screen.getAllByRole("listitem").find((li) => within(li).getByRole("heading").textContent === title);
  if (!item) throw new Error(`no ${title}`);
  await user.click(within(item).getByRole("button", { name: `Actions for ${title}` }));
  const menu = await screen.findByRole("menu");
  const items = within(menu).getAllByRole("menuitem").map((i) => i.textContent);
  await user.keyboard("{Escape}");
  return items;
};

beforeEach(() => {
  vi.stubGlobal("location", { assign: vi.fn() });
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("recently opened", () => {
  it("opens from the rail item and lists only what the person opened, newest open first, with the library's cards and menus", async () => {
    const user = userEvent.setup();
    serve([never, older, newer]);
    renderIn(<Gallery boot={bootFor()} />);
    await screen.findAllByRole("list");
    const libraryMenus = [await menuOf(user, "Opened yesterday"), await menuOf(user, "Opened today")];

    const rail = screen.getByRole("navigation", { name: "Library" });
    const recent = within(rail).getByRole("link", { name: "Recently opened" });
    expect(recent).toHaveAttribute("href", "/recent");
    await user.click(recent);

    expect(recent).toHaveAttribute("aria-current", "page");
    expect(within(rail).getByRole("link", { name: /^Artifacts/ })).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("heading", { level: 1, name: "Recently opened" })).toBeInTheDocument();
    expect(titles()).toEqual(["Opened today", "Opened yesterday"]);
    expect(screen.getAllByRole("listitem")[0]).toHaveClass("card");
    expect([await menuOf(user, "Opened yesterday"), await menuOf(user, "Opened today")]).toEqual(libraryMenus);

    await user.click(screen.getByRole("button", { name: "List" }));
    expect(titles()).toEqual(["Opened today", "Opened yesterday"]);
    expect(await menuOf(user, "Opened yesterday")).toEqual(libraryMenus[0]);
  });

  it("says what will show up when nothing has been opened yet", async () => {
    serve([never]);
    renderIn(<Gallery boot={bootFor("zh-Hant")} />, "zh-Hant", "/recent");
    expect(await screen.findByText("你打開過的頁面會依時間出現在這裡，最新的在最前面。")).toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toEqual([]);
  });
});
