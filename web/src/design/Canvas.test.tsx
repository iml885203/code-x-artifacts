import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "../viewer/Viewer";
import { DesignCanvas } from "./Canvas";
import type { DesignComments } from "./comments/types";

const frame = (id: string, name: string, w: number, h: number) => ({ id, name, file: `frames/${id}.html`, w, h });
const manifest = {
  version: 1,
  pages: [
    { id: "home", name: "Home", rows: [[frame("hero", "Hero", 1440, 900), frame("mobile", "Mobile", 390, 844)]] },
    { id: "pricing", name: "Pricing", rows: [[frame("plans", "Plans", 1000, 700)]] },
  ],
};

const design = artifact({ title: "Landing", author: "alice", kind: "design", canChange: true, canEdit: true, role: "author" });
const asReader = { ...design, canChange: false, canEdit: false, role: "reader" } satisfies Artifact;

function serve(m: unknown = manifest) {
  const fetchMock = vi.fn((url: string) => Promise.resolve(new Response(JSON.stringify({ version: 1, manifest: m, base: "/raw/x/1.2.sig/", files: "/files/art/g.sig/" }), { status: url.includes("/design") ? 200 : 404 })));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
const urls = (f: ReturnType<typeof serve>) => (f.mock.calls as unknown as [string][]).map(([u]) => u);

function frameBox(id: string): Element {
  const found = document.querySelector(`[data-frame="${id}"]`);
  if (!found) throw new Error(`no frame ${id}`);
  return found;
}
afterEach(() => {
  vi.unstubAllGlobals();
  window.history.replaceState(null, "", "/");
});

async function open(a: Artifact, older: number | null = null, comments?: DesignComments) {
  renderIn(<DesignCanvas artifact={a} older={older} {...(comments ? { comments } : {})} />, "en", `/a/${a.id}`);
  await screen.findByRole("toolbar", { name: "Canvas" });
}

describe("the canvas", () => {
  it("draws the frames of the first page, each named, in a sandboxed frame at the address the server gave", async () => {
    serve();
    await open(design);
    const hero = screen.getByTitle("Hero, 1440 by 900");
    expect(hero).toHaveAttribute("src", "/raw/x/1.2.sig/frames/hero.html");
    expect(hero).toHaveAttribute("sandbox", "allow-scripts");
    expect(screen.getByText("Mobile")).toBeInTheDocument();
    expect(screen.queryByTitle("Plans, 1000 by 700")).not.toBeInTheDocument();
  });

  it("switches page and shows that page's frames", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    await user.click(screen.getByRole("button", { name: "Page: Home" }));
    await user.click(screen.getByRole("menuitemradio", { name: "Pricing" }));
    expect(screen.getByTitle("Plans, 1000 by 700")).toBeInTheDocument();
    expect(screen.queryByTitle("Hero, 1440 by 900")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Page: Pricing" })).toBeInTheDocument();
  });

  it("keeps the one-page dropdown available for copying its page link", async () => {
    serve({ version: 1, pages: [manifest.pages[0]] });
    await open(design);
    expect(screen.getByRole("button", { name: /^Page: / })).toBeInTheDocument();
  });

  it("zooms by the buttons and the keys between 25% and 200%, and Fit and 100% come back", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    const level = () => screen.getByRole("button", { name: /^Zoom \d+%/ }).textContent;
    expect(level()).toBe("37%"); // the page fitted into the 800 x 600 the test has (2166 wide with its margins)
    await user.click(screen.getByRole("button", { name: /^Zoom \d+%/ }));
    await user.click(screen.getByRole("menuitem", { name: "Zoom in" }));
    expect(level()).toBe("50%");
    await user.keyboard("+");
    expect(level()).toBe("75%");
    await user.keyboard("1");
    expect(level()).toBe("100%");
    for (let i = 0; i < 12; i++) await user.keyboard("+");
    expect(level()).toBe("200%");
    for (let i = 0; i < 12; i++) await user.keyboard("-");
    expect(level()).toBe("25%");
    await user.click(screen.getByRole("button", { name: /^Zoom \d+%/ }));
    await user.click(screen.getByRole("menuitem", { name: "Fit" }));
    expect(level()).toBe("37%");
    await user.click(screen.getByRole("button", { name: /^Zoom 37%/ }));
    await user.click(screen.getByRole("menuitem", { name: "100%" }));
    expect(level()).toBe("100%");
  });

  it("leaves canvas shortcuts alone while the page menu has the keyboard", async () => {
    serve();
    await open(design);
    await userEvent.click(screen.getByRole("button", { name: "Page: Home" }));
    await userEvent.keyboard("1h{Escape}");
    expect(screen.getByRole("button", { name: "Zoom 37%" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hand tool" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Page: Home" })).toHaveFocus();
  });

  it("has a hand tool and a select tool, one pressed at a time, and the keys H and V choose them", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    const hand = screen.getByRole("button", { name: "Hand tool" });
    const select = screen.getByRole("button", { name: "Select" });
    expect(select).toHaveAttribute("aria-pressed", "true");
    await user.click(hand);
    expect(hand).toHaveAttribute("aria-pressed", "true");
    expect(select).toHaveAttribute("aria-pressed", "false");
    await user.keyboard("v");
    expect(select).toHaveAttribute("aria-pressed", "true");
  });

  it("acts on the frame the pointer is over, and keeps the frame a keyboard user is on when the pointer moves over another", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    const strip = screen.getByRole("group", { name: "Artboard actions" });
    expect(within(strip).getByRole("button", { name: "Artboard: Hero" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Expand Mobile/ })).toBeNull();
    await user.hover(frameBox("mobile"));
    expect(within(strip).getByRole("button", { name: "Artboard: Mobile" })).toBeInTheDocument();
    await user.hover(frameBox("hero"));
    // Focus on a button of the strip wins over the pointer.
    within(strip).getByRole("button", { name: "Expand Hero" }).focus();
    await user.hover(frameBox("mobile"));
    expect(within(strip).getByRole("button", { name: "Artboard: Hero" })).toBeInTheDocument();
    expect(within(strip).getByRole("button", { name: "Expand Hero" })).toHaveFocus();
  });

  it("has no control in the frames' own labels while the strip acts, and chooses a frame from its menu", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    expect(document.querySelectorAll(".dframe-label button")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Artboard: Hero" }));
    await user.click(screen.getByRole("menuitemradio", { name: /^Mobile/ }));
    expect(screen.getByRole("button", { name: "Artboard: Mobile" })).toBeInTheDocument();
    expect(document.querySelector('[data-frame="mobile"]')).toHaveAttribute("data-active");
  });

  it("plays a frame at its real size, switches between Fit and Fill, and Escape goes back to the canvas", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    // Fitted into the test window the canvas is zoomed out, so Play is in the strip, acting on the frame chosen there.
    await user.click(screen.getByRole("button", { name: "Artboard: Hero" }));
    await user.click(screen.getByRole("menuitemradio", { name: /^Mobile/ }));
    await user.click(screen.getByRole("button", { name: "Expand Mobile" }));
    const playing = screen.getByRole("dialog", { name: "Playing Mobile" });
    const inFrame = within(playing).getByTitle("Mobile");
    expect(inFrame).toHaveAttribute("src", "/raw/x/1.2.sig/frames/mobile.html");
    expect(inFrame).toHaveStyle({ width: "390px", height: "844px" });
    expect(within(playing).getByRole("button", { name: "Fit" })).toHaveAttribute("aria-pressed", "true");
    await user.click(within(playing).getByRole("button", { name: "Fill" }));
    expect(within(playing).getByRole("button", { name: "Fill" })).toHaveAttribute("aria-pressed", "true");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    // The keys of the canvas work again, and focus is back where Play was pressed.
    expect(screen.getByRole("button", { name: "Expand Mobile" })).toHaveFocus();
  });

  it("presents the first frame of the page that is open, and Escape brings focus back to the Present button", async () => {
    serve();
    const user = userEvent.setup();
    await open(design);
    await user.click(screen.getByRole("button", { name: "Page: Home" }));
    await user.click(screen.getByRole("menuitemradio", { name: "Pricing" }));
    await user.click(screen.getByRole("button", { name: "Present" }));
    expect(screen.getByRole("dialog", { name: "Playing Plans" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Present" })).toHaveFocus();
  });

  it("says so when the design cannot be opened", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("{}", { status: 500 }))));
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    renderIn(<DesignCanvas artifact={design} older={null} />, "en", `/a/${design.id}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't open this artifact");
  });
});

describe("who sees what on the canvas", () => {
  const editingControls = () => [
    screen.queryByRole("toolbar", { name: "Tools" }),
    screen.queryByRole("region", { name: "Outline" }),
    screen.queryByRole("region", { name: "Properties" }),
    document.querySelector("[data-handle]"),
    document.querySelector("[data-outline]"),
  ];

  it.each([
    ["reader", asReader, null],
    ["author viewing history", design, 3],
  ] as const)("keeps navigation and inspection but offers no mutation controls to a %s", async (_, artifact, older) => {
    const fetchMock = serve();
    const user = userEvent.setup();
    await open(artifact, older);
    expect(editingControls().every((c) => c === null)).toBe(true);
    for (const name of ["Text", "Artboard", "Drawing", "Shape", "Add artboard"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    await user.click(screen.getByRole("button", { name: "Page: Home" }));
    expect(screen.getAllByRole("menuitemradio").map((item) => item.textContent)).toEqual(["Home", "Pricing"]);
    expect(screen.queryByRole("menuitem", { name: "Rename page" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Add page" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitemradio", { name: "Pricing" }));
    expect(screen.getByTitle("Plans, 1000 by 700")).toBeInTheDocument();
    expect(screen.queryByTitle("Hero, 1440 by 900")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hand tool" }));
    expect(screen.getByRole("button", { name: "Hand tool" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "Select" }));
    expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "Properties" }));
    expect(screen.getByRole("button", { name: "Properties" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: /^Zoom \d+%/ }));
    await user.click(screen.getByRole("menuitem", { name: "100%" }));
    expect(screen.getByRole("button", { name: "Zoom 100%" })).toBeInTheDocument();
    for (const key of ["t", "f", "p", "r"]) {
      await user.keyboard(key);
      expect(document.querySelector(".frame-draw")).toBeNull();
      expect(screen.queryByRole("group", { name: "Draw a pencil stroke" })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute("aria-pressed", "true");
    }
    await user.click(screen.getByRole("button", { name: "Present" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(within(screen.getByRole("dialog")).getByTitle("Plans")).toHaveAttribute("src", "/raw/x/1.2.sig/frames/plans.html");
    if (older !== null) expect(urls(fetchMock)).toContain(`/api/artifacts/${design.id}/design?version=${String(older)}`);
  });
});

describe("the viewer of a design", () => {
  it("shows the canvas instead of one frame of the artifact's page", async () => {
    serve();
    renderIn(<Viewer boot={bootFor("en", { page: "viewer" })} artifact={asReader} />, "en", `/a/${asReader.id}`);
    expect(await screen.findByRole("toolbar", { name: "Canvas" })).toBeInTheDocument();
    expect(document.querySelector("iframe[data-artifact-frame]")).toBeNull();
  });
});

/** A frame of the canvas as the page sees it: what it was asked, and a way to answer as the inspector in it would. */
function frameDouble(title: string) {
  const el = screen.getByTitle(title);
  if (!(el instanceof HTMLIFrameElement) || !el.contentWindow) throw new Error(`${title} is not a frame`);
  const win = el.contentWindow;
  const sent = vi.spyOn(win, "postMessage").mockImplementation(() => undefined);
  const say = (data: unknown, source: Window | null = win) => {
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data, source }));
    });
  };
  const asked = (type: string) => {
    const messages = sent.mock.calls.map(([m]) => m as { id: number; type: string; x?: number; y?: number; pids?: string[] });
    const found = messages.filter((m) => m.type === type).at(-1);
    if (!found) throw new Error(`the frame was never asked ${type}`);
    return found;
  };
  return { win, say, asked, sent };
}

const report = (pid: string, tag: string, over: object = {}) => ({
  pid,
  tag,
  rect: { x: 40, y: 50, w: 300, h: 80 },
  attrs: [
    ["data-pid", pid],
    ["class", "title"],
  ],
  inline: [["font-size", "40px"]],
  computed: { "font-size": "40px", color: "rgb(28, 25, 23)" },
  ...over,
});

const viewport = () => {
  const v = document.querySelector(".canvas-viewport");
  if (!(v instanceof HTMLElement)) throw new Error("no canvas");
  return v;
};

describe("the view when the canvas changes size", () => {
  // jsdom has no layout: the viewport's size is the test's to change, and the observer's callback is the test's to call.
  function resizable() {
    let size = { width: 800, height: 600 };
    let notify: () => void = () => undefined;
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(private cb: () => void) {}
        observe = (target: Element) => {
          if (target.classList.contains("canvas-viewport")) notify = this.cb;
        };
        disconnect = vi.fn();
      },
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return this.classList.contains("canvas-viewport") ? ({ ...size, x: 0, y: 0, top: 0, left: 0, right: size.width, bottom: size.height, toJSON: () => ({}) }) : new DOMRect();
    });
    return (width: number) => {
      size = { width, height: 600 };
      act(() => {
        notify();
      });
    };
  }
  const level = () => screen.getByRole("button", { name: /^Zoom \d+%/ }).textContent;

  it("fits the page again to the room left when nobody has moved the view", async () => {
    serve();
    const resize = resizable();
    await open(design);
    expect(level()).toBe("37%");
    resize(400);
    expect(level()).toBe("18%");
  });

  it("keeps the clicked frame in place when Properties is explicitly opened until Fit is chosen", async () => {
    serve();
    const resize = resizable();
    const user = userEvent.setup();
    await open(asReader);
    const hero = frameDouble("Hero, 1440 by 900");
    hero.say({ pictor: "inspect", type: "ready" });
    const before = document.querySelector(".canvas-world")?.getAttribute("style");
    await user.pointer({ keys: "[MouseLeft]", target: viewport(), coords: { clientX: 200, clientY: 200 } });
    await waitFor(() => { hero.asked("hit"); });
    hero.say({ pictor: "inspect", id: hero.asked("hit").id, type: "hit", pid: "e2" });
    await waitFor(() => { hero.asked("report"); });
    hero.say({ pictor: "inspect", id: hero.asked("report").id, type: "report", items: [report("e2", "h1")] });
    await user.click(screen.getByRole("button", { name: "Properties" }));
    await screen.findByRole("region", { name: "Code" });
    resize(400);
    expect(document.querySelector(".canvas-world")?.getAttribute("style")).toBe(before);
    await user.click(screen.getByRole("button", { name: /^Zoom \d+%/ }));
    await user.click(screen.getByRole("menuitem", { name: "Fit" }));
    expect(level()).toBe("18%");
  });

  it("leaves the view alone once the person has zoomed or panned it", async () => {
    serve();
    const user = userEvent.setup();
    const resize = resizable();
    await open(design);
    await user.click(screen.getByRole("button", { name: /^Zoom \d+%/ }));
    await user.click(screen.getByRole("menuitem", { name: "Zoom in" }));
    const chosen = level();
    resize(400);
    expect(level()).toBe(chosen);
  });
});

describe("selecting an element", () => {
  // The test page is 800 x 600 and fits at 36.9%: the page's origin is at (0, 89.5) on screen, and the Hero frame at (120, 120) on the page.
  const clickHero = async (user: ReturnType<typeof userEvent.setup>, keys = "[MouseLeft]") => {
    await user.pointer({ keys, target: viewport(), coords: { clientX: 200, clientY: 200 } });
  };

  it("asks the frame under the click what is there, in the frame's own pixels, then what it is, and shows it in the Code panel", async () => {
    serve();
    const user = userEvent.setup();
    await open(asReader);
    const hero = frameDouble("Hero, 1440 by 900");
    hero.say({ pictor: "inspect", type: "ready" });
    await clickHero(user);
    await waitFor(() => {
      hero.asked("hit");
    });
    const hit = hero.asked("hit");
    expect(hit.x).toBeCloseTo(200 / 0.369 - 120, -1);
    expect(hit.y).toBeCloseTo((200 - 89.5) / 0.369 - 120, -1);
    hero.say({ pictor: "inspect", id: hit.id, type: "hit", pid: "e2" });
    await waitFor(() => {
      hero.asked("report");
    });
    expect(hero.asked("report").pids).toEqual(["e2"]);
    hero.say({ pictor: "inspect", id: hero.asked("report").id, type: "report", items: [report("e2", "h1")] });
    await user.click(screen.getByRole("button", { name: "Properties" }));
    const panel = await screen.findByRole("region", { name: "Code" });
    expect(within(panel).getByText("<h1>", { exact: false })).toBeInTheDocument();
    // The heading's font size is there twice: as the inline style says it, and as the element ends up with it.
    expect(within(panel).getAllByText("font-size")).toHaveLength(2);
    expect(within(panel).getAllByText("40px")).toHaveLength(2);
    expect(within(panel).getByRole("heading", { name: "Computed" })).toBeInTheDocument();
    // The box is where the frame said the element is, on the frame, in canvas pixels.
    const box = document.querySelector("[data-selection]");
    expect(box).toHaveStyle({ left: "160px", top: "170px", width: "300px", height: "80px" });
  });

  it("adds to the selection with Shift and says how many are selected, and Escape clears it", async () => {
    serve();
    const user = userEvent.setup();
    await open(asReader);
    const hero = frameDouble("Hero, 1440 by 900");
    hero.say({ pictor: "inspect", type: "ready" });
    await clickHero(user);
    await waitFor(() => {
      hero.asked("hit");
    });
    hero.say({ pictor: "inspect", id: hero.asked("hit").id, type: "hit", pid: "e2" });
    await waitFor(() => {
      hero.asked("report");
    });
    hero.say({ pictor: "inspect", id: hero.asked("report").id, type: "report", items: [report("e2", "h1")] });
    await user.click(screen.getByRole("button", { name: "Properties" }));
    await screen.findByRole("region", { name: "Code" });

    // A click with Shift held asks again, and the element it finds joins the selection.
    await user.keyboard("{Shift>}");
    await user.pointer({ keys: "[MouseLeft]", target: viewport(), coords: { clientX: 220, clientY: 220 } });
    await user.keyboard("{/Shift}");
    await waitFor(() => {
      expect(hero.sent.mock.calls.filter(([m]) => (m as { type: string }).type === "hit").length).toBeGreaterThan(1);
    });
    hero.say({ pictor: "inspect", id: hero.asked("hit").id, type: "hit", pid: "e3" });
    await waitFor(() => {
      expect(hero.asked("report").pids).toEqual(["e2", "e3"]);
    });
    hero.say({ pictor: "inspect", id: hero.asked("report").id, type: "report", items: [report("e2", "h1"), report("e3", "p", { rect: { x: 10, y: 10, w: 20, h: 20 } })] });
    expect(await screen.findByText("2 selected, showing the last")).toBeInTheDocument();
    expect(document.querySelectorAll("[data-selection]")).toHaveLength(2);

    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Properties" })).toHaveAttribute("aria-pressed", "true");
    expect(document.querySelectorAll("[data-selection]")).toHaveLength(0);
  });

  it("does not take an answer from a window that is not the frame, and a click on nothing selects nothing", async () => {
    serve();
    const user = userEvent.setup();
    await open(asReader);
    const hero = frameDouble("Hero, 1440 by 900");
    hero.say({ pictor: "inspect", type: "ready" });
    await clickHero(user);
    await waitFor(() => {
      hero.asked("hit");
    });
    hero.say({ pictor: "inspect", id: hero.asked("hit").id, type: "hit", pid: "e2" }, window);
    expect(screen.queryByRole("region", { name: "Code" })).not.toBeInTheDocument();
    expect(hero.sent.mock.calls.filter(([m]) => (m as { type: string }).type === "report")).toHaveLength(0);

    // The corner of the page, off every frame.
    await user.pointer({ keys: "[MouseLeft]", target: viewport(), coords: { clientX: 5, clientY: 5 } });
    expect(screen.queryByRole("region", { name: "Code" })).not.toBeInTheDocument();
  });

  it("selects nothing while the hand tool is held, and a drag pans instead of picking", async () => {
    serve();
    const user = userEvent.setup();
    await open(asReader);
    const hero = frameDouble("Hero, 1440 by 900");
    hero.say({ pictor: "inspect", type: "ready" });
    await user.click(screen.getByRole("button", { name: "Hand tool" }));
    await clickHero(user);
    expect(hero.sent).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Select" }));
    await user.pointer([
      { keys: "[MouseLeft>]", target: viewport(), coords: { clientX: 200, clientY: 200 } },
      { target: viewport(), coords: { clientX: 260, clientY: 230 } },
      { keys: "[/MouseLeft]", target: viewport(), coords: { clientX: 260, clientY: 230 } },
    ]);
    expect(hero.sent).not.toHaveBeenCalled();
  });
});


it("lets a reader comment on selected source only while Properties is closed", async () => {
  serve();
  const onCompose = vi.fn();
  const user = userEvent.setup();
  await open(asReader, null, { threads: [], active: null, onOpen: vi.fn(), onCompose });
  const hero = frameDouble("Hero, 1440 by 900");
  hero.say({ pictor: "inspect", type: "ready" });
  await user.pointer({ keys: "[MouseLeft]", target: viewport(), coords: { clientX: 200, clientY: 200 } });
  await waitFor(() => { hero.asked("hit"); });
  hero.say({ pictor: "inspect", id: hero.asked("hit").id, type: "hit", pid: "e2" });
  await waitFor(() => { hero.asked("report"); });
  hero.say({ pictor: "inspect", id: hero.asked("report").id, type: "report", items: [report("e2", "h1")] });
  await user.click(await screen.findByRole("button", { name: "Comment on selection" }));
  expect(onCompose).toHaveBeenCalledWith(expect.objectContaining({ frameId: "hero", pid: "e2" }));
  await user.click(screen.getByRole("button", { name: "Properties" }));
  expect(screen.queryByRole("button", { name: "Comment on selection" })).toBeNull();
  expect(document.querySelector("[data-selection]")).toHaveAttribute("data-selection", "e2");
});
