// A board in the viewer: the Excalidraw editor on the board's live room. It is a
// chunk of its own (Excalidraw is large), loaded only when a board is opened.
import { CaptureUpdateAction, Excalidraw, exportToBlob, exportToSvg, restoreElements } from "@excalidraw/excalidraw";
import type { BinaryFileData, ExcalidrawImperativeAPI, Collaborator, SocketId } from "@excalidraw/excalidraw/types";
import "@excalidraw/excalidraw/index.css";
import { useEffect, useReducer, useRef, useState } from "react";
import { listAssets, liveTicket, uploadAsset, type Artifact } from "../api";
import { colorOf } from "../editor/colors";
import { LiveProvider, openWebSocket } from "../editor/provider";
import { saveStateOf, type SaveState } from "../editor/SaveState";
import { format, useLang, useT } from "../i18n";
import { useToast } from "../Toast";
import { RoomPresence } from "../presence/RoomPresence";
import { bindBoard, type Binding, type Scene } from "./binding";
import { boardFile, picturesToLoad, uploadPicture } from "./files";
import { whenLoaded } from "./loaded";
import { nextZoom, zoomedAt, zoomKeyOf } from "./keys";
import "./board.css";

interface Props {
  artifact: Artifact;
  account: string;
  onAccessChanged: () => void;
  onDenied: () => void;
  onSaveState?: (state: SaveState | null) => void;
  /** Called with what makes this board's SVG or PNG as the editor draws it while it is open, and null when it is not. */
  onExporter?: (exporter: BoardExporter | null) => void;
}

export type BoardExporter = (format: "svg" | "png") => Promise<Blob>;

export function BoardStage({ artifact, account, onAccessChanged, onDenied, onSaveState, onExporter }: Props) {
  const editable = artifact.canEdit;
  const id = artifact.id;
  const [provider, setProvider] = useState<LiveProvider | null>(null);
  const callbacks = useRef({ onAccessChanged, onDenied });
  useEffect(() => {
    callbacks.current = { onAccessChanged, onDenied };
  });
  useEffect(() => {
    const made = new LiveProvider({
      ticket: () => liveTicket(id),
      open: openWebSocket,
      user: { id: account, name: account, color: colorOf(account) },
      readOnly: !editable,
      network: window,
      onAccessChanged: () => {
        callbacks.current.onAccessChanged();
      },
      onDenied: () => {
        callbacks.current.onDenied();
      },
    });
    made.start();
    setProvider(made);
    return () => {
      made.destroy();
      setProvider(null);
    };
  }, [id, account, editable]);
  if (!provider) return null;
  return <Surface artifactId={id} provider={provider} editable={editable} onSaveState={onSaveState} onExporter={onExporter} />;
}

function Surface({ artifactId, provider, editable, onSaveState, onExporter }: { artifactId: string; provider: LiveProvider; editable: boolean; onSaveState: Props["onSaveState"]; onExporter: Props["onExporter"] }) {
  const t = useT();
  const toast = useToast();
  const binding = useRef<Binding | null>(null);
  const [api, setApi] = useState<ExcalidrawImperativeAPI | null>(null);
  const [, redraw] = useReducer((n: number) => n + 1, 0);
  // A room that was thrown away and rebuilt is a new document: the scene starts again from it.
  const [generation, rebuilt] = useReducer((n: number) => n + 1, 0);
  useEffect(() => provider.onChange(redraw), [provider]);
  useEffect(
    () =>
      provider.onReset(() => {
        setApi(null); // the editor is made again; the old one's API is gone with it
        rebuilt();
      }),
    [provider],
  );

  // The editor whose first scene has loaded: a board put on it before would be wiped by that load.
  const [loaded, setLoaded] = useState<ExcalidrawImperativeAPI | null>(null);
  useEffect(() => {
    if (!api) return;
    let current = true;
    void whenLoaded(api).then(() => {
      if (current) setLoaded(api);
    });
    return () => {
      current = false;
    };
  }, [api]);

  const synced = provider.synced;
  useEffect(() => {
    if (!api || loaded !== api || !synced) return;
    binding.current = bindBoard(provider.doc, sceneOf(api), CaptureUpdateAction.NEVER as never, !editable);
    return () => {
      binding.current?.destroy();
      binding.current = null;
    };
  }, [api, loaded, synced, provider, editable, generation]);

  useEffect(() => {
    if (!api) return;
    const show = () => {
      api.updateScene({ collaborators: collaboratorsOf(provider) });
    };
    provider.awareness.on("change", show);
    return () => {
      provider.awareness.off("change", show);
    };
  }, [api, provider]);

  useEffect(() => {
    if (!api) return;
    // Before Excalidraw sees the key: on a board 0 and 1 are zoom keys, not its eraser and selection.
    const onKey = (e: KeyboardEvent) => {
      const asked = zoomKeyOf(e);
      if (!asked) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      if (asked === "fit") {
        api.scrollToContent(undefined, { fitToContent: true });
        return;
      }
      const s = api.getAppState();
      const view = zoomedAt({ zoom: s.zoom.value, scrollX: s.scrollX, scrollY: s.scrollY, width: s.width, height: s.height }, nextZoom(asked, s.zoom.value));
      api.updateScene({ appState: { zoom: { value: view.zoom as typeof s.zoom.value }, scrollX: view.scrollX, scrollY: view.scrollY } });
    };
    window.addEventListener("keydown", onKey, { capture: true });
    return () => {
      window.removeEventListener("keydown", onKey, { capture: true });
    };
  }, [api]);

  useEffect(() => {
    if (!api || !onExporter) return;
    onExporter((format) => exportOf(api, format));
    return () => {
      onExporter(null);
    };
  }, [api, onExporter]);

  // The board's files as last listed, and whether a listing is on its way: a picture someone just added is a file the
  // last listing may not have, so a picture the editor lacks asks again.
  const files = useRef<{ names: Set<string>; asking: boolean }>({ names: new Set(), asking: false });
  const showPictures = (elements: readonly { type?: unknown; fileId?: unknown; isDeleted?: unknown }[]) => {
    if (!api) return;
    const loaded = new Set(Object.keys(api.getFiles()));
    const shown = elements.filter((el) => el.type === "image" && typeof el.fileId === "string" && !loaded.has(el.fileId));
    if (shown.length === 0) return;
    const give = () => {
      const add = picturesToLoad(shown, loaded, files.current.names).map((name) => boardFile(artifactId, name) as unknown as BinaryFileData);
      if (add.length > 0) api.addFiles(add);
    };
    if (shown.every((el) => files.current.names.has(el.fileId as string)) || files.current.asking) {
      give();
      return;
    }
    files.current.asking = true;
    listAssets(artifactId).then(
      (list) => {
        files.current = { names: new Set(list.map((a) => a.name)), asking: false };
        give();
      },
      (err: unknown) => {
        files.current.asking = false;
        console.warn("list the board's files", err);
      },
    );
  };

  const lang = useLang();
  const connection = provider.state;
  const saveState = editable ? saveStateOf(connection, provider.saved) : null;
  useEffect(() => {
    onSaveState?.(saveState);
  }, [onSaveState, saveState]);
  useEffect(() => () => onSaveState?.(null), [onSaveState]);

  return (
    <div className="board" data-connection={connection} data-synced={synced}>
      <div className="ed-banner ed-connection-banner" hidden={connection === "live"}>
        <RoomPresence awareness={provider.awareness} self={provider.doc.clientID} state={connection} />
      </div>
      <div className="board-canvas">
        <Excalidraw
          key={generation}
          excalidrawAPI={setApi}
          viewModeEnabled={!editable}
          langCode={lang === "zh-Hant" ? "zh-TW" : "en"}
          isCollaborating
          aiEnabled={false}
          validateEmbeddable={false}
          onChange={(elements) => {
            binding.current?.onChange(elements);
            showPictures(elements);
          }}
          generateIdForFile={(file) =>
            uploadPicture(file, (f) => uploadAsset(artifactId, f), (reason) => {
              toast({ message: reason === "" ? t("editor.imageRefusedNoReason") : format(t("editor.imageRefused"), { reason }), error: true });
            })
          }
          onPointerUpdate={({ pointer, button }) => {
            provider.awareness.setLocalStateField("pointer", { x: pointer.x, y: pointer.y, tool: pointer.tool });
            provider.awareness.setLocalStateField("button", button);
          }}
          UIOptions={{
            canvasActions: { loadScene: false, saveToActiveFile: false, export: false, saveAsImage: false },
          }}
        />
      </div>
    </div>
  );
}

// The editor's own font (FONT_FAMILY.Excalifont), for a text written without one.
const EXCALIFONT = 5;

/**
 * The editor's API as the binding uses it. A board may hold elements an agent
 * wrote with only the fields it cared about; restoreElements gives each the
 * editor's defaults for the rest (groupIds, angle, seed...), which the editor
 * needs to draw and hit-test it, and a text without a font gets the editor's.
 */
function sceneOf(api: ExcalidrawImperativeAPI): Scene {
  return {
    getSceneElementsIncludingDeleted: () => api.getSceneElementsIncludingDeleted(),
    updateScene: ({ elements, captureUpdate }) => {
      const complete = elements.map((el) => (el.type === "text" && el.fontFamily === undefined ? { ...el, fontFamily: EXCALIFONT } : el));
      api.updateScene({ elements: restoreElements(complete as unknown as Parameters<typeof restoreElements>[0], null, { repairBindings: true }), captureUpdate });
    },
  };
}

/**
 * The board as the editor draws it, on white. A PNG is drawn with the fonts the
 * page has. An SVG names its fonts rather than carrying them: putting them in
 * the file has the editor subset them in WebAssembly, which the site's CSP does
 * not run.
 */
async function exportOf(api: ExcalidrawImperativeAPI, format: "svg" | "png"): Promise<Blob> {
  const scene = { elements: api.getSceneElements(), appState: { ...api.getAppState(), exportBackground: true, viewBackgroundColor: "#ffffff" }, files: api.getFiles() };
  if (format === "png") return exportToBlob({ ...scene, mimeType: "image/png", exportPadding: 16 });
  const svg = await exportToSvg({ ...scene, exportPadding: 16, skipInliningFonts: true });
  return new Blob([svg.outerHTML], { type: "image/svg+xml" });
}

/**
 * Everyone else in the room who has a pointer on the board, by name. Excalidraw
 * colours a collaborator from its id, so the account name as the id gives each
 * person the same colour every time.
 */
function collaboratorsOf(provider: LiveProvider): Map<SocketId, Collaborator> {
  const out = new Map<SocketId, Collaborator>();
  for (const [client, state] of provider.awareness.getStates()) {
    if (client === provider.doc.clientID) continue;
    const pointer = state.pointer as { x?: unknown; y?: unknown; tool?: unknown } | undefined;
    if (typeof pointer?.x !== "number" || typeof pointer.y !== "number") continue;
    const user = state.user as { name?: unknown } | undefined;
    const name = typeof user?.name === "string" ? user.name : null;
    out.set(String(client) as SocketId, {
      pointer: { x: pointer.x, y: pointer.y, tool: pointer.tool === "laser" ? "laser" : "pointer" },
      button: state.button === "down" ? "down" : "up",
      username: name,
      id: name ?? String(client),
    });
  }
  return out;
}
