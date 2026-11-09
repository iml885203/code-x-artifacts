const OPEN = "", CLOSE = "";

export function declarations(source: string): string[] {
  const parts: string[] = [];
  let depth = 0, start = 0, quote = "";
  for (let i = 0; i < source.length; i++) {
    const ch = source.charAt(i);
    if (quote) { if (ch === "\\") i++; else if (ch === quote) quote = ""; }
    else if (ch === '"' || ch === "'") quote = ch;
    else if (ch === "(") depth++;
    else if (ch === ")") depth = Math.max(0, depth - 1);
    else if (ch === ";" && depth === 0) { parts.push(source.slice(start, i)); start = i + 1; }
  }
  parts.push(source.slice(start));
  return parts;
}

const property = (part: string) => part.slice(0, part.indexOf(":")).trim().toLowerCase();
const covers = (prop: string, name: string) => prop.startsWith("background-") ? name === "background" : /^border-(top|right|bottom|left)-(width|style|color)$/.test(prop) && ["border", `border-${prop.split("-")[1] ?? ""}`, `border-${prop.split("-")[2] ?? ""}`].includes(name);

export function coveredSlot(source: string, prop: string): boolean {
  const parts = declarations(source);
  return parts.some((part, i) => part.includes(OPEN + prop + CLOSE) && parts.slice(i + 1).some((next) => covers(prop, property(next))));
}

/** Inline important declarations outrank every normal declaration, including later longhands. */
export function cascadeImportant(source: string, prop: string): boolean {
  return declarations(source).some((part) => (property(part) === prop || covers(prop, property(part))) && /!important\s*$/i.test(part));
}

export function needsCascade(source: string): boolean {
  return declarations(source).some((part) => /^(background(?:-[a-z-]+)?|border(?:-(top|right|bottom|left|width|style|color))?)$/.test(property(part)));
}

/** The same rendering boundary as designroom.joinStyle; the room still stores independent property entries. */
export function cascadeSource(source: string, owned: ReadonlyMap<string, string>, overrides: ReadonlyMap<string, string>): string {
  const parts = source === "" ? [] : declarations(source);
  const values = new Map([...owned, ...overrides]);
  const used = new Set<string>();
  const out: string[] = [];
  for (const part of parts) {
    const at = part.indexOf(OPEN), end = part.indexOf(CLOSE);
    if (at < 0 || end < at) { out.push(part); continue; }
    const prop = part.slice(at + OPEN.length, end);
    if (overrides.has(prop)) { out.push(part.slice(0, at) + (owned.get(prop) ?? "")); continue; }
    used.add(prop);
    const value = values.get(prop);
    if (value) out.push(part.slice(0, at) + value);
  }
  for (const [prop, value] of [...values].sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)) {
    if (used.has(prop) || value === "") continue;
    const added = `${prop}:${value}`;
    if (out.at(-1) === "") out.splice(-1, 0, added);
    else out.push(added);
  }
  return out.join(";");
}
