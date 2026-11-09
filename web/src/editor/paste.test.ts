import { describe, expect, it } from "vitest";
import { googleDocsClipboard, webClipboard, wordClipboard } from "./clipboards";
import { outline, para, person, room } from "./testRoom";
import { readyForParsing } from "./view";
import type { EditorView } from "prosemirror-view";

const paste = (html: string): EditorView => {
  const view = person(room(para("b_1", "")));
  view.pasteHTML(html, new Event("paste") as ClipboardEvent);
  return view;
};

/** The first tab with each run's marks spelled out: `p(We ship [b:on Monday])`. */
function marked(view: EditorView): string {
  const out: string[] = [];
  view.state.doc.firstChild?.descendants((node) => {
    if (!node.isTextblock) return true;
    const runs: string[] = [];
    node.forEach((run) => {
      const marks = run.marks.map((m) => (m.type.name === "link" ? `a=${String(m.attrs.href)}` : m.type.name[0])).join(",");
      runs.push(marks === "" ? (run.text ?? "") : `[${marks}:${run.text ?? ""}]`);
    });
    out.push(runs.join(""));
    return false;
  });
  return out.join(" | ");
}

describe("pasting from a web page", () => {
  it("keeps the heading, the emphasis, the link with an allowed address and the lists, and drops everything else", () => {
    const view = paste(webClipboard);
    expect(outline(view)).toBe("h2(Rollout plan) p(We ship on Monday and not before; see the full plan or this.) ul[li[p(Freeze the branch)] li[p(Cut the release) ol[li[p(Tag it)] li[p(Announce it)]]]]");
    expect(marked(view)).toContain("We ship [b:on Monday] and [i:not before]; see the [a=https://example.com/plan?a=1&b=2:full plan] or this.");
    const nested = view.state.doc.toJSON() as { content: { content: { type: string; attrs: Record<string, unknown>; content: { content: { type: string; attrs: Record<string, unknown> }[] }[] }[] }[] };
    expect(nested.content[0]?.content[2]?.content[1]?.content[1]).toMatchObject({ type: "numbered_list", attrs: { start: 3 } });
  });

  it("copies no attribute, style, handler or script, and no address that is not on the list", () => {
    const view = paste(webClipboard);
    const doc = JSON.stringify(view.state.doc.toJSON());
    for (const gone of ["window.stolen", "steal()", "javascript:", "onclick", "onmouseover", "style", "target", "rollout", "data-track"]) expect(doc, gone).not.toContain(gone);
    expect(view.dom.querySelectorAll("script, [style], [onclick], [onmouseover], a[href^=javascript]")).toHaveLength(0);
  });
});

describe("pasting from Word", () => {
  it("gives a heading, a paragraph with bold, italic and a link, and a list, without the markup Word wraps around them", () => {
    const view = paste(wordClipboard);
    expect(outline(view)).toBe("h1(Quarterly review) p(Revenue was up and costs were flat, as in the report.) ul[li[p(Hire two people)] li[p(Close the old office)]] p(\u00a0)");
    expect(marked(view)).toContain("Revenue was [b:up] and costs were [i:flat], as in the [a=https://example.com/report:report].");
    const doc = JSON.stringify(view.state.doc.toJSON());
    for (const gone of ["mso", "MsoNormal", "o:p", "Calibri", "style", "EndFragment"]) expect(doc, gone).not.toContain(gone);
  });
});

describe("pasting from Google Docs", () => {
  it("takes bold and italic from the styles and does not make everything bold because of the wrapper", () => {
    const view = paste(googleDocsClipboard);
    expect(marked(view)).toBe("[b:Bold start][i: then italic] then plain.");
  });
});

describe("a pasted link", () => {
  it.each([
    ["javascript:alert(1)", false],
    ["data:text/html,<script>1</script>", false],
    ["//evil.example/x", false],
    ["https://example.com", true],
    ["mailto:a@b.c", true],
    ["#part", true],
  ])("with the address %s is kept as a link: %s, and its text is kept either way", (href, kept) => {
    const view = paste(`<p>see <a href="${href}">the page</a></p>`);
    expect(view.state.doc.textContent).toBe("see the page");
    expect(view.dom.querySelectorAll("a")).toHaveLength(kept ? 1 : 0);
  });
});

describe("the clipboard's HTML before it is parsed", () => {
  it("has no style or event-handler attribute left to the page's CSP, and keeps the style for the paste rules", () => {
    const ready = readyForParsing('<p onclick="x()" STYLE="color:red" class="a" data-on="1">text onclick="y"</p><o:p></o:p><td onMouseOver = "z()">');
    expect(ready).toBe('<p data-pasted-onclick="x()" data-pasted-STYLE="color:red" class="a" data-on="1">text onclick="y"</p><td data-pasted-onMouseOver = "z()">');
  });
});
