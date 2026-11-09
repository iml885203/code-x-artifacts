import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import * as Y from "yjs";
import { DesignEdits } from "./edits";
import { FrameLinks } from "./inspect";
import { FILE, key, seed } from "./test-room";
import { useFramesShowRoom } from "./useRoom";

const inspector = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "../../../internal/kind/design_inspector.js"), "utf8");

it("projects the real room through the inspector after snapshot load, Reset, Undo and a peer override", async () => {
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  doc.transact(() => {
    edits.map.set(key("box", "rest"), "background-color:background-color;background:red!important;transform:rotate(2deg)");
    edits.map.set(key("box", "s|background-color"), "blue!important");
    edits.map.set(key("box", "override|background-color"), "green!important");
  });
  const element = document.createElement("div");
  element.setAttribute("data-pid", "box");
  element.setAttribute("style", "background-color:blue!important;background:red!important;background-color:green!important");
  const frame = document.createElement("iframe");
  document.body.append(element, frame);
  const win = frame.contentWindow;
  if (!win) throw new Error("missing inspector window");
  const links = new FrameLinks(); links.attach("hero", frame);
  const listeners: Record<string, ((event: unknown) => void)[]> = {};
  const parent = { postMessage: (data: unknown) => { queueMicrotask(() => { links.hear(new MessageEvent("message", { data, source: win })); }); } };
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const run = new Function("parent", "addEventListener", "scrollX", "scrollY", inspector) as (...args: unknown[]) => void;
  run(parent, (type: string, fn: (event: unknown) => void) => (listeners[type] ??= []).push(fn), 0, 0);
  vi.spyOn(win, "postMessage").mockImplementation((data: unknown) => { queueMicrotask(() => { for (const fn of listeners.message ?? []) fn({ data, source: parent }); }); });
  const hook = renderHook(() => { useFramesShowRoom(edits, links, [{ id: "hero", file: FILE }], () => undefined); });
  try {
    await act(async () => { for (const fn of listeners.DOMContentLoaded ?? []) fn({}); await Promise.resolve(); });
    await waitFor(() => { expect(element.getAttribute("style")).toContain("background-color:green!important"); });
    act(() => { edits.apply([{ file: FILE, pid: "box", style: { "background-color": "" } }]); });
    await waitFor(() => { expect(element.getAttribute("style")).not.toContain("green"); });
    expect(element.getAttribute("style")).toContain("background-color:blue!important;background:red!important");
    expect(element.getAttribute("style")).toContain("transform:rotate(2deg)");
    act(() => { edits.undo(); });
    await waitFor(() => { expect(element.getAttribute("style")).toContain("background-color:green!important"); });
    act(() => { doc.transact(() => { edits.map.set(key("box", "override|background-color"), "yellow!important"); }, "peer"); });
    await waitFor(() => { expect(element.getAttribute("style")).toContain("background-color:yellow!important"); });
    act(() => { edits.undo(); });
    expect(edits.frame(FILE).styles.get("box")?.get("background-color")).toBe("yellow!important");
  } finally { hook.unmount(); links.close(); edits.destroy(); frame.remove(); element.remove(); }
});

it("keeps the inline elements of a formatted text shown and hides only what the room does not hold", async () => {
  const doc = new Y.Doc(); seed(doc);
  const edits = new DesignEdits(doc);
  doc.transact(() => {
    edits.map.delete(key("~t1", "kind"));
    const rich = new Y.Text();
    rich.insert(0, "Hello ");
    rich.insert(6, "big", { "f:bold1": JSON.stringify({ t: "b", d: 0, a: [["", "data-pid", "bold1"]] }) });
    rich.insertEmbed(9, { raw: `<small data-pid="small1">next</small>` });
    edits.map.set(key("para", "rich"), rich);
  });
  const para = document.createElement("p");
  para.setAttribute("data-pid", "para");
  const inline = (tag: string, pid: string, text: string) => { const el = document.createElement(tag); el.setAttribute("data-pid", pid); el.textContent = text; return el; };
  para.append("Hello ", inline("b", "bold1", "big"), inline("small", "small1", "next"));
  const gone = document.createElement("div");
  gone.setAttribute("data-pid", "gone");
  const frame = document.createElement("iframe");
  document.body.append(para, gone, frame);
  const win = frame.contentWindow;
  if (!win) throw new Error("missing inspector window");
  const links = new FrameLinks(); links.attach("hero", frame);
  const listeners: Record<string, ((event: unknown) => void)[]> = {};
  const parent = { postMessage: (data: unknown) => { queueMicrotask(() => { links.hear(new MessageEvent("message", { data, source: win })); }); } };
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  const run = new Function("parent", "addEventListener", "scrollX", "scrollY", inspector) as (...args: unknown[]) => void;
  run(parent, (type: string, fn: (event: unknown) => void) => (listeners[type] ??= []).push(fn), 0, 0);
  vi.spyOn(win, "postMessage").mockImplementation((data: unknown) => { queueMicrotask(() => { for (const fn of listeners.message ?? []) fn({ data, source: parent }); }); });
  const hook = renderHook(() => { useFramesShowRoom(edits, links, [{ id: "hero", file: FILE }], () => undefined); });
  try {
    await act(async () => { for (const fn of listeners.DOMContentLoaded ?? []) fn({}); await Promise.resolve(); });
    await waitFor(() => { expect(gone.style.getPropertyValue("display")).toBe("none"); });
    const bold = para.querySelector("b");
    expect(bold?.style.getPropertyValue("display")).toBe("");
    expect(para.querySelector("small")?.style.getPropertyValue("display")).toBe("");
    expect(para.style.getPropertyValue("display")).toBe("");
  } finally { hook.unmount(); links.close(); edits.destroy(); frame.remove(); para.remove(); gone.remove(); }
});
