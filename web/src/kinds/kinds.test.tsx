import { expectHint } from "../test-tooltip";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import registered from "../registered-kinds.json";
import { I18nProvider } from "../i18n";
import { catalogues } from "../test-support";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { displayedKinds, displayOf, kindsTable, reservedKinds } from "./kinds";
import { KindMark, type KindMarkSize } from "./KindMark";

const style = readFileSync(resolve(process.cwd(), "../internal/web/static/style.css"), "utf8");
const word = (lang: keyof typeof catalogues, key: string) => catalogues[lang][key] ?? `missing ${key}`;
const sizes: KindMarkSize[] = ["chip", "square", "bar"];
const inLang = (lang: keyof typeof catalogues, ui: React.ReactElement) => render(<I18nProvider strings={catalogues[lang]} lang={lang}>{ui}</I18nProvider>);

describe("which displayed kind each artifact kind is", () => {
  it("maps every registered kind to a displayed one, html and react both to the web page", () => {
    for (const kind of registered) expect(displayedKinds, kind).toContain(displayOf(kind));
    expect(displayOf("html")).toBe("web");
    expect(displayOf("react")).toBe("web");
    expect(new Set(registered.map(displayOf)).size).toBe(5);
  });

  it("shows a kind it does not know as a web page", () => {
    expect(displayOf("hologram")).toBe("web");
  });
});

describe("the look of each kind", () => {
  const all = [...displayedKinds, ...reservedKinds];

  it("has an icon, a label and a colour pair in both themes for every kind (their contrast is held by tokens.test.ts)", () => {
    expect(all).toHaveLength(8);
    for (const id of all) {
      const k = kindsTable[id];
      expect(k.icon, id).toBeTypeOf("function");
      expect(k.label, id).toMatch(/^kinds\./);
      for (const part of ["bg", "fg"]) expect(style, `${id} ${part}`).toMatch(new RegExp(`--kind-${k.token}-${part}: light-dark\\(#`));
    }
  });

  it("draws every kind's icon differently", () => {
    const shapes = all.map((id) => inLang("en", <KindMark kind={id} />).container.querySelector("svg")?.innerHTML);
    expect(shapes.every((s) => s !== undefined && s !== "")).toBe(true);
    expect(new Set(shapes).size).toBe(all.length);
  });

  it("marks the reserved kinds as reserved and the others not", () => {
    expect(reservedKinds.map((id) => kindsTable[id].reserved)).toEqual([true, true, true]);
    expect(displayedKinds.map((id) => kindsTable[id].reserved)).toEqual([false, false, false, false, false]);
  });
});

describe("<KindMark>", () => {
  it.each(sizes.flatMap((size) => (["en", "zh-Hant"] as const).map((lang) => [size, lang] as const)))("names the kind in its tooltip and accessible name and writes no word, at size %s in %s", async (size, lang) => {
    inLang(lang, <KindMark kind="slides" size={size} />);
    const name = word(lang, "kinds.slides");
    const mark = screen.getByRole("img", { name });
    await expectHint(mark, name);
    expect(mark.textContent).toBe("");
  });

  it("names an unknown kind as a web page", () => {
    inLang("en", <KindMark kind="hologram" />);
    expect(screen.getByRole("img", { name: word("en", "kinds.web") })).toBeTruthy();
  });
});
