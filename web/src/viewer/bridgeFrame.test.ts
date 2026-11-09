import { describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// The script the server injects into every page.
const bridgeSource = readFileSync(join(import.meta.dirname, "../../../internal/web/bridge.js"), "utf8");

/** The page's side of the bridge, running in a frame of its own. */
function pageWithBridge(size: { w: number; h: number }) {
  const frame = document.createElement("iframe");
  document.body.append(frame);
  const page = frame.contentWindow as Window & typeof globalThis;
  Object.defineProperty(page, "innerWidth", { value: size.w, configurable: true, writable: true });
  Object.defineProperty(page, "innerHeight", { value: size.h, configurable: true, writable: true });
  const scrollTo = vi.fn();
  page.scrollTo = scrollTo;
  (page.eval as (code: string) => void)(bridgeSource);
  const hear = (data: unknown) => page.dispatchEvent(new MessageEvent("message", { data, source: page.parent }));
  return { page, scrollTo, hear };
}

describe("the bridge in a page", () => {
  it("scrolls to the place the viewer sends", () => {
    const { scrollTo, hear } = pageWithBridge({ w: 800, h: 600 });
    hear({ pictor: "scrollTo", x: 0, y: 900 });
    expect(scrollTo).toHaveBeenCalledWith(0, 900);
  });

  it("waits for a frame that has no size yet, and scrolls once it has one", () => {
    // Chrome gives a newly loaded frame its size after the load event: scrolling a page with no size is dropped.
    const { page, scrollTo, hear } = pageWithBridge({ w: 0, h: 0 });
    hear({ pictor: "scrollTo", x: 0, y: 900 });
    expect(scrollTo).not.toHaveBeenCalled();

    Object.defineProperty(page, "innerWidth", { value: 800 });
    Object.defineProperty(page, "innerHeight", { value: 600 });
    page.dispatchEvent(new Event("resize"));
    expect(scrollTo).toHaveBeenCalledExactlyOnceWith(0, 900);
  });

  it("describes a file to save as a call to the viewer, reading a Blob into bytes, and resolves with what the viewer answers", async () => {
    const { page, hear } = pageWithBridge({ w: 800, h: 600 });
    const sent: { id: number; op: string; args: { name: string; type: string; data: unknown } }[] = [];
    vi.spyOn(page.parent, "postMessage").mockImplementation(((m: (typeof sent)[number]) => {
      if (m.op) sent.push(m); // the page also reports "ready" and its scroll to the viewer
    }) as typeof page.parent.postMessage);
    const pictor = (page as unknown as { pictor: unknown }).pictor as { download: (f: unknown) => Promise<unknown> };

    const text = pictor.download({ name: "a.csv", type: "text/csv", data: "x,y" });
    expect(sent[0]).toMatchObject({ op: "download", args: { name: "a.csv", type: "text/csv", data: "x,y" } });
    hear({ pictor: "reply", id: sent[0]?.id, result: { name: "a.csv", size: 3 } });
    await expect(text).resolves.toEqual({ name: "a.csv", size: 3 });

    const blob = pictor.download({ name: "b", data: new page.Blob([new Uint8Array([1, 2, 3])], { type: "application/zip" }) });
    await vi.waitFor(() => {
      expect(sent).toHaveLength(2);
    });
    expect(sent[1]?.args.type).toBe("application/zip");
    // The page's own realm made the buffer, so it is not this test's ArrayBuffer.
    expect(Object.prototype.toString.call(sent[1]?.args.data)).toBe("[object ArrayBuffer]");
    expect(new Uint8Array(sent[1]?.args.data as ArrayBuffer)).toHaveLength(3);
    hear({ pictor: "reply", id: sent[1]?.id, error: "the viewer refused the download" });
    await expect(blob).rejects.toThrow("the viewer refused the download");
  });

  it("cannot save a file when the page is open on its own address, where no viewer can ask", async () => {
    const page = document.createElement("iframe");
    document.body.append(page);
    const win = page.contentWindow as Window & typeof globalThis;
    // On its own address a page is its own parent.
    Object.defineProperty(win, "parent", { value: win });
    (win.eval as (code: string) => void)(bridgeSource);
    await expect(((win as unknown as { pictor: { download: (f: unknown) => Promise<unknown> } }).pictor).download({ name: "a.csv", type: "text/csv", data: "x" })).rejects.toThrow("only when it is open in CodeXArtifacts");
  });
});

describe("the bridge asked to print", () => {
  /**
   * A page with three lazy pictures, none of which has loaded, that counts its print calls. A frame waits the same
   * way (jsdom loads a frame of its own as soon as it is added, so it cannot stand for one that has not loaded).
   */
  function printingPage() {
    const { page, hear } = pageWithBridge({ w: 800, h: 600 });
    const print = vi.fn();
    page.print = print;
    Object.defineProperty(page.document, "fonts", { value: { ready: Promise.resolve() }, configurable: true });
    const parts = ["img", "img", "img"].map((tag) => {
      const el = page.document.createElement(tag);
      el.setAttribute("loading", "lazy");
      // Nothing in jsdom fetches a picture; the browser says one that has not arrived is not complete.
      Object.defineProperty(el, "complete", { value: false });
      page.document.body.append(el);
      return el;
    });
    const loaded = (el: Element) => el.dispatchEvent(new page.Event("load"));
    return { print, parts, hear, loaded };
  }
  // Everything the bridge waits on resolves in microtasks, so a few turns of them let it run as far as it can.
  const settle = async () => {
    for (let turn = 0; turn < 20; turn++) await Promise.resolve();
  };

  it("loads what waits to come near the screen and prints only once every picture and frame has loaded", async () => {
    const { print, parts, hear, loaded } = printingPage();
    hear({ pictor: "print" });
    await settle();
    expect(parts.map((el) => el.getAttribute("loading"))).toEqual(["eager", "eager", "eager"]);
    expect(print).not.toHaveBeenCalled();

    loaded(parts[0] as Element);
    loaded(parts[1] as Element);
    await settle();
    expect(print).not.toHaveBeenCalled();

    loaded(parts[2] as Element);
    await settle();
    expect(print).toHaveBeenCalledOnce();
  });

  it("does not wait for a picture that had loaded before it was asked", async () => {
    const { print, parts, hear, loaded } = printingPage();
    parts.forEach(loaded);
    hear({ pictor: "print" });
    await settle();
    expect(print).toHaveBeenCalledOnce();
  });
});
