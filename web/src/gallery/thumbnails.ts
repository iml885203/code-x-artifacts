import { call } from "../api";
export interface Thumbnail { key: string; url?: string | undefined; source?: string | undefined; path: string }
export type ThumbnailViewport = "mobile" | "tablet" | "desktop";
export const thumbnailDimensions = { mobile: { width: 420, height: 270 }, tablet: { width: 980, height: 630 }, desktop: { width: 1400, height: 900 } } as const;
export function thumbnailMedia(query = "(prefers-color-scheme: dark)"): MediaQueryList | null { return typeof window.matchMedia === "function" ? window.matchMedia(query) : null; }
export function thumbnailViewport(): ThumbnailViewport {
  if (thumbnailMedia("(max-width: 640px)")?.matches ?? window.innerWidth <= 640) return "mobile";
  return (thumbnailMedia("(max-width: 1024px)")?.matches ?? window.innerWidth <= 1024) ? "tablet" : "desktop";
}
export function thumbnailVariant(theme: "system" | "light" | "dark", lang: string): string {
  const scheme = theme !== "system" ? theme : (thumbnailMedia()?.matches ? "dark" : "light");
  return `scheme=${scheme}&lang=${lang.startsWith("zh") ? "zh-TW" : "en"}&viewport=${thumbnailViewport()}`;
}
export async function thumbnailSource(id: string, variant: string, signal: AbortSignal): Promise<Thumbnail | null> {
  const path = `/api/artifacts/${encodeURIComponent(id)}/thumbnail?${variant}`;
  try {
    const data = await (await call("GET", path, undefined, signal)).json() as Partial<Thumbnail>;
    if (!data.key || !/^[a-f0-9]{64}$/.test(data.key)) return null;
    if (data.url && !data.url.startsWith(`/thumbnail/${encodeURIComponent(id)}?`)) return null;
    if (data.source && data.source !== `/raw/${encodeURIComponent(id)}?preview&${variant}`) return null;
    return { key: data.key, url: data.url, source: data.source, path };
  } catch { return null; }
}
// Bound visible cold documents separately from expensive bitmap captures.
function thumbnailQueue(limit: number) {
  const pending: (() => void)[] = [];
  let running = 0;
  return (signal: AbortSignal, start: (release: () => void) => void): void => {
    const run = () => {
      if (signal.aborted) return;
      running++;
      let released = false;
      const release = () => { if (released) return; released = true; running--; signal.removeEventListener("abort", release); drain(); };
      signal.addEventListener("abort", release, { once: true });
      start(release);
    };
    const drain = () => { while (running < limit && pending.length) pending.shift()?.(); };
    pending.push(run); drain();
  };
}
export const queueThumbnail = thumbnailQueue(6);
export const queueThumbnailCapture = thumbnailQueue(2);
export async function captureThumbnail(frame: HTMLIFrameElement, source: Thumbnail, signal: AbortSignal): Promise<Thumbnail | null> {
  const target = frame.contentWindow;
  if (!target || signal.aborted) return null;
  const request = crypto.randomUUID();
  return new Promise((resolve) => {
    const stop = (result: Thumbnail | null) => { clearTimeout(timer); window.removeEventListener("message", receive); signal.removeEventListener("abort", abort); resolve(result); };
    const abort = () => { stop(null); };
    const receive = (event: MessageEvent<unknown>) => {
      if (event.source !== target || event.origin !== "null" || typeof event.data !== "object" || !event.data) return;
      const data = event.data as Record<string, unknown>;
      if (data.pictorPreview !== "thumbnail" || data.request !== request) return;
      window.removeEventListener("message", receive);
      if (typeof data.image !== "string" || data.image.length > 1_400_000 || !/^data:image\/jpeg;base64,[A-Za-z0-9+/]+=*$/.test(data.image)) { stop(null); return; }
      let binary: string;
      try { binary = atob(data.image.slice("data:image/jpeg;base64,".length)); } catch { stop(null); return; }
      const body = Uint8Array.from(binary, (char) => char.charCodeAt(0));
      void fetch(`${source.path}&key=${source.key}`, { method: "POST", headers: { "X-Pictor": "1", "Content-Type": "image/jpeg" }, body, signal }).then(async (response) => {
        if (!response.ok) { stop(null); return; }
        const result = await thumbnailSource(source.path.split("/")[3] ?? "", source.path.split("?")[1] ?? "", signal);
        stop(result?.url ? result : null);
      }).catch(() => { stop(null); });
    };
    const timer = window.setTimeout(() => { stop(null); }, 25000);
    signal.addEventListener("abort", abort, { once: true });
    window.addEventListener("message", receive);
    const viewport = new URLSearchParams(source.path.split("?")[1]).get("viewport");
    const size = thumbnailDimensions[viewport === "mobile" || viewport === "tablet" ? viewport : "desktop"];
    target.postMessage({ pictorPreview: "thumbnail-capture", request, ...size, outputWidth: 1120, outputHeight: 720 }, "*");
  });
}
