import { act, configure, getConfig, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { pollEveryMs } from "./follow";
import { Viewer } from "./Viewer";

// What an open viewer does about someone else publishing: it asks the server
// every few seconds whether its version is still the newest, and shows the new
// one without being asked.

const mine = artifact({ title: "Sprint report", author: "alice", canChange: true, canEdit: true, role: "author", version: 2 });
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

/** The server as the open viewer meets it: where the artifact is now, and what it has been asked. */
function server(start: Artifact) {
  const state = { newest: start, hidden: false, changes: [] as string[], reads: 0, comments: 0, gone: false };
  const fetchMock = vi.fn((url: string) => {
    if (url.startsWith(`/api/artifacts/${start.id}/changes`)) {
      state.changes.push(url);
      if (state.gone) return Promise.resolve(json({ error: "not found" }, 404));
      const since = Number(new URL(url, "http://x").searchParams.get("sinceVersion"));
      const versions = Array.from({ length: Math.max(0, state.newest.version - since) }, (_, i) => ({ version: since + 1 + i, author: "bob", createdAt: "2026-10-20T10:00:00Z" }));
      const comments = Array.from({ length: state.comments }, (_, i) => ({ id: `c${String(i)}`, author: "bob", body: "hi", createdAt: "2026-10-20T10:00:00Z" }));
      return Promise.resolve(json({ id: start.id, version: state.newest.version, title: state.newest.title, updatedAt: "2026-10-20T10:00:00Z", versions, comments, access: [] }));
    }
    if (url === `/api/artifacts/${start.id}`) {
      state.reads++;
      return Promise.resolve(json({ ...state.newest, comments: { count: state.comments, unread: 0 } }));
    }
    if (url.endsWith("/versions")) return Promise.resolve(json([2, 1].map((n) => ({ version: n, title: "Sprint report", author: "alice", createdAt: n === 2 ? "2026-10-20T08:00:00Z" : "2026-10-01T08:00:00Z" }))));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", fetchMock);
  return state;
}

function open(a: Artifact, user = "alice") {
  return renderIn(<Viewer boot={bootFor("en", { page: "viewer", user })} artifact={a} />, "en", `/a/${a.id}`);
}

const tick = (ms = pollEveryMs) => act(() => vi.advanceTimersByTimeAsync(ms));
const frame = () => document.querySelector("iframe");
const frameWindow = (): Window => {
  const w = frame()?.contentWindow;
  if (!w) throw new Error("the viewer has no frame");
  return w;
};
const hide = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};

// Testing Library waits for a real zero-length timer after each user action; under a fake clock that never comes.
const original = getConfig();

beforeEach(() => {
  configure({ asyncWrapper: (callback) => callback() as ReturnType<typeof original.asyncWrapper> });
  vi.useFakeTimers();
  vi.spyOn(history, "pushState").mockImplementation(() => undefined);
});
afterEach(() => {
  configure({ asyncWrapper: (callback) => original.asyncWrapper(callback) });
  vi.useRealTimers();
  vi.unstubAllGlobals();
  hide(false);
});

describe("an open viewer following the newest version", () => {
  it("asks what changed since the version it shows, on a timer, and leaves the frame alone while nothing is newer", async () => {
    const s = server(mine);
    open(mine);
    await tick(pollEveryMs - 1);
    expect(s.changes).toEqual([]);
    const before = frame();
    await tick(1);
    expect(s.changes).toEqual([`/api/artifacts/${mine.id}/changes?sinceVersion=2`]);
    await tick();
    expect(s.changes).toHaveLength(2);
    expect(frame()).toBe(before);
  });

  it("shows another person's new version in a fresh frame, without being asked, and then asks from that version", async () => {
    const s = server(mine);
    open(mine);
    const before = frame();
    s.newest = { ...mine, version: 3, title: "Sprint report, final" };
    await tick();
    expect(frame()).not.toBe(before);
    expect(frame()).toHaveAttribute("src", `/raw/${mine.id}`);
    expect(screen.getByText("v3")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Sprint report, final" })).toBeInTheDocument();
    await tick();
    expect(s.changes.at(-1)).toBe(`/api/artifacts/${mine.id}/changes?sinceVersion=3`);
  });

  it("does not ask while the tab is hidden, and asks at once when it is shown again", async () => {
    const s = server(mine);
    open(mine);
    hide(true);
    await tick(pollEveryMs * 3);
    expect(s.changes).toEqual([]);
    s.newest = { ...mine, version: 3 };
    hide(false);
    await tick(0);
    expect(s.changes).toHaveLength(1);
    expect(screen.getByText("v3")).toBeInTheDocument();
  });

  it("does not reload under someone who is looking at an old version, and catches up when they go back to the newest", async () => {
    const s = server(mine);
    const user = userEvent.setup({ delay: null });
    open(mine);
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Versions" }));
    await tick(0);
    const panel = screen.getByRole("complementary", { name: "Versions" });
    await user.click(within(panel).getByRole("button", { name: /^v1/ }));
    expect(frame()).toHaveAttribute("src", `/raw/${mine.id}?version=1`);

    s.newest = { ...mine, version: 3 };
    s.changes.length = 0;
    await tick(pollEveryMs * 3);
    expect(s.changes).toEqual([]);
    expect(frame()).toHaveAttribute("src", `/raw/${mine.id}?version=1`);

    await user.click(within(panel).getByRole("button", { name: "Back to the newest" }));
    await tick();
    expect(frame()).toHaveAttribute("src", `/raw/${mine.id}`);
    expect(screen.getByText("v3")).toBeInTheDocument();
  });

  it("stops asking once the server says the artifact is gone", async () => {
    const s = server(mine);
    open(mine);
    s.gone = true;
    await tick();
    await tick(pollEveryMs * 3);
    expect(s.changes).toHaveLength(1);
  });

  it("reads the artifact again when a comment arrives, so the badge is right, but does not reload the frame", async () => {
    const s = server(mine);
    open(mine);
    await tick(0);
    const reads = s.reads;
    await tick();
    const before = frame();
    expect(s.reads).toBe(reads);
    s.comments = 1;
    await tick();
    expect(s.reads).toBe(reads + 1);
    expect(frame()).toBe(before);
    await tick();
    expect(s.reads).toBe(reads + 1);
  });

  it("serves a viewer who is not signed in from the changes alone, which tell the new version and its title", async () => {
    const open_ = artifact({ title: "Open page", author: "alice", visibility: "public", version: 2 });
    const s = server(open_);
    open(open_, "");
    const before = frame();
    s.newest = { ...open_, version: 3, title: "Open page, final" };
    await tick();
    expect(frame()).not.toBe(before);
    expect(screen.getByRole("heading", { name: "Open page, final" })).toBeInTheDocument();
    expect(s.reads).toBe(0);
  });
});

describe("keeping the place in the page across a reload", () => {
  const fromFrame = (data: unknown) => {
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data, source: frame()?.contentWindow ?? null }));
    });
  };

  it("puts the new frame back where the old one was scrolled to, once the new page says it is loaded", async () => {
    const s = server(mine);
    open(mine);
    fromFrame({ pictor: "scroll", x: 0, y: 420 });
    s.newest = { ...mine, version: 3 };
    await tick();
    const post = vi.spyOn(frameWindow(), "postMessage");
    fromFrame({ pictor: "ready" });
    expect(post).toHaveBeenCalledWith({ pictor: "scrollTo", x: 0, y: 420 }, "*");
    post.mockClear();
    fromFrame({ pictor: "ready" });
    expect(post).not.toHaveBeenCalled();
  });

  it("does not move a page that was not reloaded by a new version, and keeps the last position that was a number", async () => {
    const s = server(mine);
    open(mine);
    const post = vi.spyOn(frameWindow(), "postMessage");
    fromFrame({ pictor: "scroll", x: 0, y: 420 });
    fromFrame({ pictor: "ready" });
    expect(post).not.toHaveBeenCalled();

    fromFrame({ pictor: "scroll", x: "far", y: Infinity });
    s.newest = { ...mine, version: 3 };
    await tick();
    const next = vi.spyOn(frameWindow(), "postMessage");
    fromFrame({ pictor: "ready" });
    expect(next).toHaveBeenCalledWith({ pictor: "scrollTo", x: 0, y: 420 }, "*");
  });
});
