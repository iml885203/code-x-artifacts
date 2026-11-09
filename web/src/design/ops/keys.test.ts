// @vitest-environment node
import { describe, expect, it } from "vitest";
import { opsKey } from "./keys";

const press = (key: string, extra: Partial<Record<"metaKey" | "ctrlKey" | "altKey" | "shiftKey", boolean>> & { code?: string } = {}) =>
  opsKey({ key, code: extra.code ?? `Key${key.toUpperCase()}`, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...extra });

describe("the keys of the element operations", () => {
  it.each([
    [press("d", { metaKey: true }), { do: "duplicate" }],
    [press("D", { ctrlKey: true }), { do: "duplicate" }],
    [press("c", { metaKey: true }), { do: "copy" }],
    [press("v", { metaKey: true }), { do: "paste" }],
    [press("g", { metaKey: true }), { do: "group" }],
    [press("G", { metaKey: true, shiftKey: true }), { do: "ungroup" }],
    [press("]", { metaKey: true, code: "BracketRight" }), { do: "stack", how: "forward" }],
    [press("[", { metaKey: true, code: "BracketLeft" }), { do: "stack", how: "backward" }],
    [press("}", { metaKey: true, shiftKey: true, code: "BracketRight" }), { do: "stack", how: "front" }],
    [press("{", { metaKey: true, shiftKey: true, code: "BracketLeft" }), { do: "stack", how: "back" }],
    [press("t"), { do: "tool", shape: "text" }],
    [press("r"), { do: "tool", shape: "rect" }],
    [press("O"), { do: "tool", shape: "oval" }],
  ])("%j", (got, want) => {
    expect(got).toEqual(want);
  });

  it.each([
    press("z", { metaKey: true }),
    press("d"),
    press("d", { metaKey: true, shiftKey: true }),
    press("d", { metaKey: true, altKey: true }),
    press("r", { shiftKey: true }),
    press("r", { metaKey: true, altKey: true }),
    press("Delete"),
  ])("%j is not ours", (got) => {
    expect(got).toBeNull();
  });
});
