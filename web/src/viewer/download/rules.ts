// What a page may hand the viewer to save (window.pictor.download). The page is
// not trusted, so every limit is decided here, in the viewer, from what the
// page sent; nothing the page says makes a limit looser. The guides for html
// and react pages state the same limits (rules.test.ts reads them).

/** The largest file a page may hand over, in bytes. */
export const MAX_BYTES = 10 * 1024 * 1024;
/** The longest file name, in UTF-16 units, extension included. */
export const MAX_NAME = 100;

/** The types a page may save, and the extensions each one's file may carry (the first is the one added). */
const EXTENSIONS: Record<string, readonly string[]> = {
  "text/plain": ["txt", "text", "log"],
  "text/csv": ["csv"],
  "application/json": ["json"],
  "text/markdown": ["md", "markdown"],
  "application/pdf": ["pdf"],
  "image/png": ["png"],
  "image/jpeg": ["jpg", "jpeg"],
  "image/gif": ["gif"],
  "image/webp": ["webp"],
  "application/zip": ["zip"],
};
export const ALLOWED_TYPES = Object.keys(EXTENSIONS);

export interface Accepted {
  ok: true;
  /** The name the file is saved under, cleaned. */
  name: string;
  /** The type without parameters, lower case. */
  type: string;
  /** In bytes, as saved. */
  size: number;
  data: string | Uint8Array;
}
export interface Refused {
  ok: false;
  /** Says what is wrong and what is allowed; it goes to the page as the promise's error. */
  error: string;
}

// Path separators, control characters, what Windows refuses in a name, and the
// marks that flip the reading direction (they can make "gnp.‮txt" read as "txt.png").
// eslint-disable-next-line no-control-regex
const FORBIDDEN = /[\u0000-\u001F\u007F-\u009F<>:"|?*‎‏‪-‮⁦-⁩]/g;

function cut(text: string, budget: number): string {
  let out = "";
  for (const ch of text) {
    if (out.length + ch.length > budget) break;
    out += ch;
  }
  return out;
}

/** The name to save under: no path, no hidden file, no control characters, within the limit, and ending in an extension that fits `type`. */
export function cleanName(raw: string, type: string): string {
  const exts = EXTENSIONS[type] ?? [];
  let name = (raw.split(/[/\\]/).pop() ?? "").replace(FORBIDDEN, "").trim();
  name = name.replace(/^[.\s]+/, "").replace(/[.\s]+$/, "");
  if (name === "") name = "download";
  const dot = name.lastIndexOf(".");
  const fits = dot > 0 && exts.includes(name.slice(dot + 1).toLowerCase());
  // A name with a dot in it is not told apart from a name with an extension: when
  // the extension is not the type's, the right one is added after whatever is
  // there, so nothing the page wrote is lost.
  const suffix = fits ? name.slice(dot) : `.${exts[0] ?? ""}`;
  const stem = fits ? name.slice(0, dot) : name;
  return cut(stem, MAX_NAME - suffix.length) + suffix;
}

function bytesOf(data: unknown): string | Uint8Array | undefined {
  if (typeof data === "string") return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  if (ArrayBuffer.isView(data)) return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
  return undefined;
}

const allowed = (): string => `Allowed types: ${ALLOWED_TYPES.join(", ")}.`;
const limit = (): string => `The most a page can save is ${String(MAX_BYTES / 1024 / 1024)} MB.`;

/** Decides whether the viewer may even ask the person about this request. */
export function checkDownload(request: unknown): Accepted | Refused {
  if (typeof request !== "object" || request === null) return { ok: false, error: `pictor.download takes {name, type, data}. ${allowed()} ${limit()}` };
  const r = request as { name?: unknown; type?: unknown; data?: unknown };
  if (typeof r.name !== "string") return { ok: false, error: "pictor.download needs a name for the file, as text." };
  const type = typeof r.type === "string" ? (r.type.split(";")[0] ?? "").trim().toLowerCase() : "";
  if (!Object.hasOwn(EXTENSIONS, type)) return { ok: false, error: `This type of file cannot be saved from a page: "${typeof r.type === "string" ? r.type : ""}". ${allowed()}` };
  const data = bytesOf(r.data);
  if (data === undefined) return { ok: false, error: "pictor.download needs data as text, an ArrayBuffer or a typed array." };
  const size = typeof data === "string" ? new TextEncoder().encode(data).length : data.byteLength;
  if (size > MAX_BYTES) return { ok: false, error: `The file is ${String(size)} bytes, over the limit. ${limit()}` };
  return { ok: true, name: cleanName(r.name, type), type, size, data };
}
