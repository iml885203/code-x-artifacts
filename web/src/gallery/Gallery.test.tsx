import { expectHint } from "../test-tooltip";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { Gallery } from "./Gallery";

const mine = artifact({ title: "Sprint report", author: "alice", visibility: "organisation" });
const theirs = artifact({ title: "Cheat sheet", author: "bob" });

function serve(status: number, body: Artifact[] = []) {
  const fetchMock = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status })));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function openGallery(list: Artifact[], lang: "en" | "zh-Hant" = "en") {
  serve(200, list);
  renderIn(<Gallery boot={bootFor(lang)} />, lang);
  if (list.length > 0) await screen.findAllByRole("list");
}

function nth(i: number): HTMLElement {
  const li = screen.getAllByRole("listitem")[i];
  if (!li) throw new Error(`no card ${String(i)}`);
  return li;
}
const cards = () => screen.queryAllByRole("listitem").map((li) => within(li).getByRole("heading").textContent);

const assign = vi.fn();
beforeEach(() => {
  vi.stubGlobal("location", { assign });
  // Only the clock: the times on the cards are read against it, the rest of the test runs in real time.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the gallery", () => {
  it("reads the artifacts from the JSON API with the cookie and the bridge header", async () => {
    const fetchMock = serve(200, [mine]);
    renderIn(<Gallery boot={bootFor()} />);
    await screen.findByRole("list");
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/artifacts");
    expect(init.credentials).toBe("same-origin");
    expect(init.headers).toEqual({ "X-Pictor": "1" });
  });

  it("shows every artifact with author, relative time and an icon for who can open it, and marks the open ones", async () => {
    await openGallery([mine, theirs]);
    expect(cards()).toEqual(["Sprint report", "Cheat sheet"]);
    const card = nth(0);
    expect(within(card).getByText("alice")).toBeInTheDocument();
    expect(within(card).getByRole("img", { name: "Organisation" })).toHaveClass("open");
    expect(within(card).getByRole("link")).toHaveAttribute("href", `/a/${mine.id}`);
    expect(within(card).getByText("Edited 2 weeks ago")).toBeInTheDocument();
    expect(within(nth(1)).getByRole("img", { name: "Only me" })).not.toHaveClass("open");
  });

  it("gives every card an audience icon with a name and a tooltip, and no word for it", async () => {
    const everyone = ["private", "organisation", "public"].map((visibility) => artifact({ title: `Card ${visibility}`, author: "alice", visibility }));
    await openGallery(everyone);
    const names = { private: "Only me", organisation: "Organisation", public: "Public link" };
    for (const [i, a] of everyone.entries()) {
      const icon = within(nth(i)).getByRole("img", { name: names[a.visibility as keyof typeof names] });
      await expectHint(icon, names[a.visibility as keyof typeof names]);
      expect(icon.textContent).toBe("");
      expect(icon.querySelector("svg")).not.toBeNull();
    }
  });

  it("shows relative times with the exact time to hover and to assistive technology, in the page language", async () => {
    await openGallery([mine], "zh-Hant");
    const time = within(nth(0)).getByText("2 週前編輯");
    expect(time.tagName).toBe("TIME");
    expect(time).toHaveAttribute("datetime", mine.updatedAt);
    await expectHint(time, /^2026-10-02 \d\d:\d\d$/);
    expect(time.getAttribute("aria-label")).toMatch(/^2 週前編輯, 2026-10-02 /);
  });

  it("previews each artifact in a sandboxed iframe that runs the page's script but gets no bridge", async () => {
    await openGallery([mine]);
    const frame = document.querySelector("iframe");
    expect(frame).toHaveAttribute("src", `/raw/${mine.id}?preview`);
    // Script only: no same-origin, no forms, no popups.
    expect(frame?.getAttribute("sandbox")).toBe("allow-scripts");
    expect(frame).toHaveAttribute("loading", "lazy");
  });

  it("keeps a preview hidden until its page has loaded, out of the tab order and out of the accessibility tree", async () => {
    await openGallery([mine]);
    const frame = document.querySelector("iframe");
    if (!frame) throw new Error("no preview frame");
    const screenBox = frame.closest(".thumb-screen");
    expect(screenBox).not.toHaveClass("loaded");
    expect(frame).toHaveAttribute("aria-hidden", "true");
    expect(frame).toHaveAttribute("tabindex", "-1");
    fireEvent.load(frame);
    expect(screenBox).toHaveClass("loaded");
  });

  it("draws a quiet half-size preview whose fade reaches the footer without an unused lower band", () => {
    const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "gallery.css"), "utf8");
    const screenRule = /\.thumb-screen \{ -webkit-mask-image: linear-gradient\(to bottom, #000 (\d+)%, transparent (\d+)%\)/.exec(css);
    expect(screenRule).not.toBeNull();
    // Content stays quiet from the top fifth, but the fade uses the whole bounded screen.
    expect(Number(screenRule?.[1])).toBeLessThanOrEqual(20);
    expect(Number(screenRule?.[2])).toBe(100);
    // Half size: a third or less would put a dozen lines of small print on each card.
    expect(css).toMatch(/\.thumb iframe \{[^}]*width: 200%; height: 200%;[^}]*transform: scale\(0\.5\)/);
    // A touch screen has no hover, so nothing about the glimpse depends on it.
    expect(css).not.toMatch(/\.thumb[^{]*:hover/);
  });

  it("filters to my artifacts and to the ones shared with me", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    const group = screen.getByRole("radiogroup", { name: "Filter" });
    expect(within(group).getByRole("radio", { name: /^All/ })).toBeChecked();

    await user.click(within(group).getByRole("radio", { name: /^Mine/ }));
    expect(cards()).toEqual(["Sprint report"]);
    expect(within(group).getByRole("radio", { name: /^Mine/ })).toBeChecked();

    await user.click(within(group).getByRole("radio", { name: /^Shared with me/ }));
    expect(cards()).toEqual(["Cheat sheet"]);

    await user.click(within(group).getByRole("radio", { name: /^All/ }));
    expect(cards()).toEqual(["Sprint report", "Cheat sheet"]);
  });

  it("keeps the chosen filter when the same button is pressed again", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    const group = screen.getByRole("radiogroup", { name: "Filter" });
    await user.click(within(group).getByRole("radio", { name: /^Mine/ }));
    await user.click(within(group).getByRole("radio", { name: /^Mine/ }));
    expect(cards()).toEqual(["Sprint report"]);
    expect(within(group).getByRole("radio", { name: /^Mine/ })).toBeChecked();
  });

  it("searches title and author as you type, together with the filter", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    const search = screen.getByRole("searchbox", { name: "Search" });
    expect(search).toHaveAttribute("placeholder", "Search title or author");

    await user.type(search, "SHEET");
    expect(cards()).toEqual(["Cheat sheet"]);

    await user.clear(search);
    await user.type(search, "alice");
    expect(cards()).toEqual(["Sprint report"]);

    await user.click(screen.getByRole("radio", { name: /^Shared with me/ }));
    expect(cards()).toEqual([]);
  });

  it("says nothing matches when the filter and search leave no card", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    expect(screen.queryByText("Nothing matches")).not.toBeInTheDocument();
    await user.type(screen.getByRole("searchbox"), "zzz");
    expect(screen.getByText("Nothing matches")).toBeInTheDocument();
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    await user.clear(screen.getByRole("searchbox"));
    expect(screen.queryByText("Nothing matches")).not.toBeInTheDocument();
    expect(cards()).toHaveLength(2);
  });

  it("shows how to publish, with no filters or search, when there is nothing yet", async () => {
    await openGallery([]);
    expect(await screen.findByText(/Nothing here yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create a token" })).toHaveAttribute("href", "/connect");
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Filter" })).not.toBeInTheDocument();
  });

  it("keeps sign out in the account menu, opened from the initials", async () => {
    const user = userEvent.setup();
    await openGallery([mine]);
    const rail = screen.getByRole("navigation", { name: "Library" });
    const account = within(rail).getByRole("button", { name: "Account: alice" });
    expect(account).toHaveTextContent("A");
    await expectHint(account, "alice");
    await user.click(account);
    expect(await screen.findByRole("menuitem", { name: "Sign out" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Tokens" })).not.toBeInTheDocument();
    const out = screen.getByRole("menuitem", { name: "Sign out" });
    expect(out.closest("form")).toHaveAttribute("action", "/logout");
    expect(out.closest("form")).toHaveAttribute("method", "post");
  });

  it("counts what each filter holds, whatever is typed in the search", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    const counts = () => screen.getAllByRole("radio").map((r) => r.textContent);
    expect(counts()).toEqual(["All2", "Mine1", "Shared with me1", "Pinned0"]);
    await user.type(screen.getByRole("searchbox"), "zzz");
    expect(counts()).toEqual(["All2", "Mine1", "Shared with me1", "Pinned0"]);
  });

  it("jumps to the search when / is pressed, but types a slash into a field that has the keyboard", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    await user.keyboard("/");
    const search = screen.getByRole("searchbox");
    expect(search).toHaveFocus();
    expect(search).toHaveValue("");
    await user.keyboard("a/b");
    expect(search).toHaveValue("a/b");
  });

  it("holds plates where the cards will be while the list loads", () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
    renderIn(<Gallery boot={bootFor()} />);
    expect(screen.getByRole("main")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(document.querySelectorAll(".card.plate")).toHaveLength(6);
  });

  it("opens an artifact inside the app, keeping the link for a new tab", async () => {
    const user = userEvent.setup();
    await openGallery([mine, theirs]);
    const link = within(nth(0)).getByRole("link");
    expect(link).toHaveAttribute("href", `/a/${mine.id}`);
    const push = vi.spyOn(history, "pushState").mockImplementation(() => undefined);
    await user.click(link);
    expect(push).toHaveBeenCalledWith(null, "", `/a/${mine.id}`);
  });

  it("speaks the language the server picked", async () => {
    await openGallery([mine, theirs], "zh-Hant");
    expect(screen.getByRole("radio", { name: /^分享給我的/ })).toBeInTheDocument();
    expect(screen.getByRole("searchbox")).toHaveAttribute("placeholder", "搜尋標題或作者");
    expect(screen.getByRole("img", { name: "組織內" })).toBeInTheDocument();
  });

  it("goes to sign-in when the session has ended", async () => {
    serve(401);
    renderIn(<Gallery boot={bootFor()} />);
    await waitFor(() => {
      expect(assign).toHaveBeenCalledWith("/login?next=%2F");
    });
  });

  it("says it could not load the artifacts when the API fails", async () => {
    serve(500);
    renderIn(<Gallery boot={bootFor()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't load the artifacts");
  });

  it("tells the author how many new comments an artifact has, and says nothing when there are none", async () => {
    await openGallery([{ ...mine, comments: { count: 3, unread: 2 } }, { ...theirs, comments: { count: 5, unread: 0 } }]);
    expect(within(nth(0)).getByText("2 new")).toBeInTheDocument();
    expect(within(nth(1)).queryByText(/new/)).not.toBeInTheDocument();
  });

  it("clamps a long title to two lines with the whole title as its tooltip, and keeps author and time in the footer under it", async () => {
    const long = artifact({ title: "A very long title ".repeat(12).trim(), author: "alice" });
    await openGallery([long, theirs]);
    for (const [i, a] of [long, theirs].entries()) {
      const card = nth(i);
      const heading = within(card).getByRole("heading");
      await expectHint(heading, a.title);
      // Title, author and time share the open target, so the footer reads as one block whatever the title's length.
      const link = within(card).getByRole("link");
      expect(within(link).getByText(a.author)).toBeInTheDocument();
      expect(link.querySelector("time")).not.toBeNull();
    }
    const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "gallery.css"), "utf8");
    const rule = /\.card-title \{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(rule).toMatch(/-webkit-line-clamp:\s*2\b/);
    expect(rule).toMatch(/min-height:\s*2\.7em/);
  });

  it("puts pin and more in one corner group beside the card link, and neither opens the artifact", async () => {
    const user = userEvent.setup();
    await openGallery([mine]);
    const card = nth(0);
    const link = within(card).getByRole("link");
    const group = card.querySelector<HTMLElement>(".card-actions");
    if (!group) throw new Error("no action group");
    expect(link.contains(group)).toBe(false);
    const pin = within(group).getByRole("button", { name: /^Pin Sprint report/ });
    const more = within(group).getByRole("button", { name: "Actions for Sprint report" });
    expect(within(group).getAllByRole("button")).toEqual([more, pin]);
    await user.click(pin);
    await user.click(more);
    expect(assign).not.toHaveBeenCalled();
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("gives every icon-only control a name and a tooltip", async () => {
    await openGallery([{ ...mine, pinned: true, visibility: "organisation" }]);
    const card = nth(0);
    const iconOnly = within(card).getAllByRole("button").filter((b) => b.textContent.trim() === "");
    expect(iconOnly.length).toBe(2);
    for (const button of iconOnly) {
      expect(button.getAttribute("aria-label"), "name").toBeTruthy();
      await expectHint(button, /.+/);
    }
    await expectHint(within(card).getByRole("img", { name: "Organisation" }), "Organisation");
  });

  it("opens account settings from the account name with pointer and keyboard, without a Kit link", async () => {
    const user = userEvent.setup();
    await openGallery([mine]);
    const account = screen.getByRole("button", { name: "Account: alice" });
    await user.click(within(account).getByText("alice"));
    expect(await screen.findByRole("menu")).toBeVisible();
    expect(screen.queryByRole("menuitem", { name: "Kit" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    account.focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("menu")).toBeVisible();
  });

  it("uses the Connect page label in Chinese navigation", async () => {
    await openGallery([mine], "zh-Hant");
    expect(screen.getByRole("link", { name: "連接你的 agent" })).toHaveAttribute("href", "/connect");
    expect(screen.queryByRole("link", { name: "Kit" })).not.toBeInTheDocument();
  });

  it("opens the rail with one agent connection destination and the current account, without unavailable destinations", async () => {
    await openGallery([mine, theirs]);
    const rail = screen.getByRole("navigation", { name: "Library" });
    const links = within(rail).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual(["CodeXArtifacts", "Artifacts2", "Recently opened", "Connect your agent"]);
    expect(links[1]).toHaveAttribute("aria-current", "page");
    expect(links[2]).toHaveAttribute("href", "/recent");
    expect(links[3]).toHaveAttribute("href", "/connect");
    expect(links.filter((l) => l.getAttribute("href") === "/connect")).toHaveLength(1);
    expect(links.some((link) => link.getAttribute("href") === "/kit")).toBe(false);
    expect(within(rail).getByText("alice")).toBeVisible();
    // The same links stay on a phone, behind a button that folds them.
    const toggle = within(rail).getByRole("button", { name: "Menu" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await userEvent.setup().click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(rail.querySelector(".lib-nav-list")).toHaveClass("open");
    await userEvent.setup().click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("starts something new from the top row, beside the search", async () => {
    const user = userEvent.setup();
    await openGallery([mine]);
    await user.click(screen.getByRole("button", { name: "New" }));
    expect(await screen.findByRole("menuitem", { name: "Document" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Design" })).toBeInTheDocument();
  });

  describe("grouping and the list", () => {
    const bob = artifact({ title: "Bob's notes", author: "bob", kind: "doc" });
    const deck = artifact({ title: "Our deck", author: "alice", kind: "slides" });
    const cssText = () => readFileSync(join(dirname(fileURLToPath(import.meta.url)), "gallery.css"), "utf8");

    async function choose(user: ReturnType<typeof userEvent.setup>, group: string) {
      await user.click(screen.getByRole("button", { name: /^Group by/ }));
      await user.click(await screen.findByRole("menuitemradio", { name: group }));
    }
    const headers = () => screen.queryAllByRole("heading", { level: 2 }).filter((h) => h.classList.contains("lib-group-head")).map((h) => h.firstChild?.textContent);

    afterEach(() => {
      vi.unstubAllGlobals();
      localStorage.clear();
    });

    it("sits a second person's items under their own name when grouped by owner", async () => {
      const user = userEvent.setup();
      await openGallery([mine, bob, deck]);
      expect(headers()).toEqual([]);
      await choose(user, "Owner");
      expect(headers()).toEqual(["alice", "bob"]);
      const group = screen.getByRole("region", { name: "bob" });
      expect(within(group).getAllByRole("listitem").map((li) => within(li).getByRole("heading").textContent)).toEqual(["Bob's notes"]);
    });

    it("restores the person's group and view when the library opens again", async () => {
      const user = userEvent.setup();
      await openGallery([mine, bob]);
      await choose(user, "Owner");
      await user.click(screen.getByRole("button", { name: "List" }));
      cleanup();
      await openGallery([mine, bob]);
      expect(headers()).toEqual(["alice", "bob"]);
      expect(screen.getByRole("button", { name: "List" })).toHaveAttribute("aria-pressed", "true");
      expect(document.querySelector(".lib-list")).not.toBeNull();
    });

    it("opens ungrouped, as a grid, when the browser's storage throws", async () => {
      vi.stubGlobal("localStorage", { getItem: () => { throw new Error("blocked"); }, setItem: () => { throw new Error("blocked"); } });
      await openGallery([mine, bob]);
      expect(headers()).toEqual([]);
      expect(screen.getByRole("button", { name: "Grid" })).toHaveAttribute("aria-pressed", "true");
    });

    it("heads the groups with the date, or with the kind in its colour", async () => {
      const user = userEvent.setup();
      await openGallery([mine, bob, deck]);
      await choose(user, "Edit date");
      expect(headers()).toEqual(["Earlier"]);
      await choose(user, "Kind");
      expect(headers()).toEqual(["Web page", "Doc", "Slides"]);
      const head = (name: string) => screen.getByRole("heading", { name: new RegExp(`^${name}`) });
      expect([head("Web page"), head("Doc"), head("Slides")].map((h) => h.getAttribute("data-kind"))).toEqual(["web", "doc", "slides"]);
      // The header takes the kind's glyph colour from the same token pair the mark uses.
      expect(head("Doc").style.getPropertyValue("--kh")).toBe("var(--kind-doc-fg)");
      expect(cssText()).toContain(".lib-group-head[data-kind] { color: var(--kh); }");
    });

    it("shows a row with the kind's tile, the title, an unread dot, then pin, audience, time and the menu, and keeps a long title from pushing the right side out", async () => {
      const user = userEvent.setup();
      const long = { ...mine, title: "A long title ".repeat(15).trim(), visibility: "organisation", comments: { count: 3, unread: 2 } };
      await openGallery([long, theirs]);
      await user.click(screen.getByRole("button", { name: "List" }));
      const row = nth(0);
      expect(row).toHaveClass("lib-row");
      const link = within(row).getByRole("link");
      expect(within(link).getByRole("img", { name: "Web page" })).toBeInTheDocument();
      const title = within(row).getByRole("heading");
      await expectHint(title, long.title);
      expect(within(link).getByRole("img", { name: "2 new" })).toHaveTextContent("2");
      const side = row.querySelector<HTMLElement>(".row-side");
      if (!side) throw new Error("no right side");
      expect(Array.from(side.children).map((c) => c.tagName + (c.getAttribute("role") ?? ""))).toEqual(["BUTTON", "SPANimg", "TIME", "BUTTON"]);
      expect(within(side).getByRole("button", { name: /^Pin / })).toBeInTheDocument();
      expect(within(side).getByRole("img", { name: "Organisation" })).toBeInTheDocument();
      expect(within(side).getByRole("button", { name: /^Actions for / })).toBeInTheDocument();
      const css = cssText();
      expect(/\.lib-row \.row-title \{([^}]*)\}/.exec(css)?.[1]).toMatch(/text-overflow:\s*ellipsis[^}]*white-space:\s*nowrap|white-space:\s*nowrap[^}]*text-overflow:\s*ellipsis/);
      expect(css).toMatch(/\.row-side \{[^}]*flex:\s*none/);
      expect(css).toMatch(/\.row-link \{[^}]*min-width:\s*0/);
    });

    it("gives a row the same menu as a card", async () => {
      const user = userEvent.setup();
      await openGallery([{ ...mine, canChange: true, canEdit: true }]);
      await user.click(screen.getByRole("button", { name: "List" }));
      await user.click(within(nth(0)).getByRole("button", { name: "Actions for Sprint report" }));
      const menu = await screen.findByRole("menu");
      expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["PinP", "RenameR", "Make a copy", "Copy linkC", "Delete…"]);
      for (const item of within(menu).getAllByRole("menuitem")) {
        expect(item.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
      }
    });
  });

  describe("the card menu", () => {
    const owned = { ...mine, canChange: true, canEdit: true, role: "author" as const };
    const calls = (fetchMock: { mock: { calls: unknown[] } }) => (fetchMock.mock.calls as unknown as [string, RequestInit | undefined][]).filter(([, init]) => init?.method !== undefined && init.method !== "GET").map(([url, init]) => `${init?.method ?? ""} ${url}`);

    async function openMenu(user: ReturnType<typeof userEvent.setup>, card: HTMLElement, title: string) {
      await user.click(within(card).getByRole("button", { name: `Actions for ${title}` }));
      return screen.findByRole("menu");
    }

    it("offers the author pin, rename, copy, copy link and delete, with each letter beside its action, and a reader neither rename nor delete", async () => {
      const user = userEvent.setup();
      await openGallery([owned, theirs]);
      let menu = await openMenu(user, nth(0), "Sprint report");
      expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["PinP", "RenameR", "Make a copy", "Copy linkC", "Delete…"]);
      await user.keyboard("{Escape}");
      menu = await openMenu(user, nth(1), "Cheat sheet");
      expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["PinP", "Make a copy", "Copy linkC"]);
    });

    it("never offers to copy for an agent, whoever opens the menu", async () => {
      const user = userEvent.setup();
      await openGallery([owned, theirs]);
      for (const [i, title] of [[0, "Sprint report"], [1, "Cheat sheet"]] as const) {
        const menu = await openMenu(user, nth(i), title);
        expect(within(menu).queryByRole("menuitem", { name: /agent/i })).not.toBeInTheDocument();
        await user.keyboard("{Escape}");
      }
    });

    it("runs an action from its letter while the menu is open: P pins, C copies the link, R renames", async () => {
      const user = userEvent.setup();
      const renamed = { ...owned, title: "Sprint review" };
      const fetchMock = vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === "PUT" && url.endsWith("/title")) return Promise.resolve(new Response(JSON.stringify(renamed)));
        if (init?.method === "PUT") return Promise.resolve(new Response(JSON.stringify({ ...owned, pinned: true })));
        return Promise.resolve(new Response(JSON.stringify([owned, theirs])));
      });
      vi.stubGlobal("fetch", fetchMock);
      renderIn(<Gallery boot={bootFor()} />);
      await screen.findByRole("list");

      await openMenu(user, nth(0), "Sprint report");
      await user.keyboard("p");
      await waitFor(() => {
        expect(calls(fetchMock)).toEqual([`PUT /api/artifacts/${owned.id}/pin`]);
      });
      expect(screen.queryByRole("menu")).not.toBeInTheDocument();

      await openMenu(user, nth(0), "Sprint report");
      await user.keyboard("c");
      expect(await navigator.clipboard.readText()).toBe(`${location.origin}/a/${owned.id}`);

      await openMenu(user, nth(0), "Sprint report");
      await user.keyboard("r");
      const field = await screen.findByRole("textbox", { name: "Title" });
      await waitFor(() => {
        expect(field).toHaveFocus();
      });
      await user.clear(field);
      await user.type(field, "Sprint review{Enter}");
      await waitFor(() => {
        expect(cards()).toEqual(["Sprint review", "Cheat sheet"]);
      });
      expect(calls(fetchMock)).toContain(`PUT /api/artifacts/${owned.id}/title`);
    });

    it("keeps the old title when renaming is cancelled with Escape", async () => {
      const user = userEvent.setup();
      const fetchMock = vi.fn(() => Promise.resolve(new Response(JSON.stringify([owned]))));
      vi.stubGlobal("fetch", fetchMock);
      renderIn(<Gallery boot={bootFor()} />);
      await screen.findByRole("list");
      await openMenu(user, nth(0), "Sprint report");
      await user.keyboard("r");
      await user.type(await screen.findByRole("textbox", { name: "Title" }), "Other{Escape}");
      expect(cards()).toEqual(["Sprint report"]);
      expect(calls(fetchMock)).toEqual([]);
    });

    it("copies the page's link", async () => {
      const user = userEvent.setup();
      await openGallery([owned]);
      const menu = await openMenu(user, nth(0), "Sprint report");
      await user.click(within(menu).getByRole("menuitem", { name: "Copy link" }));
      expect(await screen.findByText("Copied")).toBeInTheDocument();
      expect(await navigator.clipboard.readText()).toBe(`${location.origin}/a/${owned.id}`);
    });

    it("makes your own copy from the card and lands on it", async () => {
      const user = userEvent.setup();
      const made = artifact({ title: "Cheat sheet (Copy)", author: "alice", canChange: true });
      const fetchMock = vi.fn((url: string, init?: RequestInit) => Promise.resolve(new Response(JSON.stringify(init?.method === "POST" ? made : [theirs]), { status: init?.method === "POST" ? 201 : 200 })));
      vi.stubGlobal("fetch", fetchMock);
      renderIn(<Gallery boot={bootFor()} />);
      await screen.findByRole("list");
      const push = vi.spyOn(history, "pushState").mockImplementation(() => undefined);
      await user.click(within(await openMenu(user, nth(0), "Cheat sheet")).getByRole("menuitem", { name: "Make a copy" }));
      await waitFor(() => {
        expect(push).toHaveBeenCalledWith(null, "", `/a/${made.id}`);
      });
      expect(calls(fetchMock)).toEqual([`POST /api/artifacts/${theirs.id}/copy`]);
    });

    it("deletes after asking once, and the card is gone", async () => {
      const user = userEvent.setup();
      const fetchMock = vi.fn((url: string, init?: RequestInit) => Promise.resolve(init?.method === "DELETE" ? new Response(null, { status: 204 }) : new Response(JSON.stringify([owned, theirs]))));
      vi.stubGlobal("fetch", fetchMock);
      renderIn(<Gallery boot={bootFor()} />);
      await screen.findByRole("list");
      await user.click(within(await openMenu(user, nth(0), "Sprint report")).getByRole("menuitem", { name: "Delete…" }));
      const dialog = await screen.findByRole("alertdialog", { hidden: true });
      expect(calls(fetchMock)).toEqual([]);
      await user.click(within(dialog).getByRole("button", { name: "Delete", hidden: true }));
      await waitFor(() => {
        expect(cards()).toEqual(["Cheat sheet"]);
      });
      expect(calls(fetchMock)).toEqual([`DELETE /api/artifacts/${owned.id}`]);
    });
  });

  describe("pinning", () => {
    function pinServer(list: Artifact[], refuse = false) {
      const fetchMock = vi.fn((url: string, init?: RequestInit) => {
        const method = init?.method ?? "GET";
        const id = /\/api\/artifacts\/([^/]+)\/pin$/.exec(url)?.[1];
        const found = list.find((a) => a.id === id);
        if (method === "GET") return Promise.resolve(new Response(JSON.stringify(list)));
        if (refuse || !found) return Promise.resolve(new Response(JSON.stringify({ error: "no" }), { status: 500 }));
        return Promise.resolve(new Response(JSON.stringify({ ...found, pinned: method === "PUT" })));
      });
      vi.stubGlobal("fetch", fetchMock);
      return fetchMock;
    }
    const pinCalls = (fetchMock: { mock: { calls: unknown[] } }) => (fetchMock.mock.calls as unknown as [string, RequestInit | undefined][]).filter(([, init]) => init?.method !== undefined && init.method !== "GET").map(([url, init]) => `${init?.method ?? ""} ${url}`);

    async function opened(list: Artifact[], refuse = false) {
      const fetchMock = pinServer(list, refuse);
      renderIn(<Gallery boot={bootFor()} />);
      await screen.findByRole("list");
      return fetchMock;
    }

    it("moves a pinned card to the top, lists it under Pinned and counts it", async () => {
      const user = userEvent.setup();
      const fetchMock = await opened([mine, theirs]);
      expect(cards()).toEqual(["Sprint report", "Cheat sheet"]);
      await user.click(within(nth(1)).getByRole("button", { name: "Pin Cheat sheet" }));
      await waitFor(() => {
        expect(cards()).toEqual(["Cheat sheet", "Sprint report"]);
      });
      expect(pinCalls(fetchMock)).toEqual([`PUT /api/artifacts/${theirs.id}/pin`]);
      expect(within(nth(0)).getByRole("button", { name: "Unpin Cheat sheet" })).toHaveAttribute("aria-pressed", "true");
      expect(within(nth(1)).getByRole("button", { name: "Pin Sprint report" })).toHaveAttribute("aria-pressed", "false");
      await user.click(screen.getByRole("radio", { name: /^Pinned/ }));
      await waitFor(() => {
        expect(cards()).toEqual(["Cheat sheet"]);
      });
      expect(screen.getByRole("radio", { name: /^Pinned/ })).toHaveTextContent("Pinned1");
    });

    it("sends the pin so that it outlives a navigation or reload that follows at once", async () => {
      const user = userEvent.setup();
      const fetchMock = await opened([mine, theirs]);
      await user.click(within(nth(1)).getByRole("button", { name: "Pin Cheat sheet" }));
      await waitFor(() => {
        expect(pinCalls(fetchMock)).toEqual([`PUT /api/artifacts/${theirs.id}/pin`]);
      });
      const [, init] = (fetchMock.mock.calls as unknown as [string, RequestInit | undefined][]).find(([, i]) => i?.method === "PUT") ?? ["", undefined];
      expect(init?.keepalive).toBe(true);
    });

    it("pins and unpins from the card's menu too", async () => {
      const user = userEvent.setup();
      const fetchMock = await opened([{ ...mine, pinned: true }, theirs]);
      expect(cards()).toEqual(["Sprint report", "Cheat sheet"]);
      await user.click(within(nth(0)).getByRole("button", { name: "Actions for Sprint report" }));
      await user.click(await screen.findByRole("menuitem", { name: "Unpin" }));
      await waitFor(() => {
        expect(pinCalls(fetchMock)).toEqual([`DELETE /api/artifacts/${mine.id}/pin`]);
      });
      expect(within(nth(0)).getByRole("button", { name: "Pin Sprint report" })).toHaveAttribute("aria-pressed", "false");
    });

    it("puts the card back and says so when the server refuses the pin", async () => {
      const user = userEvent.setup();
      await opened([mine, theirs], true);
      await user.click(within(nth(1)).getByRole("button", { name: "Pin Cheat sheet" }));
      expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't change the pin");
      expect(cards()).toEqual(["Sprint report", "Cheat sheet"]);
    });
  });
});
