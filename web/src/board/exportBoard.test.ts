import { describe, expect, it } from "vitest";
import { exportFileName } from "./exportBoard";

// The same cases as the server's exportFileName (internal/web/doc_markdown_test.go).
describe("a board's export file name", () => {
  it.each([
    ["Q3 計畫", "Q3 計畫.png"],
    ['a/b\\c:d*e?"f<g>h|i', "a b c d e f g h i.png"],
    ["  ..hidden  name  ", "hidden name.png"],
    [" /\\ ", "abc123.png"],
  ])("is %j saved as %j", (title, want) => {
    expect(exportFileName(title, "abc123", "png")).toBe(want);
  });

  it("keeps at most 100 characters of the title", () => {
    expect(Array.from(exportFileName("長".repeat(150), "id", "png"))).toHaveLength(100 + ".png".length);
  });
});
