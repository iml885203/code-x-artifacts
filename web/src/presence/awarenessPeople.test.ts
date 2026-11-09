// @vitest-environment node
import { Awareness } from "y-protocols/awareness";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { peopleOfAwareness } from "./awarenessPeople";

function room(me: object, others: Record<number, object>) {
  const awareness = new Awareness(new Y.Doc());
  awareness.setLocalState(me);
  for (const [client, state] of Object.entries(others)) (awareness.getStates() as Map<number, object>).set(Number(client), state);
  return awareness;
}

describe("who a live room says is here", () => {
  it("lists each person once under the name the server wrote, you first, an agent flagged", () => {
    const awareness = room({ user: { id: "zoe", name: "Zoe", color: "c1" } }, {
      5: { user: { id: "bob", name: "Bob", color: "c2" } },
      6: { user: { id: "bob", name: "Bob", color: "c2" } }, // a second tab
      7: { user: { id: "bob/Claude Code", name: "bob 的 Claude Code", agent: true } },
    });
    expect(peopleOfAwareness(awareness, awareness.clientID)).toEqual([
      { id: "zoe", name: "Zoe", color: "c1", agent: false, self: true },
      { id: "bob", name: "Bob", color: "c2", agent: false, self: false },
      { id: "bob/Claude Code", name: "bob 的 Claude Code", color: undefined, agent: true, self: false },
    ]);
  });

  it("ignores states with no name or a name that is not text, whatever another browser sent", () => {
    const awareness = room({ user: { name: "Zoe" } }, { 5: {}, 6: { user: { name: "" } }, 7: { user: { name: 42 } }, 8: { user: { name: "Bob", agent: "yes" } } });
    expect(peopleOfAwareness(awareness, awareness.clientID).map((p) => [p.name, p.agent])).toEqual([["Zoe", false], ["Bob", false]]);
  });
});
