import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { AccountMenu } from "./AccountMenu";
import { Header } from "../Header";
import { bootFor, catalogues, renderIn } from "../test-support";
import { forgetThemeChoice, themeKey } from "./theme";

const user = userEvent.setup();

function pageFor(lang: "en" | "zh-Hant") {
  const boot = JSON.stringify({ ...bootFor(lang), strings: catalogues[lang] });
  return `<!doctype html><html lang="${lang}"><body><script type="application/json" id="pictor-boot">${boot}</script></body></html>`;
}

beforeEach(() => {
  localStorage.clear();
  forgetThemeChoice();
  document.documentElement.removeAttribute("data-theme");
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (url === "/lang") {
        const tag = (init?.body as URLSearchParams).get("lang") === "zh-Hant" ? "zh-Hant" : "en";
        return Promise.resolve(new Response(pageFor(tag), { status: 200 }));
      }
      return Promise.resolve(new Response("[]", { status: 200 }));
    }),
  );
});
afterEach(() => {
  vi.unstubAllGlobals();
});

async function openMenu() {
  await user.click(screen.getByRole("button", { name: /^(Account|帳號)$/ }));
  return screen.findByRole("menu");
}

describe("the avatar menu", () => {
  it("keeps the rail avatar a non-shrinking square inside the account target", () => {
    const styles = document.createElement("style");
    styles.textContent = ["../app.css", "./shell.css"].map((path) => readFileSync(new URL(path, import.meta.url), "utf8")).join("\n");
    document.head.append(styles);
    try {
      renderIn(<div className="library"><AccountMenu user="alice" displayName="Alice Example" initials="AE" langs={["en"]} rail /></div>);
      const trigger = screen.getByRole("button", { name: "Account: Alice Example" });
      const avatar = within(trigger).getByText("AE");
      const shape = getComputedStyle(avatar);
      expect(shape.width).toBe("32px");
      expect(shape.height).toBe(shape.width);
      expect(shape.minWidth).toBe(shape.width);
      expect(shape.minHeight).toBe(shape.height);
      expect(shape.flexShrink).toBe("0");
      expect(shape.borderRadius).toBe("50%");
      expect(within(trigger).getByText("Alice Example")).toBeVisible();
      expect(getComputedStyle(trigger).minHeight).toBe("48px");
    } finally {
      styles.remove();
    }
  });

  it("shows the available full name and account without replacing the account identifier", async () => {
    renderIn(<AccountMenu user="alice" displayName="Alice Example" initials="AE" langs={["en", "zh-Hant"]} rail />);
    await user.click(screen.getByRole("button", { name: "Account: Alice Example" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByText("Alice Example")).toBeVisible();
    expect(within(menu).getByText("alice")).toBeVisible();
    expect(within(menu).queryByRole("menuitem", { name: "Kit" })).not.toBeInTheDocument();
  });

  it("names the person and offers language and theme with the current ones ticked", async () => {
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    const menu = await openMenu();
    expect(within(menu).getByText("alice")).toBeInTheDocument();
    expect(within(menu).getByRole("menuitemradio", { name: "English" })).toBeChecked();
    expect(within(menu).getByRole("menuitemradio", { name: "中文" })).not.toBeChecked();
    expect(within(menu).getByRole("menuitemradio", { name: "Match system" })).toBeChecked();
    expect(within(menu).getByRole("menuitemradio", { name: "Light" })).not.toBeChecked();
    expect(within(menu).getByRole("menuitemradio", { name: "Dark" })).not.toBeChecked();
    expect(within(menu).getByRole("menuitem", { name: "Sign out" }).closest("form")).toHaveAttribute("action", "/logout");
  });

  it("pins a theme at once, without a reload, remembers it, and ticks it", async () => {
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await user.click(within(await openMenu()).getByRole("menuitemradio", { name: "Dark" }));
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(localStorage.getItem(themeKey)).toBe("dark");
    expect(within(await openMenu()).getByRole("menuitemradio", { name: "Dark" })).toBeChecked();
    await user.click(within(screen.getByRole("menu")).getByRole("menuitemradio", { name: "Match system" }));
    expect(document.documentElement).not.toHaveAttribute("data-theme");
    expect(localStorage.getItem(themeKey)).toBeNull();
  });

  it("still pins a theme when storage is blocked", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await user.click(within(await openMenu()).getByRole("menuitemradio", { name: "Light" }));
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    vi.restoreAllMocks();
  });

  it("changes the language at once, without a reload, in the page and in the menu", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", { assign, pathname: "/", search: "" });
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await user.click(within(await openMenu()).getByRole("menuitemradio", { name: "中文" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "帳號" })).toBeInTheDocument();
    });
    const post = vi.mocked(fetch).mock.calls.find(([url]) => url === "/lang");
    expect((post?.[1]?.body as URLSearchParams).get("lang")).toBe("zh-Hant");
    expect(document.documentElement).toHaveAttribute("lang", "zh-Hant");
    expect(within(await openMenu()).getByRole("menuitemradio", { name: "中文" })).toBeChecked();
    expect(assign).not.toHaveBeenCalled();
  });

  it("offers language and theme to a person who is not signed in, and nothing that needs an account", async () => {
    renderIn(<Header boot={bootFor("en", { user: "", initials: "" })} />);
    const menu = await openMenu();
    expect(within(menu).getByRole("menuitemradio", { name: "Dark" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: "Sign out" })).not.toBeInTheDocument();
  });
});
