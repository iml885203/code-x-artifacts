// @vitest-environment node
import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { areas } from "./catalogue";
import source from "./catalogue.ts?raw";
import { catalogues, loadCatalogue } from "./test-support";

const here = dirname(fileURLToPath(import.meta.url));

const filesOf = (lang: string) =>
  readdirSync(join(here, "../../internal/i18n/locales", lang))
    .filter((f) => f.endsWith(".json"))
    .map((f) => f.replace(/\.json$/, ""))
    .sort();

describe("the catalogue's area list", () => {
  it("names every area file of every language, and every listed area has its file", () => {
    const listed = [...areas].sort();
    expect(listed.length).toBeGreaterThan(5);
    for (const lang of ["en", "zh-Hant"]) expect(filesOf(lang), lang).toEqual(listed);
  });

  it("types every listed area for the key type", () => {
    const typed = [...source.matchAll(/locales\/en\/([\w-]+)\.json/g)].map((m) => m[1]).sort();
    expect(typed).toEqual([...areas].sort());
  });

  it("has the same keys per area in every language", () => {
    expect(Object.keys(catalogues["zh-Hant"]).sort()).toEqual(Object.keys(catalogues.en).sort());
    for (const area of areas) {
      const keys = (lang: string) => Object.keys(loadCatalogue(lang)).filter((k) => k.startsWith(`${area}.`)).sort();
      expect(keys("zh-Hant"), area).toEqual(keys("en"));
    }
  });
});
