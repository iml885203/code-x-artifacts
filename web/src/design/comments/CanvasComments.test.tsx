import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { artifact, renderIn } from "../../test-support";
import { DesignCanvas } from "../Canvas";
import type { DesignComments } from "./types";

const manifest = { version: 1, pages: [
  { id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] },
  { id: "other", name: "Other", rows: [[{ id: "footer", name: "Footer", file: "frames/footer.html", w: 800, h: 200 }]] },
] };
const threads = [{ id: "c1", anchor: { type: "design", frameId: "footer", pid: "missing", name: "Footer", x: 50, y: 60 } }, { id: "c2", anchor: { type: "design", frameId: "footer", name: "Footer", x: 24, y: 24 } }];
const reader = artifact({ title: "Design comments", author: "alice", kind: "design", canComment: true, canEdit: false });
afterEach(() => { vi.unstubAllGlobals(); });

function serve() { vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ version: 1, manifest, base: "/raw/x/1.sig/", files: "/files/x/1.sig/" }))))); }

describe("the integrated canvas comment path", () => {
  it("lets a reader comment on a frame and reveals an existing thread on another page without editing the design", async () => {
    serve();
    const drafts: Record<string, unknown>[] = [];
    const comments: DesignComments = { threads, active: null, onCompose: (anchor) => drafts.push(anchor), onOpen: () => undefined };
    const ui = renderIn(<DesignCanvas artifact={reader} older={null} comments={comments} />);
    const user = userEvent.setup();
    const host = await screen.findByTitle<HTMLIFrameElement>("Hero, 800 by 600");
    const win = host.contentWindow;
    if (!win) throw new Error("missing Hero window");
    const sent = vi.spyOn(win, "postMessage").mockImplementation(() => undefined);
    const say = (data: unknown) => { act(() => { window.dispatchEvent(new MessageEvent("message", { data, source: win })); }); };
    const asked = (type: string) => {
      const message = sent.mock.calls.map(([m]) => m as { id: number; type: string }).findLast(m => m.type === type);
      if (!message) throw new Error(`inspector was not asked ${type}`);
      return message;
    };
    say({ pictor: "inspect", type: "ready" });
    const viewport = document.querySelector(".canvas-viewport");
    if (!(viewport instanceof HTMLElement)) throw new Error("missing canvas viewport");
    await user.pointer({ keys: "[MouseLeft]", target: viewport, coords: { clientX: 200, clientY: 200 } });
    await waitFor(() => { asked("hit"); });
    say({ pictor: "inspect", type: "hit", id: asked("hit").id, pid: "e2" });
    await waitFor(() => { asked("report"); });
    say({ pictor: "inspect", type: "report", id: asked("report").id, items: [{ pid: "e2", tag: "h1", rect: { x: 40, y: 50, w: 300, h: 80 }, attrs: [["data-pid", "e2"]], inline: [], computed: {} }] });
    await user.click(await screen.findByRole("button", { name: "Comment on selection" }));
    expect(drafts).toEqual([{ type: "design", frameId: "hero", pid: "e2", name: "Hero", x: 340, y: 50 }]);
    expect(screen.queryByRole("button", { name: "Drawing" })).toBeNull();
    ui.unmount();
    const plain = renderIn(<DesignCanvas artifact={reader} older={null} />);
    await screen.findByText("Hero");
    expect(screen.queryByRole("button", { name: "Comment on selection" })).toBeNull();
    plain.unmount();
    renderIn(<DesignCanvas artifact={reader} older={null} comments={{ ...comments, active: "c1" }} />);
    await waitFor(() => { expect(screen.getByRole("button", { name: "Page: Other" })).toBeInTheDocument(); });
    expect(screen.queryByRole("button", { name: "Comment 1 on Footer" })).toBeNull();
    const pin = await screen.findByRole("button", { name: "Comment 2 on Footer" });
    expect(pin).not.toHaveAttribute("data-fallback", "true");
    expect(pin).toHaveAttribute("aria-pressed", "false");
  });
});
