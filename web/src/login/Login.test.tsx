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

describe("OIDC sign-in", () => {
  const next = "/oauth/authorize?client_id=client_example&state=agent-request";
  const startUrl = `/auth/oidc/start?${new URLSearchParams({ next }).toString()}`;
  function show(lang: "en" | "zh-Hant" = "en", error = "") {
    const boot = bootFor(lang, {
      page: "login", user: "", initials: "", here: `/login?${new URLSearchParams({ next }).toString()}`,
      login: { next, username: "", error, oidc: { providerName: "Pocket ID", startUrl } },
    });
    return renderIn(<Login boot={boot} />, lang, "/login");
  }

  it.each([
    ["en", "Continue with Pocket ID", "Sign in with your workspace’s identity provider. You’ll return here to continue."],
    ["zh-Hant", "使用 Pocket ID 繼續", "使用工作空間的身分驗證服務登入，完成後會返回這裡繼續。"],
  ] as const)("offers an explicit provider action without password fields in %s", async (lang, label, help) => {
    const user = userEvent.setup();
    show(lang);
    const action = screen.getByRole("link", { name: label });
    expect(action).toHaveAttribute("href", startUrl);
    expect(new URL(action.getAttribute("href") ?? "", "https://artifacts.example.org").searchParams.get("next")).toBe(next);
    expect(screen.getByText(help)).toBeVisible();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Username|Password|帳號|密碼/)).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await user.tab();
    expect(action).toHaveFocus();
  });

  it.each([
    ["login.error.oidcUnavailable", "Your sign-in provider is unavailable. Try again in a moment, or contact your workspace administrator."],
    ["login.error.oidcInvalid", "This sign-in attempt expired or could not be verified. Start again to sign in."],
    ["login.error.oidcDenied", "Sign-in was cancelled or access was denied. Try again, or ask your workspace administrator for access."],
  ])("explains %s and offers a new provider attempt in English", (error, message) => {
    show("en", error);
    expect(screen.getByRole("alert")).toHaveTextContent(message);
    const retry = screen.getByRole("link", { name: "Try again with Pocket ID" });
    expect(retry).toHaveAttribute("href", startUrl);
    expect(retry).toHaveAccessibleDescription(message);
    expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
  });

  it.each([
    ["login.error.oidcUnavailable", "目前無法連線到登入服務，請稍後再試，或聯絡工作空間管理者。"],
    ["login.error.oidcInvalid", "這次登入已過期或無法驗證，請重新開始登入。"],
    ["login.error.oidcDenied", "登入已取消或存取遭到拒絕。請重試，或向工作空間管理者申請存取權限。"],
  ])("explains %s and offers a new provider attempt in Traditional Chinese", (error, message) => {
    show("zh-Hant", error);
    expect(screen.getByRole("alert")).toHaveTextContent(message);
    expect(screen.getByRole("link", { name: "使用 Pocket ID 再試一次" })).toHaveAttribute("href", startUrl);
    expect(screen.queryByLabelText("密碼")).not.toBeInTheDocument();
  });

  it("keeps the pending authorization destination when choosing a language", async () => {
    const user = userEvent.setup();
    show();
    const trigger = screen.getByRole("button", { name: "Interface language: English" });
    const form = trigger.closest("form");
    if (!form) throw new Error("language choice needs its native form");
    let submitted: FormData | undefined;
    form.addEventListener("submit", (event) => { event.preventDefault(); submitted = new FormData(form, event.submitter); });
    await user.click(trigger);
    await user.click(screen.getByRole("menuitemradio", { name: "中文" }));
    if (!submitted) throw new Error("the language choice must submit");
    expect(submitted.get("lang")).toBe("zh-Hant");
    const target = submitted.get("next");
    if (typeof target !== "string") throw new Error("language return destination is missing");
    const destination = new URL(target, "https://artifacts.example.org");
    expect(destination.pathname).toBe("/login");
    expect(destination.searchParams.get("next")).toBe(next);
  });
});
