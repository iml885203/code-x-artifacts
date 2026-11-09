// A person's colour: one of a few that read on the paper the editor draws on, picked
// from their account so it is the same every time. It travels in awareness as a name
// ("c3"; the server keeps only a short string), and the stylesheet (editor.css) holds
// the values, because the page's CSP does not allow a style attribute.
export const paletteSize = 8;

export function colorOf(account: string): string {
  let h = 0;
  for (const ch of account) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0;
  return `c${String(h % paletteSize)}`;
}

/** Whatever arrives in awareness is another browser's word: only a name from the palette is used. */
export function paletteClass(color: unknown): string {
  const match = typeof color === "string" ? /^c([0-7])$/.exec(color) : null;
  return `ed-${match ? `c${match[1] ?? "0"}` : "c0"}`;
}
