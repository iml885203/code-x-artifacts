import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import type { Artifact } from "./api";
import { ArtifactsProvider } from "./artifacts";
import type { Boot } from "./boot";
import { areas } from "./catalogue";
import { I18nProvider } from "./i18n";
import { RouterProvider } from "./router";
import { ToastProvider } from "./Toast";

const here = dirname(fileURLToPath(import.meta.url));

/** One language's catalogue: its area files merged, as the server does at startup. Read from the areas listed in catalogue.ts, not globbed: a glob is cached by Vite and went stale when an area was added. */
export function loadCatalogue(lang: string): Record<string, string> {
  const merged: Record<string, string> = {};
  for (const area of areas) {
    const file = join(here, "../../internal/i18n/locales", lang, `${area}.json`);
    Object.assign(merged, JSON.parse(readFileSync(file, "utf8")) as Record<string, string>);
  }
  return merged;
}

export const catalogues = {
  en: loadCatalogue("en"),
  "zh-Hant": loadCatalogue("zh-Hant"),
};

/** The whole app's providers around a page, with the real catalogue of the language. */
export function renderIn(ui: ReactElement, lang: keyof typeof catalogues = "en", at = "/") {
  return render(
    <I18nProvider strings={catalogues[lang]} lang={lang}>
      <RouterProvider initial={at}>
        <ArtifactsProvider>
          <ToastProvider>{ui}</ToastProvider>
        </ArtifactsProvider>
      </RouterProvider>
    </I18nProvider>,
  );
}

export function artifact(over: Partial<Artifact> & Pick<Artifact, "title" | "author">): Artifact {
  return {
    id: over.title.toLowerCase().replace(/\W+/g, "-").padEnd(18, "0"),
    kind: "html",
    version: 1,
    visibility: "private",
    createdAt: "2026-10-01T08:00:00Z",
    updatedAt: "2026-10-02T09:30:00Z",
    url: "https://pictor.test/a/x",
    canChange: false,
    canEdit: false,
    role: "reader",
    editors: [],
    organisationEdit: false,
    canComment: false,
    pinned: false,
    comments: { count: 0, unread: 0 },
    ...over,
  };
}

export const frameSandbox = "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals";

export function bootFor(lang: keyof typeof catalogues = "en", over: Partial<Boot> = {}): Boot {
  return {
    page: "gallery",
    lang,
    user: "alice",
    initials: "A",
    langs: ["en", "zh-Hant"],
    here: "/",
    strings: catalogues[lang],
    publicLinks: false,
    frameSandbox,
    exports: { html: [{ format: "html" }], doc: [{ format: "md" }] },
    ...over,
  };
}
