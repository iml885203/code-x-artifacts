// Split only at declaration boundaries; quoted data URLs and function arguments may contain semicolons.
export const declarations = (text) => {
  const parts = [];
  let start = 0, depth = 0, quote = "", comment = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i], next = text[i + 1];
    if (comment) { if (ch === "*" && next === "/") { comment = false; i++; } continue; }
    if (ch === "\\") { i++; continue; }
    if (quote) { if (ch === quote) quote = ""; continue; }
    if (ch === "/" && next === "*") { comment = true; i++; continue; }
    if (ch === '"' || ch === "'") quote = ch;
    else if (ch === "(") depth++;
    else if (ch === ")") depth--;
    else if (ch === ";" && depth === 0) { parts.push(text.slice(start, i + 1)); start = i + 1; }
  }
  if (start < text.length) parts.push(text.slice(start));
  return parts;
};

export const authoredStyle = (source, overrides) => {
  if (!overrides.size) return source;
  const base = declarations(source ?? "").filter((part) => {
    const name = /^\s*(?:\/\*[\s\S]*?\*\/\s*)*([-\w]+)\s*:/.exec(part)?.[1]?.toLowerCase();
    return !name || !overrides.has(name);
  }).join("");
  const suffix = [...overrides].sort(([a], [b]) => a.localeCompare(b)).filter(([, value]) => value !== "").map(([prop, value]) => `${prop}:${value};`).join("");
  return base + (base.trim() && !base.trimEnd().endsWith(";") && suffix ? ";" : "") + suffix;
};
