import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FrameGoneError, FrameLinks } from "./inspect";

const frameElement = () => {
  const el = document.createElement("iframe");
  document.body.append(el);
  const win = el.contentWindow;
  if (!win) throw new Error("jsdom gave the frame no window");
  return { el, win, sent: vi.spyOn(win, "postMessage").mockImplementation(() => undefined) };
};
const say = (source: Window | null, data: unknown) => {
  window.dispatchEvent(new MessageEvent("message", { data, source }));
};
const settles = (p: Promise<unknown>) => {
  const state = { done: false };
  void p.then(
    () => (state.done = true),
    () => (state.done = true),
  );
  return state;
};
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

let links: FrameLinks;
const hear = (e: MessageEvent) => {
  links.hear(e);
};
beforeEach(() => {
  links = new FrameLinks();
  window.addEventListener("message", hear);
});
afterEach(() => {
  window.removeEventListener("message", hear);
  links.close();
  document.body.replaceChildren();
});

describe("asking a frame", () => {
  it("ends a question to a frame that is loaded again or taken away before it answers as gone, at once, not as unanswered", async () => {
    vi.useFakeTimers();
    try {
      const a = frameElement();
      links.attach("a", a.el);
      say(a.win, { pictor: "inspect", type: "ready" });
      const replaced = links.hit("a", 1, 2);
      await vi.advanceTimersByTimeAsync(0);
      links.attach("a", frameElement().el);
      await expect(replaced).rejects.toBeInstanceOf(FrameGoneError);
      say(a.win, { pictor: "inspect", type: "ready" });
      const b = frameElement();
      links.attach("b", b.el);
      say(b.win, { pictor: "inspect", type: "ready" });
      const removed = links.hit("b", 1, 2);
      await vi.advanceTimersByTimeAsync(0);
      links.attach("b", null);
      await expect(removed).rejects.toBeInstanceOf(FrameGoneError);
    } finally {
      vi.useRealTimers();
    }
  });

  it("waits until the frame says it is ready, then asks, and answers with the reply that carries its number", async () => {
    const a = frameElement();
    links.attach("a", a.el);
    const answer = links.hit("a", 10, 20);
    const state = settles(answer);
    await flush();
    expect(a.sent).not.toHaveBeenCalled();
    say(a.win, { pictor: "inspect", type: "ready" });
    await flush();
    expect(a.sent).toHaveBeenCalledTimes(1);
    const [asked] = a.sent.mock.calls[0] as [{ pictor: string; id: number; type: string; x: number; y: number }];
    expect(asked).toMatchObject({ pictor: "inspect", type: "hit", x: 10, y: 20 });
    expect(state.done).toBe(false);
    say(a.win, { pictor: "inspect", id: asked.id, type: "hit", pid: "e4" });
    await expect(answer).resolves.toBe("e4");
  });

  it("ignores a reply from a window that is not the frame it asked", async () => {
    const a = frameElement();
    const other = frameElement();
    links.attach("a", a.el);
    links.attach("b", other.el);
    say(a.win, { pictor: "inspect", type: "ready" });
    const answer = links.hit("a", 1, 1);
    const state = settles(answer);
    await flush();
    const [asked] = a.sent.mock.calls[0] as [{ id: number }];
    // Another frame, this page itself, and a window that is nobody's: none of them answers for a.
    say(other.win, { pictor: "inspect", id: asked.id, type: "hit", pid: "stolen" });
    say(window, { pictor: "inspect", id: asked.id, type: "hit", pid: "stolen" });
    say(null, { pictor: "inspect", id: asked.id, type: "hit", pid: "stolen" });
    await flush();
    expect(state.done).toBe(false);
    say(a.win, { pictor: "inspect", id: asked.id, type: "hit", pid: "e1" });
    await expect(answer).resolves.toBe("e1");
  });

  it("ignores what is not shaped like an answer, and reports the elements a frame knows", async () => {
    const a = frameElement();
    links.attach("a", a.el);
    say(a.win, { pictor: "inspect", type: "ready" });
    const answer = links.report("a", ["e1"]);
    await flush();
    const [asked] = a.sent.mock.calls[0] as [{ id: number; pids: string[] }];
    expect(asked.pids).toEqual(["e1"]);
    say(a.win, "hello");
    say(a.win, { pictor: "something else", id: asked.id });
    say(a.win, { pictor: "inspect", id: "not a number" });
    const item = { pid: "e1", tag: "h1", rect: { x: 1, y: 2, w: 3, h: 4 }, attrs: [], inline: [], computed: {} };
    say(a.win, { pictor: "inspect", id: asked.id, type: "report", items: [item] });
    await expect(answer).resolves.toEqual([item]);
  });

  it("gives up on a frame that never answers, so one that did not load cannot hang the page", async () => {
    vi.useFakeTimers();
    try {
      const a = frameElement();
      links.attach("a", a.el);
      say(a.win, { pictor: "inspect", type: "ready" });
      const answer = links.hit("a", 1, 1);
      const failed = expect(answer).rejects.toThrow("did not answer");
      await vi.advanceTimersByTimeAsync(5000);
      await failed;
    } finally {
      vi.useRealTimers();
    }
  });

  it("treats a new element for the same frame as a frame that has not said it is ready", async () => {
    const first = frameElement();
    links.attach("a", first.el);
    say(first.win, { pictor: "inspect", type: "ready" });
    const second = frameElement();
    links.attach("a", second.el);
    const state = settles(links.hit("a", 1, 1));
    await flush();
    expect(second.sent).not.toHaveBeenCalled();
    expect(state.done).toBe(false);
  });

  it("tells what the room holds to a frame once it is ready, in the order told, and is answered when the frame has done it", async () => {
    const a = frameElement();
    links.attach("a", a.el);
    const first = links.apply("a", [["e1", "left", "10px"]], ["e1", "e2"]);
    const second = links.apply("a", [["e1", "left", null]]);
    await flush();
    expect(a.sent).not.toHaveBeenCalled();
    say(a.win, { pictor: "inspect", type: "ready" });
    await flush();
    const asked = a.sent.mock.calls.map(([m]) => m as { id: number; type: string; styles: unknown[]; alive?: string[] });
    expect(asked.map((m) => [m.type, m.styles, m.alive])).toEqual([
      ["apply", [["e1", "left", "10px"]], ["e1", "e2"]],
      ["apply", [["e1", "left", null]], undefined],
    ]);
    say(a.win, { pictor: "inspect", id: asked[0]?.id, type: "apply", ok: true });
    say(a.win, { pictor: "inspect", id: asked[1]?.id, type: "apply", ok: true });
    await expect(Promise.all([first, second])).resolves.toBeDefined();
  });

  it("says which frame became ready, each time one does, and stops when told to", () => {
    const a = frameElement();
    const b = frameElement();
    links.attach("a", a.el);
    links.attach("b", b.el);
    const heard: string[] = [];
    const stop = links.onReady((id) => heard.push(id));
    say(a.win, { pictor: "inspect", type: "ready" });
    say(null, { pictor: "inspect", type: "ready" });
    say(b.win, { pictor: "inspect", type: "ready" });
    say(a.win, { pictor: "inspect", type: "ready" });
    expect(heard).toEqual(["a", "b", "a"]);
    stop();
    say(b.win, { pictor: "inspect", type: "ready" });
    expect(heard).toHaveLength(3);
  });
});

describe("telling a frame about the artifact's files", () => {
  it("writes them with the grant the frame loads them by, in styles, built markup and image sources, and says which elements it then has", async () => {
    const a = frameElement();
    links.attach("a", a.el);
    links.setFiles("/files/art/", "/files/art/g.sig/");
    say(a.win, { pictor: "inspect", type: "ready", pids: ["b"] });
    const have = links.has("a");
    expect(have ? [...have] : null).toEqual(["b"]);
    const styled = links.apply("a", [["e1", "background-image", "url(/files/art/p.png)"], ["e1", "left", "1px"], ["e1", "color", null]]);
    const built = links.mount("a", [{ parent: "b", before: null, html: '<img src="/files/art/p.png" data-pid="n1"><i data-pid="n2"></i>' }], [["n1", "src", "/files/art/q.png"], ["n1", "alt", null]]);
    await flush();
    const sent = a.sent.mock.calls.map(([m]) => m as { id: number; styles: [string, string, string | null][]; create?: { html: string }[]; attrs?: unknown });
    expect(sent[0]?.styles).toEqual([["e1", "background-image", "url(/files/art/g.sig/p.png)"], ["e1", "left", "1px"], ["e1", "color", null]]);
    expect(sent[1]?.create?.[0]?.html).toBe('<img src="/files/art/g.sig/p.png" data-pid="n1"><i data-pid="n2"></i>');
    expect(sent[1]?.attrs).toEqual([["n1", "src", "/files/art/g.sig/q.png"], ["n1", "alt", null]]);
    for (const m of sent) say(a.win, { pictor: "inspect", id: m.id, type: "apply", ok: true });
    await Promise.all([styled, built]);
    expect([...(links.has("a") ?? [])].sort()).toEqual(["b", "n1", "n2"]);
  });

  it("does not know what a frame has until it says it is ready, and forgets it when the frame is replaced", () => {
    const a = frameElement();
    links.attach("a", a.el);
    expect(links.has("a")).toBeNull();
    say(a.win, { pictor: "inspect", type: "ready", pids: ["b"] });
    expect(links.has("a")).not.toBeNull();
    links.attach("a", frameElement().el);
    expect(links.has("a")).toBeNull();
  });
});
