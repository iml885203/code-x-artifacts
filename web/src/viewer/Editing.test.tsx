import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { FakeRoom } from "../editor/fakeRoom";
import { para, person, reload, room as seeded } from "../editor/testRoom";
import * as Y from "yjs";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "./Viewer";

// The browser's WebSocket, speaking to a stand-in for the room: the provider's own work with a socket is
// in editor/provider.test.ts and the real server's in e2e.
let room: FakeRoom;
class RoomSocket extends EventTarget {
  binaryType = "arraybuffer";
  private readonly end: ReturnType<FakeRoom["open"]>;
  constructor(url: string) {
    super();
    this.end = room.open(url);
    this.end.onopen = () => this.dispatchEvent(new Event("open"));
    this.end.onmessage = (e) => this.dispatchEvent(new MessageEvent("message", { data: e.data }));
    this.end.onclose = (e) => this.dispatchEvent(Object.assign(new Event("close"), { code: e.code, reason: e.reason }));
  }
  send(data: Uint8Array) {
    this.end.send(data);
  }
  close() {
    this.end.close();
  }
}

const doc = (over: Partial<Artifact> = {}) => artifact({ title: "Plan", author: "alice", kind: "doc", visibility: "organisation", ...over });
const asEditor = { canEdit: true, role: "editor" } as const;

function open(a: Artifact, user = "alice", over: Parameters<typeof bootFor>[1] = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (url.endsWith("/versions")) return Promise.resolve(new Response(JSON.stringify([{ version: 2, title: "Plan", author: "alice", createdAt: "2026-10-03T10:00:00Z" }, { version: 1, title: "Plan", author: "alice", createdAt: "2026-10-01T08:00:00Z" }])));
      if (url.includes("/people")) return Promise.resolve(new Response(JSON.stringify({ people: [{ user: "carol", name: "Carol", canOpen: false }] })));
      if (init?.method === "POST" && url.endsWith("/live")) return Promise.resolve(new Response(JSON.stringify({ ticket: "t1", epoch: "e1", path: `/ws/${a.id}` })));
      return Promise.resolve(new Response(JSON.stringify(a)));
    }),
  );
  return renderIn(<Viewer boot={bootFor("en", { page: "viewer", user, ...over })} artifact={a} />, "en", `/a/${a.id}`);
}

beforeEach(() => {
  room = new FakeRoom();
  vi.stubGlobal("WebSocket", RoomSocket);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

const insertRow = () => screen.queryByRole("toolbar", { name: "Editing tools" });
const document_ = () => screen.findByRole("textbox", { name: "Document" });

describe("opening a doc", () => {
  it("mounts the editor for an editor, with the insert row and no Edit, Done or Watch live button", async () => {
    open(doc(asEditor));
    expect(await document_()).toHaveAttribute("contenteditable", "true");
    expect(insertRow()).toBeInTheDocument();
    expect(screen.queryByTitle("Plan")).not.toBeInTheDocument();
    for (const name of ["Edit", "Done", "Watch live"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    expect(room.urls[0]).toMatch(/^ws:\/\/[^/]+\/ws\/.+\?ticket=t1$/);
  });

  it("shows a reader the same content in the same room, read only, with no insert row and nothing to switch", async () => {
    Y.applyUpdate(room.doc, Y.encodeStateAsUpdate(seeded(para("b_1", "Shared words"))));
    open(doc(), "bob");
    const document = await document_();
    expect(await screen.findByText("Shared words")).toBeInTheDocument();
    expect(document).toHaveAttribute("contenteditable", "false");
    expect(insertRow()).not.toBeInTheDocument();
    expect(screen.queryByRole("toolbar")).not.toBeInTheDocument();
    expect(screen.queryByText("Saved")).not.toBeInTheDocument();
    for (const name of ["Edit", "Done", "Watch live"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
  });

  it("does not mount the editor for a page that is not a doc, nor for someone who is not signed in", () => {
    open(artifact({ title: "Page", author: "alice", kind: "html", ...asEditor }));
    expect(insertRow()).not.toBeInTheDocument();
    expect(screen.getByTitle("Page")).toBeInTheDocument();
  });

  it("says it is connecting, shows no document until the room's state has arrived, and shows connection feedback only while connecting, offline or reconnecting", async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    open(doc(asEditor));
    const fetchNow = vi.mocked(fetch);
    const answer = fetchNow.getMockImplementation();
    fetchNow.mockImplementation(async (url, init) => {
      if (init?.method === "POST" && typeof url === "string" && url.endsWith("/live")) await held;
      return answer ? answer(url, init) : new Response("{}");
    });
    expect(await screen.findByRole("status", { name: "Connecting…" })).toBeVisible();
    expect(screen.queryByRole("textbox", { name: "Document" })).not.toBeInTheDocument();
    release();
    await document_();
    expect(globalThis.document.querySelector(".ed-connection-banner")).toHaveAttribute("hidden");
    expect(globalThis.document.querySelector(".ed-conn-live")).toHaveAttribute("aria-label", "Live");
    window.dispatchEvent(new Event("offline"));
    expect(await screen.findByRole("status", { name: "Offline" })).toBeVisible();
    expect(screen.getByText(/What you type stays here/)).toBeVisible();
    let reconnect: () => void = () => undefined;
    const retry = new Promise<void>((resolve) => { reconnect = resolve; });
    fetchNow.mockImplementation(async (url, init) => {
      if (init?.method === "POST" && typeof url === "string" && url.endsWith("/live")) await retry;
      return answer ? answer(url, init) : new Response("{}");
    });
    window.dispatchEvent(new Event("online"));
    expect(await screen.findByRole("status", { name: "Reconnecting…" })).toBeVisible();
    reconnect();
    await waitFor(() => { expect(globalThis.document.querySelector(".ed[data-connection=live] .ed-connection-banner")).toHaveAttribute("hidden"); });
    expect(room.urls).toHaveLength(2);
    expect(globalThis.document.querySelectorAll(".ed-conn")).toHaveLength(1);
    expect(globalThis.document.querySelector(".ed-status, .ed-comment-button")).toBeNull();
    expect(globalThis.document.querySelector(".ed .ed-banner[role=status]")).toBeNull();
    expect(await document_()).toBeInTheDocument();
  });
});

describe("discovering inline fields in the actual Docs surface", () => {
  it("chooses a real Date from plus, persists it through the room and undoes it once", async () => {
    Y.applyUpdate(room.doc, Y.encodeStateAsUpdate(seeded(para("b_field", "Words"))));
    open(doc(asEditor));
    const editor = await document_();
    editor.focus();
    await userEvent.click(screen.getByRole("button", { name: "Insert block" }));
    await userEvent.type(screen.getByRole("combobox", { name: "Find a block" }), "Date{Enter}");
    const picker = screen.getByRole("form", { name: "Date" });
    expect(editor.querySelector(".ed-date")).toBeNull();
    fireEvent.change(within(picker).getByLabelText("Choose date"), { target: { value: "2026-10-24" } });
    await userEvent.click(within(picker).getByRole("button", { name: "Save" }));
    expect(editor.querySelector(".ed-date")).toHaveAttribute("datetime", "2026-10-24");
    expect(person(reload(room.doc)).dom.querySelector(".ed-date")).toHaveTextContent("2026-10-24");
    await userEvent.click(screen.getByRole("button", { name: "More" }));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(editor.querySelector(".ed-date")).toBeNull();
    expect(editor).toHaveTextContent("Words");
  });

  it("uses slash and the surface's catalog words to insert a real Dropdown and reload its selected value", async () => {
    Y.applyUpdate(room.doc, Y.encodeStateAsUpdate(seeded(para("b_field", "Words"))));
    open(doc(asEditor));
    const editor = await document_();
    editor.focus();
    await userEvent.keyboard("/");
    await userEvent.type(await screen.findByRole("combobox", { name: "Find a block" }), "Dropdown{Enter}");
    const picker = screen.getByRole("dialog", { name: "Dropdown" });
    expect(editor.querySelector(".ed-dropdown")).toBeNull();
    await userEvent.clear(within(picker).getByLabelText("Options 1"));
    await userEvent.type(within(picker).getByLabelText("Options 1"), "Planned");
    await userEvent.clear(within(picker).getByLabelText("Options 2"));
    await userEvent.type(within(picker).getByLabelText("Options 2"), "Done");
    await userEvent.selectOptions(within(picker).getByLabelText("Selected option"), "1");
    await userEvent.click(within(picker).getByRole("button", { name: "Save" }));
    expect(editor.querySelector(".ed-dropdown-trigger")).toHaveTextContent("Done");
    expect(person(reload(room.doc)).dom.querySelector(".ed-dropdown-trigger")).toHaveTextContent("Done");
    expect(editor).not.toHaveTextContent("/Dropdown");
  });
});

describe("quiet document feedback", () => {
  it("tells a read-only viewer when the connection is offline without adding editing tools", async () => {
    open(doc(), "bob");
    const document = await document_();
    window.dispatchEvent(new Event("offline"));
    const banner = await screen.findByRole("status", { name: "Offline" });
    expect(banner).toBeVisible();
    expect(banner).toHaveAttribute("role", "status");
    expect(document).toHaveAttribute("contenteditable", "false");
    expect(insertRow()).not.toBeInTheDocument();
  });
});

describe("mentioning a person", () => {
  it("lists the people the server finds for the @ and tells the author when the one picked cannot open the doc", async () => {
    const user = userEvent.setup();
    open(doc(asEditor));
    (await document_()).focus();
    await user.keyboard("@car");
    await user.click(await screen.findByRole("option", { name: /Carol/ }));
    expect(await screen.findByText("@Carol")).toBeInTheDocument();
    expect(await screen.findByText(/Carol cannot open this doc\./)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText(/cannot open this doc/)).not.toBeInTheDocument();
    expect(screen.getByText("@Carol")).toBeInTheDocument();
  });
});

describe("the saved state beside the title", () => {
  it("follows the room: saved once it is stored, saving while an edit waits, offline when the connection is gone, and nothing for a reader", async () => {
    const user = userEvent.setup();
    open(doc(asEditor));
    const facts = () => within(screen.getByRole("banner")).getByRole("status");
    expect(await within(screen.getByRole("banner")).findByText("Saved")).toBeInTheDocument();

    // The room stops answering, then the person changes something: it is sent and not stored.
    room.silent = true;
    const row = await screen.findByRole("toolbar", { name: "Editing tools" });
    await user.click(within(row).getByRole("button", { name: "Tabs" }));
    await user.click(within(row).getByRole("button", { name: "Add tab" }));
    expect(await within(screen.getByRole("banner")).findByText("Saving…")).toBeInTheDocument();

    window.dispatchEvent(new Event("offline"));
    expect(await within(screen.getByRole("banner")).findByText("Not saved yet")).toBeInTheDocument();
    expect(facts()).toHaveTextContent("Not saved yet");
  });

  it("says nothing to a reader, who has nothing to save", async () => {
    open(doc(), "bob");
    await document_();
    expect(within(screen.getByRole("banner")).queryByText(/Saved|Saving|Offline|Not saved/)).not.toBeInTheDocument();
  });
});

describe("a doc's other pages", () => {
  it("shows an older version in the page's frame, read only and without the row, and the editor again with the newest", async () => {
    const user = userEvent.setup();
    open(doc({ ...asEditor, version: 2 }));
    await document_();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Versions" }));
    const panel = await screen.findByRole("complementary", { name: "Versions" });
    await user.click(await within(panel).findByRole("button", { name: /^v1/ }));
    expect(screen.queryByRole("textbox", { name: "Document" })).not.toBeInTheDocument();
    expect(insertRow()).not.toBeInTheDocument();
    expect(screen.getByTitle("Plan")).toHaveAttribute("src", expect.stringMatching(/\?version=1$/));
    await user.click(within(panel).getByRole("button", { name: "Back to the newest" }));
    expect(await document_()).toHaveAttribute("contenteditable", "true");
    expect(insertRow()).toBeInTheDocument();
  });

  it("prints a doc open in the editor from the page loaded off-screen, as the PDF in the menu asks", async () => {
    const user = userEvent.setup();
    const a = doc(asEditor);
    open(a, "alice", { exports: { doc: [{ format: "pdf", browser: true }] } });
    await document_();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "PDF" }));
    const printing = await waitFor(() => {
      const frame = globalThis.document.querySelector<HTMLIFrameElement>("iframe.print-frame");
      if (!frame) throw new Error("no frame to print from");
      return frame;
    });
    expect(printing).toHaveAttribute("src", `/raw/${a.id}`);
    const target = printing.contentWindow;
    if (!target) throw new Error("the frame has no window");
    const asked = vi.spyOn(target, "postMessage");
    printing.dispatchEvent(new Event("load"));
    expect(asked).toHaveBeenCalledWith({ pictor: "print" }, "*");
  });

  it("still prints when a newer version lands while the page is loading off-screen, and prints the newer one next", async () => {
    const user = userEvent.setup();
    const a = doc({ ...asEditor, version: 2 });
    open(a, "alice", { exports: { doc: [{ format: "pdf", browser: true }] } });
    await document_();
    const printFrame = () => globalThis.document.querySelector<HTMLIFrameElement>("iframe.print-frame");
    const askToPrint = async () => {
      await user.click(screen.getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "PDF" }));
      return waitFor(() => {
        const frame = printFrame();
        if (!frame?.contentWindow) throw new Error("no frame to print from");
        return { frame, asked: vi.spyOn(frame.contentWindow, "postMessage") };
      });
    };
    const first = await askToPrint();
    const earlier = fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (url.startsWith(`/api/artifacts/${a.id}/changes`)) {
          return Promise.resolve(new Response(JSON.stringify({ id: a.id, version: 3, title: a.title, updatedAt: "2026-10-20T10:00:00Z", versions: [{ version: 3, author: "bob", createdAt: "2026-10-20T10:00:00Z" }], comments: [], access: [] })));
        }
        if (url === `/api/artifacts/${a.id}`) return Promise.resolve(new Response(JSON.stringify({ ...a, version: 3 })));
        return earlier(url, init);
      }),
    );
    const hidden = (value: boolean) => {
      Object.defineProperty(globalThis.document, "hidden", { configurable: true, get: () => value });
      globalThis.document.dispatchEvent(new Event("visibilitychange"));
    };
    try {
      hidden(true);
      hidden(false);
      expect(await within(screen.getByRole("banner")).findByText("v3")).toBeInTheDocument();
    } finally {
      hidden(false);
    }
    expect(printFrame()).toBe(first.frame);
    first.frame.dispatchEvent(new Event("load"));
    expect(first.asked).toHaveBeenCalledExactlyOnceWith({ pictor: "print" }, "*");

    const second = await askToPrint();
    expect(second.frame).not.toBe(first.frame);
    expect(second.frame).toHaveAttribute("src", `/raw/${a.id}`);
    expect(first.asked).toHaveBeenCalledOnce();
    second.frame.dispatchEvent(new Event("load"));
    expect(second.asked).toHaveBeenCalledExactlyOnceWith({ pictor: "print" }, "*");
  });
});

describe("the insert row", () => {
  async function openSeeded() {
    Y.applyUpdate(room.doc, Y.encodeStateAsUpdate(seeded(para("b_1", "Plan"))));
    open(doc(asEditor));
    const document = await document_();
    return { document, row: await screen.findByRole("toolbar", { name: "Editing tools" }) };
  }

  it("makes the block a checklist from the row", async () => {
    const { document, row } = await openSeeded();
    await userEvent.click(within(row).getByRole("button", { name: "To-do list" }));
    await waitFor(() => {
      expect(document.querySelector("input.ed-check")).not.toBeNull();
    });
  });

  it("inserts a table from the row", async () => {
    const { document, row } = await openSeeded();
    await userEvent.click(within(row).getByRole("button", { name: "Insert table" }));
    await waitFor(() => {
      expect(document.querySelector("table")).not.toBeNull();
    });
  });

  it("widens the page from the row", async () => {
    const { document, row } = await openSeeded();
    const width = within(row).getByRole("button", { name: "Page width" });
    expect(width).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(width);
    expect(width).toHaveAttribute("aria-pressed", "true");
    expect(document.closest(".ed")).toHaveClass("ed-wide");
    await userEvent.click(width);
    expect(document.closest(".ed")).not.toHaveClass("ed-wide");
  });

  it("lists the tabs in a menu, and adds one that then shows in it", async () => {
    const { row } = await openSeeded();
    await userEvent.click(within(row).getByRole("button", { name: "Tabs" }));
    const menu = within(row).getByRole("navigation", { name: "Tabs" });
    expect(within(menu).getAllByRole("button", { name: "Untitled tab" })).toHaveLength(1);
    await userEvent.click(within(menu).getByRole("button", { name: "Add tab" }));
    await userEvent.click(within(row).getByRole("button", { name: "Tabs" }));
    expect(within(within(row).getByRole("navigation", { name: "Tabs" })).getAllByRole("button", { name: "Untitled tab" })).toHaveLength(2);
  });

  it("opens the picture picker from the row", async () => {
    open(doc(asEditor));
    await document_();
    const row = await screen.findByRole("toolbar", { name: "Editing tools" });
    expect(within(row).getByRole("button", { name: "Image" })).toBeInTheDocument();
    expect(screen.getByTestId("image-picker")).toBeInTheDocument();
  });

  it("closes the menu it opened when the person presses outside it or Escape", async () => {
    const user = userEvent.setup();
    open(doc(asEditor));
    await document_();
    const more = await screen.findByRole("button", { name: "More" });
    await user.click(more);
    expect(screen.getByRole("group", { name: "More" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("group", { name: "More" })).not.toBeInTheDocument();
    await user.click(more);
    await user.click(globalThis.document.body);
    expect(screen.queryByRole("group", { name: "More" })).not.toBeInTheDocument();
  });
});
