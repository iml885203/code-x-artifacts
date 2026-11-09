import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { bootFor, renderIn } from "../test-support";
import { Login } from "./Login";

describe("the sign-in composition", () => {
  it.each([
    ["en", "Sign in", "Sign in with the account provided by your workspace administrator."],
    ["zh-Hant", "登入", "使用工作空間管理者提供的帳號登入。"],
  ] as const)("introduces the form and workspace account requirement in %s", (lang, title, help) => {
    const boot = bootFor(lang, { page: "login", user: "", initials: "", login: { next: "/", username: "", error: "" } });
    const { container } = renderIn(<Login boot={boot} />, lang, "/login");
    const form = screen.getByRole("form", { name: title });
    expect(within(form).getByRole("heading", { name: title, level: 1 })).toBeVisible();
    expect(form).toHaveAccessibleDescription(help);
    expect(within(form).getByText(help)).toBeVisible();
    const brand = container.querySelector(".brand");
    if (!brand) throw new Error("the sign-in page has no brand");
    expect(brand).toBeVisible();
    expect(form).not.toContainElement(brand as HTMLElement);
    expect(brand.compareDocumentPosition(form) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});


describe("the sign-in language menu", () => {
  function show(lang: "en" | "zh-Hant" = "en") {
    const boot = bootFor(lang, { page: "login", user: "", initials: "", here: "/login?next=%2Fa%2Fx", login: { next: "/a/x", username: "", error: "" } });
    return renderIn(<Login boot={boot} />, lang, "/login");
  }

  it.each(["en", "zh-Hant"] as const)("submits the chosen language through the existing form in %s", async (lang) => {
    const user = userEvent.setup();
    show(lang);
    const trigger = screen.getByRole("button", { name: lang === "en" ? "Interface language: English" : "介面語言: 中文" });
    const form = trigger.closest("form");
    if (!form) throw new Error("the language trigger must belong to its form");
    let submitted: FormData | undefined;
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      submitted = new FormData(form, event.submitter);
    });
    await user.click(trigger);
    const current = screen.getByRole("menuitemradio", { name: lang === "en" ? "English" : "中文" });
    expect(current).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("menuitemradio", { name: lang === "en" ? "中文" : "English" }));
    if (!submitted) throw new Error("the choice must submit the language form");
    expect(Object.fromEntries(submitted)).toEqual({ next: "/login?next=%2Fa%2Fx", lang: lang === "en" ? "zh-Hant" : "en" });
    expect(form).toHaveAttribute("action", "/lang");
    expect(form).toHaveAttribute("method", "post");
  });

  it("opens and navigates by keyboard, dismisses on Escape and returns focus", async () => {
    const user = userEvent.setup();
    show();
    const trigger = screen.getByRole("button", { name: "Interface language: English" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");
    await waitFor(() => expect(screen.getByRole("menuitemradio", { name: "English" })).toHaveFocus());
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitemradio", { name: "中文" })).toHaveFocus();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("dismisses outside without taking focus from the sign-in fields", async () => {
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Interface language: English" }));
    const username = screen.getByLabelText("Username");
    await user.click(username);
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
    expect(username).toHaveFocus();
  });
});
