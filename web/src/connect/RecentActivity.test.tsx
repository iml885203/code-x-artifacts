import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderIn, bootFor } from "../test-support";
import { expectHint } from "../test-tooltip";
import { formatTime } from "../format";
import { Connect } from "./Connect";
import { RecentActivity } from "./RecentActivity";
import { readAgentActivity, type AgentActivityPage, type AgentActivityItem } from "./activity";

const event: AgentActivityItem = { id: "e1", tokenID: "t1", tokenLabel: "Laptop", revoked: false, transport: "mcp", client: "Codex", clientVersion: "1.2", occurredAt: "2026-10-21T12:00:00Z", operation: "publish", target: { id: "artifact1", title: "Planning notes", url: "/a/artifact1" } };
const tokens = [{ id: "t1", name: "Laptop", revoked: false, lastUsed: event.occurredAt }, { id: "t2", name: "Old laptop", revoked: true, lastConnected: event.occurredAt }];
const response = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const page = (items: AgentActivityItem[] = [event], nextCursor?: string): AgentActivityPage => ({ items, tokens, ...(nextCursor ? { nextCursor } : {}) });
function Harness() {
  const [id, setID] = useState("");
  return <RecentActivity tokenID={id} onTokenIDChange={setID} refreshKey={0} />;
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("personal recent activity", () => {
  it("reads the frozen contract with encoded filter, cursor, cookie header and cancellation", async () => {
    const fetcher = vi.fn(() => Promise.resolve(response(page())));
    vi.stubGlobal("fetch", fetcher);
    const controller = new AbortController();
    await expect(readAgentActivity("token & one", "opaque+/=", controller.signal)).resolves.toEqual(page());
    const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    const query = new URL(url, "https://pictor.test").searchParams;
    expect(query.get("tokenID")).toBe("token & one");
    expect(query.get("cursor")).toBe("opaque+/=");
    expect(query.get("limit")).toBe("50");
    expect(init).toMatchObject({ credentials: "same-origin", headers: { "X-Pictor": "1" }, signal: controller.signal });
  });

  it("shows current allowed targets, self-reported client/version and truthful missing connection summaries", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ items: [event], tokens: [tokens[0]] }))));
    renderIn(<Harness />);
    expect(screen.getByRole("status")).toHaveTextContent("Loading activity");
    expect(await screen.findByRole("link", { name: "Planning notes" })).toHaveAttribute("href", "/a/artifact1");
    expect(screen.getByText("Codex")).toHaveTextContent("Codex 1.2");
    expect(screen.getByText("Last connected").parentElement).toHaveTextContent("Not recorded");
    const time = document.querySelector(".activity-list time");
    expect(time).toHaveAttribute("datetime", event.occurredAt);
    expect(time).toHaveAttribute("tabindex", "0");
    expect(time?.parentElement).not.toHaveAttribute("tabindex");
    expect(time?.getAttribute("aria-label")).toContain(formatTime(event.occurredAt));
    await expectHint(time as HTMLElement, formatTime(event.occurredAt));
    expect(screen.queryByText(/online/i)).not.toBeInTheDocument();
  });

  it.each([ ["en", "2 hours ago"], ["zh-Hant", "2 小時前"] ] as const)("shows populated %s activity with relative time and exact local time on pointer and keyboard access", async (lang, relative) => {
    vi.spyOn(Date, "now").mockReturnValue(Date.parse("2026-10-21T14:00:00Z"));
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response(page()))));
    renderIn(<Harness />, lang);
    const link = await screen.findByRole("link", { name: "Planning notes" });
    const row = link.closest("li");
    if (!row) throw new Error("activity must appear in its history row");
    const time = within(row).getByText(relative);
    const exact = new Intl.DateTimeFormat("sv-SE", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(event.occurredAt));
    expect(time).toHaveAttribute("datetime", event.occurredAt);
    expect(time).toHaveAccessibleName(`${relative}, ${exact}`);
    expect(time).not.toHaveAttribute("title");
    const user = userEvent.setup();
    await user.hover(time);
    // Radix listens for pointer movement, not only pointer entry.
    fireEvent.pointerMove(time, { pointerType: "mouse" });
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent(exact);
    expect(time).toHaveAttribute("aria-describedby", tooltip.id);
    await user.unhover(time);
    fireEvent.pointerMove(document.body, { pointerType: "mouse", clientX: 500, clientY: 500 });
    await waitFor(() => { expect(screen.queryByRole("tooltip")).not.toBeInTheDocument(); });
    for (let step = 0; step < 5 && document.activeElement !== time; step++) await user.tab();
    expect(time).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(exact);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    expect(time).toHaveFocus();
  });

  it("chooses the most recent use by its timestamp, including fractional seconds", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ items: [], tokens: [
      { ...tokens[0], lastUsed: "2026-10-21T12:00:00Z" },
      { ...tokens[1], lastUsed: "2026-10-21T12:00:00.500Z" },
    ] }))));
    renderIn(<Harness />);
    await screen.findByText(/No activity recorded/);
    expect(screen.getByText("Last used").parentElement?.querySelector("time")).toHaveAttribute("datetime", "2026-10-21T12:00:00.500Z");
  });

  it("does not infer client identity from a token label, and redacted or nonlocal targets stay unavailable", async () => {
    const unnamed = { ...event, client: "", tokenLabel: "Claude Code token", revoked: true };
    delete unnamed.clientVersion;
    delete unnamed.target;
    const connection = { ...event, id: "e3", operation: "connect" as const };
    delete connection.target;
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response(page([
      unnamed,
      { ...event, id: "e2", target: { id: "artifact2", title: "Unsafe destination", url: "https://elsewhere.test/a/artifact2" } },
      connection,
    ])))));
    renderIn(<Harness />);
    expect(await screen.findByText("Unknown client")).toBeVisible();
    expect(screen.getByText("Claude Code token")).toHaveTextContent("Revoked");
    expect(screen.getAllByText("Unavailable artifact")).toHaveLength(2);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByText("Unsafe destination")).not.toBeInTheDocument();
  });

  it("cancels superseded token reads and ignores stale responses even when the read boundary resolves them", async () => {
    const pending: { resolve: (response: Response) => void; signal: AbortSignal }[] = [];
    vi.stubGlobal("fetch", vi.fn((_url: string, init: RequestInit) => new Promise<Response>((resolve) => { if (!init.signal) throw new Error("activity reads must be cancellable"); pending.push({ resolve, signal: init.signal }); })));
    renderIn(<Harness />);
    pending[0]?.resolve(response(page()));
    await screen.findByText("Planning notes");
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("Token"), "t1");
    await user.selectOptions(screen.getByLabelText("Token"), "t2");
    expect(pending[1]?.signal.aborted).toBe(true);
    pending[2]?.resolve(response(page([{ ...event, id: "current", tokenID: "t2", client: "Current client" }])));
    await screen.findByText("Current client");
    pending[1]?.resolve(response(page([{ ...event, id: "stale", client: "Stale client" }])));
    await waitFor(() => { expect(screen.queryByText("Stale client")).not.toBeInTheDocument(); });
    expect(screen.getByLabelText("Token")).toHaveValue("t2");
  });

  it("keeps loaded rows on a pagination error, retries the same cursor and deduplicates overlapping events", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(response(page([event], "next"))).mockResolvedValueOnce(response({}, 500)).mockResolvedValueOnce(response(page([event, { ...event, id: "e2", client: "Claude Code" }])));
    vi.stubGlobal("fetch", fetcher);
    renderIn(<Harness />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("current list is still here");
    expect(screen.getByRole("link", { name: "Planning notes" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("Claude Code");
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(fetcher.mock.calls[1]?.[0]).toBe(fetcher.mock.calls[2]?.[0]);
    expect(fetcher.mock.calls[1]?.[0]).toContain("cursor=next");
  });

  it("recovers from an invalid read response and speaks Chinese in the empty state", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(response([])).mockResolvedValueOnce(response(page([]))));
    renderIn(<Harness />, "zh-Hant");
    expect(await screen.findByRole("alert")).toHaveTextContent("無法載入活動");
    await userEvent.setup().click(screen.getByRole("button", { name: "再試一次" }));
    expect(await screen.findByText(/尚無活動紀錄/)).toBeVisible();
    expect(screen.getByRole("heading", { name: "最近活動" })).toBeVisible();
  });

  it("filters from a token row and refreshes retained revoked activity after existing revoke confirmation", async () => {
    let revoked = false;
    vi.stubGlobal("fetch", vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "DELETE") { revoked = true; return Promise.resolve(new Response(null, { status: 204 })); }
      if (url.startsWith("/api/agent-activity")) return Promise.resolve(response({ items: [{ ...event, revoked }], tokens: [{ ...tokens[0], revoked }] }));
      return Promise.resolve(response([]));
    }));
    renderIn(<Connect boot={bootFor("en", { page: "connect", connect: { baseUrl: "https://pictor.test", tokens: [{ id: "t1", name: "Laptop", createdAt: event.occurredAt }] } })} />);
    await screen.findByText("Planning notes");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Show activity for Laptop" }));
    expect(screen.getByLabelText("Token")).toHaveValue("t1");
    await user.click(screen.getByRole("button", { name: "Revoke Laptop" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Revoke Laptop" })).not.toBeInTheDocument(); });
    expect(await screen.findByText("Revoked", { selector: ".activity-revoked" })).toBeVisible();
  });
});
