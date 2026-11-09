// The interface text lives in internal/i18n/locales/ and nowhere else (the keys
// are typed in catalogue.ts). The server sends the language it picked (see boot.ts).
import { createContext, useContext, useMemo, useState, type ReactNode } from "react";
import type { Key } from "./catalogue";

export type { Key };
export type Translate = (key: Key) => string;

/** A missing key shows as the key itself, as on the server, so the page still renders. */
export function translator(strings: Record<string, string>): Translate {
  return (key) => strings[key] ?? key;
}

/** Fills {name} placeholders: the position of a word differs between languages, so a sentence is one catalogue entry. */
export function format(text: string, values: Record<string, string>): string {
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => values[name] ?? whole);
}

const Context = createContext<Translate | null>(null);
const LangContext = createContext("en");
const ReplaceContext = createContext<(strings: Record<string, string>, lang: string) => void>(() => undefined);

/** `strings` and `lang` are the language the server picked; the account menu can swap them without a reload (see useReplaceCatalogue). */
export function I18nProvider({ strings, lang, children }: { strings: Record<string, string>; lang: string; children: ReactNode }) {
  const [current, setCurrent] = useState({ strings, lang });
  const replace = useMemo(() => (s: Record<string, string>, l: string) => { setCurrent({ strings: s, lang: l }); }, []);
  const t = useMemo(() => translator(current.strings), [current.strings]);
  return (
    <ReplaceContext value={replace}>
      <LangContext value={current.lang}>
        <Context value={t}>{children}</Context>
      </LangContext>
    </ReplaceContext>
  );
}

/** Swaps the whole catalogue and the page language: the language switch in the account menu. */
export function useReplaceCatalogue(): (strings: Record<string, string>, lang: string) => void {
  return useContext(ReplaceContext);
}

/** The page language's tag, for what the catalogue cannot say (the wording of a time). */
export function useLang(): string {
  return useContext(LangContext);
}

export function useT(): Translate {
  const t = useContext(Context);
  if (!t) throw new Error("useT outside I18nProvider");
  return t;
}
