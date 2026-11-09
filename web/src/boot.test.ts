import { describe, expect, it } from "vitest";
import { readBoot } from "./boot";
import { translator } from "./i18n";

function docWith(json: string): Document {
  const doc = document.implementation.createHTMLDocument("");
  const block = doc.createElement("script");
  block.type = "application/json";
  block.id = "pictor-boot";
  block.textContent = json;
  doc.body.append(block);
  return doc;
}

describe("readBoot", () => {
  it("reads the page, the viewer and the strings the server sent", () => {
    const boot = readBoot(docWith('{"page":"gallery","lang":"zh-Hant","user":"alice","initials":"A","langs":["en","zh-Hant"],"here":"/","strings":{"nav.signOut":"Token"},"publicLinks":true,"frameSandbox":"allow-scripts","exports":{"html":[{"format":"html"}]}}'));
    expect(boot).toEqual({ page: "gallery", lang: "zh-Hant", user: "alice", initials: "A", langs: ["en", "zh-Hant"], here: "/", strings: { "nav.signOut": "Token" }, publicLinks: true, frameSandbox: "allow-scripts", exports: { html: [{ format: "html" }] } });
  });

  it.each([
    ["no block", document.implementation.createHTMLDocument("")],
    ["a block that is not the contract", docWith('{"page":"gallery"}')],
  ])("refuses %s instead of rendering a half-working page", (_name, doc) => {
    expect(() => readBoot(doc)).toThrow();
  });
});

describe("translator", () => {
  it("shows a missing key as the key, so the page still renders", () => {
    expect(translator({})("nav.signOut")).toBe("nav.signOut");
  });
});
