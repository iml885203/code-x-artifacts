import { useEffect, useRef, useState } from "react";
import { useLang } from "../i18n";
import { useThemeChoice } from "../shell/theme";
import { captureThumbnail, queueThumbnail, queueThumbnailCapture, thumbnailMedia, thumbnailSource, thumbnailVariant, type Thumbnail } from "./thumbnails";
interface View { identity: string; source: Thumbnail; signal: AbortSignal; loaded: boolean; pending?: Thumbnail | undefined; deadline?: ((ms: number) => void) | undefined; release?: (() => void) | undefined }
/** Warm cards are images. Cold documents are visible while bounded generation runs and never blocks opening a card. */
export function Preview({ id, version }: { id: string; version: number }) {
  const lang = useLang();
  const [theme] = useThemeChoice();
  const captures = useRef(new WeakSet<HTMLIFrameElement>());
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
        let timer: number;
        const deadline = (ms: number) => {
          window.clearTimeout(timer);
          timer = window.setTimeout(() => { generation?.abort(); release(); setView((old) => old?.signal === signal ? null : old); }, ms);
        };
        deadline(15000);
        const done = () => { window.clearTimeout(timer); release(); };
        signal.addEventListener("abort", done, { once: true });
        setView({ identity, source, signal, loaded: false, deadline, release: done });
      });
    };
    const intersection = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) { visible = true; void load(); intersection?.disconnect(); }
    }, { rootMargin: "200px" });
    intersection?.observe(element);
    const refresh = () => { void load(true); };
    const focus = () => { void load(); };
    const media = [thumbnailMedia(), thumbnailMedia("(max-width: 640px)"), thumbnailMedia("(max-width: 1024px)")];
    media.forEach((query) => { query?.addEventListener("change", refresh); });
    window.addEventListener("languagechange", refresh); window.addEventListener("focus", focus);
    void load();
    return () => { controller.abort(); generation?.abort(); intersection?.disconnect(); media.forEach((query) => { query?.removeEventListener("change", refresh); }); window.removeEventListener("languagechange", refresh); window.removeEventListener("focus", focus); };
  }, [id, version, identity, theme, lang]);
  const image = current?.pending ?? (current?.source.url ? current.source : null);
  const viewport = new URLSearchParams(current?.source.path.split("?")[1]).get("viewport") ?? "desktop";
  return <span className="thumb" data-thumb="">
    <span ref={host} className={current?.loaded ? "thumb-screen loaded" : "thumb-screen"}>
      {image?.url && <img src={image.url} alt="" aria-hidden="true" decoding="async" onLoad={() => {
        if (!current) return;
        current.release?.();
        setView((old) => old?.signal === current.signal ? { ...old, source: image, pending: undefined, loaded: true, deadline: undefined, release: undefined } : old);
      }} onError={() => { current?.release?.(); setView(null); }} />}
      {current?.source.source && <iframe key={`${identity}/${current.source.key}`} className="thumbnail-generator" data-thumbnail-viewport={viewport} data-thumbnail-scheme={current.source.path.includes("scheme=dark") ? "dark" : "light"} src={current.source.source} sandbox="allow-scripts" tabIndex={-1} aria-hidden="true" title="" onLoad={(event) => {
        const frame = event.currentTarget;
        setView((old) => old?.signal === current.signal ? { ...old, loaded: true } : old);
        if (captures.current.has(frame)) return;
        captures.current.add(frame);
        current.deadline?.(60000);
        queueThumbnailCapture(current.signal, (releaseCapture) => {
          current.deadline?.(30000);
          void captureThumbnail(frame, current.source, current.signal).then((result) => {
            releaseCapture();
            if (current.signal.aborted) return;
            if (!result) current.release?.();
            setView((old) => old?.signal !== current.signal ? old : result ? { ...old, pending: result } : null);
          });
        });
      }} />}
    </span>
  </span>;
}
