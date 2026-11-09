import { Awareness } from "y-protocols/awareness";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { markMs } from "./agents";
import { room } from "./testRoom";
import { mount, type LiveView } from "./view";

const open: LiveView[] = [];
beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  for (const v of open.splice(0)) v.destroy();
  vi.useRealTimers();
});

const para = (id: string, s: string) => ({ type: "paragraph", attrs: { id }, content: [{ type: "text", text: s }] });

function editor(): { awareness: Awareness; dom: HTMLElement } {
  const doc = room(para("b_1", "one"), para("b_2", "two"));
  const awareness = new Awareness(doc);
  const parent = document.createElement("div");
  document.body.append(parent);
  const live = mount(parent, doc, awareness, false, "Document");
  open.push({ view: live.view, destroy: () => { live.destroy(); parent.remove(); } });
  return { awareness, dom: live.view.dom };
}

/** What the server does: an entry of its own in awareness, which the browser did not make. */
function announce(awareness: Awareness, client: number, name: string, block: string, at: number): void {
  awareness.states.set(client, { user: { name, agent: true }, block, at });
  awareness.emit("change", [{ added: [client], updated: [], removed: [] }, "remote"]);
}

describe("an agent the server shows", () => {
  it("marks the block it changed with its name, and the mark goes after a few seconds", () => {
    const { awareness, dom } = editor();
    expect(dom.querySelector("[data-agent]")).toBeNull();

    announce(awareness, 900, "alice 的 Claude Code", "b_2", 1);
    const marked = dom.querySelector("[data-agent]");
    expect(marked?.getAttribute("data-agent")).toBe("alice 的 Claude Code");
    expect(marked?.textContent).toBe("two");
    expect(dom.querySelectorAll("[data-agent]")).toHaveLength(1);

    vi.advanceTimersByTime(markMs + 100);
    expect(dom.querySelector("[data-agent]")).toBeNull();
  });

  it("moves to the block of its next announcement and does not mark a block that is not there", () => {
    const { awareness, dom } = editor();
    announce(awareness, 900, "alice 的 agent", "b_1", 1);
    announce(awareness, 900, "alice 的 agent", "b_2", 2);
    expect(dom.querySelector("[data-agent]")?.textContent).toBe("two");
    announce(awareness, 900, "alice 的 agent", "b_missing", 3);
    expect(dom.querySelector("[data-agent]")).toBeNull();
  });

  it("is not made by a person: only an entry that says it is an agent is drawn", () => {
    const { awareness, dom } = editor();
    awareness.states.set(901, { user: { name: "bob" }, block: "b_1", at: 1 });
    awareness.emit("change", [{ added: [901], updated: [], removed: [] }, "remote"]);
    expect(dom.querySelector("[data-agent]")).toBeNull();
  });
});
