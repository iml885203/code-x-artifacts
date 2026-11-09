import * as Y from "yjs";
import { DesignEdits } from "../edits";
import { FILE, seed } from "../test-room";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FrameLinks } from "../inspect";
import { useCommentPins } from "./useCommentPins";

const frame = { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 };
const frames = [{ frame, x: 120, y: 120 }];
const threads = [{ id: "c1", anchor: { type: "design", frameId: "hero", pid: "heading", x: 100, y: 80 } }];

function openFrame() {
  const el = document.createElement("iframe");
  document.body.appendChild(el);
  const win = el.contentWindow;
  if (!win) throw new Error("missing frame window");
  const links = new FrameLinks();
  links.attach("hero", el);
  const say = (data: unknown) => { links.hear(new MessageEvent("message", { data, source: win })); };
  say({ pictor: "inspect", type: "ready", pids: ["heading"] });
  return { links, el, win, say };
}

describe("pins follow source elements without losing their threads", () => {
  it("removes a deleted element's pin without losing its thread and reconnects the same restored element", async () => {
    const { links, el, win, say } = openFrame();
    let items = [{ pid: "heading", rect: { x: 30, y: 40, w: 200, h: 60 } }];
    vi.spyOn(win, "postMessage").mockImplementation((message: unknown) => {
      const data = message as { id: number };
      queueMicrotask(() => { say({ pictor: "inspect", id: data.id, items }); });
    });
    const hook = renderHook(() => useCommentPins(threads, frames, links));
    await waitFor(() => { expect(hook.result.current.pins[0]).toMatchObject({ id: "c1", x: 350, y: 160, fallback: false }); });
    items = [];
    act(() => { hook.result.current.refresh(); });
    await waitFor(() => { expect(hook.result.current.pins).toEqual([]); });
    expect(threads[0]?.anchor.pid).toBe("heading");
    items = [{ pid: "heading", rect: { x: 50, y: 60, w: 200, h: 60 } }];
    act(() => { hook.result.current.refresh(); });
    await waitFor(() => { expect(hook.result.current.pins[0]).toMatchObject({ id: "c1", x: 370, y: 180, fallback: false }); });
    hook.unmount(); links.close(); el.remove();
  });

  it("drops late geometry from a page no longer shown and preserves unrelated anchors", async () => {
    const { links, el, win, say } = openFrame();
    let request = 0;
    vi.spyOn(win, "postMessage").mockImplementation((message: unknown) => { request = (message as { id: number }).id; });
    const hook = renderHook(({ shown }) => useCommentPins(threads, shown, links), { initialProps: { shown: frames } });
    await waitFor(() => { expect(request).toBeGreaterThan(0); });
    hook.rerender({ shown: [] });
    act(() => { say({ pictor: "inspect", id: request, items: [{ pid: "heading", rect: { x: 1, y: 2, w: 10, h: 20 } }] }); });
    await waitFor(() => { expect(hook.result.current.pins).toEqual([]); });
    expect(threads).toHaveLength(1);
    hook.unmount(); links.close(); el.remove();
  });
});

it("distinguishes unavailable geometry from a removed room identity and ignores a stale report until Undo restores that identity", async () => {
  const { links, el, win, say } = openFrame();
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  const anchored = [{ id: "c1", anchor: { type: "design", frameId: "hero", pid: "h1", x: 100, y: 80 } }];
  const requests: number[] = [];
  vi.spyOn(win, "postMessage").mockImplementation((message: unknown) => { requests.push((message as { id: number }).id); });
  const source = { edits, frames: [frame] };
  const hook = renderHook(() => useCommentPins(anchored, frames, links, source));
  await waitFor(() => { expect(requests.length).toBeGreaterThan(0); });
  expect(hook.result.current.targets.c1).toBe("unavailable");
  const stale = requests[0];
  act(() => { edits.apply([{ file: FILE, pid: "h1", remove: true }]); });
  expect(hook.result.current.targets.c1).toBe("removed");
  act(() => { say({ pictor: "inspect", id: stale, items: [{ pid: "h1", rect: { x: 1, y: 2, w: 10, h: 20 } }] }); });
  expect(hook.result.current.pins).toEqual([]);
  await act(async () => { await Promise.resolve(); });
  act(() => { edits.undo(); });
  expect(hook.result.current.targets.c1).toBe("unavailable");
  await waitFor(() => { expect(requests.length).toBeGreaterThan(1); });
  act(() => { say({ pictor: "inspect", id: requests.at(-1), items: [{ pid: "h1", rect: { x: 1, y: 2, w: 10, h: 20 } }] }); });
  await waitFor(() => { expect(hook.result.current.targets.c1).toBe("available"); });
  expect(hook.result.current.pins[0]?.id).toBe("c1");
  hook.unmount(); links.close(); el.remove(); edits.destroy(); doc.destroy();
});

it("keeps a pin on an inline element of formatted text, which the room holds as part of that text", async () => {
  const { links, el, win, say } = openFrame();
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  doc.transact(() => {
    edits.map.delete(`${FILE}|n|~t1|kind`);
    const rich = new Y.Text();
    rich.insert(0, "Hello ");
    rich.insert(6, "big", { "f:bold1": JSON.stringify({ t: "b", d: 0, a: [["", "data-pid", "bold1"]] }) });
    edits.map.set(`${FILE}|n|para|rich`, rich);
  });
  const anchored = [{ id: "c1", anchor: { type: "design", frameId: "hero", pid: "bold1", x: 10, y: 8 } }];
  vi.spyOn(win, "postMessage").mockImplementation((message: unknown) => {
    const data = message as { id: number };
    queueMicrotask(() => { say({ pictor: "inspect", id: data.id, items: [{ pid: "bold1", rect: { x: 30, y: 40, w: 20, h: 10 } }] }); });
  });
  const hook = renderHook(() => useCommentPins(anchored, frames, links, { edits, frames: [frame] }));
  expect(hook.result.current.targets.c1).not.toBe("removed");
  await waitFor(() => { expect(hook.result.current.targets.c1).toBe("available"); });
  expect(hook.result.current.pins[0]?.id).toBe("c1");
  hook.unmount(); links.close(); el.remove(); edits.destroy(); doc.destroy();
});
