import { ApiError, SignedOutError, call } from "../api";

export interface PreviewSource {
  src: string;
  path: string;
  key?: string;
}

/** Some test/embedded environments have no media-query API. */
export function previewMedia(query: string): MediaQueryList | null {
  return typeof window.matchMedia === "function" ? window.matchMedia(query) : null;
}

/** The server validates current access, version, assets and grant lifetime before a hit. */
export async function previewSource(id: string, width: number, height: number, signal: AbortSignal): Promise<PreviewSource | null> {
  const scheme = previewMedia("(prefers-color-scheme: dark)")?.matches ? "dark" : "light";
  const motion = previewMedia("(prefers-reduced-motion: reduce)")?.matches ? "reduce" : "no-preference";
  const locale = navigator.language || "en";
  const dpr = Math.max(0.5, Math.min(4, window.devicePixelRatio || 1));
  const path = `/api/artifacts/${encodeURIComponent(id)}/preview?w=${String(width)}&h=${String(height)}&scheme=${scheme}&motion=${motion}&lang=${encodeURIComponent(locale)}&dpr=${String(dpr)}`;
  try {
    const data: unknown = await (await call("GET", path, undefined, signal)).json();
    if (typeof data !== "object" || data === null || !("key" in data) || typeof data.key !== "string" || !/^[a-f0-9]{64}$/.test(data.key)) throw new Error("invalid preview metadata");
    const url = "url" in data ? data.url : undefined;
    if (url !== undefined && (typeof url !== "string" || !url.startsWith(`/preview/${encodeURIComponent(id)}?`))) throw new Error("invalid preview address");
    return { path, key: data.key, src: typeof url === "string" ? url : `/raw/${encodeURIComponent(id)}?preview` };
  } catch (error) {
    if (signal.aborted || error instanceof SignedOutError || (error instanceof ApiError && (error.status === 403 || error.status === 404))) return null;
    // A cache outage must not remove a currently accessible artifact's normal preview.
    return { path, src: `/raw/${encodeURIComponent(id)}?preview` };
  }
}

/** Only this opaque frame may supply its own bounded, inert snapshot. No API is exposed to it. */
export function capturePreview(frame: HTMLIFrameElement, source: PreviewSource, signal: AbortSignal): void {
  if (!source.key || source.src.startsWith("/preview/") || signal.aborted || !frame.contentWindow) return;
  const target = frame.contentWindow;
  const request = crypto.randomUUID();
  const stop = () => {
    window.removeEventListener("message", receive);
    signal.removeEventListener("abort", stop);
    window.clearTimeout(timer);
  };
  const receive = (event: MessageEvent<unknown>) => {
    if (event.source !== target || event.origin !== "null" || typeof event.data !== "object" || event.data === null) return;
    const data = event.data as Record<string, unknown>;
    if (data.pictorPreview !== "snapshot" || data.request !== request) return;
    stop();
    if (signal.aborted || typeof data.html !== "string" || data.html.length === 0 || data.html.length > (2 << 20)) return;
    // The server treats HTML as untrusted and serves it without script execution.
    void call("POST", source.path, { key: source.key, html: data.html }, signal).catch(() => undefined);
  };
  const timer = window.setTimeout(stop, 20000);
  window.addEventListener("message", receive);
  signal.addEventListener("abort", stop, { once: true });
  target.postMessage({ pictorPreview: "capture", request }, "*");
}
