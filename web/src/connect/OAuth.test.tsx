import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { bootFor, renderIn } from "../test-support";

const grant = { id: "g1", clientId: "client-one", clientName: "My agent", createdAt: "2026-10-01T08:00:00Z" };
function open(lang: "en" | "zh-Hant" = "en") {
  return renderIn(<App boot={bootFor(lang, { page: "connect", here: "/connect", connect: { baseUrl: "https://workspace.example", tokens: [], oauthEnabled: true } })} />, lang, "/connect");
}
function mockApi({ loadFails = false, revokeFails = false, grants = [grant] } = {}) {
  let failed = loadFails;
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url === "/api/oauth/grants") {
      const status = failed ? 500 : 200;
      failed = false;
      return Promise.resolve(new Response(JSON.stringify(grants), { status }));
    }
    if (url.startsWith("/api/oauth/grants/") && init?.method === "DELETE") return Promise.resolve(new Response(null, { status: revokeFails ? 500 : 204 }));
    return Promise.resolve(new Response(JSON.stringify({ items: [], tokens: [] }), { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
afterEach(() => { vi.unstubAllGlobals(); });

describe("browser authorization on Connect", () => {
  it("starts with a URL and secret-free client commands, while PAT setup is optional", async () => {
    mockApi({ grants: [] });
    const user = userEvent.setup();
    open();
    expect(screen.getByRole("heading", { name: "Connect with your browser" })).toBeInTheDocument();
    expect(screen.getByText("https://workspace.example/api/mcp")).toBeInTheDocument();
    const manual = screen.getByText("Use a personal access token instead").closest("details");
    expect(manual).not.toHaveAttribute("open");
    const setup = screen.getByRole("heading", { name: "Connect with your browser" }).closest("section");
    expect(setup).toHaveTextContent("codex mcp login codexartifacts");
    expect(setup).not.toHaveTextContent("Bearer");
    await user.click(screen.getByText("Use a personal access token instead"));
    expect(manual).toHaveAttribute("open");
    expect(screen.getByRole("button", { name: "Create token" })).toBeInTheDocument();
    expect(await screen.findByText("No authorized clients yet. Connect your agent using the server URL above.")).toBeInTheDocument();
  });

  it("shows loading, retries a failed list and asks before revoking exactly one grant", async () => {
    const fetchMock = mockApi({ loadFails: true });
    const user = userEvent.setup();
    open();
    expect(screen.getByText("Loading authorized clients…")).toHaveAttribute("role", "status");
    await screen.findByText("Authorized clients could not be loaded. Try again.");
    const grants = screen.getByRole("heading", { name: "Authorized clients" }).closest("section");
    if (!grants) throw new Error("missing grants");
    await user.click(within(grants).getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("client-one")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Revoke authorization for My agent" }));
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "DELETE")).toHaveLength(0);
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent("Other authorizations and personal tokens keep working");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("client-one")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Revoke authorization for My agent" }));
    await user.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Revoke" }));
    await vi.waitFor(() => { expect(screen.queryByText("client-one")).not.toBeInTheDocument(); });
    expect(fetchMock).toHaveBeenCalledWith("/api/oauth/grants/g1", expect.objectContaining({ method: "DELETE", headers: { "X-Pictor": "1" }, credentials: "same-origin" }));
  });

  it("preserves the grant and provides a retry when revocation fails", async () => {
    mockApi({ revokeFails: true });
    const user = userEvent.setup();
    open();
    await user.click(await screen.findByRole("button", { name: "Revoke authorization for My agent" }));
    const dialog = screen.getByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The authorization could not be revoked");
    expect(screen.getByText("client-one")).toBeInTheDocument();
  });

  it("explains browser connection in Traditional Chinese", async () => {
    mockApi({ grants: [] });
    open("zh-Hant");
    expect(screen.getByRole("heading", { name: "透過瀏覽器連接" })).toBeInTheDocument();
    expect(await screen.findByText("尚無已授權的 client，請使用上方的 server 網址連接 agent。")).toBeInTheDocument();
  });
});

describe("OAuth consent", () => {
  it.each(["en", "zh-Hant"] as const)("identifies the client and permissions and posts allow/deny with the server fields in %s", (lang) => {
    renderIn(<App boot={bootFor(lang, { page: "oauth", here: "/oauth/authorize", oauth: { clientName: "<Not verified>", clientId: "client-one", scope: "artifacts", action: "/oauth/authorize", fields: { csrf: "bound-consent", client_id: "client-one", state: "state123", redirect_uri: "http://127.0.0.1:1234/callback" } } })} />, lang, "/oauth/authorize");
    const allow = screen.getByRole("button", { name: lang === "en" ? "Allow access" : "允許存取" });
    const deny = screen.getByRole("button", { name: lang === "en" ? "Deny" : "拒絕" });
    const form = allow.closest("form");
    expect(form).toHaveAttribute("method", "post");
    expect(form).toHaveAttribute("action", "/oauth/authorize");
    expect(deny.closest("form")).toBe(form);
    expect(allow).toHaveAttribute("name", "decision");
    expect(allow).toHaveAttribute("value", "allow");
    expect(deny).toHaveAttribute("value", "deny");
    expect(form?.querySelector('input[name="csrf"]')).toHaveValue("bound-consent");
    expect(form?.querySelector('input[name="state"]')).toHaveValue("state123");
    expect(form).toHaveTextContent("<Not verified>");
    expect(form).toHaveTextContent("client-one");
    expect(form).toHaveTextContent("http://127.0.0.1:1234/callback");
    expect(form).toHaveTextContent(lang === "en" ? "Client names are not verified" : "Client 名稱未經驗證");
    expect(form).toHaveTextContent(lang === "en" ? "Your existing sharing permissions still apply" : "既有的分享權限仍然適用");
  });
});
