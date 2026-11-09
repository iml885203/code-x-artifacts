import { act, renderHook } from "@testing-library/react";
import { Awareness } from "y-protocols/awareness";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as Y from "yjs";
import { markMs } from "./presenceAgent";
import type { Live } from "./useRoom";
import { useAgentMarks } from "./useAgentMarks";

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

function join() {
  const awareness = new Awareness(new Y.Doc());
  const live: Live = { awareness, self: awareness.clientID, state: "live" };
  const announce = (client: number, state: object | null) => {
    act(() => {
      const states = awareness.getStates() as Map<number, object>;
      if (state) states.set(client, state);
      else states.delete(client);
      awareness.emit("change", [{ added: [client], updated: [], removed: [] }, "net"]);
    });
  };
  return { live, announce };
}

const agent = (at: number, pid = "title") => ({ user: { name: "alice 的 Claude Code", agent: true }, touched: { hero: [pid] }, at });

describe("the marks of what an agent changed", () => {
  it("show for markMs after the announcement and then go, without anything else happening", () => {
    const { live, announce } = join();
    const { result } = renderHook(() => useAgentMarks(live));
    expect(result.current).toEqual([]);
    announce(7, agent(1));
    expect(result.current.map((p) => [p.name, p.selection])).toEqual([["alice 的 Claude Code", [{ frameId: "hero", pid: "title" }]]]);
    act(() => {
      vi.advanceTimersByTime(markMs - 100);
    });
    expect(result.current).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toEqual([]);
  });

  it("start again when the agent changes something else", () => {
    const { live, announce } = join();
    const { result } = renderHook(() => useAgentMarks(live));
    announce(7, agent(1));
    act(() => {
      vi.advanceTimersByTime(markMs + 100);
    });
    expect(result.current).toEqual([]);
    announce(7, agent(2, "box"));
    expect(result.current.map((p) => p.selection)).toEqual([[{ frameId: "hero", pid: "box" }]]);
  });

  it("are nothing without a room", () => {
    const { result } = renderHook(() => useAgentMarks(null));
    expect(result.current).toEqual([]);
  });
});
