// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { ALLOWED_TYPES, MAX_BYTES, MAX_NAME } from "./rules";

// The guides are what an agent learns the limits from; they are Markdown the
// server serves, so the numbers are checked against the rules here.
const guides = ["html_guide.md", "react_guide.md"].map((f) => [f, readFileSync(join(import.meta.dirname, "../../../../internal/kind", f), "utf8")] as const);

describe.each(guides)("the %s", (_name, text) => {
  it("states the size limit, the name limit and every type the viewer allows", () => {
    expect(text).toContain(`${String(MAX_BYTES / 1024 / 1024)} MB`);
    expect(text).toContain(`${String(MAX_NAME)} characters`);
    for (const type of ALLOWED_TYPES) expect(text).toContain(`\`${type}\``);
  });
});
