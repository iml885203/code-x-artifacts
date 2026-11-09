import { expectHint } from "../test-tooltip";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { bootFor, renderIn } from "../test-support";
import { claudeCommand, codexCommand, codexConfig, mcpJson } from "./snippets";

const base = "https://pictor.test";
const secret = "pct_0123456789abcdef";

beforeEach(() => {
  // Only the clock: the age of a token is read against it.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});

const existing = [{ id: "t1", name: "ci", createdAt: "2026-10-01T08:00:00Z" }];

function open(lang: "en" | "zh-Hant" = "en", tokens: typeof existing = []) {
  return renderIn(<App boot={bootFor(lang, { page: "connect", here: "/connect", connect: { baseUrl: base, tokens } })} />, lang, "/connect");
}

/**
 * The token API as the server answers it: making one answers with its secret
 * once and adds it to the list, listing gives the list, revoking removes one.
 */
function tokenApi(status = 201, listed: typeof existing = []) {
  const list = [...listed];
  const json = (body: unknown, code: number) => Promise.resolve(new Response(JSON.stringify(body), { status: code, headers: { "Content-Type": "application/json" } }));
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url.startsWith("/api/agent-activity")) return json({ items: [], tokens: [] }, 200);
    const method = init?.method ?? "GET";
    if (method === "POST") {
      if (status === 201) list.unshift({ id: "t2", name: "laptop", createdAt: "2026-10-20T08:00:00Z" });
      return json({ token: secret, name: "laptop" }, status);
    }
    if (method === "DELETE") {
      const id = url.split("/").pop();
      const at = list.findIndex((x) => x.id === id);
      if (at >= 0) list.splice(at, 1);
      return Promise.resolve(new Response(null, { status: status === 500 ? 500 : 204 }));
    }
    return json(list, 200);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

/** The text of every code block on the page. */
function codeBlocks(): string {
  return [...document.querySelectorAll(".code pre")].map((p) => p.textContent).join("\n");
}

async function pick(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole("tab", { name }));
}

describe("the connect page", () => {
  it("writes every client's snippet with a placeholder until a token exists", async () => {
    const user = userEvent.setup();
    open();
    expect(screen.getByText(/<YOUR_TOKEN>/, { selector: "p" })).toBeInTheDocument();
    expect(codeBlocks()).toContain(`--header "Authorization: Bearer <YOUR_TOKEN>"`);
    await pick(user, "Codex");
    expect(codeBlocks()).toContain("export PICTOR_TOKEN=<YOUR_TOKEN>");
    await pick(user, "Cursor & others");
    expect(codeBlocks()).toContain(`"Authorization": "Bearer <YOUR_TOKEN>"`);
  });

  it("turns the name field into the token, fills it into every tab and keeps it only in memory", async () => {
    const user = userEvent.setup();
    const fetchMock = tokenApi();
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    open();
    await user.type(screen.getByRole("textbox", { name: "Token name" }), "laptop");
    await user.click(screen.getByRole("button", { name: "Create token" }));

    const field = await screen.findByRole("textbox", { name: "Your token" });
    expect(field).toHaveValue(secret);
    expect(field).toHaveAttribute("readonly");
    expect(screen.getByText(/Shown only once\. It is already filled into step 2\./)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage tokens" })).toHaveAttribute("href", "#tokens");
    expect(screen.queryByText(/<YOUR_TOKEN>/)).not.toBeInTheDocument();

    const [url, init] = (fetchMock.mock.calls as unknown as [string, RequestInit][]).find(([url]) => url === "/api/tokens" || url === "/api/tokens/t1") ?? ["", {}];
    expect(url).toBe("/api/tokens");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ name: "laptop" });

    expect(codeBlocks()).toContain(`--header "Authorization: Bearer ${secret}"`);
    await pick(user, "Codex");
    expect(codeBlocks()).toContain(`export PICTOR_TOKEN=${secret}`);
    await pick(user, "Cursor & others");
    expect(codeBlocks()).toContain(`"Authorization": "Bearer ${secret}"`);

    // Not in the address, not in browser storage.
    expect(location.href).not.toContain(secret);
    expect(setItem).not.toHaveBeenCalled();
    for (const store of [localStorage, sessionStorage]) {
      for (let i = 0; i < store.length; i++) expect(store.getItem(store.key(i) ?? "")).not.toContain(secret);
    }
  });

  it("is gone when the page is opened again", async () => {
    const user = userEvent.setup();
    tokenApi();
    const first = open();
    await user.click(screen.getByRole("button", { name: "Create token" }));
    await screen.findByRole("textbox", { name: "Your token" });
    first.unmount();
    open();
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument();
    expect(codeBlocks()).not.toContain(secret);
    expect(codeBlocks()).toContain("<YOUR_TOKEN>");
  });

  it("copies the token and each snippet, and says so on the button that was pressed", async () => {
    const user = userEvent.setup();
    tokenApi();
    open();
    await user.click(screen.getByRole("button", { name: "Create token" }));
    const field = await screen.findByRole("textbox", { name: "Your token" });
    const row = field.closest("form");
    if (!row) throw new Error("no form");
    await user.click(within(row).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe(secret);
    expect(within(row).getByRole("button", { name: "Copied" })).toBeInTheDocument();

    const command = screen.getByText(/claude mcp add/).closest(".code");
    if (!(command instanceof HTMLElement)) throw new Error("no code block");
    await user.click(within(command).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe(claudeCommand(base, secret));
  });

  it("says so when the token could not be made, and keeps the name for another try", async () => {
    const user = userEvent.setup();
    tokenApi(500);
    open();
    await user.type(screen.getByRole("textbox", { name: "Token name" }), "laptop");
    await user.click(screen.getByRole("button", { name: "Create token" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The token could not be created");
    expect(screen.getByRole("textbox", { name: "Token name" })).toHaveValue("laptop");
    expect(codeBlocks()).toContain("<YOUR_TOKEN>");
  });

  it("lists the person's tokens by name and when they were made, and says so when there are none", async () => {
    const first = open();
    expect(screen.getByRole("heading", { name: "Your tokens" })).toBeInTheDocument();
    expect(screen.getByText(/No tokens yet/)).toBeInTheDocument();
    first.unmount();
    open("en", existing);
    const row = screen.getByRole("row", { name: /ci/ });
    await expectHint(within(row).getByText("2 weeks ago"), /^2026-10-01 \d\d:\d\d$/);
    expect(screen.getByRole("heading", { name: "Your tokens" }).closest("section")).toHaveAttribute("id", "tokens");
  });

  it("lists a token as soon as it is made", async () => {
    const user = userEvent.setup();
    tokenApi(201, existing);
    open("en", existing);
    await user.type(screen.getByRole("textbox", { name: "Token name" }), "laptop");
    await user.click(screen.getByRole("button", { name: "Create token" }));
    expect(await screen.findByRole("row", { name: /laptop/ })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /ci/ })).toBeInTheDocument();
  });

  it("asks before revoking, revokes on the server, and drops the row", async () => {
    const user = userEvent.setup();
    const fetchMock = tokenApi(204, existing);
    open("en", existing);
    await user.click(screen.getByRole("button", { name: "Revoke ci" }));
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent("Revoke ci?");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));
    await vi.waitFor(() => {
      expect(screen.queryByRole("row", { name: /ci/ })).not.toBeInTheDocument();
    });
    const [url, init] = (fetchMock.mock.calls as unknown as [string, RequestInit][]).find(([url]) => url === "/api/tokens" || url === "/api/tokens/t1") ?? ["", {}];
    expect(url).toBe("/api/tokens/t1");
    expect(init.method).toBe("DELETE");
  });

  it("keeps the row and says so when the revoke fails", async () => {
    const user = userEvent.setup();
    tokenApi(500, existing);
    open("en", existing);
    await user.click(screen.getByRole("button", { name: "Revoke ci" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("could not be revoked");
    expect(screen.getByRole("row", { name: /ci/ })).toBeInTheDocument();
  });

  it("switches between the clients with the arrow keys as well as the mouse", async () => {
    const user = userEvent.setup();
    open();
    expect(screen.getByRole("tab", { name: "Claude Code" })).toHaveAttribute("aria-selected", "true");
    screen.getByRole("tab", { name: "Claude Code" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Codex" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("codex mcp add pictor");
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(screen.getByRole("tab", { name: "Cursor & others" })).toHaveAttribute("aria-selected", "true");
  });

  it("offers the first prompt and connection details in Traditional Chinese", () => {
    open("zh-Hant");
    expect(screen.getByRole("heading", { name: "連接你的 agent" })).toBeInTheDocument();
    expect(codeBlocks()).toContain("把這個 repo 的一頁摘要發佈到 CodeXArtifacts。");
    expect(codeBlocks()).not.toContain("plugin marketplace");
  });

});

describe("the snippets", () => {
  it("use this server's address and the forms each client accepts", () => {
    expect(claudeCommand(base, secret)).toBe(`claude mcp add --transport http pictor ${base}/api/mcp --header "Authorization: Bearer ${secret}"`);
    expect(codexCommand(base)).toBe(`codex mcp add pictor --url ${base}/api/mcp --bearer-token-env-var PICTOR_TOKEN`);
    expect(codexConfig(base)).toBe(`[mcp_servers.pictor]\nurl = "${base}/api/mcp"\nbearer_token_env_var = "PICTOR_TOKEN"`);
    expect(JSON.parse(mcpJson(base, secret))).toEqual({ mcpServers: { pictor: { url: `${base}/api/mcp`, headers: { Authorization: `Bearer ${secret}` } } } });

  });
});
