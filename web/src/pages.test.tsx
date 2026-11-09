import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "./App";
import { artifact, bootFor, renderIn } from "./test-support";

describe("the sign-in page", () => {
  const login = { next: "/a/x", error: "", username: "" };

  it("is a form post to /login that carries where to go next, with nothing signed-in on it", () => {
    renderIn(<App boot={bootFor("en", { page: "login", user: "", initials: "", here: "/login?next=%2Fa%2Fx", login })} />, "en", "/login");
    const form = screen.getByRole("button", { name: "Sign in" }).closest("form");
    expect(form).toHaveAttribute("method", "post");
    expect(form).toHaveAttribute("action", "/login");
    expect(form?.querySelector('input[name="next"]')).toHaveValue("/a/x");
    expect(screen.getByLabelText("Username")).toBeRequired();
    expect(screen.getByLabelText("Password")).toHaveAttribute("type", "password");
    expect(screen.queryByRole("button", { name: "Account" })).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("says what went wrong in the language served and keeps the username that was typed", () => {
    renderIn(<App boot={bootFor("zh-Hant", { page: "login", user: "", initials: "", login: { ...login, error: "login.error.invalid", username: "alice" } })} />, "zh-Hant", "/login");
    expect(screen.getByRole("alert")).toHaveTextContent("帳號或密碼錯誤");
    expect(screen.getByLabelText("帳號")).toHaveValue("alice");
    expect(screen.getByLabelText("密碼")).toHaveValue("");
  });

  it("keeps the language menu in a separate footer form with the sign-in destination", async () => {
    const user = userEvent.setup();
    renderIn(<App boot={bootFor("en", { page: "login", user: "", initials: "", here: "/login?next=%2Fa%2Fx", login })} />, "en", "/login");
    const trigger = screen.getByRole("button", { name: "Interface language: English" });
    const form = trigger.closest("form");
    expect(form).toHaveAttribute("action", "/lang");
    expect(form).toHaveAttribute("method", "post");
    expect(form?.querySelector('input[name="next"]')).toHaveValue("/login?next=%2Fa%2Fx");
    expect(form).not.toBe(screen.getByRole("button", { name: "Sign in" }).closest("form"));
    await user.click(trigger);
    expect(screen.getByRole("menuitemradio", { name: "English" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("menuitemradio", { name: "中文" })).toHaveAttribute("aria-checked", "false");
  });

  it("shows a wrong password under the fields, keeps the account name and marks both fields invalid", () => {
    renderIn(<App boot={bootFor("en", { page: "login", user: "", initials: "", login: { ...login, error: "login.error.invalid", username: "alice" } })} />, "en", "/login");
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Wrong username or password");
    const password = screen.getByLabelText("Password");
    // The message follows the password field, inside the form: under the fields, not above the form.
    expect(password.compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(alert.closest("form")).toBe(password.closest("form"));
    expect(screen.getByLabelText("Username")).toHaveValue("alice");
    for (const field of [screen.getByLabelText("Username"), password]) {
      expect(field).toBeInvalid();
      expect(field).toHaveAccessibleDescription("Wrong username or password");
    }
  });
});

describe("the page for something that is not there", () => {
  it("says so in the language served, and offers the way back", () => {
    renderIn(<App boot={bootFor("zh-Hant", { page: "error", status: 404 })} />, "zh-Hant");
    expect(screen.getByRole("heading", { name: "找不到這個頁面" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "回到作品列表" })).toHaveAttribute("href", "/");
  });
});

describe("the viewer page as the server hands it over", () => {
  it("shows the artifact from the boot data, so a direct visit needs no list", () => {
    const a = artifact({ title: "Sprint report", author: "alice", canChange: true });
    renderIn(<App boot={bootFor("en", { page: "viewer", artifact: a, here: `/a/${a.id}` })} />, "en", `/a/${a.id}`);
    expect(screen.getByTitle("Sprint report")).toHaveAttribute("src", `/raw/${a.id}`);
  });
});

describe("the kit reference", () => {
  it("shows the sample page in light and dark, each in an empty sandbox, behind the account menu's link", () => {
    renderIn(<App boot={bootFor("en", { page: "kit", here: "/kit" })} />, "en", "/kit");
    expect(screen.getByRole("heading", { name: "Kit" })).toBeInTheDocument();
    for (const [name, theme] of [["Light", "light"], ["Dark", "dark"]] as const) {
      const frame = screen.getByTitle(name);
      expect(frame).toHaveAttribute("src", `/kit/v4/sample.html?theme=${theme}`);
      expect(frame).toHaveAttribute("sandbox", "");
    }
    expect(screen.getByRole("link", { name: "Open the stylesheet" })).toHaveAttribute("href", "/kit/v4/pictor.css");
  });

  it("is in the language served", () => {
    renderIn(<App boot={bootFor("zh-Hant", { page: "kit", here: "/kit" })} />, "zh-Hant", "/kit");
    expect(screen.getByTitle("暗色")).toHaveAttribute("src", "/kit/v4/sample.html?theme=dark");
  });
});

describe("the account menu", () => {
  it("links the connect page before signing out, and has no Tokens item of its own", async () => {
    const user = userEvent.setup();
    renderIn(<App boot={bootFor("en", { page: "kit", here: "/kit" })} />, "en", "/kit");
    await user.click(screen.getByRole("button", { name: "Account" }));
    const items = screen.getAllByRole("menuitem").map((i) => i.textContent);
    expect(items).not.toContain("Tokens");
    expect(items.indexOf("Sign out")).toBe(items.indexOf("Connect your agent") + 1);
    expect(screen.getByRole("menuitem", { name: "Connect your agent" })).toHaveAttribute("href", "/connect");
  });

  it("keeps the direct Kit reference available without advertising it in account navigation", async () => {
    const user = userEvent.setup();
    renderIn(<App boot={bootFor("en", { page: "kit", here: "/kit" })} />, "en", "/kit");
    await user.click(screen.getByRole("button", { name: "Account" }));
    expect(screen.queryByRole("menuitem", { name: "Kit" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the stylesheet" })).toHaveAttribute("href", "/kit/v4/pictor.css");
  });
});
