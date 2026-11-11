import { useEffect, useRef, useState } from "react";
import { useLang } from "../i18n";
import { useThemeChoice } from "../shell/theme";
import { captureThumbnail, queueThumbnail, thumbnailMedia, thumbnailSource, thumbnailVariant, type Thumbnail } from "./thumbnails";
interface View { identity: string; source: Thumbnail; signal: AbortSignal; loaded: boolean; release?: (() => void) | undefined }
/** Warm cards are images. Cold generation is hidden, bounded and never blocks opening a card. */
export function Preview({ id, version }: { id: string; version: number }) {
  const lang = useLang();
  const [theme] = useThemeChoice();
  const host = useRef<HTMLSpanElement>(null);
  const [view, setView] = useState<View | null>(null);
  const identity = `${id}/${String(version)}`;
  const current = view?.identity === identity ? view : null;
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    const controller = new AbortController();
    let generation: AbortController | undefined;
    let visible = typeof IntersectionObserver === "undefined";
    let checking = false;
    let attempt = 0;
    let lastAttempt = 0;
    const load = async (force = false) => {
      if (!visible || (checking && !force) || (!force && Date.now() - lastAttempt < 60000)) return;
      const thisAttempt = ++attempt;
      checking = true; lastAttempt = Date.now();
      generation?.abort(); generation = new AbortController();
      const signal = generation.signal;
      const source = await thumbnailSource(id, thumbnailVariant(theme, lang), controller.signal);
      if (thisAttempt !== attempt) return;
      checking = false;
      if (signal.aborted || controller.signal.aborted) return;
      if (!source) { setView(null); return; }
      if (source.url) { setView((old) => old?.identity === identity && old.source.url === source.url ? old : { identity, source, signal, loaded: false }); return; }
      setView(null);
      if (source.source) queueThumbnail(signal, (release) => {
        const timer = window.setTimeout(() => { generation?.abort(); release(); setView((old) => old?.signal === signal ? null : old); }, 30000);
        const done = () => { window.clearTimeout(timer); release(); };
        signal.addEventListener("abort", done, { once: true });
        setView({ identity, source, signal, loaded: false, release: done });
      });
    };
    const intersection = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) { visible = true; void load(); intersection?.disconnect(); }
    }, { rootMargin: "200px" });
    intersection?.observe(element);
    const refresh = () => { void load(true); };
    const focus = () => { void load(); };
    const scheme = thumbnailMedia();
    scheme?.addEventListener("change", refresh);
    window.addEventListener("languagechange", refresh); window.addEventListener("focus", focus);
    void load();
    return () => { controller.abort(); generation?.abort(); intersection?.disconnect(); scheme?.removeEventListener("change", refresh); window.removeEventListener("languagechange", refresh); window.removeEventListener("focus", focus); };
  }, [id, version, identity, theme, lang]);
  return <span className="thumb" data-thumb="">
    <span ref={host} className={current?.loaded ? "thumb-screen loaded" : "thumb-screen"}>
      {current?.source.url && <img src={current.source.url} alt="" aria-hidden="true" decoding="async" onLoad={() => { setView((old) => old && old.source.url === current.source.url ? { ...old, loaded: true } : old); }} onError={() => { setView(null); }} />}
    </span>
    {current?.source.source && <iframe className="thumbnail-generator" data-thumbnail-scheme={current.source.path.includes("scheme=dark") ? "dark" : "light"} src={current.source.source} sandbox="allow-scripts" tabIndex={-1} aria-hidden="true" title="" onLoad={(event) => {
      const source = current.source;
      void captureThumbnail(event.currentTarget, source, current.signal).then((result) => {
        current.release?.();
        if (current.signal.aborted) return;
        setView((old) => old?.signal !== current.signal ? old : result ? { ...old, source: result, loaded: false, release: undefined } : null);
      });
    }} />}
  </span>;
}
