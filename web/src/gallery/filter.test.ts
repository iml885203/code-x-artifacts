// @vitest-environment node
import { describe, expect, it } from "vitest";
import { artifact } from "../test-support";
import { visibleArtifacts } from "./filter";

const list = [
  artifact({ title: "Sprint report", author: "alice" }),
  artifact({ title: "Cheat sheet", author: "bob" }),
  artifact({ title: "Alice's notes", author: "bob", pinned: true }),
];
const titles = (filter: "all" | "mine" | "shared" | "pinned", query: string) =>
  visibleArtifacts(list, filter, query, "alice").map((a) => a.title);

describe("visibleArtifacts", () => {
  it.each([
    ["all", "", ["Alice's notes", "Sprint report", "Cheat sheet"]],
    ["pinned", "", ["Alice's notes"]],
    ["pinned", "sprint", []],
    ["mine", "", ["Sprint report"]],
    ["shared", "", ["Alice's notes", "Cheat sheet"]],
    ["all", "  SHEET ", ["Cheat sheet"]],
    ["all", "alice", ["Alice's notes", "Sprint report"]],
    ["shared", "alice", ["Alice's notes"]],
    ["mine", "bob", []],
    ["all", "no such thing", []],
  ] as const)("filter %s with search %j shows %j", (filter, query, want) => {
    expect(titles(filter, query)).toEqual(want);
  });

  it("matches the title and the author together, as the server-rendered gallery did", () => {
    expect(titles("all", "report alice")).toEqual(["Sprint report"]);
  });
});
