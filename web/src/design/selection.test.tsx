import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { act, renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import * as Y from "yjs";
import { Editor, type Target } from "./controller";
import { DesignEdits } from "./edits";
import { FrameLinks } from "./inspect";
import { mountBuilt } from "./ops/built";
import { DesignOps } from "./ops/DesignOps";
import { FILE, key, seed } from "./test-room";
import { useFramesShowRoom } from "./useRoom";
import { keyOf, useSelection } from "./useSelection";

const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "../../../internal/kind/design_inspector.js"), "utf8");

it("keeps a duplicate selected and movable when its frame becomes ready after duplication", async () => {
  const doc = new Y.Doc();
  seed(doc);
  const edits = new DesignEdits(doc);
  doc.transact(() => {
    edits.map.set(key("box", "s|position"), "absolute");
    edits.map.set(key("box", "s|height"), "40px");
  });
  const links = new FrameLinks();
  const frame = document.createElement("iframe");
  const body = document.createElement("div");
  body.setAttribute("data-pid", "b");
  const content = new DOMParser().parseFromString('<div data-pid="box"><div data-pid="para">Hello</div></div><h1 data-pid="h1"></h1>', "text/html");
  body.replaceChildren(...content.body.childNodes);
  document.body.append(body, frame);
  const win = frame.contentWindow;
  if (!win) throw new Error("no frame window");
  links.attach("hero", frame);
  // The real inspector over a queued window transport, with readiness held by the test.
  const listeners: Record<string, ((e: unknown) => void)[]> = {};
  const parent = { postMessage: (data: unknown) => {
    queueMicrotask(() => { links.hear(new MessageEvent("message", { data, source: win })); });
  } };
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const run = new Function("parent", "addEventListener", "scrollX", "scrollY", source) as (...args: unknown[]) => void;
  run(parent, (type: string, listener: (e: unknown) => void) => (listeners[type] ??= []).push(listener), 0, 0);
  vi.spyOn(win, "postMessage").mockImplementation((data: unknown) => {
    queueMicrotask(() => { for (const listener of listeners.message ?? []) listener({ data, source: parent }); });
  });
  const stop = mountBuilt(edits, links, () => [{ id: "hero", file: FILE }]);
  const hook = renderHook(() => {
    const selection = useSelection(links);
    useFramesShowRoom(edits, links, [{ id: "hero", file: FILE }], () => { void selection.rerun(); });
    return selection;
  });
  try {
    const original: Target = { frameId: "hero", file: FILE, pid: "box", report: {
      pid: "box", tag: "div", rect: { x: 10, y: 20, w: 30, h: 40 }, attrs: [], inline: [],
      computed: { position: "absolute", left: "10px", top: "20px", width: "30px", height: "40px" },
    } };
    const outcome = new DesignOps(edits).duplicate([original]);
    if ("refused" in outcome || !outcome.done[0]) throw new Error("duplicate refused");
    const chosen = { frameId: "hero", pid: outcome.done[0].pid };
    let selecting: Promise<void> | undefined;
    act(() => { selecting = hook.result.current.choose(chosen, false, []); });
    await act(async () => {
      for (const ready of listeners.DOMContentLoaded ?? []) ready({});
      await selecting;
    });
    expect(hook.result.current.selected).toEqual([chosen]);
    const report = hook.result.current.reports[keyOf(chosen)];
    expect(report?.computed).toMatchObject({ position: "absolute", left: "26px", top: "36px" });
    if (!report) throw new Error("duplicate has no report");
    const { drag } = new Editor(edits, links).move([{ ...chosen, file: FILE, report }]);
    expect(drag).not.toBeNull();
    await act(async () => {
      drag?.update(60, 30);
      drag?.end();
      await Promise.resolve();
    });
    expect(edits.frame(FILE).styles.get(chosen.pid)?.get("left")).toBe("86px");
    expect(edits.frame(FILE).styles.get("box")?.get("left")).toBe(" 10px");
    await act(async () => {
      edits.undo();
      await Promise.resolve();
    });
    expect(edits.frame(FILE).styles.get(chosen.pid)?.get("left")).toBe("26px");
  } finally {
    hook.unmount();
    stop();
    links.close();
    edits.destroy();
    doc.destroy();
    frame.remove();
    body.remove();
  }
});
