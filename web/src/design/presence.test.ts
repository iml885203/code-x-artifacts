// @vitest-environment node
import { Awareness, applyAwarenessUpdate, encodeAwarenessUpdate } from "y-protocols/awareness";
import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { MAX_PUBLISHED, peersOf, publishSelection, selectionField } from "./presence";

const sel = (frameId: string, pid: string) => ({ frameId, pid });

/** Two awareness instances that hear each other, as two browsers through the server. */
function two() {
  const a = new Awareness(new Y.Doc());
  const b = new Awareness(new Y.Doc());
  a.on("update", ({ added, updated, removed }: { added: number[]; updated: number[]; removed: number[] }) => {
    applyAwarenessUpdate(b, encodeAwarenessUpdate(a, [...added, ...updated, ...removed]), "net");
  });
  return { a, b };
}

describe("what a person publishes of their selection", () => {
  it("is the selected pids of each frame, and nothing when nothing is selected", () => {
    expect(selectionField([sel("hero", "a"), sel("hero", "b"), sel("mobile", "c")])).toEqual({ hero: ["a", "b"], mobile: ["c"] });
    expect(selectionField([])).toBeNull();
  });

  it("is capped so the awareness message stays small", () => {
    const many = Array.from({ length: MAX_PUBLISHED + 30 }, (_, i) => sel("hero", `p${String(i)}`));
    expect(selectionField(many)?.hero).toHaveLength(MAX_PUBLISHED);
  });
});

describe("what others' awareness says", () => {
  it("shows another person's selection under the name in `user`, and not me", () => {
    const { a, b } = two();
    a.setLocalStateField("user", { name: "alice", color: "c3" });
    publishSelection(a, [sel("hero", "title")]);
    expect(peersOf(b, b.clientID)).toEqual([{ client: a.clientID, name: "alice", color: "c3", agent: false, selection: [sel("hero", "title")] }]);
    expect(peersOf(a, a.clientID)).toEqual([]);
  });

  it("drops the selection when it is cleared", () => {
    const { a, b } = two();
    a.setLocalStateField("user", { name: "alice", color: "c3" });
    publishSelection(a, [sel("hero", "title")]);
    publishSelection(a, []);
    expect(peersOf(b, b.clientID)[0]?.selection).toEqual([]);
  });

  it("takes a name only from user and ignores a name smuggled into the selection or malformed fields", () => {
    const b = new Awareness(new Y.Doc());
    const states = b.getStates() as Map<number, Record<string, unknown>>;
    states.set(7, { user: { name: "alice", color: "c1" }, selection: { name: "carol", hero: ["ok", 5, "", "x".repeat(200)], 9: "no", "": ["z"] } });
    states.set(8, { selection: { hero: ["a"] } });
    states.set(9, { user: { name: "" }, selection: { hero: ["a"] } });
    states.set(10, { user: { name: "bob" }, selection: "everything" });
    expect(peersOf(b, b.clientID)).toEqual([
      { client: 7, name: "alice", color: "c1", agent: false, selection: [sel("hero", "ok")] },
      { client: 10, name: "bob", color: undefined, agent: false, selection: [] },
    ]);
  });
});
