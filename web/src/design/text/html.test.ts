import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { htmlOfDelta } from "./html";
import type { DeltaOp } from "./units";

// The cases internal/designroom writes its room from and reads it back to: an element's markup and the runs of the text that holds it.
const dir = join(dirname(fileURLToPath(import.meta.url)), "../../../../internal/designroom/testdata/cases");
const cases = readdirSync(dir).filter((f) => f.endsWith(".json")).map((f) => JSON.parse(readFileSync(join(dir, f), "utf8")) as { name: string; body: string; delta: DeltaOp[] });

/** The parsed markup as a browser serialises it, so two ways of writing the same markup are equal. */
function parsed(markup: string): string {
  return new DOMParser().parseFromString(`<!doctype html><body>${markup}`, "text/html").body.innerHTML;
}

describe("the markup of a formatted text", () => {
  it("has the cases", () => {
    expect(cases.length).toBeGreaterThan(2);
  });
  it.each(cases.map((c) => [c.name, c] as const))("is the same markup the server writes for %s", (_name, c) => {
    const inner = c.body.replace(/^<[^>]*>/, "").replace(/<\/[a-z0-9]+>$/i, "");
    expect(parsed(htmlOfDelta(c.delta))).toBe(parsed(inner));
  });

  it("closes and opens a format that ends in the middle of another and escapes what is text", () => {
    const b = '{"t":"b","d":0,"a":[["","data-pid","e1"]]}';
    const i = '{"t":"i","d":1,"a":[["","data-pid","e2"]]}';
    const html = htmlOfDelta([
      { insert: "a<b>&", attributes: { "f:e1": b } },
      { insert: "x", attributes: { "f:e1": b, "f:e2": i } },
      { insert: "y", attributes: { "f:e2": i } },
    ]);
    expect(html).toBe('<b data-pid="e1">a&lt;b&gt;&amp;<i data-pid="e2">x</i></b><i data-pid="e2">y</i>');
  });

  it("writes no format that is not an inline element of text, no event handler and no broken attribute", () => {
    const html = htmlOfDelta([
      { insert: "x", attributes: { "f:e1": '{"t":"script","d":0,"a":[]}', "f:e2": '{"t":"a","d":0,"a":[["","onclick","x"],["","href","#a\\"b"]]}', "f:e3": "not json", "other": "kept out" } },
    ]);
    expect(html).toBe('<a href="#a&quot;b">x</a>');
  });
});
