// @vitest-environment node
import { describe, expect, it } from "vitest";
import { encodeConfig, readConfig, validConfig } from "./config";

const config = (options: unknown, value: unknown = "One") => ({ options, value });
describe("dropdown configuration", () => {
  it.each([
    [null], ["{}"], [{}], [{ options: ["One"] }], [{ options: ["One"], value: "One", color: "red" }],
    [config([])], [config("One")], [config(["One", "One"])], [config([1])],
    [config([""])], [config([" One"])], [config(["\uFEFFOne"], "\uFEFFOne")], [config(["One "])], [config(["One\nTwo"])],
    [config(["One\tTwo"])], [config(["One\u0000Two"])], [config(["One\u007fTwo"])],
    [config(["One\u0085Two"])], [config(["One\u2028Two"])], [config(["One\u2029Two"])],
    [config(["One"], "Two")], [config(["One"], 1)], [config(["x".repeat(101)])],
    [config(Array.from({ length: 51 }, (_, i) => `Option ${String(i)}`), "Option 0")],
  ])("rejects invalid stored configuration %j", (raw) => {
    expect(validConfig(raw)).toBe(false);
    expect(readConfig(JSON.stringify(raw))).toBeNull();
  });

  it("counts Unicode code points and accepts the inclusive option and length limits", () => {
    const options = ["😀".repeat(100), ...Array.from({ length: 49 }, (_, i) => `Option ${String(i)}`)];
    const good = { options, value: options[0] ?? "" };
    expect(validConfig(good)).toBe(true);
    expect(readConfig(encodeConfig(good))).toEqual(good);
    expect(validConfig({ options: ["😀".repeat(101)], value: "😀".repeat(101) })).toBe(false);
    expect(validConfig({ options: ["王 小明"], value: "王 小明" })).toBe(true);
  });

  it("reads only valid JSON and writes the canonical options-then-value shape", () => {
    expect(readConfig("not json")).toBeNull();
    expect(readConfig({ options: ["One"], value: "One" })).toBeNull();
    const parsed = readConfig('{"value":"One","options":["One"]}');
    expect(parsed).not.toBeNull();
    expect(parsed && encodeConfig(parsed)).toBe('{"options":["One"],"value":"One"}');
  });
});
