// @vitest-environment node
import { describe, expect, it } from "vitest";
import { fileName } from "./api";

describe("the file name of a download", () => {
  const cases: [string, string, string | undefined][] = [
    ["plain ASCII without spaces, as the server sends it", `attachment; filename="Landing.zip"; filename*=UTF-8''Landing.zip`, "Landing.zip"],
    ["spaces", `attachment; filename="Landing page.zip"; filename*=UTF-8''Landing%20page.zip`, "Landing page.zip"],
    ["CJK: the encoded name wins over the fallback", `attachment; filename="__ Report.docx"; filename*=UTF-8''%E9%80%B1%E5%A0%B1%20Report.docx`, "週報 Report.docx"],
    ["quotes", `attachment; filename="say _hi_.md"; filename*=UTF-8''say%20%22hi%22.md`, 'say "hi".md'],
    ["a slash that was turned into a space is kept as one", `attachment; filename="a b.md"; filename*=UTF-8''a%20b.md`, "a b.md"],
    ["only a quoted filename", `attachment; filename="sprint.md"`, "sprint.md"],
    ["an escaped quote in a quoted filename", `attachment; filename="say \\"hi\\".md"`, 'say "hi".md'],
    ["only a plain token", "attachment; filename=Landing.zip", "Landing.zip"],
    ["only filename*, any case of the charset", `attachment; filename*=utf-8''Q3%20%E8%A8%88%E7%95%AB.md`, "Q3 計畫.md"],
    ["a filename* that does not decode falls back to filename", `attachment; filename="x.md"; filename*=UTF-8''%E0%A4%A`, "x.md"],
    ["no name at all", "attachment", undefined],
  ];
  it.each(cases)("%s", (_label, header, want) => {
    expect(fileName(header)).toBe(want);
  });
});
