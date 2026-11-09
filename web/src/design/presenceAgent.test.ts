// @vitest-environment node
import { Awareness } from "y-protocols/awareness";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { announcements, freshMarks, markMs } from "./presenceAgent";

/** An awareness that holds the states as the server relays them. */
function room(states: Record<number, object>) {
  const awareness = new Awareness(new Y.Doc());
  const map = awareness.getStates() as Map<number, object>;
  for (const [client, state] of Object.entries(states)) map.set(Number(client), state);
  return awareness;
}

const agent = (name: string, touched: unknown, at = 1) => ({ user: { name, agent: true }, touched, at });

describe("what an agent says it changed", () => {
  it("is the elements of each frame, under the name the server gave the agent", () => {
    const got = announcements(room({ 7: agent("alice 的 Claude Code", { hero: ["title", "box"], foot: ["p"] }, 42) }));
    expect(got).toEqual([
      {
        client: 7,
        name: "alice 的 Claude Code",
        color: undefined,
        agent: true,
        at: 42,
        selection: [
          { frameId: "hero", pid: "title" },
          { frameId: "hero", pid: "box" },
          { frameId: "foot", pid: "p" },
        ],
      },
    ]);
  });

  it("ignores what is not an agent's word: a person who lists elements, no name, no time, nothing touched, malformed fields", () => {
    const got = announcements(
      room({
        1: { user: { name: "mallory", agent: false }, touched: { hero: ["title"] }, at: 1 },
        2: { user: { name: "mallory" }, touched: { hero: ["title"] }, at: 1 },
        3: agent("", { hero: ["title"] }),
        4: { user: { name: "a 的 agent", agent: true }, touched: { hero: ["title"] } },
        5: agent("b 的 agent", {}),
        6: agent("c 的 agent", "hero"),
        8: agent("d 的 agent", { hero: ["ok", 5, "", "x".repeat(200)], 9: "no" }),
      }),
    );
    expect(got.map((a) => [a.name, a.selection])).toEqual([["d 的 agent", [{ frameId: "hero", pid: "ok" }]]]);
  });
});

describe("how long an agent's mark stays", () => {
  const a = (at: number) => announcements(room({ 7: agent("alice 的 agent", { hero: ["title"] }, at) }));

  it("is markMs from when this browser first saw the announcement, whatever the server's clock says", () => {
    const seen = new Map<string, number>();
    expect(freshMarks(a(5), seen, 1000)).toHaveLength(1);
    expect(freshMarks(a(5), seen, 1000 + markMs - 1)).toHaveLength(1);
    expect(freshMarks(a(5), seen, 1000 + markMs)).toHaveLength(0);
  });

  it("starts again for the next announcement of the same agent", () => {
    const seen = new Map<string, number>();
    freshMarks(a(5), seen, 1000);
    expect(freshMarks(a(5), seen, 1000 + markMs + 10)).toHaveLength(0);
    expect(freshMarks(a(6), seen, 1000 + markMs + 20)).toHaveLength(1);
  });

  it("forgets an announcement that is gone", () => {
    const seen = new Map<string, number>();
    freshMarks(a(5), seen, 1000);
    freshMarks([], seen, 2000);
    expect(seen.size).toBe(0);
  });
});
