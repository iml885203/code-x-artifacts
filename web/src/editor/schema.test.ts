// @vitest-environment node
import type { MarkType, NodeType } from "prosemirror-model";
import { describe, expect, it } from "vitest";
import { schema as shared } from "../../../internal/docroom/interop/schema.mjs";
import { safeHref, safeImageSrc, schema } from "./schema";

describe("the editor's schema", () => {
  it("is the one the Go bridge reads and writes: the same nodes, attributes, content rules and marks in the same order", () => {
    const shape = (s: { nodes: Record<string, NodeType>; marks: Record<string, MarkType> }) => ({
      nodes: Object.entries(s.nodes).map(([name, type]) => [name, type.spec.content ?? null, type.spec.group ?? null, Object.keys(type.spec.attrs ?? {}), type.spec.marks ?? null]),
      marks: Object.entries(s.marks).map(([name, type]) => [name, Object.keys(type.spec.attrs ?? {})]),
    });
    expect(shape(schema)).toEqual(shape(shared));
  });

  it("draws every node and every mark", () => {
    for (const [name, type] of Object.entries(schema.nodes)) {
      if (name !== "text" && name !== "doc") expect(type.spec.toDOM, `node ${name}`).toBeTypeOf("function");
    }
    for (const [name, type] of Object.entries(schema.marks)) expect(type.spec.toDOM, `mark ${name}`).toBeTypeOf("function");
  });

  it.each([
    ["https://example.com/a", "https://example.com/a"],
    ["http://example.com", "http://example.com"],
    ["mailto:a@b.c", "mailto:a@b.c"],
    ["#part", "#part"],
    ["javascript:alert(1)", null],
    [" JaVaScRiPt:alert(1)", null],
    ["data:text/html,<script>", null],
    ["//evil.example/x", null],
    ["file:///etc/passwd", null],
    [42, null],
  ])("links: %j becomes %j", (href, want) => {
    expect(safeHref(href)).toBe(want);
  });

  it.each([
    ["/files/abc/pic.png", "/files/abc/pic.png"],
    ["data:image/png;base64,AAAA", "data:image/png;base64,AAAA"],
    ["https://example.com/x.png", "https://example.com/x.png"],
    ["http://example.com/x.png", null],
    ["https://", null],
    ["//evil.example/x.png", null],
    ["/api/artifacts/abc", null],
    ["/files/abc", null],
    ["/files/abc/a/b.png", null],
    ["/files//b.png", null],
    ["javascript:alert(1)", null],
    ["https://example.com/a b.png", null],
    ["data:image/svg+xml;base64,AAAA", null],
    ["data:text/html;base64,AAAA", null],
    [`data:image/png;base64,${"A".repeat(300_000)}`, null],
  ])("images: %j becomes %j", (src, want) => {
    expect(safeImageSrc(src)).toBe(want);
  });

  it("never draws an address from a link that is not on the list, even when a room holds one", () => {
    const link = schema.marks.link;
    const mark = link?.create({ href: "javascript:alert(1)" });
    const dom = mark && link?.spec.toDOM?.(mark, true);
    expect(dom).toEqual(["span", { class: "ed-link" }, 0]);
  });
});
