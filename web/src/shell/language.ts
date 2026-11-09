import { useCallback } from "react";
import { readBoot } from "../boot";
import { useReplaceCatalogue } from "../i18n";
import { crossfade } from "./crossfade";

/**
 * Picks the interface language without a reload: the same form post the
 * server's switch takes (it remembers the choice and answers with the page
 * you were on), whose answer carries the new catalogue in its boot block.
 * Returns whether the language changed.
 */
export function useChangeLanguage(): (tag: string) => Promise<boolean> {
  const replace = useReplaceCatalogue();
  return useCallback(
    async (tag) => {
      const next = location.pathname + location.search;
      const res = await fetch("/lang", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ lang: tag, next }),
      });
      if (!res.ok) return false;
      const boot = readBoot(new DOMParser().parseFromString(await res.text(), "text/html"));
      crossfade(() => {
        document.documentElement.lang = boot.lang;
        replace(boot.strings, boot.lang);
      });
      return true;
    },
    [replace],
  );
}
