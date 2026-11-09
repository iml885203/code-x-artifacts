import { useEffect, useMemo, useRef, useState, type ComponentProps, type ComponentType } from "react";
import { call, downloadArtifact, getArtifact, saveDownload, setVisibility, type Artifact, type Changes, type CommentSummary, type Visibility } from "../api";
import { useArtifactActions } from "../artifactActions";
import { useArtifacts } from "../artifacts";
import type { Boot } from "../boot";
import { DesignCanvas } from "../design/Canvas";
import { anchorName } from "../design/comments/anchor";
import { SlidesStage } from "../slides/SlidesStage";
import { format, useT } from "../i18n";
import { useRouter } from "../router";
import { useDocumentTitle } from "../title";
import { useToast } from "../Toast";
import { bridgeHost } from "./bridge";
import { useDownloadPrompt } from "./download/DownloadPrompt";
import type { DocEditor as DocEditorType } from "../editor/DocEditor";
import type { BoardExporter, BoardStage as BoardStageType } from "../board/BoardStage";
import { exportFileName, pngFromSvg } from "../board/exportBoard";
import type { SaveState } from "../editor/SaveState";
import { CommentsPanel } from "./CommentsPanel";
import { useComments } from "./useComments";
import { ConfirmDialog } from "./ConfirmDialog";
import { FilesPanel } from "./FilesPanel";
import { useFollowChanges } from "./follow";
import { useRecordOpen } from "./useRecordOpen";
import { scrollKeeper } from "./scroll";
import { TopBar } from "./TopBar";
import { VersionsPanel } from "./VersionsPanel";
import { isAudience } from "./audience";

interface Props {
  boot: Boot;
  artifact: Artifact;
  /** The same trusted viewer host, with only the artifact filling the viewport. */
  fullPage?: boolean;
  initialVersion?: number | null;
}

export function Viewer({ boot, artifact: initial, fullPage = false, initialVersion = null }: Props) {
  const t = useT();
  const router = useRouter();
  const store = useArtifacts();
  const toast = useToast();
  const actions = useArtifactActions();
  const [artifact, setArtifact] = useState(initial);
  // What the artifact is right now, for the undo that runs after this render is gone.
  const live = useRef(initial);
  const apply = (a: Artifact) => {
    live.current = a;
    setArtifact(a);
  };
  const [deleting, setDeleting] = useState(false);
  // One panel beside the page at a time.
  const [panel, setPanel] = useState<"history" | "comments" | "files" | "notes" | null>(null);
  const history = panel === "history";
  // The older version showing in the frame, or null for the newest.
  const [older, setOlder] = useState<number | null>(initialVersion);
  // ?slide=N on the viewer's address opens a slides deck at that slide: the frame's page reads the same parameter.
  const [slide] = useState(() => {
    const n = new URLSearchParams(location.search).get("slide");
    return n !== null && /^[1-9]\d{0,3}$/.test(n) ? n : null;
  });
  const frame = useRef<HTMLIFrameElement>(null);
  // A signed-in person opens a doc straight into the live editor (editors type, everyone else watches the same room read-only).
  // The editor is a chunk of its own, fetched when a doc is opened: the gallery and every other page never load it. Until it
  // has arrived the stage is empty, and if it cannot be had or the person loses access the page's own frame stands in.
  const [Editor, setEditor] = useState<ComponentType<ComponentProps<typeof DocEditorType>> | null>(null);
  const [editorFailed, setEditorFailed] = useState(false);
  // A board opens in its own chunk the same way: Excalidraw is large, and only a board's page loads it.
  const [Board, setBoard] = useState<ComponentType<ComponentProps<typeof BoardStageType>> | null>(null);
  // While a board is open in its editor, its exports are the editor's own (hand-drawn, as drawn).
  const boardExporter = useRef<BoardExporter | null>(null);
  // What the doc open in the editor says about being saved, shown beside the title.
  const [docSave, setDocSave] = useState<SaveState | null>(null);
  const shownKey = `${String(artifact.version)}-${String(older)}`;
  // Editors and the design canvas have no printable page; load the shown version off-screen. A newer version landing
  // while it loads does not take the frame (and the print asked of it) away; the next print loads the newer one.
  const [printFrame, setPrintFrame] = useState<{ key: string; older: number | null } | null>(null);
  const printStale = printFrame !== null && printFrame.older === older && printFrame.key !== shownKey;
  const printWhenLoaded = useRef(false);
  const signedIn = boot.user !== "";
  // The comments are read here because the doc editor draws them (the words marked) as well as the panel; what the editor says of them is kept beside.
  const liveDoc = !fullPage && signedIn && artifact.kind === "doc" && !editorFailed;
  const liveBoard = !fullPage && signedIn && artifact.kind === "board" && !editorFailed;
  const comments = useComments(artifact.id, signedIn && artifact.canComment && (liveDoc || !fullPage && artifact.kind === "design"), artifact.comments.count);
  // The words selected for a new comment, the thread that is open, and the threads whose words are gone from the doc.
  const [draft, setDraft] = useState<Record<string, unknown> | null>(null);
  const [activeThread, setActiveThread] = useState<string | null>(null);
  const [designTargets, setDesignTargets] = useState<Record<string, import("../design/comments/types").CommentTarget>>({});
  const [standings, setStandings] = useState<Record<string, string>>({});
  const changed = useMemo(() => new Set(Object.entries(standings).flatMap(([id, s]) => (s === "changed" ? [id] : []))), [standings]);
  const docThreads = useMemo(() => (comments.load.status === "ready" ? comments.load.list.flatMap((c) => !c.parentId && c.anchor !== undefined ? [{ id: c.id, anchor: c.anchor }] : []) : []), [comments.load]);
  const visibility = isAudience(artifact.visibility) ? artifact.visibility : "private";
  useDocumentTitle(artifact.title);

  // A page that wants to hand over a file asks through this; only the person's click on "Save" lets it.
  const { ask: askToSave, prompt: downloadPrompt } = useDownloadPrompt(artifact.title);

  // Where the page in the frame is scrolled to, kept for the next version's frame.
  // The frame is only read when a message arrives, never while rendering.
  // eslint-disable-next-line react-hooks/refs
  const [keeper] = useState(() => scrollKeeper(() => frame.current?.contentWindow ?? null));
  useEffect(() => {
    const host = bridgeHost(artifact.id, () => frame.current?.contentWindow ?? null, older !== null, askToSave);
    const listener = (e: MessageEvent) => {
      keeper.hear(e);
      void host(e);
    };
    window.addEventListener("message", listener);
    return () => {
      window.removeEventListener("message", listener);
    };
  }, [artifact.id, older, keeper, askToSave]);

  // Opening an artifact shows its newest version, whatever the overview's list
  // held: the frame always loads the newest, and a restore is measured against
  // the version this page shows.
  const id = initial.id;
  useRecordOpen(id, signedIn);
  useEffect(() => {
    if (!signedIn) return;
    let current = true;
    getArtifact(id).then(
      (fresh) => {
        if (!current || fresh.version === live.current.version) return;
        apply(fresh);
        store.replace(fresh);
      },
      (err: unknown) => {
        console.error("read the artifact being opened", err);
      },
    );
    return () => {
      current = false;
    };
    // apply and store.replace only set state, and this runs once per opened artifact.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, signedIn]);

  function accessChanged() {
    getArtifact(id).then(
      (fresh) => {
        apply(fresh);
        store.replace(fresh);
      },
      (err: unknown) => {
        console.error("read the artifact after who may edit it changed", err);
      },
    );
  }

  /** Reads the artifact as the server has it now. */
  function refresh(changes: Changes) {
    getArtifact(id).then(
      (fresh) => {
        if (live.current.id !== fresh.id) return;
        apply(fresh);
        store.replace(fresh);
      },
      (err: unknown) => {
        console.error(`read the artifact after what changed since v${String(changes.version)}`, err);
      },
    );
  }

  // While it is open, the viewer follows the artifact: a version someone else
  // publishes replaces the frame without anyone asking, and a new comment or
  // change of audience or editors refreshes what the header and panels say. Not
  // while an older version is shown: that frame stays what the person picked.
  // What was last seen of the activity is the baseline; the first answer sets it.
  const seenActivity = useRef<string | null>(null);
  useFollowChanges(
    id,
    older === null,
    () => live.current.version,
    (changes) => {
      const activity = `${String(changes.comments.length)}/${String(changes.access.length)}`;
      const baseline = seenActivity.current;
      if (changes.version > live.current.version) {
        seenActivity.current = null;
        keeper.carryOver();
        if (signedIn) refresh(changes);
        else apply({ ...live.current, version: changes.version, title: changes.title, updatedAt: changes.updatedAt });
      } else if (baseline !== null && baseline !== activity) {
        seenActivity.current = activity;
        if (signedIn) refresh(changes);
      } else {
        seenActivity.current = activity;
      }
    },
  );

  // The newest choice wins: an answer to an earlier one must not undo it.
  const latest = useRef(0);
  // The share dialog says what happened itself, so this only answers whether the change was saved.
  function choose(next: Visibility): Promise<boolean> {
    const before = live.current.visibility;
    if (next === before) return Promise.resolve(true);
    const mine = ++latest.current;
    return setVisibility(artifact.id, next).then(
      (saved) => {
        if (mine === latest.current) {
          apply(saved);
          store.replace(saved);
        }
        return true;
      },
      (err: unknown) => {
        console.error("change visibility", err);
        return false;
      },
    );
  }

  useEffect(() => {
    if (!liveDoc || Editor) return;
    let current = true;
    import("../editor/DocEditor").then(
      (loaded) => {
        if (current) setEditor(() => loaded.DocEditor);
      },
      (err: unknown) => {
        console.error("load the editor", err);
        if (!current) return;
        setEditorFailed(true);
        toast({ message: t("editor.loadFailed"), error: true });
      },
    );
    return () => {
      current = false;
    };
    // The toast and the words are the ones of the moment the load fails.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveDoc, Editor]);

  useEffect(() => {
    if (!liveBoard || Board) return;
    let current = true;
    import("../board/BoardStage").then(
      (loaded) => {
        if (current) setBoard(() => loaded.BoardStage);
      },
      (err: unknown) => {
        console.error("load the board", err);
        if (!current) return;
        setEditorFailed(true);
        toast({ message: t("editor.loadFailed"), error: true });
      },
    );
    return () => {
      current = false;
    };
    // The toast and the words are the ones of the moment the load fails.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveBoard, Board]);

  async function remove() {
    await actions.remove(artifact);
    setDeleting(false);
    router.go("/");
  }

  // The page prints itself (its own print stylesheet, no viewer around it); the frame is sandboxed, so it is asked by message.
  // The live editor has no frame, so the page is loaded off-screen for it and asked to print once it is there.
  function printPage() {
    const target = frame.current?.contentWindow;
    if (target && !printStale) {
      target.postMessage({ pictor: "print" }, "*");
      return;
    }
    printWhenLoaded.current = true;
    setPrintFrame({ key: shownKey, older });
  }

  async function exportAs(format: string) {
    try {
      await downloadArtifact(artifact.id, format, older ?? undefined);
    } catch (err) {
      console.error("download", err);
      toast({ message: t("viewer.downloadFailed"), error: true });
    }
  }

  // A deck's filmstrip saved a version, or found a newer one: the frame shows it at once, the header once read.
  function deckSaved(version: number) {
    if (version > live.current.version) apply({ ...live.current, version });
    getArtifact(id).then(
      (fresh) => {
        if (live.current.id !== fresh.id || fresh.version < live.current.version) return;
        apply(fresh);
        store.replace(fresh);
      },
      (err: unknown) => {
        console.error("read the deck after its slides changed", err);
      },
    );
  }

  function commentsChanged(comments: CommentSummary) {
    apply({ ...live.current, comments });
    store.replace(live.current);
  }

  function restored(saved: Artifact, from: number) {
    apply(saved);
    store.replace(saved);
    setOlder(null);
    toast({ message: format(t("versions.restored"), { from: String(from), to: String(saved.version) }) });
  }

  // The export route needs a session, so someone who is not signed in is offered only what the browser makes.
  const exportOptions = (boot.exports[artifact.kind] ?? []).filter((o) => signedIn || o.browser === true);
  const exportAsChosen = (f: string) => {
    if (artifact.kind === "board") void exportBoard(f);
    else if (exportOptions.find((o) => o.format === f)?.browser === true) printPage();
    else void exportAs(f);
  };
  // A board's SVG or PNG: the open editor's, else drawn from the server's SVG of the version shown.
  async function exportBoard(f: string) {
    const editor = older === null ? boardExporter.current : null;
    if (!editor && f === "svg") return exportAs(f);
    try {
      const blob = editor ? await editor(f === "png" ? "png" : "svg") : await pngFromSvg(await (await call("GET", `/api/artifacts/${encodeURIComponent(artifact.id)}/export/svg${older === null ? "" : `?version=${String(older)}`}`)).text());
      saveDownload(blob, exportFileName(artifact.title, artifact.id, f));
    } catch (err) {
      console.error("download", err);
      toast({ message: t("viewer.downloadFailed"), error: true });
    }
  }
  return (
    <div className="viewer">
      {!fullPage && <TopBar
        artifact={artifact}
        boot={boot}
        visibility={visibility}
        commentsOpen={panel === "comments"}
        onComments={() => {
          setPanel(panel === "comments" ? null : "comments");
          setOlder(null);
        }}
        onChanged={(saved) => {
          apply(saved);
          store.replace(saved);
        }}
        onVisibility={(v) => choose(v)}
        env={{
          copyLink: (a) => void actions.copyLink(a),
          copy: (a) => void actions.copy(a),
          togglePin: () => {
            actions.togglePin(live.current, !live.current.pinned, apply);
          },
          openPanel: (p) => {
            setPanel(p);
            if (p !== "history") setOlder(null);
          },
          askDelete: () => {
            setDeleting(true);
          },
        }}
        saveState={docSave}
        exports={exportOptions.map((o) => o.format)}
        onExport={exportAsChosen}
      />}
      <div className="workspace">
        <main className="stage" data-stage>
          {!fullPage && artifact.kind === "design" ? (
            <DesignCanvas artifact={artifact} older={older} account={boot.user} onAccessChanged={accessChanged} comments={artifact.canComment ? {
              threads: docThreads,
              onTargets: setDesignTargets,
              active: activeThread,
              onOpen: (threadId) => { setActiveThread(threadId); setPanel("comments"); },
              onCompose: (anchor) => { setDraft(anchor); setPanel("comments"); },
            } : undefined} />
          ) : !fullPage && artifact.kind === "slides" && older === null ? (
            <SlidesStage
              key={artifact.id}
              artifact={artifact}
              frame={frame}
              sandbox={boot.frameSandbox}
              start={slide === null ? 1 : Number(slide)}
              exports={exportOptions.map((o) => o.format)}
              onExport={exportAsChosen}
              notes={{ open: panel === "notes", onToggle: () => { setPanel(panel === "notes" ? null : "notes"); } }}
              onSaved={deckSaved}
            />
          ) : liveBoard && older === null ? (
            Board && (
              <Board
                artifact={artifact}
                account={boot.user}
                onSaveState={setDocSave}
                onExporter={(exporter) => {
                  boardExporter.current = exporter;
                }}
                onAccessChanged={accessChanged}
                onDenied={() => {
                  setEditorFailed(true);
                  toast({ message: t("editor.lostAccess"), error: true });
                }}
              />
            )
          ) : liveDoc && older === null && !Editor ? null : liveDoc && Editor && older === null ? (
            <Editor
              artifact={artifact}
              account={boot.user}
              onSaveState={setDocSave}
              onAccessChanged={accessChanged}
              onDenied={() => {
                setEditorFailed(true);
                toast({ message: t("editor.lostAccess"), error: true });
              }}
              comments={
                artifact.canComment
                  ? {
                      threads: docThreads,
                      active: activeThread,
                      onOpen: (threadId) => {
                        setActiveThread(threadId);
                        setPanel("comments");
                      },
                      onCompose: (anchor) => {
                        setDraft(anchor);
                        setPanel("comments");
                      },
                      onStandings: setStandings,
                    }
                  : undefined
              }
            />
          ) : (
            /* A new version, or another one shown, is a new frame: the same address would not reload, and an old page must not outlive the switch to a writable bridge. */
            <iframe
              key={shownKey}
              ref={frame}
              data-artifact-frame=""
              src={frameSrc(artifact.id, older, slide)}
              allow="fullscreen"
              allowFullScreen
              title={artifact.title}
              sandbox={boot.frameSandbox}
            />
          )}
        </main>
        {printFrame !== null && printFrame.older === older && (
          <iframe
            key={printFrame.key}
            ref={frame}
            className="print-frame"
            src={frameSrc(artifact.id, printFrame.older, null)}
            title={artifact.title}
            tabIndex={-1}
            aria-hidden="true"
            sandbox={boot.frameSandbox}
            onLoad={() => {
              if (!printWhenLoaded.current) return;
              printWhenLoaded.current = false;
              frame.current?.contentWindow?.postMessage({ pictor: "print" }, "*");
            }}
          />
        )}
        {artifact.canEdit && history && (
          <VersionsPanel
            artifact={artifact}
            shown={older}
            onShow={setOlder}
            onClose={() => {
              setPanel(null);
              setOlder(null);
            }}
            onRestored={restored}
            onCopy={(version) => {
              void actions.copy(artifact, version);
            }}
          />
        )}
        {artifact.canEdit && panel === "files" && (
          <FilesPanel
            artifact={artifact}
            onClose={() => {
              setPanel(null);
            }}
          />
        )}
        {artifact.canComment && panel === "comments" && (
          <CommentsPanel
            artifact={artifact}
            onClose={() => {
              setPanel(null);
            }}
            onSummary={commentsChanged}
            comments={comments}
            draft={draft}
            onDraftDone={() => {
              setDraft(null);
            }}
            active={activeThread}
            onActivate={setActiveThread}
            changed={changed}
            targets={artifact.kind === "design" ? designTargets : undefined}
            anchorLabel={artifact.kind === "design" ? anchorName : undefined}
          />
        )}
      </div>
      {artifact.canChange && (
        <ConfirmDialog
          open={deleting}
          onOpenChange={setDeleting}
          title={t("viewer.confirmDelete")}
          hint={t("viewer.confirmDeleteHint")}
          confirm={t("viewer.deleteConfirm")}
          failure={t("viewer.deleteFailed")}
          danger
          onConfirm={remove}
        />
      )}
      {downloadPrompt}
    </div>
  );
}

/** The address of the artifact's page: the newest version unless one is chosen, opened at a slide when asked. */
function frameSrc(id: string, older: number | null, slide: string | null): string {
  const query = new URLSearchParams();
  if (older !== null) query.set("version", String(older));
  if (slide !== null) query.set("slide", slide);
  const text = query.toString();
  return `/raw/${id}${text === "" ? "" : `?${text}`}`;
}
