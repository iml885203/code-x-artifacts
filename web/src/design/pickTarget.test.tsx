import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { FrameLinks } from "./inspect";
import { useSelection } from "./useSelection";

/** A frame whose inspector says the point is inside bold text in a paragraph in a box. */
function openFrame() {
  const el = document.createElement("iframe");
  document.body.appendChild(el);
  const win = el.contentWindow;
  if (!win) throw new Error("missing frame window");
  const links = new FrameLinks();
  links.attach("hero", el);
  const say = (data: unknown) => { links.hear(new MessageEvent("message", { data, source: win })); };
  say({ pictor: "inspect", type: "ready", pids: ["b", "box", "para", "bold1"] });
  vi.spyOn(win, "postMessage").mockImplementation((message: unknown) => {
    const m = message as { id: number; type: string; pids?: string[] };
    const reply = m.type === "hit" ? { pid: "bold1" } : m.type === "chain" ? { pids: ["bold1", "para", "box", "b"] } : { items: (m.pids ?? []).map((pid) => ({ pid, tag: "p", rect: { x: 0, y: 0, w: 10, h: 10 }, attrs: [], inline: [], computed: {} })) };
    queueMicrotask(() => { say({ pictor: "inspect", id: m.id, ...reply }); });
  });
  return { links, el };
}

it("selects the nearest element the room holds when a click lands on an inline element of formatted text", async () => {
  const { links, el } = openFrame();
  const room = new Set(["b", "box", "para"]);
  const hook = renderHook(() => useSelection(links, (_frame, pid) => room.has(pid)));
  await act(async () => { await hook.result.current.pick("hero", 5, 5, false, []); });
  await waitFor(() => { expect(hook.result.current.selected).toEqual([{ frameId: "hero", pid: "para" }]); });
  hook.unmount(); links.close(); el.remove();
});

it("selects what is under the pointer when there is no room to say what its elements are", async () => {
  const { links, el } = openFrame();
  const hook = renderHook(() => useSelection(links));
  await act(async () => { await hook.result.current.pick("hero", 5, 5, false, []); });
  await waitFor(() => { expect(hook.result.current.selected).toEqual([{ frameId: "hero", pid: "bold1" }]); });
  hook.unmount(); links.close(); el.remove();
});
