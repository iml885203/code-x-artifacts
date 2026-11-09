// Replace only the platform's generated stylesheet; authored declarations keep their var() bindings.
export const showTokens = (tokens) => {
  if (!tokens || typeof tokens !== "object" || Array.isArray(tokens)) return null;
  const entries = Object.entries(tokens);
  if (entries.some(([name, value]) => !/^--[A-Za-z_][A-Za-z0-9_-]{0,79}$/.test(name) || typeof value !== "string" || /[;{}<>\\@!\x00-\x1f\x7f]/.test(value))) return null;
  let sheet = document.querySelector("style[data-pictor-tokens]");
  if (!sheet) {
    sheet = document.createElement("style");
    sheet.setAttribute("data-pictor-tokens", "");
    document.head.append(sheet);
  }
  sheet.textContent = ":root:root{" + entries.map(([name, value]) => name + ":" + value + ";").join("") + "}";
  return { ok: true };
};
