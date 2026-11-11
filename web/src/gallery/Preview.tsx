import { useEffect, useRef, useState } from "react";
import { capturePreview, previewMedia, previewSource, type PreviewSource } from "./previewCache";

interface View { identity: string; source: PreviewSource; signal: AbortSignal; loaded: boolean; frameKey: string }

/** Cold previews run normally; ready snapshots skip the artifact's scripts on subsequent visits. */
export function Preview({ id, version }: { id: string; version: number }) {
  const host = useRef<HTMLSpanElement>(null);
  const [view, setView] = useState<View | null>(null);
  const identity = `${id}/${String(version)}`;
  const current = view?.identity === identity ? view : null;

  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let visible = typeof IntersectionObserver === "undefined";
    let size = "";
    let request: AbortController | undefined;
    const lifetime = new AbortController();
    let resizeTimer: number | undefined;
    const load = () => {
      if (!visible) return;
      const box = element.getBoundingClientRect();
      const width = Math.max(240, Math.min(2000, Math.round(box.width * 2) || 640));
      const height = Math.max(120, Math.min(2000, Math.round(box.height * 2) || 400));
      const dimensions = `${String(width)}/${String(height)}`;
      if (dimensions === size) return;
      size = dimensions;
      request?.abort();
      const controller = new AbortController();
      request = controller;
      void previewSource(id, width, height, controller.signal).then((source) => {
        if (controller.signal.aborted) return;
        if (!source) { setView(null); return; }
        const frameKey = `${identity}/${dimensions}/${source.key ?? "live"}`;
        setView((previous) => previous?.frameKey === frameKey && previous.source.src === source.src
          ? previous
          : { identity, source, signal: lifetime.signal, loaded: false, frameKey });
      });
    };
    const intersection = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) { visible = true; load(); intersection?.disconnect(); }
    }, { rootMargin: "200px" });
    intersection?.observe(element);
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(load, 150);
    });
    resize?.observe(element);
    const revalidate = () => { size = ""; load(); };
    const scheme = previewMedia("(prefers-color-scheme: dark)");
    const motion = previewMedia("(prefers-reduced-motion: reduce)");
    scheme?.addEventListener("change", revalidate);
    motion?.addEventListener("change", revalidate);
    window.addEventListener("languagechange", revalidate);
    window.addEventListener("focus", revalidate);
    load();
    return () => { request?.abort(); lifetime.abort(); window.clearTimeout(resizeTimer); intersection?.disconnect(); resize?.disconnect(); window.removeEventListener("focus", revalidate); window.removeEventListener("languagechange", revalidate); scheme?.removeEventListener("change", revalidate); motion?.removeEventListener("change", revalidate); };
  }, [id, version, identity]);

  return <span className="thumb" data-thumb="">
    <span ref={host} className={current?.loaded ? "thumb-screen loaded" : "thumb-screen"}>
      <iframe key={current?.frameKey ?? identity} src={current?.source.src} sandbox="allow-scripts" loading="lazy" tabIndex={-1} aria-hidden="true" title=""
        onLoad={(event) => {
          if (!current) return;
          setView((value) => value?.frameKey === current.frameKey ? { ...value, loaded: true } : value);
          capturePreview(event.currentTarget, current.source, current.signal);
        }} />
    </span>
  </span>;
}
