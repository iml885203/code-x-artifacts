import { act, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { renderIn } from "../test-support";
import { DesignEdits } from "./edits";
import { Play } from "./Play";
import { setToken } from "./tokens";

const frame = { id: "hero", name: "Hero", file: "frames/hero.html", w: 400, h: 300 };

interface Message { pictor: string; id: number; type: string; tokens?: Record<string, string> }

function ready() {
  const page = screen.getByTitle<HTMLIFrameElement>("Hero");
  const target = page.contentWindow;
  if (!target) throw new Error("Play has no frame");
  const messages: Message[] = [];
  vi.spyOn(target, "postMessage").mockImplementation((message: unknown) => {
    const m = message as Message;
    messages.push(m);
    window.dispatchEvent(new MessageEvent("message", { source: target, data: { pictor: "inspect", id: m.id, type: m.type, ok: true } }));
  });
  act(() => {
    window.dispatchEvent(new MessageEvent("message", { source: target, data: { pictor: "inspect", type: "ready" } }));
  });
  return { page, messages };
}

describe("shared tokens while a frame is played", () => {
  it("shows the room's current tokens and follows an unsaved change without reloading Play", async () => {
    const doc = new Y.Doc();
    const edits = new DesignEdits(doc);
    setToken(edits, "--accent", "#336699");
    const view = renderIn(<Play frame={frame} src="/raw/x/live/frames/hero.html" edits={edits} onExit={() => undefined} />, "en", "/a/x");
    const { page, messages } = ready();
    await waitFor(() => { expect(messages.at(-1)?.tokens).toEqual({ "--accent": "#336699" }); });
    act(() => { setToken(edits, "--accent", "#112233"); });
    await waitFor(() => { expect(messages.at(-1)?.tokens).toEqual({ "--accent": "#112233" }); });
    expect(page).toHaveAttribute("src", "/raw/x/live/frames/hero.html");
    view.unmount();
    edits.destroy();
    doc.destroy();
  });

  it("leaves a historical Play frame on its own saved tokens when the live room changes", () => {
    const doc = new Y.Doc();
    const edits = new DesignEdits(doc);
    const view = renderIn(<Play frame={frame} src="/raw/x/history/frames/hero.html" edits={null} onExit={() => undefined} />, "en", "/a/x");
    const { page, messages } = ready();
    act(() => { setToken(edits, "--accent", "#112233"); });
    expect(messages).toEqual([]);
    expect(page).toHaveAttribute("src", "/raw/x/history/frames/hero.html");
    view.unmount();
    edits.destroy();
    doc.destroy();
  });
});
