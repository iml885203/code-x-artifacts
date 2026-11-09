import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent } from "react";
import { CommentsLayer } from "./comments/CommentsLayer";
import { useCommentPins } from "./comments/useCommentPins";
import { useCommentFocus } from "./comments/useCommentFocus";
import type { CommentTarget, DesignComments } from "./comments/types";
import { getDesign, type Artifact } from "../api";
import { Artboards } from "./toolbar/Artboards";
import { ArtboardStrip } from "./toolbar/ArtboardStrip";
import { labelLayout } from "./labelLayout";
import { format, useT } from "../i18n";
import { Editor, type Drag, type Rect, type Target } from "./controller";
import { Handles } from "./Handles";
import { SelectionBar } from "./toolbar/SelectionBar";
import { useSelectionMenu } from "./toolbar/useSelectionMenu";
import { anchorAt } from "./comments/anchor";
import { ShellBar } from "./toolbar/ShellBar";
import { ArtboardProperties } from "./pages/ArtboardProperties";
import { usePageLink } from "./pages/pageLinks";
import { FrameDraw } from "./toolbar/FrameDraw";
import { PenDraw } from "./pen/PenDraw";
import { PathDraw } from "./pen/PathDraw";
import { usePenSettings } from "./pen/settings";
import { isShape, type Tool } from "./toolbar/tools";
import { useShellKeys, type Held } from "./toolbar/useShellKeys";
import { LineDraw } from "./toolbar/LineDraw";
import type { Ref } from "./ops/nodes";
import { useBuiltElements } from "./ops/useBuilt";
import { useDesignOps } from "./ops/useDesignOps";
import "./ops/ops.css";
import { FrameLinks } from "./inspect";
import { layoutPage, readDesign, type Design, type DesignFrame } from "./manifest";
import { Play } from "./Play";
import { paletteClass } from "../editor/colors";
import { publishSelection } from "./presence";
import { Outline } from "./Outline";
import { ArtboardList } from "./pages/ArtboardList";
import { PageStructure, useRoomDesign } from "./pages/structure";
import { refusalText } from "./pages/words";
import { useServed } from "./pages/useServed";
import "./pages/pages.css";
import { RoomPresence } from "../presence/RoomPresence";
import { SidePanel } from "./properties/SidePanel";
import { useTextEditing } from "./text/useTextEditing";
import { usePeerBoxes, usePeers } from "./usePeers";
import { useFramesShowRoom, useRoom } from "./useRoom";
import { useLiveFrames } from "./useLiveFrames";
import { keyOf, useSelection, type Selected } from "./useSelection";
import { BAR_BAND, fit, FIT_FLOOR, MIN_ZOOM, panBy, stepZoom, zoomAt, type View } from "./view";

interface Props {
  artifact: Artifact;
  /** The older version shown, or null for the newest. */
  older: number | null;
  /** The signed-in account; with it, someone who may edit the newest version can move, resize and delete. */
  account?: string;
  comments?: DesignComments | undefined;
  onAccessChanged?: () => void;
}

type Load = { status: "loading" } | { status: "failed" } | { status: "ready"; design: Design; base: string; files: string; live: string; version: number };

/** How far the pointer moves before a press on the canvas is a drag. */
const DRAG = 4;

/**
 * A design on a canvas: its frames laid out by page and row, each drawn in the
 * sandbox iframe the server serves it in, with pan and zoom, a page switch and
 * Play. Everyone who can open the design sees the same thing, and a version shown
 * from the history is the same canvas: nothing here changes the design. The
 * frames take no pointer events; the canvas owns the pointer.
 */
export function DesignCanvas({ artifact, older, account, comments, onAccessChanged }: Props) {
  const t = useT();
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const [pageId, setPageId] = usePageLink();
  const [view, setView] = useState<View>({ x: 0, y: 0, z: 1 });
  // After interacting with the canvas, keep its position until the person chooses Fit.
  const moved = useRef(false);
  const [tool, setTool] = useState<Tool>("select");
  const pen = usePenSettings();
  const [panel, setPanel] = useState(false);
  const [space, setSpace] = useState(false);
  const [panning, setPanning] = useState(false);
  const [playing, setPlaying] = useState<DesignFrame | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const playButton = useRef<HTMLElement | null>(null);
  const [pointerFrame, setPointerFrame] = useState<string | null>(null);
  const stripFocused = useRef(false);

  // The conversation with the inspector in each frame, and what it has said about what is selected.
  const [links] = useState(() => new FrameLinks());
  // Everyone signed in who sees the newest version joins its room, to see the others and what they have selected; only an editor gets the means to change it.
  const { edits, live } = useRoom(artifact, account ?? "", older === null && !!account, onAccessChanged);
  // The file of each frame on the canvas, for asking the room what a frame's elements are.
  const fileOfFrame = useRef(new Map<string, string>());
  // A click selects an element the room holds (not an inline element of formatted text); a frame the room knows nothing of yet has no such distinction.
  const isElement = useCallback((frameId: string, pid: string) => {
    const file = fileOfFrame.current.get(frameId);
    const state = edits && file !== undefined ? edits.frame(file) : null;
    return !state || state.alive.size === 0 || state.alive.has(pid);
  }, [edits]);
  const { selected, selectedFrame, chooseFrame, reports, pick, choose, clear, rerun } = useSelection(links, edits && older === null ? isElement : undefined);
  const editable = artifact.canEdit && older === null;
  const editor = useMemo(() => (edits && editable ? new Editor(edits, links) : null), [edits, editable, links]);
  // Where the selected boxes are while a gesture is under way, by selection key; null when none is.
  const [preview, setPreview] = useState<Record<string, Rect> | null>(null);
  const [notice, setNotice] = useState("");
  const armTool = useCallback(
    (next: Tool) => {
      setTool(next);
      setNotice("");
    },
    [],
  );
  useEffect(() => {
    const listener = (e: MessageEvent) => {
      links.hear(e);
    };
    window.addEventListener("message", listener);
    return () => {
      window.removeEventListener("message", listener);
      links.close();
    };
  }, [links]);
  // The view the page was last fitted at, and whether the page on screen has been fitted yet.
  const fitZoom = useRef(1);
  const fitted = useRef<string | null>(null);

  useEffect(() => {
    let current = true;
    getDesign(artifact.id, older ?? undefined).then(
      (data) => {
        const design = readDesign(data.manifest);
        if (!current) return;
        setLoad(design ? { status: "ready", design, base: data.base, files: data.files, live: data.live ?? "", version: data.version } : { status: "failed" });
      },
      (err: unknown) => {
        console.error("read the design", err);
        if (current) setLoad({ status: "failed" });
      },
    );
    return () => {
      current = false;
    };
    // A new version of the artifact, or another one chosen from the history, is read again.
  }, [artifact.id, artifact.version, older]);

  // The newest version is shown as the room holds it (pages and frames change live); an older one as it was saved.
  const roomDesign = useRoomDesign(older === null ? edits : null);
  const savedDesign = load.status === "ready" ? load.design : null;
  const design = roomDesign ?? savedDesign;
  useEffect(() => {
    fileOfFrame.current = new Map(design?.pages.flatMap((p) => p.rows.flat().map((f) => [f.id, f.file] as const)) ?? []);
  }, [design]);
  // A frame added since the newest save is not a file of any version: the server draws it from the room, once it has the person's change.
  const savedFiles = useMemo(() => new Set(savedDesign?.pages.flatMap((p) => p.rows.flat().map((f) => f.file))), [savedDesign]);
  const structure = useMemo(() => (edits && editable ? new PageStructure(edits, () => savedFiles) : null), [edits, editable, savedFiles]);
  const page = design ? (design.pages.find((p) => p.id === pageId) ?? design.pages[0]) : undefined;
  const layout = page ? layoutPage(page) : null;
  const base = load.status === "ready" ? load.base : "";
  // What is put into a frame names the artifact's files the way the frame can load them.
  const files = load.status === "ready" ? load.files : "";
  useEffect(() => {
    links.setFiles(`/files/${artifact.id}/`, files);
  }, [links, artifact.id, files]);
  // Someone in the room sees the frames as the room holds them, and a new version does not load them again.
  const liveBase = older === null && load.status === "ready" ? load.live : "";
  // A frame no version holds yet is only at the live address: it is not loaded until the server has it from the room.
  const roomBase = liveBase !== "" ? liveBase : base;
  // Until the saved version is read there is no address to ask for a frame at: a relative one would go to the page's own folder (a 404).
  const fresh = layout && older === null && load.status === "ready" ? layout.frames.filter(({ frame }) => !savedFiles.has(frame.file)).map(({ frame }) => roomBase + frame.file) : [];
  const served = useServed(fresh);
  const hasFile = (file: string) => load.status === "ready" && (older !== null || savedFiles.has(file) || served.has(roomBase + file));
  const shownFrames = layout?.frames.filter(({ frame }) => hasFile(frame.file)).map(({ frame }) => ({ id: frame.id, file: frame.file }));
  const readLive = useCallback(() => getDesign(artifact.id).then((data) => data.live ?? ""), [artifact.id]);
  const liveFrames = useLiveFrames(edits, liveBase, shownFrames?.map((f) => f.file) ?? [], readLive);
  const commentPins = useCommentPins(comments?.threads ?? [], layout?.frames ?? [], links, { edits: older === null && roomDesign ? edits : null, frames: design?.pages.flatMap((p) => p.rows.flat()) ?? [] });
  const onCommentTargets = comments?.onTargets;
  const commentTargetKey = JSON.stringify(commentPins.targets);
  useEffect(() => { onCommentTargets?.(JSON.parse(commentTargetKey) as Record<string, CommentTarget>); }, [onCommentTargets, commentTargetKey]);
  const peers = usePeers(live);
  const { boxes: peerBoxes, refresh: refreshPeers } = usePeerBoxes(links, peers, shownFrames?.map((f) => f.id) ?? []);
  // Before the hook that shows the room's styles and order: that one only touches elements the frame has.
  useBuiltElements(edits, links, shownFrames ?? []);
  const afterShown = () => {
    void rerun();
    refreshPeers();
    commentPins.refresh();
  };
  useFramesShowRoom(edits, links, shownFrames ?? [], afterShown);
  const text = useTextEditing({ edits, editable: editor !== null, links, frames: shownFrames ?? [], shown: afterShown });
  const awareness = live?.awareness;
  useEffect(() => {
    if (awareness) publishSelection(awareness, selected);
  }, [awareness, selected]);

  /** The selected elements as the editor works on them. */
  const targets = (): Target[] =>
    selected.flatMap((s) => {
      const report = reports[keyOf(s)];
      const file = shownFrames?.find((f) => f.id === s.frameId)?.file;
      return report && file ? [{ frameId: s.frameId, file, pid: s.pid, report }] : [];
    });
  // What an operation made is selected once the frame has it.
  const frameList = useRef<{ id: string; file: string }[]>([]);
  useEffect(() => {
    frameList.current = shownFrames ?? [];
  });
  const selectRefs = useCallback(
    async (refs: Ref[]) => {
      moved.current = true;
      let chosen: Selected[] = [];
      for (const r of refs) {
        const frameId = frameList.current.find((f) => f.file === r.file)?.id;
        if (!frameId) continue;
        const hit = { frameId, pid: r.pid };
        await choose(hit, chosen.length > 0, chosen);
        chosen = [...chosen, hit];
      }
    },
    [choose],
  );
  const ops = useDesignOps({ artifactId: artifact.id, edits, editable, targets, frames: shownFrames ?? [], select: (refs) => void selectRefs(refs), say: setNotice });
  const removeSelection = () => {
    if (selectedFrame && structure) setNotice(refusalText(t, structure.deleteFrame(selectedFrame)) ?? "");
    else if (editor) { editor.remove(targets()); clear(); }
  };
  const context = useSelectionMenu(editor !== null && (!!selectedFrame || selected.length > 0), removeSelection);
  const held = useRef<Held>({ edits, editor, targets, selected, ops, structure, tool });
  useEffect(() => {
    held.current = { edits, editor, targets, selected, ops, structure, tool };
  });

  const size = useCallback(() => {
    const r = viewport.current?.getBoundingClientRect();
    return { w: r && r.width > 0 ? r.width : 800, h: r && r.height > 0 ? r.height : 600 };
  }, []);

  const moveView = useCallback((change: (v: View) => View) => {
    moved.current = true;
    setView(change);
  }, []);

  useCommentFocus(comments, design, commentPins.pins, setPageId, moveView, size);

  // The size of the canvas the view was last fitted to.
  const fittedTo = useRef<{ w: number; h: number } | null>(null);
  const fitPage = useCallback(() => {
    if (!layout) return;
    const { w, h } = size();
    const bars = viewport.current?.parentElement?.querySelector(".canvas-bars");
    const top = Math.max(BAR_BAND, (bars?.getBoundingClientRect().height ?? 0) + 32);
    const next = fit(layout.width, layout.height, w, h, top);
    fitZoom.current = next.z;
    fittedTo.current = { w, h };
    moved.current = false;
    setView(next);
  }, [layout, size]);

  // The canvas changes size when the outline joins it or the window resizes; a page nobody has moved is fitted to what is left.
  // Checked in every commit, before it is painted, so the page never shows off centre; the observer catches a resize that no commit brings.
  const refitIfResized = () => {
    const was = fittedTo.current;
    const { w, h } = size();
    if (was && !moved.current && (was.w !== w || was.h !== h)) fitPage();
  };
  const refit = useRef(refitIfResized);
  useLayoutEffect(() => {
    refit.current = refitIfResized;
    refitIfResized();
  });
  useEffect(() => {
    const el = viewport.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const watch = new ResizeObserver(() => {
      refit.current();
    });
    watch.observe(el);
    return () => {
      watch.disconnect();
    };
  }, [load.status]);

  // A page is fitted when it first appears, and again when another page is chosen; a new version of the same page keeps where the reader is.
  const shownPage = page?.id ?? null;
  useLayoutEffect(() => {
    if (shownPage === null || fitted.current === shownPage) return;
    fitted.current = shownPage;
    fitPage();
  }, [shownPage, fitPage]);

  // What was selected belongs to the page and the version it was selected on; in the room, the frames outlast versions.
  const shownBase = liveBase !== "" ? "live" : load.status === "ready" ? load.base : "";
  const frameKey = shownFrames?.map((f) => f.id).join(" ") ?? "";
  useEffect(() => {
    clear();
  }, [shownPage, shownBase, frameKey, clear]);

  const floor = () => Math.min(MIN_ZOOM, Math.max(FIT_FLOOR, fitZoom.current));
  const zoomTo = (z: number) => {
    const { w, h } = size();
    moveView((v) => zoomAt(v, z, w / 2, h / 2, floor()));
  };

  // The wheel pans, and with Ctrl or Command (a pinch on a trackpad) it zooms at the pointer; a listener of its own because React's are passive.
  useEffect(() => {
    const el = viewport.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      if (e.ctrlKey || e.metaKey) moveView((v) => zoomAt(v, v.z * Math.exp(-e.deltaY * 0.01), e.clientX - r.left, e.clientY - r.top, floor()));
      else moveView((v) => panBy(v, -e.deltaX, -e.deltaY));
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      el.removeEventListener("wheel", onWheel);
    };
    // floor reads a ref.

  }, [load.status, moveView]);

  useShellKeys(playing !== null, held, {
    armTool,
    zoomStep: (direction) => { moveView((v) => zoomAt(v, stepZoom(v.z, direction), size().w / 2, size().h / 2, floor())); },
    zoomTo,
    fit: fitPage,
    setTool,
    clear,
    clearNotice: () => { setNotice(""); },
    removeFrame: () => {
      if (!structure || !selectedFrame) return false;
      setNotice(refusalText(t, structure.deleteFrame(selectedFrame)) ?? "");
      return true;
    },
    holdSpace: setSpace,
    togglePanel: () => { setPanel((open) => !open); },
  });

  // A press that moves is a drag of the page, in either tool; with the hand tool, or Space held, a press is only a drag.
  const press = useRef<{ x: number; y: number; moved: boolean; last: { x: number; y: number }; grab: boolean; drag: Drag | null } | null>(null);
  /** Whether a press at the screen point is on a selected element, which a drag then moves instead of the page. */
  const onSelected = (clientX: number, clientY: number, el: HTMLElement) => {
    if (!layout) return false;
    const r = el.getBoundingClientRect();
    const wx = (clientX - r.left - view.x) / view.z;
    const wy = (clientY - r.top - view.y) / view.z;
    return targets().some((t) => {
      const at = layout.frames.find((p) => p.frame.id === t.frameId);
      const b = t.report.rect;
      return !!at && wx >= at.x + b.x && wx <= at.x + b.x + b.w && wy >= at.y + b.y && wy <= at.y + b.y + b.h;
    });
  };
  function onPointerDown(e: ReactPointerEvent<HTMLDivElement>) {
    if (e.button !== 0 && e.button !== 1) return;
    if (e.target instanceof Element && e.target.closest("button")) return;
    moved.current = true;
    const grab = editor !== null && e.button === 0 && tool === "select" && !space && !e.shiftKey && onSelected(e.clientX, e.clientY, e.currentTarget);
    press.current = { x: e.clientX, y: e.clientY, moved: false, last: { x: e.clientX, y: e.clientY }, grab, drag: null };
    e.currentTarget.setPointerCapture(e.pointerId);
    if (tool === "hand" || space || e.button === 1) setPanning(true);
  }
  function onPointerMove(e: ReactPointerEvent<HTMLDivElement>) {
    const p = press.current;
    if (!p) return;
    if (!p.moved && Math.hypot(e.clientX - p.x, e.clientY - p.y) < DRAG) return;
    if (p.grab && editor) {
      if (!p.moved) {
        p.moved = true;
        const { drag, still } = editor.move(targets());
        p.drag = drag;
        setNotice(still.length === 0 ? "" : still.length === 1 ? format(t("design.stillOne"), { tag: still[0]?.report.tag ?? "" }) : format(t("design.stillMany"), { count: String(still.length) }));
      }
      if (p.drag) setPreview(p.drag.update((e.clientX - p.x) / view.z, (e.clientY - p.y) / view.z));
      return;
    }
    p.moved = true;
    setPanning(true);
    const dx = e.clientX - p.last.x;
    const dy = e.clientY - p.last.y;
    p.last = { x: e.clientX, y: e.clientY };
    moveView((v) => panBy(v, dx, dy));
  }
  function onPointerUp(e: ReactPointerEvent<HTMLDivElement>) {
    const p = press.current;
    press.current = null;
    setPanning(false);
    if (p?.drag) {
      if (e.type === "pointerup") p.drag.end();
      else p.drag.cancel();
      setPreview(null);
      void rerun();
    }
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    // A press that did not move, with an insert tool armed, places a new element where the pointer is.
    if (p && !p.moved && e.type === "pointerup" && e.button === 0 && isShape(tool) && !space && layout && ops) {
      const r = e.currentTarget.getBoundingClientRect();
      const wx = (e.clientX - r.left - view.x) / view.z;
      const wy = (e.clientY - r.top - view.y) / view.z;
      const on = layout.frames.find(({ frame, x, y }) => hasFile(frame.file) && wx >= x && wy >= y && wx < x + frame.w && wy < y + frame.h);
      if (on) {
        ops.place(tool, on.frame.id, { x: wx - on.x, y: wy - on.y });
        setTool("select");
      }
      return;
    }
    // A press that did not move, with the select tool, picks what is under the pointer.
    if (!p || p.moved || e.type !== "pointerup" || e.button !== 0 || tool !== "select" || space || !layout) return;
    const under = frameAt(e.clientX, e.clientY, e.currentTarget);
    if (under) void pick(under.frame.id, under.x, under.y, e.shiftKey, selected);
    else if (!e.shiftKey) clear();
  }
  /** The frame at a screen point and where that is in the frame's own pixels. */
  function frameAt(clientX: number, clientY: number, el: HTMLElement) {
    if (!layout) return null;
    const r = el.getBoundingClientRect();
    const wx = (clientX - r.left - view.x) / view.z;
    const wy = (clientY - r.top - view.y) / view.z;
    const under = layout.frames.find(({ frame, x, y }) => hasFile(frame.file) && wx >= x && wy >= y && wx < x + frame.w && wy < y + frame.h);
    return under ? { frame: under.frame, x: wx - under.x, y: wy - under.y } : null;
  }
  function onDoubleClick(e: ReactMouseEvent<HTMLDivElement>) {
    if (!text.begin || e.button !== 0 || tool !== "select" || space || e.target instanceof Element && e.target.closest("button")) return;
    const under = frameAt(e.clientX, e.clientY, e.currentTarget);
    if (under) void text.begin(under.frame.id, under.frame.file, under.x, under.y);
  }

  if (load.status === "loading") return <div className="canvas" aria-busy="true" />;
  if (load.status === "failed" || !design || !page || !layout) {
    return (
      <div className="canvas">
        <p className="error canvas-error" role="alert">
          {t("design.loadFailed")}
        </p>
      </div>
    );
  }

  const percent = Math.round(view.z * 100);
  const labels = labelLayout(view.z, layout.frames.map(({ frame }) => frame.w));
  const selectedArtboard = layout.frames.find(({ frame }) => frame.id === selectedFrame)?.frame;
  const activeFrame = [selectedFrame, pointerFrame, selected[selected.length - 1]?.frameId].find((id) => id && layout.frames.some(({ frame }) => frame.id === id)) ?? layout.frames[0]?.frame.id ?? "";
  const present = (button: HTMLElement) => {
    const first = layout.frames.find(({ frame }) => hasFile(frame.file));
    if (!first) return;
    playButton.current = button;
    setPlaying(first.frame);
  };
  /** A frame drawn on the page is added at the size dragged (the usual size for a click), and the page is fitted again with it. */
  const addFrame = (size: { w: number; h: number } | null) => {
    if (!structure) return;
    const out = structure.addFrame(page.id, (n) => format(t("structure.newFrame"), { n: String(n) }), undefined, size ?? undefined);
    const why = refusalText(t, out);
    if (why) {
      setNotice(why);
      return;
    }
    if (out.kind === "changed") fitted.current = null;
    armTool("select");
  };
  const world: CSSProperties & Record<"--z", string> = { transform: `translate(${String(view.x)}px, ${String(view.y)}px) scale(${String(view.z)})`, "--z": String(view.z) };
  const grabbing = panning ? "grabbing" : tool === "hand" || space ? "grab" : isShape(tool) ? "crosshair" : "default";
  return (
    <div className="canvas" data-canvas data-comments={comments ? "" : undefined}>

      <div className="canvas-main">
      <div
        ref={viewport}
        className="canvas-viewport"
        style={{ cursor: grabbing }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onDoubleClick={onDoubleClick}
        onContextMenu={context.open}
      >
        <div className="canvas-world" style={world}>
          <Artboards frames={layout.frames} labels={labels} selected={selectedFrame} active={activeFrame} editing={text.active?.frameId} hasFile={hasFile} liveFrames={liveFrames} base={base} links={links}
            onSelect={structure ? (id) => { moved.current = true; chooseFrame(id); } : undefined}
            onExpand={(frame, button) => { playButton.current = button; setPlaying(frame); }}
            onHover={(id) => { if (!stripFocused.current) setPointerFrame(id); }} />
          {selected.map((s) => {
            const report = reports[keyOf(s)];
            const at = layout.frames.find((p) => p.frame.id === s.frameId);
            if (!report || !at) return null;
            const box = preview?.[keyOf(s)] ?? report.rect;
            return <div key={keyOf(s)} className="selbox" data-selection={s.pid} style={{ left: at.x + box.x, top: at.y + box.y, width: box.w, height: box.h }} />;
          })}
          {peerBoxes.map((b) => {
            const at = layout.frames.find((p) => p.frame.id === b.frameId);
            if (!at) return null;
            return (
              <div key={b.key} className={`peerbox ${paletteClass(b.color)}${b.agent ? " peerbox-agent" : ""}`} data-peer={b.name} style={{ left: at.x + b.rect.x, top: at.y + b.rect.y, width: b.rect.w, height: b.rect.h }}>
                <span className="peer-tag">{b.name}</span>
              </div>
            );
          })}
          {editor && selected.length === 1 && (
            <Handles
              target={targets()[0]}
              origin={layout.frames.find((p) => p.frame.id === selected[0]?.frameId)}
              zoom={view.z}
              preview={preview}
              editor={editor}
              onPreview={setPreview}
              onDone={() => void rerun()}
            />
          )}
        </div>
        {comments && !playing && <CommentsLayer comments={comments} frames={layout.frames} pins={commentPins.pins} selected={!ops && !panel ? selected : []} reports={reports} view={view} />}
      </div>
      {tool === "frame" && structure && !playing && <FrameDraw zoom={view.z} onDraw={addFrame} />}
      <LineDraw arrow={tool === "arrow"} active={(tool === "line" || tool === "arrow") && !playing} editable={editor !== null} edits={edits} frames={layout.frames} view={view} style={pen.style} onCreated={(ref) => { void selectRefs([ref]); armTool("select"); }} />
      <PathDraw active={tool === "pen" && !playing} editable={editor !== null} edits={edits} frames={layout.frames} view={view} style={pen.style} labels={{ draw: t("toolbar.pathDraw"), path: t("toolbar.path") }} onCreated={(ref) => { void selectRefs([ref]); armTool("select"); }} />
      <PenDraw active={tool === "pencil" && !playing} editable={editor !== null} edits={edits} frames={layout.frames} view={view} style={pen.style} />
      <ShellBar
        tool={tool}
        onTool={armTool}
        page={{ pages: design.pages, active: page.id, onSelect: setPageId, structure }}
        onPicture={editable ? (file) => void ops?.picture(file) : null}
        frames={editable}
        ready={ops !== null}
        onPresent={layout.frames.some(({ frame }) => hasFile(frame.file)) ? present : null}
        panel={{ open: panel, onToggle: () => { setPanel((open) => !open); } }}
        zoom={{ percent, onStep: (direction) => { zoomTo(stepZoom(view.z, direction)); }, onReset: () => { zoomTo(1); }, onFit: fitPage, onPreset: zoomTo }}
        live={
          live && (
            <div className="canvas-presence" hidden={live.state === "live"}>
              <RoomPresence awareness={live.awareness} self={live.self} state={live.state} />
            </div>
          )
        }
      />
      {labels.actions === "dock" && <ArtboardStrip frames={layout.frames.map(({ frame }) => frame)} active={activeFrame} onActive={(id) => { setPointerFrame(id); if (structure) { moved.current = true; chooseFrame(id); } }} onExpand={(frame, button) => { playButton.current = button; setPlaying(frame); }} onFocusWithin={(inside) => { stripFocused.current = inside; }} />}
      {ops && edits && selected.length > 0 && !playing && !panel && <SelectionBar edits={edits} targets={targets()} ops={ops} onProperties={() => { setPanel(true); }} onDelete={removeSelection} onComment={comments ? () => {
        const target = targets().at(-1), frame = layout.frames.find(({ frame }) => frame.id === target?.frameId)?.frame;
        if (target && frame) {
          const name = target.report.attrs.find(([key]) => key === "data-name")?.[1] ?? frame.name;
          comments.onCompose(anchorAt(frame, { x: target.report.rect.x + target.report.rect.w, y: target.report.rect.y }, target.pid, name));
        }
      } : undefined} />}
      {context.surface}
      {editor && (
        <div className="canvas-status" role="status">
          {notice || text.notice}
        </div>
      )}
      {playing && (
        <Play
          edits={older === null ? edits : null}
          frame={playing}
          src={roomBase + playing.file}
          onExit={() => {
            setPlaying(null);
            playButton.current?.focus();
          }}
        />
      )}
      </div>
      {!playing && panel && (
        <SidePanel report={reports[keyOf(selected[selected.length - 1] ?? { frameId: "", pid: "" })]} count={selected.length} onClose={() => { setPanel(false); }}
          layers={edits ? (<Outline
          edits={edits}
          frames={layout.frames.map(({ frame }) => ({ id: frame.id, name: frame.name, file: frame.file }))}
          selected={selected}
          editable={editor !== null}
          frameList={
            <ArtboardList
              page={page}
              structure={structure}
              selected={selectedFrame}
              onSelect={(id) => { moved.current = true; chooseFrame(id); }}
              onAdded={() => {
                // The page is fitted again, with the new frame in it.
                fitted.current = null;
              }}
            />
          }
          onSelect={(hit, additive) => { moved.current = true; void choose(hit, additive, selected); }}
        />) : undefined}
          artboard={selectedArtboard ? <ArtboardProperties frame={selectedArtboard} structure={structure} /> : undefined} {...(editor && edits ? { edit: { edits, targets: targets(), artifactId: artifact.id } } : {})} />
      )}
    </div>
  );
}
