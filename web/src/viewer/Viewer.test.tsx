import { expectHint } from "../test-tooltip";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, frameSandbox, renderIn } from "../test-support";
import kinds from "../registered-kinds.json";
import { Viewer } from "./Viewer";

/** The menu items that are export formats: the ones that differ by kind. */
const exportItem = /^(HTML|Markdown|PDF)/;

const mine = artifact({ title: "Sprint report", author: "alice", canChange: true, canEdit: true, role: "author" });
const theirs = artifact({ title: "Cheat sheet", author: "bob", visibility: "organisation" });

// The artifact the open viewer was given: asking the server for it again, as
// the viewer does on opening, answers the same unless a test says otherwise.
let opened: Artifact = mine;

function api(handler: (method: string, url: string, body: unknown) => Response | undefined = () => undefined) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    const body = typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined;
    const itself = method === "GET" && url === `/api/artifacts/${opened.id}` ? json(opened) : undefined;
    return Promise.resolve(handler(method, url, body) ?? itself ?? new Response("{}", { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

/** What the viewer sent that changes something: opening it also reads the artifact, which is not what these tests are about. */
/** What the viewer changed on the server; the heartbeat that says the page is open, and the open recorded for the person's own list (RecordOpen.test.tsx), are not changes. */
function writes(fetchMock: ReturnType<typeof api>) {
  return (fetchMock.mock.calls as unknown as [string, RequestInit][]).filter(([url, init]) => (init.method ?? "GET") !== "GET" && !url.endsWith("/viewers") && !url.endsWith("/open"));
}

function write(fetchMock: ReturnType<typeof api>, n: number): [string, RequestInit] {
  const call = writes(fetchMock)[n];
  if (!call) throw new Error(`the viewer made fewer than ${String(n + 1)} writes`);
  return call;
}

/** A PATCH that answers with the artifact as the server would after the change. */
function patching(a: Artifact) {
  return api((method, _url, body) => (method === "PATCH" ? json({ ...a, visibility: (body as { visibility: string }).visibility }) : undefined));
}

function open(a: Artifact, over: Parameters<typeof bootFor>[1] = {}, lang: "en" | "zh-Hant" = "en") {
  opened = a;
  return renderIn(<Viewer boot={bootFor(lang, { page: "viewer", ...over })} artifact={a} />, lang, `/a/${a.id}`);
}

/** Opens the share dialog from the bar. */
async function openShare(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /^(Share|分享)$/ }));
  return screen.findByRole("dialog");
}

let pushed: MockInstance<History["pushState"]>;
beforeEach(() => {
  pushed = vi.spyOn(history, "pushState").mockImplementation(() => undefined);
  // Only the clock: the times in the viewer are read against it, the rest of the test runs in real time.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the viewer", () => {
  it("fills the page with the artifact in an iframe that has exactly the sandbox the server sent", () => {
    open(mine);
    const frame = screen.getByTitle("Sprint report");
    expect(frame).toHaveAttribute("src", `/raw/${mine.id}`);
    expect(frame.getAttribute("sandbox")).toBe(frameSandbox);
    expect(frame.getAttribute("sandbox")).not.toContain("allow-same-origin");
    expect(frame.closest("main")).toBeInTheDocument();
  });

  it("shows who is viewing in the header of a signed-in person and tracks no signed-out visitor", async () => {
    const fetchMock = api((method, url) => (method === "POST" && url.endsWith("/viewers") ? json([{ id: "bob", name: "Bob", agent: false, self: false }]) : undefined));
    open(theirs, { user: "alice" });
    const header = screen.getByRole("banner");
    expect(await within(header).findByRole("group", { name: "Viewing now" })).toBeInTheDocument();
    expect(within(header).getByText("Bob")).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([url]) => url.endsWith("/viewers"))).toBe(true);
  });

  it("keeps presence active on a phone and folds viewers and account controls into More", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: true, media: query, addEventListener: () => undefined, removeEventListener: () => undefined }));
    const user = userEvent.setup();
    const viewers = ["Bob", "Carol", "Dave"].map((name) => ({ name, agent: false, self: false }));
    const calls = api((method, url) => (method === "POST" && url.endsWith("/viewers") ? json(viewers) : undefined));
    open(theirs, { user: "alice" });
    await waitFor(() => { expect(calls.mock.calls.some(([url]) => url.endsWith("/viewers"))).toBe(true); });
    expect(within(screen.getByRole("banner")).queryByRole("button", { name: "Account" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    expect(await screen.findByText("Carol")).toBeVisible();
    await user.click(screen.getByRole("menuitem", { name: "Account" }));
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{Enter}");
    await waitFor(() => { expect(document.documentElement).toHaveAttribute("data-theme", "dark"); });
  });

  it.each(["html", "react", "doc", "design"])("lists the guests of a %s artifact in the same stack at the top right, marked as guests", async (kind) => {
    api((method, url) => (method === "POST" && url.endsWith("/viewers") ? json([{ id: "bob", name: "Bob", agent: false, self: false }, { id: "guest:1", name: "Swift Otter", agent: false, guest: true, self: false }]) : undefined));
    open({ ...theirs, kind, visibility: "public" }, { user: "alice" });
    const stack = await within(screen.getByRole("banner")).findByRole("group", { name: "Viewing now" });
    expect(await within(stack).findByRole("button", { name: "Swift Otter (Guest)" })).toBeInTheDocument();
    expect(within(stack).getByRole("button", { name: "Bob" })).toBeInTheDocument();
  });

  it("shows a signed-out visitor of a public link itself as its browser's guest, on a phone in More, and never asks for the names", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: true, media: query, addEventListener: () => undefined, removeEventListener: () => undefined }));
    const user = userEvent.setup();
    const fetchMock = api((method, url) => (method === "POST" && url.endsWith("/viewers") ? json([{ id: "guest:1", name: "Swift Otter", agent: false, guest: true, self: true }]) : undefined));
    open({ ...theirs, visibility: "public" }, { user: "", initials: "" });
    await waitFor(() => { expect(fetchMock.mock.calls.some(([url]) => url.endsWith("/viewers"))).toBe(true); });
    const beat = (fetchMock.mock.calls as unknown as [string, RequestInit][]).find(([url]) => url.endsWith("/viewers"));
    expect(JSON.parse(beat?.[1].body as string)).toEqual({ guest: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/) as unknown });
    expect((fetchMock.mock.calls as unknown as [string, RequestInit][]).filter(([url, init]) => url.endsWith("/viewers") && (init.method ?? "GET") === "GET")).toEqual([]);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    expect(await screen.findByText("Swift Otter (You) · Guest")).toBeVisible();
  });

  it("lets the page in the frame go full screen, which a slides deck needs to present", () => {
    open(mine);
    expect(screen.getByTitle("Sprint report")).toHaveAttribute("allow", "fullscreen");
  });

  it("opens the frame at the slide named in the address and ignores anything that is not a slide number", () => {
    history.replaceState(null, "", `/a/${mine.id}?slide=3`);
    const first = open(mine);
    expect(screen.getByTitle("Sprint report")).toHaveAttribute("src", `/raw/${mine.id}?slide=3`);
    first.unmount();
    history.replaceState(null, "", `/a/${mine.id}?slide=%3Cscript%3E`);
    open(mine);
    expect(screen.getByTitle("Sprint report")).toHaveAttribute("src", `/raw/${mine.id}`);
    history.replaceState(null, "", "/");
  });

  it("shows the title, who made it, when, which version and who can open it", async () => {
    open(theirs);
    expect(screen.getByRole("heading", { name: "Cheat sheet" })).toBeInTheDocument();
    const facts = within(screen.getByRole("banner")).getByText("bob").closest("p");
    expect(facts).toHaveTextContent("bob");
    expect(facts).toHaveTextContent("Edited 2 weeks ago");
    await expectHint(within(facts as HTMLElement).getByText("Edited 2 weeks ago"), /^2026-10-02 /);
    expect(facts).toHaveTextContent("v1");
    // The audience is an icon with a name, not a word on the bar.
    expect(within(facts as HTMLElement).getByRole("img", { name: "Organisation" })).toBeInTheDocument();
    expect(facts).not.toHaveTextContent("Organisation");
  });

  it.each(kinds)("names a %s artifact's kind only in the mark's tooltip and accessible name, and shows no format version anywhere", async (kind) => {
    const user = userEvent.setup();
    const a = { ...mine, kind, format: kind === "react" ? "v1" : "1", title: "Sprint report", version: 3 };
    open(a);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByRole("menuitem", { name: "Open in new tab" });
    // Everything a person can read or hear: text and the labels read by assistive technology (the artifact's own frame is not chrome).
    // The names of the file types it exports as are not kinds, though "HTML" is both; the mark is the one place a kind is named.
    const text = (el: Element) => (el.childNodes.length > 0 ? [...el.childNodes].filter((n) => n.nodeType === Node.TEXT_NODE).map((n) => n.textContent ?? "").join("") : "");
    const chrome = [...document.body.querySelectorAll("*:not(.kind-mark)")]
      .flatMap((el) => [exportItem.test(text(el)) ? "" : text(el), el.getAttribute("aria-label") ?? "", el.getAttribute("title") ?? ""])
      .join(" ");
    expect(chrome).toContain("v3");
    for (const word of [...kinds, "kind", "format", "v1"]) expect(chrome).not.toMatch(new RegExp(`(^|[^\\w.])${word}($|[^\\w])`, "i"));
    const mark = document.querySelector(".kind-mark");
    await user.keyboard("{Escape}");
    await expectHint(mark as HTMLElement, mark?.getAttribute("aria-label") ?? "");
    expect(mark?.textContent).toBe("");
  });

  it("shows every registered kind the same bar items in the same order and the same menus", async () => {
    const user = userEvent.setup();
    api((_method, url) => (url.endsWith("/comments") ? json([]) : undefined));
    const shapes: string[] = [];
    for (const kind of kinds) {
      const view = open({ ...mine, kind, canComment: true });
      // Roles and names of what the bar offers; the kind's own name is only the mark's, so it is read as "kind".
      const bar = [...screen.getByRole("banner").querySelectorAll('a[href], button, [role="img"], h1')].map((el) => (el.classList.contains("kind-mark") ? "kind" : `${el.tagName} ${el.getAttribute("aria-label") ?? el.textContent}`));
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      const items = screen.getAllByRole("menuitem").map((i) => i.textContent);
      shapes.push(JSON.stringify([bar, items.filter((i) => !exportItem.test(i))]));
      await user.keyboard("{Escape}");
      view.unmount();
    }
    expect([...new Set(shapes)]).toHaveLength(1);
    expect((JSON.parse(shapes[0] ?? "[]") as string[][])[0]).toEqual(expect.arrayContaining(["kind", "H1 Sprint report", "BUTTON Share", "BUTTON More actions"]));
  });

  it("goes back to the overview inside the app", async () => {
    const user = userEvent.setup();
    open(mine);
    const back = screen.getByRole("link", { name: "Back to artifacts" });
    expect(back).toHaveAttribute("href", "/");
    await user.click(back);
    expect(pushed).toHaveBeenCalledWith(null, "", "/");
  });

  it("offers the author the three audiences, each with a sentence, and the current one chosen", async () => {
    const user = userEvent.setup();
    open({ ...mine, visibility: "organisation" }, { publicLinks: true });
    const dialog = await openShare(user);
    const group = within(dialog).getByRole("radiogroup", { name: "Who can open this" });
    expect(within(group).getAllByRole("radio").map((r) => r.closest("label")?.textContent)).toEqual([
      "Only meOnly you and the people you add can open it.",
      "OrganisationAnyone in the company who signs in can open it and comment.",
      "Public linkAnyone with the link can open it, without signing in.",
    ]);
    expect(within(group).getByRole("radio", { name: /^Organisation/ })).toBeChecked();
  });

  it("gives someone who did not make it no way to change who can open it, and no delete", async () => {
    const user = userEvent.setup();
    open(theirs, { user: "carol" });
    const dialog = await openShare(user);
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(within(dialog).getByText("Organisation")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await user.click(screen.getByRole("button", { name: "More actions" }));
    expect(await screen.findByRole("menuitem", { name: "Open in new tab" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Delete…" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Cheat sheet" }));
    expect(await screen.findByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
    for (const name of ["Rename", "Versions", "Delete…"]) expect(screen.queryByRole("menuitem", { name })).not.toBeInTheDocument();
  });

  it("says why the public link is unavailable instead of hiding it, and offers it where the server has public links", async () => {
    const user = userEvent.setup();
    const { unmount } = open(mine);
    const off = within(await openShare(user)).getByRole("radio", { name: /^Public link/ });
    expect(off).toBeDisabled();
    expect(off).toHaveAccessibleDescription("This server has no public links.");
    unmount();
    open(mine, { publicLinks: true });
    expect(within(await openShare(user)).getByRole("radio", { name: /^Public link/ })).toBeEnabled();
  });

  it("keeps a public artifact's choice usable after the server turned public links off, and says what it means now", async () => {
    const user = userEvent.setup();
    open({ ...mine, visibility: "public" });
    const chosen = within(await openShare(user)).getByRole("radio", { name: /^Public link/ });
    expect(chosen).toBeEnabled();
    expect(chosen).toBeChecked();
    expect(chosen).toHaveAccessibleDescription(expect.stringContaining("for now it works like Organisation"));
  });

  it("changes who can open it the moment a row is chosen, saves it, and offers to undo in the dialog", async () => {
    const user = userEvent.setup();
    const fetchMock = patching(mine);
    open(mine);
    const dialog = await openShare(user);
    await user.click(within(dialog).getByRole("radio", { name: /^Organisation/ }));
    expect(within(dialog).getByRole("radio", { name: /^Organisation/ })).toBeChecked();
    const [url, init] = write(fetchMock, 0);
    expect(url).toBe(`/api/artifacts/${mine.id}`);
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe('{"visibility":"organisation"}');
    expect(init.headers).toMatchObject({ "X-Pictor": "1" });
    // Said inside the dialog: a modal leaves nothing outside it to click.
    expect(await within(dialog).findByRole("status")).toHaveTextContent("Changed to Organisation");
    expect(within(screen.getByRole("banner")).getByRole("img", { name: "Organisation" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Undo" }));
    expect(within(dialog).getByRole("radio", { name: /^Only me/ })).toBeChecked();
    expect(within(screen.getByRole("banner")).getByRole("img", { name: "Only me" })).toBeInTheDocument();
    await waitFor(() => {
      expect(writes(fetchMock)).toHaveLength(2);
    });
    expect(write(fetchMock, 1)[1].body).toBe('{"visibility":"private"}');
    expect(within(dialog).getByRole("status")).toHaveTextContent("Access saved.");
  });

  it("keeps saved choices on Done and offers no misleading Cancel", async () => {
    const user = userEvent.setup();
    patching(mine);
    open(mine);
    const dialog = await openShare(user);
    await user.click(within(dialog).getByRole("radio", { name: /^Organisation/ }));
    await waitFor(() => { expect(within(dialog).getByRole("radio", { name: /^Organisation/ })).toBeChecked(); });
    expect(within(dialog).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    expect(within(screen.getByRole("banner")).getByRole("img", { name: "Organisation" })).toBeInTheDocument();
  });

  it("puts the choice back and says so when the server refuses it", async () => {
    const user = userEvent.setup();
    api((method) => (method === "PATCH" ? json({ error: "forbidden" }, 403) : undefined));
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    open(mine);
    const dialog = await openShare(user);
    await user.click(within(dialog).getByRole("radio", { name: /^Organisation/ }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Couldn't change who can open this");
    expect(within(dialog).getByRole("radio", { name: /^Only me/ })).toBeChecked();
    expect(within(dialog).queryByRole("status")).not.toBeInTheDocument();
  });

  it("lists who has access with their role in the share dialog", async () => {
    const user = userEvent.setup();
    open({ ...mine, visibility: "organisation", editors: ["carol"], organisationEdit: false });
    const dialog = await openShare(user);
    expect(within(dialog).getByRole("heading", { name: "Who can open this" })).toBeVisible();
    const rows = within(dialog).getAllByRole("listitem");
    for (const [i, name, role] of [[0, "alice", "Owner"], [1, "carol", "Can edit"], [2, "Everyone in the organisation", "Can view"]] as const) {
      const row = rows[i];
      if (!row) throw new Error("Missing access row");
      expect(within(row).getByText(name)).toBeVisible();
      expect(within(row).getByText(role)).toBeVisible();
    }
  });

  it("copies the link to this artifact from the share dialog and says so on the button, then goes back to its label", async () => {
    // The button's own timer is held back so the test steps over it instead of waiting for it.
    const held: (() => void)[] = [];
    const real = window.setTimeout.bind(window);
    vi.spyOn(window, "setTimeout").mockImplementation(((fn: () => void, ms?: number) => {
      if (ms === 1600) {
        held.push(fn);
        return 0;
      }
      return real(fn, ms);
    }) as typeof window.setTimeout);
    const user = userEvent.setup();
    open(mine);
    const dialog = await openShare(user);
    const button = within(dialog).getByRole("button", { name: "Copy link" });
    await user.click(button);
    expect(await navigator.clipboard.readText()).toBe(`${location.origin}/a/${mine.id}`);
    expect(button).toHaveTextContent("Copied");
    expect(held).toHaveLength(1);
    act(() => {
      held[0]?.();
    });
    expect(button).toHaveTextContent("Copy link");
  });

  it("copies the link from the title's menu and says so", async () => {
    const user = userEvent.setup();
    open(mine);
    await user.click(screen.getByRole("button", { name: "Sprint report" }));
    await user.click(await screen.findByRole("menuitem", { name: "Copy link" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(await navigator.clipboard.readText()).toBe(`${location.origin}/a/${mine.id}`);
  });

  it("offers versions, copy link and delete from the title, asks once before deleting, and renames for someone who may edit", async () => {
    const user = userEvent.setup();
    const fetchMock = api((method) => (method === "DELETE" ? new Response(null, { status: 204 }) : method === "PUT" ? json({ ...mine, title: "Renamed" }) : undefined));
    open(mine);
    await user.click(screen.getByRole("button", { name: "Sprint report" }));
    expect((await screen.findAllByRole("menuitem")).map((i) => i.textContent)).toEqual(["Rename", "Versions", "Copy link", "Delete…"]);
    await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
    expect(await screen.findByRole("alertdialog", { name: "Delete this artifact?" })).toBeInTheDocument();
    expect(writes(fetchMock)).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Sprint report" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    const field = await screen.findByRole("textbox", { name: "Title" });
    await user.clear(field);
    await user.type(field, "Renamed{Enter}");
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Renamed" })).toBeInTheDocument();
    });
  });

  it("opens in a new tab from the menu", async () => {
    const user = userEvent.setup();
    open(mine);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    const link = await screen.findByRole("menuitem", { name: "Open in new tab" });
    expect(link).toHaveAttribute("href", `/a/${mine.id}?view=full`);
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("asks before deleting, deletes through the API and returns to the overview", async () => {
    const user = userEvent.setup();
    const fetchMock = api((method) => (method === "DELETE" ? new Response(null, { status: 204 }) : undefined));
    open(mine);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete this artifact?" });
    expect(writes(fetchMock)).toEqual([]);

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(writes(fetchMock)).toEqual([]);

    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => {
      expect(pushed).toHaveBeenCalledWith(null, "", "/");
    });
    const [url, init] = write(fetchMock, 0);
    expect([url, init.method]).toEqual([`/api/artifacts/${mine.id}`, "DELETE"]);
  });

  it("keeps the dialog and says so when the delete fails", async () => {
    const user = userEvent.setup();
    api((method) => (method === "DELETE" ? json({ error: "internal error" }, 500) : undefined));
    open(mine);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    expect(await screen.findByText("Couldn't delete this artifact. Try again.")).toBeInTheDocument();
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(pushed).not.toHaveBeenCalled();
  });

  it("keeps the audience out of the menu", async () => {
    const user = userEvent.setup();
    open(mine, { here: `/a/${mine.id}` });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    expect(within(await screen.findByRole("menu")).queryByRole("menuitemradio")).not.toBeInTheDocument();
  });

  it("is the same page for someone who is not signed in, minus everything that is signed-in's", () => {
    open({ ...theirs, visibility: "public" }, { user: "", initials: "" });
    expect(screen.queryByRole("link", { name: "Back to artifacts" })).not.toBeInTheDocument();
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Share" })).toBeInTheDocument();
    expect(screen.getByTitle("Cheat sheet")).toHaveAttribute("src", `/raw/${theirs.id}`);
  });

  it("speaks the language the server picked", async () => {
    const user = userEvent.setup();
    open(mine, {}, "zh-Hant");
    const dialog = await openShare(user);
    expect(within(dialog).getByRole("radio", { name: /^組織內/ })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "複製連結" })).toBeInTheDocument();
  });

  it("answers its artifact's whoami through the bridge, and nobody else's call", async () => {
    api((_method, url) => (url.endsWith("/me") ? json({ username: "alice" }) : undefined));
    open(mine);
    const frame = screen.getByTitle<HTMLIFrameElement>("Sprint report");
    const target = frame.contentWindow;
    if (!target) throw new Error("the iframe has no window");
    const post = vi.spyOn(target, "postMessage").mockImplementation(() => undefined);
    const ask = (source: MessageEventSource | null) => {
      act(() => {
        window.dispatchEvent(new MessageEvent("message", { data: { pictor: "call", id: 1, op: "whoami" }, source }));
      });
    };
    ask(window);
    await new Promise((r) => setTimeout(r, 0));
    expect(post).not.toHaveBeenCalled();
    ask(target);
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith({ pictor: "reply", id: 1, result: { username: "alice" } }, "*");
    });
  });

  it("shows which version it is and offers history and download in the menu only where they apply", async () => {
    const user = userEvent.setup();
    open({ ...mine, version: 4 });
    expect(screen.getByText("v4")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent);
    expect(items.indexOf("Versions")).toBeLessThan(items.indexOf("Open in new tab"));
    expect(items).toContain("HTML (.html)");
    expect(items.indexOf("HTML (.html)")).toBeLessThan(items.indexOf("Delete…"));
  });

  it("gives someone who did not make it no history, and nothing to download when the kind cannot export", async () => {
    const user = userEvent.setup();
    open({ ...theirs, kind: "sheet" }, { user: "carol" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByRole("menuitem", { name: "Open in new tab" });
    expect(screen.queryByRole("menuitem", { name: "Versions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: exportItem })).not.toBeInTheDocument();
  });

  it("offers no download to someone who is not signed in, because the export route needs a session", async () => {
    const user = userEvent.setup();
    open({ ...theirs, visibility: "public" }, { user: "", initials: "" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await screen.findByRole("menuitem", { name: "Open in new tab" });
    expect(screen.queryByRole("menuitem", { name: exportItem })).not.toBeInTheDocument();
  });

  it("downloads in the first file type the artifact exports through the export route, with the header a link cannot send", async () => {
    const user = userEvent.setup();
    const fetchMock = api((method, url) =>
      method === "GET" && url.endsWith("/export/md")
        ? new Response("# hi", { status: 200, headers: { "Content-Disposition": 'attachment; filename="sprint.md"' } })
        : undefined,
    );
    const created = vi.fn(() => "blob:x");
    const revoked = vi.fn();
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: created, revokeObjectURL: revoked }));
    const saved: { href: string; name: string }[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      saved.push({ href: this.href, name: this.download });
    });
    open({ ...mine, kind: "doc" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Markdown (.md)" }));
    await waitFor(() => {
      expect(saved).toEqual([{ href: "blob:x", name: "sprint.md" }]);
    });
    const call = fetchMock.mock.calls.find(([url]) => (url).endsWith("/export/md")) as unknown as [string, RequestInit];
    expect(call[0]).toBe(`/api/artifacts/${mine.id}/export/md`);
    expect(call[1].headers).toMatchObject({ "X-Pictor": "1" });
    // The browser reads the blob after click() returns: the URL must outlive the
    // click and is released by the next download, not right away.
    expect(revoked).not.toHaveBeenCalled();
    created.mockReturnValueOnce("blob:y");
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Markdown (.md)" }));
    await waitFor(() => {
      expect(revoked).toHaveBeenCalledWith("blob:x");
    });
    expect(revoked).not.toHaveBeenCalledWith("blob:y");
  });

  it("lists in the Export menu the formats the kind declares and no others", async () => {
    const user = userEvent.setup();
    for (const [kind, want] of [["doc", ["Markdown (.md)"]], ["html", ["HTML (.html)"]], ["sheet", [".csv", "Markdown (.md)"]]] as const) {
      const view = open({ ...mine, kind }, { exports: { doc: [{ format: "md" }], html: [{ format: "html" }], sheet: [{ format: "csv" }, { format: "md" }] } });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      const offered = screen.getAllByRole("menuitem").map((i) => i.textContent).filter((i) => /\(\.\w+\)$|^\.\w+$/.test(i));
      expect(offered.sort(), kind).toEqual([...want].sort());
      await user.keyboard("{Escape}");
      view.unmount();
    }
  });

  it("saves under the file name the server gave, also when the name is not ASCII", async () => {
    const user = userEvent.setup();
    api((method, url) =>
      method === "GET" && url.endsWith("/export/md")
        ? new Response("# hi", { status: 200, headers: { "Content-Disposition": "attachment; filename*=utf-8''Q3%20%E8%A8%88%E7%95%AB.md" } })
        : undefined,
    );
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:x", revokeObjectURL: vi.fn() }));
    const saved: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      saved.push(this.download);
    });
    open({ ...mine, kind: "doc" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Markdown (.md)" }));
    await waitFor(() => {
      expect(saved).toEqual(["Q3 計畫.md"]);
    });
  });

  describe("PDF, which the browser makes", () => {
    const withPdf = { exports: { doc: [{ format: "md" }, { format: "pdf", browser: true }], html: [{ format: "html" }] } };

    it("prints the design being viewed, including an older version, without downloading it or printing the viewer", async () => {
      const user = userEvent.setup();
      const fetchMock = api((_method, url) => url.endsWith("/versions") ? json([
        { version: 2, title: "Design", author: "alice", createdAt: "2026-10-03T10:00:00Z" },
        { version: 1, title: "Design", author: "alice", createdAt: "2026-10-01T10:00:00Z" },
      ]) : undefined);
      open({ ...mine, kind: "design", version: 2 }, { exports: { design: [{ format: "zip" }, { format: "pdf", browser: true }] } });
      const outerPrint = vi.spyOn(window, "print");
      const printShown = async (src: string) => {
        await user.click(screen.getByRole("button", { name: "More actions" }));
        await user.click(await screen.findByRole("menuitem", { name: "PDF" }));
        const printing = document.querySelector<HTMLIFrameElement>("iframe.print-frame");
        expect(printing).toHaveAttribute("src", src);
        expect(printing).toHaveAttribute("sandbox", frameSandbox);
        if (!printing?.contentWindow) throw new Error("no print frame");
        const asked = vi.spyOn(printing.contentWindow, "postMessage");
        printing.dispatchEvent(new Event("load"));
        expect(asked).toHaveBeenCalledExactlyOnceWith({ pictor: "print" }, "*");
      };
      await printShown(`/raw/${mine.id}`);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
      const panel = await screen.findByRole("complementary", { name: "Versions" });
      await user.click(await within(panel).findByRole("button", { name: /^v1/ }));
      expect(document.querySelector("iframe.print-frame")).not.toBeInTheDocument();
      await printShown(`/raw/${mine.id}?version=1`);
      expect(outerPrint).not.toHaveBeenCalled();
      expect(fetchMock.mock.calls.map(([url]) => url).filter((url) => url.includes("/export/"))).toEqual([]);
    });

    it("asks the page to print itself and never calls the export route", async () => {
      const user = userEvent.setup();
      const fetchMock = api();
      open({ ...theirs, kind: "doc", visibility: "public" }, { ...withPdf, user: "", initials: "" });
      const inFrame = screen.getByTitle<HTMLIFrameElement>("Cheat sheet").contentWindow;
      if (!inFrame) throw new Error("the frame has no window");
      const asked = vi.spyOn(inFrame, "postMessage");
      await user.click(screen.getByRole("button", { name: "More actions" }));
      const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent);
      expect(items.indexOf("Markdown (.md)")).toBeLessThan(items.indexOf("PDF"));
      await user.click(screen.getByRole("menuitem", { name: "PDF" }));
      expect(asked).toHaveBeenCalledWith({ pictor: "print" }, "*");
      expect(fetchMock.mock.calls.map(([url]) => url).filter((url) => url.includes("/export/"))).toEqual([]);
    });

    it("offers a web page as PDF after its HTML and asks the page to print itself", async () => {
      const user = userEvent.setup();
      const fetchMock = api();
      open({ ...mine, kind: "html" }, { exports: { html: [{ format: "html" }, { format: "pdf", browser: true }] } });
      const inFrame = screen.getByTitle<HTMLIFrameElement>(mine.title).contentWindow;
      if (!inFrame) throw new Error("the frame has no window");
      const asked = vi.spyOn(inFrame, "postMessage");
      await user.click(screen.getByRole("button", { name: "More actions" }));
      const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent);
      expect(items.indexOf("HTML (.html)")).toBeLessThan(items.indexOf("PDF"));
      await user.click(screen.getByRole("menuitem", { name: "PDF" }));
      expect(asked).toHaveBeenCalledExactlyOnceWith({ pictor: "print" }, "*");
      expect(fetchMock.mock.calls.map(([url]) => url).filter((url) => url.includes("/export/"))).toEqual([]);
    });

    it("is all that someone who is not signed in is offered, because the others need a session", async () => {
      const user = userEvent.setup();
      open({ ...theirs, kind: "doc", visibility: "public" }, { ...withPdf, user: "", initials: "" });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      expect(screen.getAllByRole("menuitem").map((i) => i.textContent).filter((i) => exportItem.test(i))).toEqual(["PDF"]);
    });
  });

  it("says so when the download fails", async () => {
    const user = userEvent.setup();
    api((method, url) => (method === "GET" && url.includes("/export/") ? new Response("{}", { status: 500 }) : undefined));
    open(mine);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: exportItem }));
    expect(await screen.findByText("Couldn't download this artifact. Try again.")).toBeInTheDocument();
  });

  describe("version history", () => {
    const history = [
      { version: 3, title: "Sprint report", author: "alice", createdAt: "2026-10-03T10:00:00Z", restoredFrom: 1 },
      { version: 2, title: "Sprint report draft", author: "bob-bot", createdAt: "2026-10-02T09:00:00Z" },
      { version: 1, title: "Sprint", author: "alice", createdAt: "2026-10-01T08:00:00Z" },
    ];
    const current = { ...mine, version: 3 };

    function withHistory(over: (method: string, url: string) => Response | undefined = () => undefined) {
      return api((method, url) => (url.endsWith("/versions") ? json(history) : over(method, url)));
    }
    async function openPanel(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
      return screen.findByRole("complementary", { name: "Versions" });
    }
    const frameSrc = () => document.querySelector("iframe")?.getAttribute("src");

    it("lists every version newest first with who saved it, when, and what it was restored from", async () => {
      const user = userEvent.setup();
      withHistory();
      open(current);
      const panel = await openPanel(user);
      const rows = await within(panel).findAllByRole("button", { pressed: false }).then((r) => r.concat(within(panel).getAllByRole("button", { pressed: true })));
      expect(rows.map((r) => r.querySelector(".version-number")?.textContent).sort()).toEqual(["v1", "v2", "v3"]);
      const newest = within(panel).getByRole("button", { pressed: true });
      expect(newest).toHaveTextContent("v3");
      expect(newest).toHaveTextContent("Current");
      expect(newest).toHaveTextContent("Restored from v1");
      expect(newest).toHaveTextContent("alice");
      expect(newest).toHaveTextContent("2 weeks ago");
      expect(within(panel).getByRole("button", { name: /^v2/ })).toHaveTextContent("bob-bot");
    });

    it.each(["en", "zh-Hant"] as const)("names every author of a version, not only the first (%s)", async (lang) => {
      const user = userEvent.setup();
      api((_method, url) => (url.endsWith("/versions") ? json([{ version: 2, title: "Plan", author: "alice", authors: ["alice", "bob"], createdAt: "2026-10-03T10:00:00Z" }, { version: 1, title: "Plan", author: "alice", authors: ["alice"], createdAt: "2026-10-01T08:00:00Z" }]) : undefined));
      open({ ...mine, version: 2 }, {}, lang);
      const words = bootFor(lang).strings;
      const word = (key: string) => words[key] ?? key;
      await user.click(screen.getByRole("button", { name: word("viewer.more") }));
      await user.click(await screen.findByRole("menuitem", { name: word("viewer.versions") }));
      const panel = await screen.findByRole("complementary", { name: word("versions.title") });
      const row = await within(panel).findByRole("button", { name: /^v2/ });
      expect(row).toHaveTextContent("alice");
      expect(row).toHaveTextContent("bob");
      expect(within(panel).getByRole("button", { name: /^v1/ })).not.toHaveTextContent("bob");
    });

    describe("grouping", () => {
      // Alice saved three times in ten minutes after an hour away; bob saved once between.
      const stretches = [
        { version: 5, title: "Plan", author: "alice", createdAt: "2026-10-03T10:30:00Z" },
        { version: 4, title: "Plan", author: "alice", createdAt: "2026-10-03T10:25:00Z" },
        { version: 3, title: "Plan", author: "alice", createdAt: "2026-10-03T10:22:00Z" },
        { version: 2, title: "Plan", author: "bob", createdAt: "2026-10-03T10:21:00Z" },
        { version: 1, title: "Plan", author: "alice", createdAt: "2026-10-03T09:00:00Z" },
      ];
      const openStretches = async (user: ReturnType<typeof userEvent.setup>, lang: "en" | "zh-Hant" = "en") => {
        api((_method, url) => (url.endsWith("/versions") ? json(stretches) : undefined));
        open({ ...mine, version: 5 }, {}, lang);
        const words = bootFor(lang).strings;
        await user.click(screen.getByRole("button", { name: words["viewer.more"] ?? "" }));
        await user.click(await screen.findByRole("menuitem", { name: words["viewer.versions"] ?? "" }));
        return screen.findByRole("complementary", { name: words["versions.title"] ?? "" });
      };

      it("shows the newest of a stretch by the same person as one entry and keeps the others one click away", async () => {
        const user = userEvent.setup();
        const panel = await openStretches(user);
        expect(await within(panel).findByRole("button", { name: /^v5/ })).toBeInTheDocument();
        expect(within(panel).queryByRole("button", { name: /^v4/ })).not.toBeInTheDocument();
        expect(within(panel).queryByRole("button", { name: /^v3/ })).not.toBeInTheDocument();
        // A different person, and the same person long before, are entries of their own.
        expect(within(panel).getByRole("button", { name: /^v2/ })).toBeInTheDocument();
        expect(within(panel).getByRole("button", { name: /^v1/ })).toBeInTheDocument();

        const more = within(panel).getByRole("button", { name: "2 earlier" });
        expect(more).toHaveAttribute("aria-expanded", "false");
        await user.click(more);
        expect(more).toHaveAttribute("aria-expanded", "true");
        expect(within(panel).getByRole("button", { name: /^v4/ })).toBeInTheDocument();

        await user.click(within(panel).getByRole("button", { name: /^v3/ }));
        expect(frameSrc()).toBe(`/raw/${mine.id}?version=3`);
        expect(within(panel).getByText("Viewing v3, read only")).toBeInTheDocument();
        expect(within(panel).getByRole("button", { name: /^v3/ })).toHaveAttribute("aria-pressed", "true");

        await user.click(more);
        expect(within(panel).queryByRole("button", { name: /^v4/ })).not.toBeInTheDocument();
      });

      it("says it in the page's language", async () => {
        const user = userEvent.setup();
        const panel = await openStretches(user, "zh-Hant");
        expect(await within(panel).findByRole("button", { name: "較早的 2 個" })).toBeInTheDocument();
      });
    });

    it.each(["en", "zh-Hant"] as const)("names the agent that made a version as the person's own (%s)", async (lang) => {
      const user = userEvent.setup();
      api((_method, url) => (url.endsWith("/versions") ? json([{ version: 2, title: "Plan", author: "alice", authors: ["alice", "bob"], agents: [{ by: "alice", client: "Claude Code" }], createdAt: "2026-10-03T10:00:00Z" }, { version: 1, title: "Plan", author: "alice", agents: [], createdAt: "2026-10-01T08:00:00Z" }]) : undefined));
      open({ ...mine, version: 2 }, {}, lang);
      const words = bootFor(lang).strings;
      await user.click(screen.getByRole("button", { name: words["viewer.more"] ?? "" }));
      await user.click(await screen.findByRole("menuitem", { name: words["viewer.versions"] ?? "" }));
      const panel = await screen.findByRole("complementary", { name: words["versions.title"] ?? "" });
      const made = lang === "en" ? "alice's Claude Code" : "alice 的 Claude Code";
      expect(await within(panel).findByRole("button", { name: /^v2/ })).toHaveTextContent(`alice, bob, ${made}`);
      expect(within(panel).getByRole("button", { name: /^v1/ })).not.toHaveTextContent("Claude Code");
    });

    it("takes the newest version from the list when the viewer's own artifact is behind: the room cut one while the editor was open", async () => {
      const user = userEvent.setup();
      // The viewer still holds version 1; the list says the room has made version 2.
      withHistory();
      open({ ...mine, version: 1 });
      const panel = await openPanel(user);
      const newest = await within(panel).findByRole("button", { name: /^v3/ });
      expect(newest).toHaveTextContent("Current");
      expect(within(panel).getByRole("button", { name: /^v1/ })).not.toHaveTextContent("Current");
      await user.click(within(panel).getByRole("button", { name: /^v1/ }));
      expect(await within(panel).findByText("Viewing v1, read only")).toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Restore" })).toBeInTheDocument();
    });

    it("shows an older version in the frame, read only, until the newest is picked again or the panel closes", async () => {
      const user = userEvent.setup();
      withHistory();
      open(current);
      const panel = await openPanel(user);
      expect(frameSrc()).toBe(`/raw/${mine.id}`);
      await user.click(await within(panel).findByRole("button", { name: /^v2/ }));
      expect(frameSrc()).toBe(`/raw/${mine.id}?version=2`);
      expect(within(panel).getByText("Viewing v2, read only")).toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: /^v2/ })).toHaveAttribute("aria-pressed", "true");

      await user.click(within(panel).getByRole("button", { name: "Back to the newest" }));
      expect(frameSrc()).toBe(`/raw/${mine.id}`);
      expect(within(panel).queryByText(/read only/)).not.toBeInTheDocument();

      await user.click(within(panel).getByRole("button", { name: /^v1/ }));
      expect(frameSrc()).toBe(`/raw/${mine.id}?version=1`);
      await user.click(within(panel).getByRole("button", { name: "Close" }));
      expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
      expect(frameSrc()).toBe(`/raw/${mine.id}`);
    });

    it("downloads the version being viewed, and the newest again once the newest is shown", async () => {
      const user = userEvent.setup();
      const fetchMock = withHistory((method, url) =>
        method === "GET" && url.includes("/export/html") ? new Response("<p>hi", { status: 200, headers: { "Content-Disposition": 'attachment; filename="Sprint.html"' } }) : undefined,
      );
      vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: vi.fn(() => "blob:x"), revokeObjectURL: vi.fn() }));
      vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
      const exported = () => fetchMock.mock.calls.map(([url]) => url).filter((url) => url.includes("/export/"));
      const download = async () => {
        await user.click(screen.getByRole("button", { name: "More actions" }));
        await user.click(await screen.findByRole("menuitem", { name: "HTML (.html)" }));
      };
      open(current);
      const panel = await openPanel(user);
      await user.click(await within(panel).findByRole("button", { name: /^v2/ }));
      await download();
      await waitFor(() => {
        expect(exported()).toEqual([`/api/artifacts/${mine.id}/export/html?version=2`]);
      });
      await user.click(within(panel).getByRole("button", { name: "Back to the newest" }));
      await download();
      await waitFor(() => {
        expect(exported()).toEqual([`/api/artifacts/${mine.id}/export/html?version=2`, `/api/artifacts/${mine.id}/export/html`]);
      });
    });

    it("makes the frame of an older version read only for the page's shared data, and the newest writable again", async () => {
      const user = userEvent.setup();
      const fetchMock = withHistory();
      open(current);
      const panel = await openPanel(user);
      const ask = async (op: string) => {
        const target = document.querySelector("iframe")?.contentWindow;
        if (!target) throw new Error("no frame");
        const post = vi.spyOn(target, "postMessage").mockImplementation(() => undefined);
        act(() => {
          window.dispatchEvent(new MessageEvent("message", { data: { pictor: "call", id: 9, op, args: { collection: "c", id: "d", data: {} } }, source: target }));
        });
        await waitFor(() => {
          expect(post).toHaveBeenCalled();
        });
        return post.mock.calls[0]?.[0] as { error?: string };
      };
      await user.click(await within(panel).findByRole("button", { name: /^v2/ }));
      expect((await ask("db.set")).error).toBe("viewing an old version; restore it to make changes");
      expect(writes(fetchMock)).toEqual([]);

      await user.click(within(panel).getByRole("button", { name: "Back to the newest" }));
      expect((await ask("db.set")).error).toBeUndefined();
      expect(writes(fetchMock)).toHaveLength(1);
    });

    it("restores after asking, from the version it was opened on, and then shows the new version with a toast", async () => {
      const user = userEvent.setup();
      let saved = false;
      const fetchMock = withHistory((method, url) => {
        if (method === "POST" && url.endsWith("/versions/1/restore")) {
          saved = true;
          return json({ ...current, version: 4, title: "Sprint" });
        }
        return undefined;
      });
      open(current);
      const panel = await openPanel(user);
      await user.click(await within(panel).findByRole("button", { name: /^v1/ }));
      await user.click(within(panel).getByRole("button", { name: "Restore" }));
      const dialog = await screen.findByRole("alertdialog", { name: "Restore v1?" });
      expect(saved).toBe(false);

      await user.click(within(dialog).getByRole("button", { name: "Restore" }));
      expect(await screen.findByText("Restored v1 as v4")).toBeInTheDocument();
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method, init.body]).toEqual([`/api/artifacts/${mine.id}/versions/1/restore`, "POST", '{"baseVersion":3}']);
      expect(screen.getByText("v4", { selector: "p span" })).toBeInTheDocument();
      expect(frameSrc()).toBe(`/raw/${mine.id}`);
      expect(screen.getByTitle("Sprint")).toBeInTheDocument();
      expect(within(panel).queryByText(/read only/)).not.toBeInTheDocument();
    });

    it("keeps the page as it was and says why when someone saved a newer version meanwhile", async () => {
      const user = userEvent.setup();
      withHistory((method) => (method === "POST" ? json({ error: "newer", currentVersion: 4 }, 409) : undefined));
      open(current);
      const panel = await openPanel(user);
      await user.click(await within(panel).findByRole("button", { name: /^v1/ }));
      await user.click(within(panel).getByRole("button", { name: "Restore" }));
      await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }));
      expect(await screen.findByText("A newer version was saved. Reopen this page to see it.")).toBeInTheDocument();
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
      expect(screen.getByText("v3", { selector: "p span" })).toBeInTheDocument();
    });

    it("keeps the dialog and says so when the restore fails for another reason", async () => {
      const user = userEvent.setup();
      withHistory((method) => (method === "POST" ? new Response("{}", { status: 500 }) : undefined));
      open(current);
      const panel = await openPanel(user);
      await user.click(await within(panel).findByRole("button", { name: /^v1/ }));
      await user.click(within(panel).getByRole("button", { name: "Restore" }));
      await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }));
      expect(await screen.findByText("Couldn't restore this version. Try again.")).toBeInTheDocument();
      expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    });

    it("says so when the history cannot be loaded", async () => {
      const user = userEvent.setup();
      api((_method, url) => (url.endsWith("/versions") ? new Response("{}", { status: 500 }) : undefined));
      open(current);
      const panel = await openPanel(user);
      expect(await within(panel).findByText("Couldn't load the versions. Try again.")).toBeInTheDocument();
    });

    it("speaks the language the server picked", async () => {
      const user = userEvent.setup();
      withHistory();
      open(current, {}, "zh-Hant");
      await user.click(screen.getByRole("button", { name: "更多操作" }));
      await user.click(await screen.findByRole("menuitem", { name: "版本紀錄" }));
      const panel = await screen.findByRole("complementary", { name: "版本紀錄" });
      expect(await within(panel).findByText("還原自 v1")).toBeInTheDocument();
    });
  });

  it("shows the newest version when it is opened from an overview that was behind, so a restore is measured against what the frame shows", async () => {
    const user = userEvent.setup();
    const server = { ...mine, version: 3 };
    const fetchMock = api((method, url) => {
      if (method === "GET" && url === `/api/artifacts/${mine.id}`) return json(server);
      if (url.endsWith("/versions")) return json([{ version: 3, title: "Sprint report", author: "alice", createdAt: "2026-10-03T10:00:00Z" }, { version: 1, title: "Sprint report", author: "alice", createdAt: "2026-10-01T08:00:00Z" }]);
      if (method === "POST") return json({ ...server, version: 4 });
      return undefined;
    });
    open({ ...mine, version: 1 });
    expect(await screen.findByText("v3")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
    await user.click(await screen.findByRole("button", { name: /^v1/ }));
    await user.click(screen.getByRole("button", { name: "Restore" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }));
    await waitFor(() => {
      expect((write(fetchMock, 0))[1].body).toBe('{"baseVersion":3}');
    });
  });

  describe("comments", () => {
    const conversation = [
      { id: "c1", author: "bob", body: "The chart is missing October.", createdAt: "2026-10-03T10:00:00Z", unread: true },
      { id: "c2", author: "alice", body: "Added, thanks.", parentId: "c1", createdAt: "2026-10-03T11:00:00Z" },
    ];
    const readable = { ...mine, canComment: true, comments: { count: 2, unread: 1 } };

    function withComments(over: (method: string, url: string, body: unknown) => Response | undefined = () => undefined) {
      return api((method, url, body) => (url.endsWith("/comments") && method === "GET" ? json(conversation) : over(method, url, body)));
    }
    const openPanel = async (user: ReturnType<typeof userEvent.setup>) => {
      await user.click(screen.getByRole("button", { name: /^Comments/ }));
      return screen.findByRole("complementary", { name: "Comments" });
    };

    it("shows when each comment was made as a relative time with the exact time to hover and to assistive technology", async () => {
      const user = userEvent.setup();
      withComments();
      open(readable);
      const panel = await openPanel(user);
      const times = await within(panel).findAllByText(/ago$/);
      expect(times).toHaveLength(2);
      for (const time of times) {
        expect(time.tagName).toBe("TIME");
        await expectHint(time, /^2026-10-03 \d\d:\d\d$/);
        expect(time.getAttribute("aria-label")).toMatch(new RegExp(`^${time.textContent}, 2026-10-03 `));
      }
    });

    it("offers comments only to someone the server says may comment", () => {
      api();
      open({ ...mine, canComment: false });
      expect(screen.queryByRole("button", { name: /^Comments/ })).not.toBeInTheDocument();
    });

    it("tells the author how many new comments there are on the button, and shows the thread with replies under the comment they answer", async () => {
      const user = userEvent.setup();
      withComments();
      open(readable);
      expect(screen.getByRole("button", { name: "Comments, 1 new" })).toHaveTextContent("1");
      const panel = await openPanel(user);
      const articles = await within(panel).findAllByRole("article");
      expect(articles).toHaveLength(2);
      expect(articles[0]).toHaveTextContent("The chart is missing October.");
      expect(articles[0]).toHaveTextContent("New");
      expect(articles[1]).toHaveClass("reply");
      expect(articles[1]).toHaveTextContent("Added, thanks.");
    });

    it("marks what it showed as seen, up to the newest comment, and clears the new count", async () => {
      const user = userEvent.setup();
      const fetchMock = withComments();
      open(readable);
      await openPanel(user);
      await waitFor(() => {
        expect(screen.getByRole("button", { name: "Comments" })).toBeInTheDocument();
      });
      const [url, init] = write(fetchMock, 0);
      expect(url).toBe(`/api/artifacts/${mine.id}/comments/seen`);
      expect(init.body).toBe('{"upTo":"c2"}');
    });

    it("does not mark anything seen for a reader who is not the author", async () => {
      const user = userEvent.setup();
      const fetchMock = withComments();
      open({ ...theirs, canComment: true, comments: { count: 2, unread: 0 } });
      await user.click(screen.getByRole("button", { name: /^Comments/ }));
      await screen.findAllByRole("article");
      expect(writes(fetchMock)).toHaveLength(0);
    });

    it("sends a comment as typed, shows it in the thread and counts it", async () => {
      const user = userEvent.setup();
      const fetchMock = withComments((method, _url, body) =>
        method === "POST" ? json({ id: "c3", author: "alice", body: (body as { body: string }).body, createdAt: "2026-10-04T08:00:00Z" }, 201) : undefined,
      );
      open({ ...readable, comments: { count: 2, unread: 0 } });
      const panel = await openPanel(user);
      await within(panel).findAllByRole("article");
      const send = within(panel).getByRole("button", { name: "Comment" });
      expect(send).toBeDisabled();
      await user.type(within(panel).getByRole("textbox", { name: "Write a comment" }), "  <b>Looks good</b>  ");
      await user.click(send);
      expect(await within(panel).findByText("<b>Looks good</b>")).toBeInTheDocument();
      expect(within(panel).queryByRole("strong")).not.toBeInTheDocument();
      const post = writes(fetchMock).find(([url]) => url.endsWith("/comments"));
      expect(post?.[1].body).toBe('{"body":"<b>Looks good</b>"}');
      expect(screen.getByRole("button", { name: "Comments" })).toHaveTextContent("3");
      expect(within(panel).getByRole("textbox", { name: "Write a comment" })).toHaveValue("");
    });

    it("replies into the thread of the comment it answers, even when answering a reply", async () => {
      const user = userEvent.setup();
      const fetchMock = withComments((method) => (method === "POST" ? json({ id: "c3", author: "alice", body: "ok", parentId: "c1", createdAt: "2026-10-04T08:00:00Z" }, 201) : undefined));
      open({ ...readable, comments: { count: 2, unread: 0 } });
      const panel = await openPanel(user);
      await within(panel).findAllByRole("article");
      await user.click(within(panel).getByRole("button", { name: "Reply" }));
      expect(within(panel).getByText("Replying to bob")).toBeInTheDocument();
      await user.type(within(panel).getByRole("textbox", { name: "Write a comment" }), "ok");
      await user.click(within(panel).getByRole("button", { name: "Comment" }));
      await waitFor(() => {
        expect(writes(fetchMock).find(([url]) => url.endsWith("/comments"))?.[1].body).toBe('{"body":"ok","parentId":"c1"}');
      });
    });

    it("keeps the draft and says so when the comment could not be sent", async () => {
      const user = userEvent.setup();
      withComments((method) => (method === "POST" ? new Response("{}", { status: 500 }) : undefined));
      open({ ...readable, comments: { count: 2, unread: 0 } });
      const panel = await openPanel(user);
      await within(panel).findAllByRole("article");
      await user.type(within(panel).getByRole("textbox", { name: "Write a comment" }), "my draft");
      await user.click(within(panel).getByRole("button", { name: "Comment" }));
      expect(await within(panel).findByText("Couldn't send the comment. Try again.")).toBeInTheDocument();
      expect(within(panel).getByRole("textbox", { name: "Write a comment" })).toHaveValue("my draft");
    });

    it("refuses a comment over the limit before sending it", async () => {
      const user = userEvent.setup();
      const fetchMock = withComments();
      open({ ...readable, comments: { count: 2, unread: 0 } });
      const panel = await openPanel(user);
      await within(panel).findAllByRole("article");
      const box = within(panel).getByRole("textbox", { name: "Write a comment" });
      await user.click(box);
      await user.paste("x".repeat(4001));
      expect(within(panel).getByText("A comment can be at most 4000 characters.")).toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Comment" })).toBeDisabled();
      expect(writes(fetchMock).filter(([url]) => url.endsWith("/comments"))).toHaveLength(0);
    });

    it("says so when the comments cannot be loaded", async () => {
      const user = userEvent.setup();
      api((method, url) => (method === "GET" && url.endsWith("/comments") ? new Response("{}", { status: 500 }) : undefined));
      open(readable);
      const panel = await openPanel(user);
      expect(await within(panel).findByText("Couldn't load the comments. Try again.")).toBeInTheDocument();
    });

    it("shows an empty conversation as an invitation", async () => {
      const user = userEvent.setup();
      api((method, url) => (method === "GET" && url.endsWith("/comments") ? json([]) : undefined));
      open({ ...theirs, canComment: true });
      const panel = await openPanel(user);
      expect(await within(panel).findByText("No comments yet. Start the conversation.")).toBeInTheDocument();
    });

    it("speaks the language the server picked", () => {
      withComments();
      open(readable, {}, "zh-Hant");
      expect(screen.getByRole("button", { name: "留言，1 則新留言" })).toBeInTheDocument();
    });
  });

  describe("files", () => {
    const logo = { name: "logo.png", contentType: "image/png", size: 2048, url: "https://pictor.test/files/x/logo.png", createdBy: "alice", createdAt: "2026-10-03T10:00:00Z" };

    function withFiles(over: (method: string, url: string) => Response | undefined = () => undefined) {
      return api((method, url) => (url.endsWith("/assets") && method === "GET" ? json([logo]) : over(method, url)));
    }
    async function openPanel(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Files" }));
      return screen.findByRole("complementary", { name: "Files" });
    }
    const chooser = (panel: HTMLElement) => within(panel).getByLabelText("Drop files here, or choose files");

    it("shows when a file was uploaded as a relative time with the exact time to hover and to assistive technology", async () => {
      const user = userEvent.setup();
      withFiles();
      open(mine);
      const panel = await openPanel(user);
      const time = await within(panel).findByText("2 weeks ago");
      expect(time.tagName).toBe("TIME");
      await expectHint(time, /^2026-10-03 \d\d:\d\d$/);
      expect(time.getAttribute("aria-label")).toMatch(/^2 weeks ago, 2026-10-03 /);
    });

    it("gives the author a Files item and a reader none", async () => {
      const user = userEvent.setup();
      api();
      const { unmount } = open(mine);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      expect(await screen.findByRole("menuitem", { name: "Files" })).toBeInTheDocument();
      unmount();
      open({ ...theirs, canChange: false }, { user: "carol" });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      expect(screen.queryByRole("menuitem", { name: "Files" })).not.toBeInTheDocument();
    });

    it("lists the files with their size and copies a file's URL", async () => {
      const user = userEvent.setup();
      withFiles();
      open(mine);
      const panel = await openPanel(user);
      expect(await within(panel).findByText("logo.png")).toBeInTheDocument();
      expect(within(panel).getByText("2 KB")).toBeInTheDocument();
      const writeText = vi.fn(() => Promise.resolve());
      vi.spyOn(navigator.clipboard, "writeText").mockImplementation(writeText);
      await user.click(within(panel).getByRole("button", { name: "Copy URL" }));
      expect(writeText).toHaveBeenCalledWith(logo.url);
    });

    it("uploads a chosen file as the request body and lists it", async () => {
      const user = userEvent.setup();
      const fetchMock = withFiles((method) => (method === "PUT" ? json({ ...logo, name: "chart.png", url: "https://pictor.test/files/x/chart.png" }, 201) : undefined));
      open(mine);
      const panel = await openPanel(user);
      const file = new File(["png"], "chart.png", { type: "image/png" });
      await user.upload(chooser(panel), file);
      expect(await within(panel).findByText("chart.png")).toBeInTheDocument();
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method, init.body]).toEqual([`/api/artifacts/${mine.id}/assets/chart.png`, "PUT", file]);
      expect(within(panel).queryByRole("alert")).not.toBeInTheDocument();
    });

    it("shows the server's reason when it refuses a file", async () => {
      const user = userEvent.setup();
      withFiles((method) => (method === "PUT" ? json({ error: "this looks like an executable; allowed: png, jpg" }, 400) : undefined));
      open(mine);
      const panel = await openPanel(user);
      await user.upload(chooser(panel), new File(["MZ"], "tool.png", { type: "image/png" }));
      expect(await within(panel).findByRole("alert")).toHaveTextContent("Couldn't upload tool.png: this looks like an executable; allowed: png, jpg");
      expect(within(panel).queryByText("tool.png", { selector: ".file-name" })).not.toBeInTheDocument();
    });

    it("asks before deleting a file, then removes it", async () => {
      const user = userEvent.setup();
      const fetchMock = withFiles((method) => (method === "DELETE" ? new Response(null, { status: 204 }) : undefined));
      open(mine);
      const panel = await openPanel(user);
      await user.click(await within(panel).findByRole("button", { name: "Delete logo.png" }));
      const dialog = await screen.findByRole("alertdialog");
      expect(writes(fetchMock)).toHaveLength(0);
      await user.click(within(dialog).getByRole("button", { name: "Delete" }));
      await waitFor(() => {
        expect(within(panel).queryByText("logo.png")).not.toBeInTheDocument();
      });
      expect(write(fetchMock, 0)[0]).toBe(`/api/artifacts/${mine.id}/assets/logo.png`);
    });

    it("says so when the files cannot be loaded", async () => {
      const user = userEvent.setup();
      api((method, url) => (method === "GET" && url.endsWith("/assets") ? new Response("{}", { status: 500 }) : undefined));
      open(mine);
      const panel = await openPanel(user);
      expect(await within(panel).findByText("Couldn't load the files. Try again.")).toBeInTheDocument();
    });

    it("speaks the language the server picked", async () => {
      const user = userEvent.setup();
      withFiles();
      open(mine, {}, "zh-Hant");
      await user.click(screen.getByRole("button", { name: "更多操作" }));
      await user.click(await screen.findByRole("menuitem", { name: "檔案" }));
      const panel = await screen.findByRole("complementary", { name: "檔案" });
      expect(within(panel).getByText("把檔案拖到這裡，或選擇檔案")).toBeInTheDocument();
    });
  });

    it("lets an editor open the versions panel", async () => {
      const user = userEvent.setup();
      api((method, url) => (method === "GET" && url.endsWith("/versions") ? json([{ version: 1, title: "T", author: "bob", createdAt: "2026-10-01T08:00:00Z" }]) : undefined));
      open({ ...theirs, canEdit: true, role: "editor" }, { user: "carol" });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
      expect(await screen.findByRole("complementary", { name: "Versions" })).toBeInTheDocument();
    });

  describe("pinning", () => {
    const pinned = { ...theirs, pinned: true };

    it("pins from the menu, shows it at once and sends it to the server", async () => {
      const user = userEvent.setup();
      const fetchMock = api((method, url) => (method === "PUT" && url === `/api/artifacts/${theirs.id}/pin` ? json(pinned) : undefined));
      open(theirs);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Pin" }));
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method]).toEqual([`/api/artifacts/${theirs.id}/pin`, "PUT"]);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      expect(await screen.findByRole("menuitem", { name: "Unpin" })).toBeInTheDocument();
    });

    it("unpins what is pinned", async () => {
      const user = userEvent.setup();
      const fetchMock = api((method, url) => (method === "DELETE" && url === `/api/artifacts/${theirs.id}/pin` ? json(theirs) : undefined));
      open(pinned);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Unpin" }));
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method]).toEqual([`/api/artifacts/${theirs.id}/pin`, "DELETE"]);
    });

    it("takes the pin back and says so when the server refuses", async () => {
      const user = userEvent.setup();
      api((method) => (method === "PUT" ? json({ error: "nope" }, 500) : undefined));
      open(theirs);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Pin" }));
      expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't change the pin");
      await user.click(screen.getByRole("button", { name: "More actions" }));
      expect(await screen.findByRole("menuitem", { name: "Pin" })).toBeInTheDocument();
    });

    it("offers no pin to a visitor who is not signed in", async () => {
      const user = userEvent.setup();
      api();
      open(theirs, { user: "" });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      expect(screen.queryByRole("menuitem", { name: "Pin" })).not.toBeInTheDocument();
    });
  });

  describe("copying", () => {
    const made = artifact({ title: "Cheat sheet (Copy)", author: "alice", visibility: "private", canChange: true, canEdit: true, role: "author", copiedFrom: { id: theirs.id, title: "Cheat sheet" } });

    it("makes your own copy from the menu and lands on it", async () => {
      const user = userEvent.setup();
      const fetchMock = api((method, url) => (method === "POST" && url === `/api/artifacts/${theirs.id}/copy` ? json(made, 201) : undefined));
      open(theirs);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Make a copy" }));
      expect(await screen.findByText("Copied. This is your own private copy.")).toBeInTheDocument();
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method, init.body]).toEqual([`/api/artifacts/${theirs.id}/copy`, "POST", undefined]);
      await waitFor(() => {
        expect(pushed).toHaveBeenCalledWith(null, "", `/a/${made.id}`);
      });
    });

    it("says so and stays where it is when the copy is refused", async () => {
      const user = userEvent.setup();
      api((method) => (method === "POST" ? json({ error: "not found" }, 404) : undefined));
      open(theirs);
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Make a copy" }));
      expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't make a copy");
      expect(pushed).not.toHaveBeenCalled();
    });

    it("offers a copy to someone who is signed in and none to a visitor", async () => {
      const user = userEvent.setup();
      api();
      open(theirs, { user: "" });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await screen.findByRole("menuitem", { name: "Open in new tab" });
      expect(screen.queryByRole("menuitem", { name: "Make a copy" })).not.toBeInTheDocument();
    });

    it("offers no copy for an agent, to the author or to a reader", async () => {
      const user = userEvent.setup();
      api();
      for (const a of [mine, theirs]) {
        const view = open(a);
        await user.click(screen.getByRole("button", { name: "More actions" }));
        await screen.findByRole("menuitem", { name: "Make a copy" });
        expect(screen.queryByRole("menuitem", { name: /agent/i })).not.toBeInTheDocument();
        await user.keyboard("{Escape}");
        view.unmount();
      }
    });

    it("says where a copy came from, with a link while the source can be opened and without one when it cannot", () => {
      const first = open(made);
      const link = screen.getByRole("link", { name: "Copied from Cheat sheet" });
      expect(link).toHaveAttribute("href", `/a/${theirs.id}`);
      first.unmount();
      open({ ...made, copiedFrom: { id: "", title: "Cheat sheet" } });
      expect(screen.getByText("Copied from Cheat sheet")).toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "Copied from Cheat sheet" })).not.toBeInTheDocument();
    });

    it("copies the version of a row in the versions panel", async () => {
      const user = userEvent.setup();
      const rows = [
        { version: 2, title: "Sprint report", author: "alice", createdAt: "2026-10-03T10:00:00Z" },
        { version: 1, title: "Sprint", author: "alice", createdAt: "2026-10-01T08:00:00Z" },
      ];
      const fetchMock = api((method, url) => {
        if (url.endsWith("/versions")) return json(rows);
        return method === "POST" && url.endsWith("/copy") ? json(made, 201) : undefined;
      });
      open({ ...mine, version: 2 });
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
      const panel = await screen.findByRole("complementary", { name: "Versions" });
      const copyButtons = await within(panel).findAllByRole("button", { name: "Make a copy" });
      expect(copyButtons).toHaveLength(2);
      // Newest first: the second row is version 1.
      await user.click(copyButtons.slice(1)[0] ?? panel);
      await waitFor(() => {
        expect(writes(fetchMock)).toHaveLength(1);
      });
      const [url, init] = write(fetchMock, 0);
      expect([url, init.method, init.body]).toEqual([`/api/artifacts/${mine.id}/copy`, "POST", '{"version":1}']);
    });
  });
});
