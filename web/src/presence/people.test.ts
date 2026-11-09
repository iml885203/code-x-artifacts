// @vitest-environment node
import { describe, expect, it } from "vitest";
import { initialOf, mergePeople, type Person } from "./people";

const person = (name: string, over: Partial<Person> = {}): Person => ({ id: name.toLowerCase(), name, agent: false, self: false, ...over });

describe("who is here, from every source", () => {
  it("lists a person once however many sources know them, and you first", () => {
    const room = [person("Bob", { color: "c1" }), person("Alice", { self: true, color: "c3" })];
    const beats = [person("Alice"), person("Bob"), person("Carol")];
    expect(mergePeople(room, beats).map((p) => p.name)).toEqual(["Alice", "Bob", "Carol"]);
  });

  it("is you when any source says so, and keeps the first colour given", () => {
    const merged = mergePeople([person("Bob", { color: "c1" })], [person("Bob", { self: true, color: "c5" })]);
    expect(merged).toEqual([{ id: "bob", name: "Bob", agent: false, self: true, color: "c1" }]);
  });

  it("does not take an agent for the person with the same name, and sorts the rest by name", () => {
    const merged = mergePeople([person("bob", { agent: true }), person("bob")], [person("Zed"), person("Amy")]);
    expect(merged.map((p) => `${p.agent ? "agent" : "person"}:${p.name}`)).toEqual(["person:Amy", "agent:bob", "person:bob", "person:Zed"]);
  });

  // The shapes the two sources really carry. The room's awareness keeps your own entry as your browser wrote it
  // (the account, "alice"); the server's viewers list names you by your directory display name ("ALICE").
  const inRoom = (over: Partial<Person>): Person => ({ id: "alice", name: "alice", agent: false, self: true, color: "c3", ...over });
  const heartbeat = (over: Partial<Person>): Person => ({ id: "alice", name: "ALICE", agent: false, self: true, ...over });

  it("is one entry for one account whichever way each source spells the name, with the better name", () => {
    const merged = mergePeople([inRoom({})], [heartbeat({})]);
    expect(merged).toEqual([{ id: "alice", name: "ALICE", agent: false, self: true, color: "c3" }]);
    expect(mergePeople([heartbeat({})], [inRoom({})]).map((p) => p.name)).toEqual(["ALICE"]);
  });

  it("keeps a name the server gave when the other source only knows the account, and two accounts apart even when their names match", () => {
    const merged = mergePeople(
      [inRoom({ name: "Alice Lim" }), inRoom({ id: "alice2", name: "Alice Lim", self: false })],
      [heartbeat({ name: "ALICE" }), heartbeat({ id: "alice2", name: "Alice Lim", self: false })],
    );
    expect(merged.map((p) => `${p.id}:${p.name}`)).toEqual(["alice:Alice Lim", "alice2:Alice Lim"]);
  });

  it("keeps an agent beside the person it works for, and one agent per client", () => {
    const merged = mergePeople(
      [inRoom({}), { id: "alice/Claude Code", name: "alice 的 Claude Code", agent: true, self: false }],
      [heartbeat({}), { id: "alice/Claude Code", name: "ALICE 的 Claude Code", agent: true, self: false }, { id: "alice/Cursor", name: "alice 的 Cursor", agent: true, self: false }],
    );
    expect(merged.map((p) => `${p.agent ? "agent" : "person"}:${p.id}`)).toEqual(["person:alice", "agent:alice/Claude Code", "agent:alice/Cursor"]);
  });

  it("keeps a guest apart from a person, an agent and another guest, even under the same id or name", () => {
    const guest = (id: string, name: string): Person => ({ id, name, agent: false, self: false, guest: true });
    const merged = mergePeople(
      [person("Swift Otter"), { id: "guest:1", name: "Swift Otter", agent: true, self: false }],
      [guest("guest:1", "Swift Otter #ab12"), guest("guest:2", "Swift Otter #cd34"), guest("guest:1", "Swift Otter #ab12"), guest("swift otter", "Swift Otter")],
    );
    expect(merged.map((p) => `${p.guest === true ? "guest" : p.agent ? "agent" : "person"}:${p.id}`)).toEqual([
      "person:swift otter",
      "agent:guest:1",
      "guest:swift otter",
      "guest:guest:1",
      "guest:guest:2",
    ]);
  });

  it("is empty when nobody is", () => {
    expect(mergePeople([], [])).toEqual([]);
  });
});

describe("the letter in an avatar", () => {
  it.each([
    ["alice", "A"],
    ["  bob", "B"],
    ["王小明", "王"],
    ["😀 smile", "😀"],
    ["", "?"],
  ])("%j is %j", (name, letter) => {
    expect(initialOf(name)).toBe(letter);
  });
});
