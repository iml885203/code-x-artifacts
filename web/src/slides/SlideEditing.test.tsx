import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "../viewer/Viewer";
import type { SlideEntry } from "./api";
import type { Slide } from "./fields";
import { SlideFields } from "./SlideFields";
import type { Saved, SlideEdit } from "./useSlideEdit";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

const deckBy = (over: Partial<Artifact> = {}) =>
  artifact({ id: "deck1", title: "Quarter", author: "alice", kind: "slides", version: 3, canEdit: true, canChange: true, role: "author", ...over });

const targets: Slide = {
  title: "Q3 targets",
  notes: "Revenue is behind.",
  elements: [{ type: "table", columns: ["Metric", "Now"], rows: [["Revenue", { text: "$1.1M", tone: "warning" }]] }],
};

interface Call {
  method: string;
  url: string;
  body: unknown;
}

/** The deck on the server: its version, its slides (`slide` first, then `others`), and how a call is answered. */
interface Server {
  version: number;
  slide: Slide;
  others?: Slide[];
  overflows?: boolean;
  /** Answers a save; by default it is saved as the next version. */
  save?: (c: Call) => Response | Promise<Response> | undefined;
  /** Answers a read of the slide list in place of the list. */
  listing?: () => Response | undefined;
}

function serve(s: Server) {
  const calls: Call[] = [];
  const all = () => [s.slide, ...(s.others ?? [])];
  const list = (): { version: number; slides: SlideEntry[] } => ({
    version: s.version,
    slides: all().map((x, i) => ({ number: i + 1, title: String(x.title), ...(s.overflows && i === 0 ? { overflows: true } : {}) })),
  });
  const put = (n: number, slide: Slide) => {
    if (n === 1) s.slide = slide;
    else s.others = (s.others ?? []).map((x, i) => (i === n - 2 ? slide : x));
  };
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const c = { method: init?.method ?? "GET", url, body: typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined };
      calls.push(c);
      const one = /^\/api\/artifacts\/deck1\/slides\/(\d+)$/.exec(url);
      const at = Number(one?.[1] ?? 0);
      if (c.method === "PUT" && one) {
        const decided = s.save?.(c);
        if (decided) return Promise.resolve(decided);
        s.version += 1;
        put(at, (c.body as { slide: Slide }).slide);
        return Promise.resolve(json(list()));
      }
      if (c.method === "POST" && url === "/api/artifacts/deck1/slides") {
        s.version += 1;
        s.others = [...(s.others ?? []), (c.body as { slide: Slide }).slide];
        return Promise.resolve(json(list()));
      }
      if (c.method === "GET" && url === "/api/artifacts/deck1/slides") return Promise.resolve(s.listing?.() ?? json(list()));
      if (c.method === "GET" && one) return Promise.resolve(json({ ...list(), number: at, slide: all()[at - 1] }));
      if (c.method === "GET" && url === "/api/artifacts/deck1") return Promise.resolve(json(deckBy({ version: s.version })));
      return Promise.resolve(json({}));
    }),
  );
  return calls;
}

const saves = (calls: Call[]) => calls.filter((c) => c.method === "PUT");

function open(a: Artifact) {
  return renderIn(<Viewer boot={bootFor("en", { page: "viewer", exports: { slides: [{ format: "pptx" }] } })} artifact={a} />, "en", `/a/${a.id}`);
}

/** The deck's page in the stage says where the strings of slide 1 are. */
function report(...paths: string[]) {
  reportOn(1, ...paths);
}

/** The deck's page in the stage says where the strings of slide `number` are. */
function reportOn(number: number, ...paths: string[]) {
  const stage = screen.getByTitle<HTMLIFrameElement>("Quarter");
  const fields = paths.map((path, i) => ({ path, x: 0.1, y: 0.1 + i * 0.1, w: 0.5, h: 0.08, size: 0.04, line: 0.045, weight: 600, align: "left" }));
  act(() => {
    window.dispatchEvent(new MessageEvent("message", { source: stage.contentWindow, data: { pictor: "fields", number, fields } }));
  });
}

const field = () => screen.getByRole("textbox", { name: "Text on the slide" });
const place = (text: string) => screen.findByRole("button", { name: `Edit “${text}”` });

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime("2026-10-20T12:00:00Z");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("editing what is on a slide", () => {
  it("saves a title and then a table cell with replace_slide, the whole slide and the version it was read at, and Escape changes nothing", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: targets });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("title", "elements/0/rows/0/1");

    await user.click(await place("Q3 targets"));
    expect(field()).toHaveValue("Q3 targets");
    expect(field()).toHaveFocus();
    await user.clear(field());
    await user.type(field(), "Q4 目標{Enter}");
    await waitFor(() => { expect(saves(calls)).toHaveLength(1); });
    expect(saves(calls)[0]).toEqual({ method: "PUT", url: "/api/artifacts/deck1/slides/1", body: { slide: { ...targets, title: "Q4 目標" }, baseVersion: 3 } });
    await waitFor(() => { expect(screen.getByText("v4")).toBeInTheDocument(); });

    report("title", "elements/0/rows/0/1");
    await user.click(await place("$1.1M"));
    await user.clear(field());
    await user.type(field(), "$1.4M{Enter}");
    await waitFor(() => { expect(saves(calls)).toHaveLength(2); });
    expect(saves(calls)[1]?.body).toEqual({
      slide: { ...targets, title: "Q4 目標", elements: [{ type: "table", columns: ["Metric", "Now"], rows: [["Revenue", { text: "$1.4M", tone: "warning" }]] }] },
      baseVersion: 4,
    });

    report("title", "elements/0/rows/0/1");
    await user.click(await place("Q4 目標"));
    await user.type(field(), " nope{Escape}");
    expect(screen.queryByRole("textbox", { name: "Text on the slide" })).toBeNull();
    expect(await place("Q4 目標")).toBeInTheDocument();
    await user.click(await place("$1.4M"));
    await user.click(document.body);
    expect(saves(calls)).toHaveLength(2);
  });

  it("leaves Enter to an input method that is composing, and Shift+Enter breaks a line only in a text element", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: { title: "Hi", elements: [{ type: "text", text: "one" }] } });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("title", "elements/0/text");

    await user.click(await place("Hi"));
    fireEvent.change(field(), { target: { value: "你好" } });
    fireEvent.keyDown(field(), { key: "Enter", isComposing: true });
    fireEvent.keyDown(field(), { key: "Enter", keyCode: 229 });
    fireEvent.keyDown(field(), { key: "Enter", shiftKey: true });
    expect(saves(calls)).toEqual([]);
    expect(field()).toHaveValue("你好");
    await user.keyboard("{Escape}");

    await user.click(await place("one"));
    await user.type(field(), "{Shift>}{Enter}{/Shift}two{Enter}");
    await waitFor(() => { expect(saves(calls)).toHaveLength(1); });
    expect((saves(calls)[0]?.body as { slide: Slide }).slide.elements).toEqual([{ type: "text", text: "one\ntwo" }]);
  });

  it("inserts text, bullets, a table, cards and big numbers at the end of the slide, each with content to type over, and opens the new text to type in", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: { layout: "title", title: "Hi" } });
    open(deckBy());
    const tools = await screen.findByRole("toolbar", { name: "Insert" });
    await waitFor(() => { expect(within(tools).getByRole("button", { name: "Text" })).toBeEnabled(); });

    await user.click(within(tools).getByRole("button", { name: "Text" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(1); });
    report("title", "elements/0/text");
    await waitFor(() => { expect(field()).toHaveValue("New text"); });
    await user.keyboard("{Escape}");

    await user.click(within(tools).getByRole("button", { name: "Bullets" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(2); });
    await user.click(within(tools).getByRole("button", { name: "Table" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(3); });
    await user.click(within(tools).getByRole("button", { name: "Shape" }));
    await user.click(await screen.findByRole("menuitem", { name: "Cards" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(4); });
    await user.click(within(tools).getByRole("button", { name: "Shape" }));
    await user.click(await screen.findByRole("menuitem", { name: "Big numbers" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(5); });

    expect(saves(calls).map((c) => (c.body as { baseVersion: number }).baseVersion)).toEqual([3, 4, 5, 6, 7]);
    expect((saves(calls)[4]?.body as { slide: Slide }).slide).toEqual({
      layout: "title",
      title: "Hi",
      elements: [
        { type: "text", text: "New text" },
        { type: "bullets", items: ["New point"] },
        { type: "table", columns: ["Column 1", "Column 2"], rows: [["", ""], ["", ""]] },
        { type: "cards", items: [{ title: "Card 1", text: "What it is" }, { title: "Card 2", text: "What it is" }] },
        { type: "stats", items: [{ value: "42", label: "What it counts" }] },
      ],
    });
  });

  it("offers no insert on a full slide and says a slide likely to overflow does so", async () => {
    const calls = serve({ version: 3, overflows: true, slide: { title: "Full", elements: Array.from({ length: 8 }, () => ({ type: "text", text: "x" })) } });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    const tools = await screen.findByRole("toolbar", { name: "Insert" });
    await waitFor(() => { expect(within(tools).getAllByRole("button", { name: "This slide holds 8 elements, the most a slide can" })).toHaveLength(3); });
    within(tools).getAllByRole("button", { name: "This slide holds 8 elements, the most a slide can" }).forEach((b) => { expect(b).toBeDisabled(); });
    expect(within(tools).getByRole("button", { name: "Shape" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("This slide is likely to overflow. Shorten it or split it over two slides.");
  });

  it("keeps what was typed when a newer version refused it, shows the newer deck, and applies it to the newer slide when asked", async () => {
    const user = userEvent.setup();
    const s: Server = { version: 3, slide: targets };
    const agents = { ...targets, title: "The agent's title" };
    s.save = (c) => {
      if ((c.body as { baseVersion: number }).baseVersion !== 3) return undefined;
      s.version = 9;
      s.slide = agents;
      return json({ error: "this artifact is now at version 9", currentVersion: 9 }, 409);
    };
    const calls = serve(s);
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("title", "elements/0/rows/0/1");

    await user.click(await place("$1.1M"));
    await user.clear(field());
    await user.type(field(), "$1.4M{Enter}");
    const kept = await screen.findByRole("alert");
    expect(kept).toHaveTextContent("Someone saved a newer version of this deck, so your change was not saved.");
    expect(within(kept).getByText("$1.4M")).toBeInTheDocument();
    await waitFor(() => { expect(screen.getByText("v9")).toBeInTheDocument(); });
    expect(within(screen.getByRole("list", { name: "Slide list" })).getByRole("button", { name: "Slide 1: The agent's title" })).toBeInTheDocument();

    await user.click(within(kept).getByRole("button", { name: "Apply to the newer slide" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(2); });
    expect(saves(calls)[1]?.body).toEqual({
      slide: { ...agents, elements: [{ type: "table", columns: ["Metric", "Now"], rows: [["Revenue", { text: "$1.4M", tone: "warning" }]] }] },
      baseVersion: 9,
    });
    await waitFor(() => { expect(screen.queryByRole("alert")).toBeNull(); });
    await waitFor(() => { expect(screen.getByText("v10")).toBeInTheDocument(); });
  });

  it("undoes and redoes a change of this session, each saved on the version the last one made", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: targets });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    const tools = await screen.findByRole("toolbar", { name: "Insert" });
    expect(within(tools).getByRole("button", { name: "Undo" })).toBeDisabled();
    report("title");
    await user.click(await place("Q3 targets"));
    await user.clear(field());
    await user.type(field(), "Q4{Enter}");
    await waitFor(() => { expect(within(tools).getByRole("button", { name: "Undo" })).toBeEnabled(); });

    await user.click(within(tools).getByRole("button", { name: "Undo" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(2); });
    expect(saves(calls)[1]?.body).toEqual({ slide: targets, baseVersion: 4 });
    await waitFor(() => { expect(within(tools).getByRole("button", { name: "Redo" })).toBeEnabled(); });
    await user.keyboard("{Control>}{Shift>}z{/Shift}{/Control}");
    await waitFor(() => { expect(saves(calls)).toHaveLength(3); });
    expect(saves(calls)[2]?.body).toEqual({ slide: { ...targets, title: "Q4" }, baseVersion: 5 });
    await waitFor(() => { expect(within(tools).getByRole("button", { name: "Undo" })).toBeEnabled(); });
    expect(within(tools).getByRole("button", { name: "Redo" })).toBeDisabled();
  });

  it("gives a reader no field to type in and no insert tool, whatever the page reports", async () => {
    const calls = serve({ version: 3, slide: targets });
    open(deckBy({ author: "bob", canEdit: false, canChange: false, role: "reader", visibility: "organisation" }));
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides")).toBe(true); });
    report("title", "elements/0/rows/0/1");
    expect(screen.queryByRole("button", { name: /^Edit “/ })).toBeNull();
    expect(screen.queryByRole("toolbar", { name: "Insert" })).toBeNull();
    expect(calls.filter((c) => c.url === "/api/artifacts/deck1/slides/1")).toEqual([]);
  });

  it("hears where the strings are only from the stage's own frame, and only places the slide has", async () => {
    const calls = serve({ version: 3, slide: targets });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { source: window, data: { pictor: "fields", number: 1, fields: [{ path: "title", x: 0, y: 0, w: 1, h: 1, size: 0.04, line: 0.05, weight: 400, align: "left" }] } }));
    });
    expect(screen.queryByRole("button", { name: /^Edit “/ })).toBeNull();
    report("notes", "elements/7/text", "title");
    expect(await screen.findAllByRole("button", { name: /^Edit “/ })).toHaveLength(1);
  });
});

describe("editing what is on a slide, when things go another way", () => {
  const keptNote = async () => {
    const note = (await screen.findByText(/what you typed is kept/i)).closest("[role=alert]");
    if (!(note instanceof HTMLElement)) throw new Error("the kept text is not in an alert");
    return note;
  };
  const edited = (rows: unknown[][]): Slide => ({ ...targets, elements: [{ type: "table", columns: ["Metric", "Now"], rows }] });

  async function retitle(text: string, from: string) {
    const user = userEvent.setup();
    report("title");
    await user.click(await place(from));
    await user.clear(field());
    await user.type(field(), `${text}{Enter}`);
  }

  it("undoes and redoes several changes in turn, and offers neither once the deck moved on by another change", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: targets });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    const tools = await screen.findByRole("toolbar", { name: "Insert" });
    const undo = () => within(tools).getByRole("button", { name: "Undo" });
    const redo = () => within(tools).getByRole("button", { name: "Redo" });
    await retitle("A", "Q3 targets");
    await waitFor(() => { expect(screen.getByText("v4")).toBeInTheDocument(); });
    await retitle("B", "A");
    await waitFor(() => { expect(screen.getByText("v5")).toBeInTheDocument(); });

    for (const want of [6, 7]) {
      await waitFor(() => { expect(undo()).toBeEnabled(); });
      await user.click(undo());
      await waitFor(() => { expect(screen.getByText(`v${String(want)}`)).toBeInTheDocument(); });
    }
    expect(undo()).toBeDisabled();
    for (const want of [8, 9]) {
      await waitFor(() => { expect(redo()).toBeEnabled(); });
      await user.click(redo());
      await waitFor(() => { expect(screen.getByText(`v${String(want)}`)).toBeInTheDocument(); });
    }
    expect(saves(calls).map((c) => [(c.body as { slide: Slide }).slide.title, (c.body as { baseVersion: number }).baseVersion])).toEqual([
      ["A", 3], ["B", 4], ["A", 5], ["Q3 targets", 6], ["A", 7], ["B", 8],
    ]);
    expect(redo()).toBeDisabled();
    await waitFor(() => { expect(undo()).toBeEnabled(); });

    await user.click(screen.getByRole("button", { name: "New slide" }));
    await waitFor(() => { expect(screen.getByText("v10")).toBeInTheDocument(); });
    expect(undo()).toBeDisabled();
  });

  it("applies a kept change only where the newer slide still holds the text it replaced, and otherwise keeps it to copy", async () => {
    const user = userEvent.setup();
    const s: Server = { version: 3, slide: targets };
    s.save = () => {
      s.version = 9;
      s.slide = { title: "Moved here", elements: [{ type: "table", columns: ["Metric", "Now"], rows: [["Cost", "$9M"]] }] };
      return json({ error: "this artifact is now at version 9", currentVersion: 9 }, 409);
    };
    const calls = serve(s);
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("elements/0/rows/0/1");
    await user.click(await place("$1.1M"));
    await user.clear(field());
    await user.type(field(), "$1.4M{Enter}");
    const kept = await keptNote();
    await user.click(within(kept).getByRole("button", { name: "Apply to the newer slide" }));
    await waitFor(() => { expect(kept).toHaveTextContent("The newer slide has no place for your change any more."); });
    expect(within(kept).getByText("$1.4M")).toBeInTheDocument();
    expect(within(kept).queryByRole("button", { name: "Apply to the newer slide" })).toBeNull();
    expect(saves(calls)).toHaveLength(1);
  });

  it("keeps what was typed, out of the field, when a newer version refused it and the newer deck could not be read", async () => {
    const user = userEvent.setup();
    const s: Server = { version: 3, slide: targets };
    let refuse = true;
    s.save = () => {
      if (!refuse) return undefined;
      refuse = false;
      s.version = 9;
      return json({ error: "this artifact is now at version 9", currentVersion: 9 }, 409);
    };
    s.listing = () => (refuse ? undefined : json({ error: "down" }, 500));
    const calls = serve(s);
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("elements/0/rows/0/1");
    await user.click(await place("$1.1M"));
    await user.clear(field());
    await user.type(field(), "$1.4M{Enter}");
    const kept = await keptNote();
    expect(within(kept).getByText("$1.4M")).toBeInTheDocument();
    await waitFor(() => { expect(screen.queryByRole("textbox", { name: "Text on the slide" })).toBeNull(); });

    await user.click(within(kept).getByRole("button", { name: "Apply to the newer slide" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(2); });
    expect(saves(calls)[1]?.body).toEqual({ slide: edited([["Revenue", { text: "$1.4M", tone: "warning" }]]), baseVersion: 9 });
  });

  it("offers only the places the server marks, whatever else the page claims", async () => {
    const calls = serve({ version: 3, slide: { title: "Code", elements: [{ type: "code", code: "x()", language: "go" }, { type: "progress", items: [{ label: "Bar", value: 5, text: "half" }] }] } });
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("elements/0/code", "elements/0/language", "elements/0/type", "elements/1/items/0/text", "elements/1/type", "elements/1/items/0/label", "title");
    const offered = await screen.findAllByRole("button", { name: /^Edit “/ });
    expect(offered.map((b) => b.getAttribute("aria-label"))).toEqual(["Edit “Bar”", "Edit “Code”"]);
  });

  it("does not open the new element's field on another slide the person moved to", async () => {
    const user = userEvent.setup();
    const calls = serve({ version: 3, slide: { layout: "title", title: "Hi" }, others: [{ title: "Two", elements: [{ type: "text", text: "second" }] }] });
    open(deckBy());
    const tools = await screen.findByRole("toolbar", { name: "Insert" });
    await waitFor(() => { expect(within(tools).getByRole("button", { name: "Text" })).toBeEnabled(); });
    await user.click(within(tools).getByRole("button", { name: "Text" }));
    await waitFor(() => { expect(saves(calls)).toHaveLength(1); });
    await user.click(within(screen.getByRole("list", { name: "Slide list" })).getByRole("button", { name: "Slide 2: Two" }));
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/2")).toBe(true); });
    reportOn(2, "title", "elements/0/text");
    expect(await screen.findByRole("button", { name: "Edit “second”" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Text on the slide" })).toBeNull();
  });

  it("leaves focus where the person put it while a save is under way", async () => {
    const user = userEvent.setup();
    const s: Server = { version: 3, slide: targets };
    let answer: (r: Response) => void = () => undefined;
    s.save = () => new Promise<Response>((resolve) => { answer = resolve; });
    const calls = serve(s);
    open(deckBy());
    await waitFor(() => { expect(calls.some((c) => c.url === "/api/artifacts/deck1/slides/1")).toBe(true); });
    report("title", "elements/0/rows/0/1");
    await user.click(await place("Q3 targets"));
    await user.type(field(), "!");
    await user.click(document.body);
    await waitFor(() => { expect(saves(calls)).toHaveLength(1); });
    report("title", "elements/0/rows/0/1");
    expect(field()).not.toHaveFocus();
    expect(field()).toHaveAttribute("readonly");
    act(() => { answer(json({ version: 4, slides: [{ number: 1, title: "Q3 targets!" }] })); });
    await waitFor(() => { expect(screen.getByText("v4")).toBeInTheDocument(); });
  });
});

describe("a field on the stage", () => {
  it("stays open with what was typed when the save could not be sent yet, and says to wait", async () => {
    const user = userEvent.setup();
    const stage = document.createElement("iframe");
    document.body.append(stage);
    const save = vi.fn((): Promise<Saved> => Promise.resolve("skipped"));
    const edit: SlideEdit = { slide: targets, busy: false, save, conflict: null, applyConflict: vi.fn(), dropConflict: vi.fn(), canUndo: false, canRedo: false, undo: vi.fn(), redo: vi.fn() };
    renderIn(<SlideFields frame={{ current: stage }} selected={1} edit={edit} size={{ width: 960, height: 540 }} want={null} />);
    const fields = [{ path: "title", x: 0.1, y: 0.1, w: 0.5, h: 0.08, size: 0.04, line: 0.045, weight: 600, align: "left" }];
    act(() => { window.dispatchEvent(new MessageEvent("message", { source: stage.contentWindow, data: { pictor: "fields", number: 1, fields } })); });
    await user.click(await place("Q3 targets"));
    await user.type(field(), " now{Enter}");
    expect(save).toHaveBeenCalledWith({ op: "text", path: "title", text: "Q3 targets now" });
    expect(await screen.findByText("Wait for the last change to be saved, then try again.")).toBeInTheDocument();
    expect(field()).toHaveValue("Q3 targets now");
    stage.remove();
  });
});
