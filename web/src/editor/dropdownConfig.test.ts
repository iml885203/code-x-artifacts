// @vitest-environment node
import { describe, expect, it } from "vitest";
import { readDropdownConfig } from "./schema";

describe("dropdown label trimming agrees with the stored contract", () => {
  it.each(["\uFEFFA", "A\uFEFF", "\uFEFFA\uFEFF"])("refuses boundary BOM in %j", (value) => {
    expect(readDropdownConfig(JSON.stringify({ options: [value], value }))).toBeNull();
  });
  it.each(["A\uFEFFB", "A B", "王😀"])("preserves a valid label %j", (value) => {
    expect(readDropdownConfig(JSON.stringify({ options: [value], value }))).toEqual({ options: [value], value });
  });
});
