import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, frameSandbox, renderIn } from "../test-support";
import { Viewer } from "../viewer/Viewer";
import type { SlideEntry } from "./api";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const entries = (...titles: string[]): SlideEntry[] => titles.map((title, i) => ({ number: i + 1, title }));

const deckBy = (over: Partial<Artifact> = {}) =>
  artifact({ id: "deck1", title: "Quarter", author: "alice", kind: "slides", version: 3, canEdit: true, canChange: true, role: "author", ...over });

interface Call {
  method: string;
  url: string;
  body: unknown;
}

/** The server: the slide list and the artifact as `state` holds them, and `answer` for everything a test decides. */
function server(state: { artifact: Artifact; list: { version: number; slides: SlideEntry[] } }, answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const c = { method: init?.method ?? "GET", url, body: typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined };
      calls.push(c);
      const decided = answer(c);
      if (decided) return Promise.resolve(decided);
      if (c.method === "GET" && url === "/api/artifacts/deck1/slides") return Promise.resolve(json(state.list));
      if (c.method === "GET" && url === "/api/artifacts/deck1") return Promise.resolve(json(state.artifact));
      return Promise.resolve(json({}));
    }),
  );
  return calls;
}

/** What the page asked the slide routes to change. */
const edits = (calls: Call[]) => calls.filter((c) => c.method !== "GET" && c.url.includes("/slides"));

function open(a: Artifact) {
  return renderIn(<Viewer boot={bootFor("en", { page: "viewer", exports: { slides: [{ format: "pdf", browser: true }, { format: "pptx" }] } })} artifact={a} />, "en", `/a/${a.id}`);
}

const filmstrip = () => screen.getByRole("list", { name: "Slide list" });
const thumbnails = () => within(filmstrip()).getAllByRole("button");
const names = () => thumbnails().map((b) => b.getAttribute("aria-label"));
const thumb = (name: string) => within(filmstrip()).getByRole("button", { name });

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("a deck in the viewer", () => {
  it("lists every slide in order, as sandboxed previews of the deck opened at each slide, beside a stage that is the deck's own page", async () => {
    const a = deckBy();
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } });
    open(a);
    await waitFor(() => { expect(names()).toEqual(["Slide 1: Q3", "Slide 2: Two", "Slide 3: Three"]); });
    expect(thumb("Slide 1: Q3")).toHaveAttribute("aria-current", "true");
    const previews = thumbnails().map((b) => b.querySelector("iframe"));
    previews.forEach((f, i) => {
      expect(f).toHaveAttribute("src", `/raw/deck1?preview#slide-${String(i + 1)}`);
      expect(f).toHaveAttribute("sandbox", "allow-scripts");
      expect(f).toHaveAttribute("tabindex", "-1");
    });
    const stage = screen.getByTitle("Quarter");
    expect(stage).toHaveAttribute("src", "/raw/deck1?slide=1&embed");
    expect(stage).toHaveAttribute("sandbox", frameSandbox);
    expect(stage).toHaveAttribute("data-artifact-frame");
  });

  it("adds after the selected slide, duplicates, moves by drag and deletes after asking, each with the version shown, and shows the list each answer brings", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    const calls = server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } }, (c) => {
      if (c.method === "POST" && c.url === "/api/artifacts/deck1/slides" && (c.body as { baseVersion: number }).baseVersion === 3) return json({ version: 4, slides: entries("Q3", "New slide", "Two", "Three") });
      if (c.method === "GET" && c.url === "/api/artifacts/deck1/slides/2") return json({ version: 4, number: 2, slide: { layout: "title", title: "New slide" }, slides: [] });
      if (c.method === "POST" && c.url === "/api/artifacts/deck1/slides") return json({ version: 5, slides: entries("Q3", "New slide", "New slide", "Two", "Three") });
      if (c.method === "POST" && c.url === "/api/artifacts/deck1/slides/1/move") return json({ version: 6, slides: entries("New slide", "New slide", "Q3", "Two", "Three") });
      if (c.method === "DELETE") return json({ version: 7, slides: entries("New slide", "Q3", "Two", "Three") });
      return undefined;
    });
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(3); });

    await user.click(screen.getByRole("button", { name: "New slide" }));
    await waitFor(() => { expect(names()).toEqual(["Slide 1: Q3", "Slide 2: New slide", "Slide 3: Two", "Slide 4: Three"]); });
    expect(edits(calls)[0]).toEqual({ method: "POST", url: "/api/artifacts/deck1/slides", body: { after: 1, slide: { layout: "title", title: "New slide" }, baseVersion: 3 } });
    expect(thumb("Slide 2: New slide")).toHaveAttribute("aria-current", "true");

    await user.click(screen.getByRole("button", { name: "More for slide 2" }));
    await user.click(await screen.findByRole("menuitem", { name: "Duplicate" }));
    await waitFor(() => { expect(names()).toHaveLength(5); });
    expect(edits(calls)[1]).toEqual({ method: "POST", url: "/api/artifacts/deck1/slides", body: { after: 2, slide: { layout: "title", title: "New slide" }, baseVersion: 4 } });

    fireEvent.dragStart(thumb("Slide 1: Q3"), { dataTransfer: { setData: vi.fn(), effectAllowed: "" } });
    fireEvent.dragOver(thumb("Slide 3: New slide"), { clientX: 10 });
    fireEvent.drop(thumb("Slide 3: New slide"));
    await waitFor(() => { expect(names()[2]).toBe("Slide 3: Q3"); });
    expect(edits(calls)[2]).toEqual({ method: "POST", url: "/api/artifacts/deck1/slides/1/move", body: { to: 3, baseVersion: 5 } });
    expect(thumb("Slide 3: Q3")).toHaveAttribute("aria-current", "true");

    thumb("Slide 3: Q3").focus();
    await user.keyboard("{ArrowLeft}");
    expect(thumb("Slide 2: New slide")).toHaveFocus();
    await user.keyboard("{Delete}");
    const ask = await screen.findByRole("alertdialog", { name: "Delete slide 2?" });
    expect(edits(calls)).toHaveLength(3);
    await user.click(within(ask).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(names()).toEqual(["Slide 1: New slide", "Slide 2: Q3", "Slide 3: Two", "Slide 4: Three"]); });
    expect(edits(calls)[3]).toEqual({ method: "DELETE", url: "/api/artifacts/deck1/slides/2?baseVersion=6", body: undefined });
    await waitFor(() => { expect(screen.getByTitle("Quarter")).toHaveAttribute("src", "/raw/deck1?slide=2&embed"); });
  });

  it("shows the newer deck and says so when a newer version was saved meanwhile, and changes nothing", async () => {
    const user = userEvent.setup();
    const state = { artifact: deckBy(), list: { version: 3, slides: entries("Q3", "Two", "Three") } };
    const calls = server(state, (c) => (c.method === "DELETE" ? json({ error: "this artifact is now at version 9", currentVersion: 9 }, 409) : undefined));
    open(state.artifact);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    state.list = { version: 9, slides: entries("Agent's", "Q3") };
    state.artifact = deckBy({ version: 9 });

    thumb("Slide 1: Q3").focus();
    await user.keyboard("{Delete}");
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));

    await waitFor(() => { expect(names()).toEqual(["Slide 1: Agent's", "Slide 2: Q3"]); });
    expect(await screen.findByText(/Someone saved a newer version of this deck, so your change was not made/)).toBeInTheDocument();
    expect(edits(calls)).toEqual([{ method: "DELETE", url: "/api/artifacts/deck1/slides/1?baseVersion=3", body: undefined }]);
    await waitFor(() => { expect(screen.getByText("v9")).toBeInTheDocument(); });
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("gives a reader the filmstrip, the views and zoom to move around in, and nothing that edits", async () => {
    const user = userEvent.setup();
    const a = deckBy({ author: "bob", canEdit: false, canChange: false, role: "reader", visibility: "organisation" });
    const calls = server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } });
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    expect(screen.queryByRole("button", { name: "New slide" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^More for slide/ })).toBeNull();
    thumbnails().forEach((b) => { expect(b).not.toHaveAttribute("draggable", "true"); });

    const stage = screen.getByTitle<HTMLIFrameElement>("Quarter");
    const inside = stage.contentWindow;
    if (!inside) throw new Error("the stage has no window");
    const post = vi.spyOn(inside, "postMessage");
    await user.click(thumb("Slide 2: Two"));
    await user.keyboard("{ArrowRight}");
    expect(thumb("Slide 3: Three")).toHaveFocus();
    expect(post).toHaveBeenLastCalledWith({ pictor: "slides", show: 3 }, "*");
    await user.keyboard("{Delete}");
    fireEvent.dragStart(thumb("Slide 1: Q3"));
    fireEvent.drop(thumb("Slide 3: Three"));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(edits(calls)).toEqual([]);

    await user.click(screen.getByRole("button", { name: /^Zoom \d+%$/ }));
    await user.click(await screen.findByRole("menuitem", { name: "100%" }));
    expect(stage.parentElement).toHaveStyle({ width: "1920px", height: "1080px" });
    expect(screen.getByRole("button", { name: "Zoom 100%" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Every slide in a grid" }));
    expect(screen.getByRole("button", { name: "Every slide in a grid" })).toHaveAttribute("aria-pressed", "true");
    expect(names()).toEqual(["Slide 1: Q3", "Slide 2: Two", "Slide 3: Three"]);
  });

  it("follows the slide the deck's own page shows, and only from its own frame", async () => {
    const a = deckBy();
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } });
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    const stage = screen.getByTitle<HTMLIFrameElement>("Quarter");
    act(() => { window.dispatchEvent(new MessageEvent("message", { source: window, data: { pictor: "slide", number: 3 } })); });
    expect(thumb("Slide 1: Q3")).toHaveAttribute("aria-current", "true");
    act(() => { window.dispatchEvent(new MessageEvent("message", { source: stage.contentWindow, data: { pictor: "slide", number: 1 } })); });
    act(() => { window.dispatchEvent(new MessageEvent("message", { source: stage.contentWindow, data: { pictor: "slide", number: 3 } })); });
    expect(thumb("Slide 3: Three")).toHaveAttribute("aria-current", "true");
  });

  it("keeps the newest selection when the page answers an earlier one late, and keeps a slide number the page reports inside the deck", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } });
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    const stage = screen.getByTitle<HTMLIFrameElement>("Quarter");
    const says = (number: number) => {
      act(() => { window.dispatchEvent(new MessageEvent("message", { source: stage.contentWindow, data: { pictor: "slide", number } })); });
    };
    says(1);
    await user.click(thumb("Slide 2: Two"));
    await user.click(thumb("Slide 3: Three"));
    says(2);
    expect(thumb("Slide 3: Three")).toHaveAttribute("aria-current", "true");
    says(3);
    says(1);
    expect(thumb("Slide 1: Q3")).toHaveAttribute("aria-current", "true");
    says(9);
    expect(thumb("Slide 3: Three")).toHaveAttribute("aria-current", "true");
  });

  it("lets someone who is not signed in move between the slides of a public deck", async () => {
    const user = userEvent.setup();
    const a = deckBy({ author: "bob", canEdit: false, canChange: false, canComment: false, role: "reader", visibility: "public" });
    const calls = server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } });
    renderIn(<Viewer boot={bootFor("en", { page: "viewer", user: "", initials: "" })} artifact={a} />, "en", `/a/${a.id}`);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    const stage = screen.getByTitle<HTMLIFrameElement>("Quarter");
    const inside = stage.contentWindow;
    if (!inside) throw new Error("the stage has no window");
    const post = vi.spyOn(inside, "postMessage");
    await user.click(thumb("Slide 2: Two"));
    expect(post).toHaveBeenLastCalledWith({ pictor: "slides", show: 2 }, "*");
    expect(screen.queryByRole("button", { name: "New slide" })).toBeNull();
    expect(edits(calls)).toEqual([]);
  });

  it("says so when the slides cannot be read, and reads them again when asked", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    let fail = true;
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two") } }, (c) => (fail && c.url === "/api/artifacts/deck1/slides" ? json({ error: "boom" }, 500) : undefined));
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    open(a);
    expect(await screen.findByText("The slide list of this deck could not be read.")).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => { expect(names()).toHaveLength(2); });
    expect(screen.queryByText("The slide list of this deck could not be read.")).toBeNull();
  });

  it("deletes the slide at the version the person was asked about, so a newer deck meanwhile is refused rather than losing another slide", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    let answerAdd: (r: Response) => void = () => undefined;
    const calls = server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two", "Three") } }, (c) => {
      if (c.method === "POST") return new Promise<Response>((done) => { answerAdd = done; });
      if (c.method === "DELETE") return json({ error: "stale" }, 409);
      return undefined;
    });
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(3); });
    await user.click(thumb("Slide 2: Two"));
    await user.click(screen.getByRole("button", { name: "New slide" }));
    thumb("Slide 2: Two").focus();
    await user.keyboard("{Alt>}{ArrowRight}{/Alt}");
    expect(await screen.findByText("Wait for the last change to be saved, then try again.")).toBeInTheDocument();
    await user.keyboard("{Delete}");
    const ask = await screen.findByRole("alertdialog", { name: "Delete slide 2?" });
    act(() => { answerAdd(json({ version: 4, slides: entries("Q3", "Two", "New slide", "Three") })); });
    await waitFor(() => { expect(names()).toHaveLength(4); });
    await user.click(within(ask).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(edits(calls).map((c) => `${c.method} ${c.url}`)).toEqual(["POST /api/artifacts/deck1/slides", "DELETE /api/artifacts/deck1/slides/2?baseVersion=3"]); });
  });

  it("says a newer version was saved even when it cannot be read", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    let lists = 0;
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two") } }, (c) => {
      if (c.method === "DELETE") return json({ error: "stale" }, 409);
      if (c.url === "/api/artifacts/deck1/slides" && ++lists > 1) return json({ error: "down" }, 503);
      return undefined;
    });
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(2); });
    thumb("Slide 1: Q3").focus();
    await user.keyboard("{Delete}");
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    expect(await screen.findByText("Someone saved a newer version of this deck, so your change was not made. Reload the page to see it.")).toBeInTheDocument();
    expect(names()).toEqual(["Slide 1: Q3", "Slide 2: Two"]);
  });

  it("shows the selected slide's speaker notes in the side panel", async () => {
    const user = userEvent.setup();
    const a = deckBy();
    server({ artifact: a, list: { version: 3, slides: entries("Q3", "Two") } }, (c) =>
      c.method === "GET" && c.url === "/api/artifacts/deck1/slides/2" ? json({ version: 3, number: 2, slide: { title: "Two", notes: "Say <b>two</b>" }, slides: [] }) : undefined,
    );
    open(a);
    await waitFor(() => { expect(names()).toHaveLength(2); });
    await user.click(thumb("Slide 2: Two"));
    await user.click(screen.getByRole("button", { name: "Speaker notes" }));
    const panel = screen.getByRole("complementary", { name: "Speaker notes" });
    expect(await within(panel).findByText("Say <b>two</b>")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Speaker notes" })).toHaveAttribute("aria-pressed", "true");
  });
});
