import type { Tokens } from "../tokens";
import { parseColor, type Rgba } from "./css";

/** Resolve simple token aliases for previews without turning the chosen binding into a literal. */
export function tokenColor(name: string, tokens: Tokens): string | null {
  const visited = new Set<string>();
  let next = name;
  while (!visited.has(next)) {
    visited.add(next);
    const value = tokens[next]?.trim();
    if (!value) return null;
    const alias = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
    if (alias) { next = alias; continue; }
    return parseColor(value) !== null || (typeof CSS !== "undefined" && CSS.supports("color", value)) ? value : null;
  }
  return null;
}

export function paintColor(source: string | undefined, tokens: Tokens): Rgba | null {
  const value = (source ?? "").replace(/\s*!important\s*$/i, "").trim();
  if (value === "transparent") return {r:0,g:0,b:0,a:0};
  const binding = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
  return parseColor(binding ? tokenColor(binding,tokens) ?? "" : value);
}
