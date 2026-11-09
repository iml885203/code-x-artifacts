import { useEffect, useState } from "react";
import type { DesignEdits } from "./edits";

export type Tokens = Record<string, string>;
export const TOKEN_PREFIX = "t|";
export const tokenName = /^--[A-Za-z_][A-Za-z0-9_-]{0,79}$/;

/** The room owns each token separately, so edits to different names merge. */
export function readTokens(edits: DesignEdits): Tokens {
  const out: Tokens = {};
  edits.map.forEach((value, key) => {
    if (key.startsWith(TOKEN_PREFIX) && typeof value === "string") out[key.slice(TOKEN_PREFIX.length)] = value;
  });
  return out;
}

export function useTokens(edits: DesignEdits | null): Tokens {
  const [tokens, setTokens] = useState<Tokens>({});
  useEffect(() => {
    const update = () => { setTokens(edits ? readTokens(edits) : {}); };
    update();
    return edits?.onChange(update);
  }, [edits]);
  return tokens;
}

/** Mirrors the format boundary; the server validates every socket write too. */
export function validToken(name: string, value: string): boolean {
  if (!tokenName.test(name) || !value.trim() || new TextEncoder().encode(value).length > 1000 || /[;{}<>\\@!]|\/\*|\*\//.test(value)) return false;
  for (const match of value.matchAll(/([A-Za-z_-][A-Za-z0-9_-]*)\s*\(/g)) {
    if (!/^(calc|min|max|clamp|var|env|rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|color|color-mix)$/i.test(match[1] ?? "")) return false;
  }
  let depth = 0, quote = "";
  for (const c of value) {
    const code=c.charCodeAt(0);
    if (code<32 || (code>=127 && code<=159)) return false;
    if (quote) { if (c === quote) quote = ""; continue; }
    if (c === '"' || c === "'") quote = c;
    else if (c === "(") depth++;
    else if (c === ")" && --depth < 0) return false;
  }
  return depth === 0 && !quote;
}

export function setToken(edits: DesignEdits, name: string, value: string): boolean {
  if (!validToken(name, value)) return false;
  if (edits.map.get(TOKEN_PREFIX + name) !== value) edits.transact((map) => { map.set(TOKEN_PREFIX + name, value); });
  return true;
}
