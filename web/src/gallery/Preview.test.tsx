import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Preview } from "./Preview";

const key = "a".repeat(64);
const cached = `/preview/example?w=640&h=400&key=${key}`;
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const frameOf = () => {
  const frame = document.querySelector("iframe");
  if (!frame) throw new Error("missing preview iframe");
  return frame;
};
async function ready(src: string) {
  await waitFor(() => { expect(frameOf()).toHaveAttribute("src", src); });
  return frameOf();
}
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("version-aware library previews", () => {
  it("uses a validated ready snapshot and keeps it hidden until it loads", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ key, version: 1, url: cached }))));
    render(<Preview id="example" version={1} />);
    const frame = await ready(cached);
    expect(frame).toHaveAttribute("sandbox", "allow-scripts");
    expect(frame).toHaveAttribute("loading", "lazy");
    expect(frame.closest(".thumb-screen")).not.toHaveClass("loaded");
    fireEvent.load(frame);
    expect(frame.closest(".thumb-screen")).toHaveClass("loaded");
    expect(document.querySelector('iframe[src^="/raw/"]')).toBeNull();
  });

  it("keeps an already-loaded snapshot visible after unchanged focus revalidation", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ key, version: 1, url: cached }))));
    render(<Preview id="example" version={1} />);
    const frame = await ready(cached);
    fireEvent.load(frame);
    expect(frame.closest(".thumb-screen")).toHaveClass("loaded");
    await act(async () => { fireEvent.focus(window); await Promise.resolve(); });
    expect(frameOf()).toBe(frame);
    expect(frameOf().closest(".thumb-screen")).toHaveClass("loaded");
  });

  it("accepts a pending cold capture after unchanged focus revalidation", async () => {
    const uploads: string[] = [];
    vi.stubGlobal("fetch", vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        if (typeof init.body !== "string") throw new Error("snapshot upload must be JSON");
        uploads.push(init.body);
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(response({ key, version: 1 }));
    }));
    render(<Preview id="example" version={1} />);
    const frame = await ready("/raw/example?preview");
    const target = frame.contentWindow;
    if (!target) throw new Error("missing preview window");
    const sent = vi.spyOn(target, "postMessage").mockImplementation(() => undefined);
    fireEvent.load(frame);
    const message = sent.mock.calls[0]?.[0] as { request: string } | undefined;
    if (!message) throw new Error("missing capture request");
    await act(async () => { fireEvent.focus(window); await Promise.resolve(); });
    expect(frameOf()).toBe(frame);
    expect(frameOf().closest(".thumb-screen")).toHaveClass("loaded");
    const html = "<!doctype html><p>Still ready</p>";
    window.dispatchEvent(new MessageEvent("message", { source: target, origin: "null", data: { pictorPreview: "snapshot", request: message.request, html } }));
    await waitFor(() => { expect(uploads.map((body) => JSON.parse(body) as unknown)).toEqual([{ key, html }]); });
  });

  it("loads the live preview after a miss and revalidates a changed artifact version", async () => {
    let version = 1;
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ key, version, ...(version === 1 ? { url: cached } : {}) }))));
    const view = render(<Preview id="example" version={1} />);
    const previous = await ready(cached);
    version = 2;
    view.rerender(<Preview id="example" version={2} />);
    const current = await ready("/raw/example?preview");
    expect(current).not.toBe(previous);
    expect(current.closest(".thumb-screen")).not.toHaveClass("loaded");
  });

  it.each([401, 403, 404])("does not fall back to private content after an authorization response of %s", async (status) => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ error: "refused" }, status))));
    await act(async () => { render(<Preview id="example" version={1} />); await Promise.resolve(); });
    expect(frameOf()).not.toHaveAttribute("src");
    expect(frameOf().closest(".thumb-screen")).not.toHaveClass("loaded");
  });

  it("keeps a live fallback usable when the cache service fails", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ error: "unavailable" }, 503))));
    render(<Preview id="example" version={1} />);
    expect(await ready("/raw/example?preview")).toHaveAttribute("sandbox", "allow-scripts");
  });

  it("admits only the requested snapshot from its own opaque preview window", async () => {
    const uploads: { key: string; html: string }[] = [];
    vi.stubGlobal("fetch", vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        if (typeof init.body !== "string") throw new Error("snapshot upload must be JSON");
        uploads.push(JSON.parse(init.body) as { key: string; html: string });
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(response({ key, version: 1 }));
    }));
    render(<Preview id="example" version={1} />);
    const frame = await ready("/raw/example?preview");
    const target = frame.contentWindow;
    if (!target) throw new Error("missing preview window");
    const sent = vi.spyOn(target, "postMessage").mockImplementation(() => undefined);
    fireEvent.load(frame);
    const message = sent.mock.calls[0]?.[0] as { request: string } | undefined;
    if (!message) throw new Error("missing capture request");
    const data = { pictorPreview: "snapshot", request: message.request, html: "<!doctype html><p>Ready</p>" };
    const deliver = (source: Window, origin: string, payload: unknown) => window.dispatchEvent(new MessageEvent("message", { source, origin, data: payload }));
    deliver(window, "null", data);
    deliver(target, "https://another.example", data);
    deliver(target, "null", { ...data, request: "a different preview" });
    expect(uploads).toEqual([]);
    deliver(target, "null", data);
    await waitFor(() => { expect(uploads).toEqual([{ key, html: data.html }]); });
    deliver(target, "null", data);
    expect(uploads).toHaveLength(1);
  });

  it("does not upload oversized or no-longer-mounted preview snapshots", async () => {
    const uploads: unknown[] = [];
    vi.stubGlobal("fetch", vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") uploads.push(init.body);
      return Promise.resolve(response({ key, version: 1 }));
    }));
    const view = render(<Preview id="example" version={1} />);
    const frame = await ready("/raw/example?preview");
    const target = frame.contentWindow;
    if (!target) throw new Error("missing preview window");
    const sent = vi.spyOn(target, "postMessage").mockImplementation(() => undefined);
    fireEvent.load(frame);
    const message = sent.mock.calls[0]?.[0] as { request: string } | undefined;
    if (!message) throw new Error("missing capture request");
    const deliver = (html: string) => window.dispatchEvent(new MessageEvent("message", { source: target, origin: "null", data: { pictorPreview: "snapshot", request: message.request, html } }));
    deliver("x".repeat((2 << 20) + 1));
    view.unmount();
    deliver("<p>Late snapshot</p>");
    expect(uploads).toEqual([]);
  });

  it("defers offscreen previews until the card approaches the viewport", async () => {
    let reveal: () => void = () => { throw new Error("observer not attached"); };
    vi.stubGlobal("IntersectionObserver", class {
      constructor(callback: IntersectionObserverCallback) {
        reveal = () => { callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver); };
      }
      observe() { return undefined; }
      disconnect() { return undefined; }
    });
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ key, version: 1 }))));
    await act(async () => { render(<Preview id="example" version={1} />); await Promise.resolve(); });
    expect(frameOf()).not.toHaveAttribute("src");
    act(reveal);
    await ready("/raw/example?preview");
  });
});
