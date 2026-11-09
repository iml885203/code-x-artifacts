// The Docs editor, loaded only when someone opens a doc to edit or to watch
// (Viewer imports it on demand, so the gallery's bundle does not carry it).
import { useEffect, useReducer, useRef, useState } from "react";
import { EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import type { Command } from "prosemirror-state";
import { liveTicket, searchPeople, type Artifact, type Person } from "../api";
import { format, useT } from "../i18n";
import { colorOf } from "./colors";
import { readAnchor } from "./comments/anchor";
import { useDocComments, type DocComments } from "./comments/useDocComments";
import "./editor.css";
import type { ToolContext } from "./feature";
import { dropdownWords } from "./features/dropdown";
import { holds } from "./features/mention";
import { useImageAdding } from "./imageAdding";
import { useLinkEditing } from "./linkEditing";
import { LiveProvider, openWebSocket } from "./provider";
import { RoomPresence } from "../presence/RoomPresence";
import { TabList } from "./TabList";
import { schema } from "./schema";
import { addTab, deleteTab, moveTab, renameTab } from "./tabs";
import { UndoKeeper } from "./undoKeeper";
import { useTabs } from "./useTabs";
import { FormatBubble, useTyping } from "./FormatBubble";
import { InsertionMenu } from "./InsertionMenu";
import { InsertRow } from "./InsertRow";
import { saveStateOf, type SaveState } from "./SaveState";
import { RowPopover } from "./RowPopover";
import { usePageWidth } from "./pageWidth";
import { mount } from "./view";

interface Props {
  artifact: Artifact;
  /** The signed-in account: the colour comes from it, and it is what the first awareness says until the server names the person. */
  account: string;
  /** Who may see or edit this changed: read the artifact again. */
  onAccessChanged: () => void;
  /** The person can no longer open the artifact. */
  onDenied: () => void;
  /** The doc's comments, for a person who may comment: absent for anyone else. */
  comments?: DocComments | undefined;
  /** What the room says about being saved, for the page to show beside the title; null when nothing is being edited. */
  onSaveState?: (state: SaveState | null) => void;
}

export function DocEditor({ artifact, account, onAccessChanged, onDenied, comments, onSaveState }: Props) {
  const editable = artifact.canEdit;
  const id = artifact.id;
  const [provider, setProvider] = useState<LiveProvider | null>(null);
  // The latest callbacks, so the provider made once per artifact calls the current ones.
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
  return <Surface provider={provider} editable={editable} artifactId={id} comments={comments} onSaveState={onSaveState} />;
}

/** The state the toolbar draws from before the room's content has arrived. */
const idle = EditorState.create({ schema });

function Surface({ provider, editable, artifactId, comments, onSaveState }: { provider: LiveProvider; editable: boolean; artifactId: string; comments: DocComments | undefined; onSaveState: ((state: SaveState | null) => void) | undefined }) {
  const t = useT();
  const parent = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const [, redraw] = useReducer((n: number) => n + 1, 0);
  const [generation, rebuilt] = useReducer((n: number) => n + 1, 0);
  // What the caret is in, for the toolbar; until the room's content arrives it is an empty document.
  const [editorState, setEditorState] = useState(idle);
  const [inserting, setInserting] = useState(false);

  useEffect(() => provider.onChange(redraw), [provider]);
  useEffect(() => provider.onReset(rebuilt), [provider]);

  const run = (command: Command) => {
    const v = view.current;
    if (!v) return;
    v.focus();
    command(v.state, v.dispatch, v);
  };
  // Which tab is shown is the person's own; it falls back to the first when it is gone (someone deleted it).
  const tabs = useTabs(provider.doc);
  const [chosen, setChosen] = useState("");
  const shown = tabs.find((x) => x.id === chosen)?.id ?? tabs[0]?.id ?? "";
  // The editor puts the caret in a tab it has just made.
  const focusNext = useRef(false);
  const opened = useRef(false);
  // Each tab's undo history, kept while the doc is open so going to another tab and back keeps Undo.
  const [undos] = useState(() => new UndoKeeper());
  const tabIds = tabs.map((x) => x.id).join(",");
  useEffect(() => {
    undos.keepOnly(tabIds === "" ? [] : tabIds.split(","));
  }, [undos, tabIds]);
  useEffect(
    () => () => {
      undos.release();
    },
    [undos],
  );
  const commenting = useDocComments(view, provider.doc, provider.synced, editorState, comments);
  // Opening a thread whose words are in another tab shows that tab; moving to another tab afterwards is the person's own.
  const active = comments?.active ?? null;
  const [openedThread, setOpenedThread] = useState(active);
  if (active !== openedThread) {
    setOpenedThread(active);
    const at = readAnchor(comments?.threads.find((c) => c.id === active)?.anchor)?.tab;
    if (at !== undefined && tabs.some((x) => x.id === at)) setChosen(at);
  }
  const link = useLinkEditing(view, run, editorState, editable);
  const pictures = useImageAdding(view, artifactId);
  // The latest handlers, so the view made once per room calls the current ones.
  const hint = t("editor.emptyHint");
  // Someone mentioned who cannot open the doc: the author is told, with nothing to do about it here (sharing is its own step).
  const [outsider, setOutsider] = useState<Person | null>(null);
  const mention = { label: t("mention.list"), none: t("mention.none"), incomplete: t("mention.incomplete") };
  const dropdown = dropdownWords(t);
  const latest = useRef({ openLink: link.openLink, addImages: pictures.addImages, words: pictures.words, hint, mention, dropdown });
  useEffect(() => {
    latest.current = { openLink: link.openLink, addImages: pictures.addImages, words: pictures.words, hint, mention, dropdown };
  });

  const synced = provider.synced;
  const label = t("editor.document");
  const doc = provider.doc;
  const noTabs = tabs.length === 0;
  const firstTabTitle = t("editor.document");
  // A room with no tab at all (nothing was ever written to it): the first person who may edit it makes the first.
  useEffect(() => {
    if (synced && editable && noTabs) addTab(doc, firstTabTitle);
  }, [synced, editable, noTabs, doc, firstTabTitle]);

  useEffect(() => {
    const host = parent.current;
    if (!host || !synced || (editable && shown === "")) return;
    const live = mount(host, provider.doc, provider.awareness, editable, label, {
      onUpdate: (state: EditorState) => {
        setEditorState(state);
      },
      onInsert: () => { setInserting(true); },
      onLink: () => {
        latest.current.openLink();
      },
      onImages: (files) => {
        latest.current.addImages(files);
      },
      say: (kind) => latest.current.words[kind],
      hint: () => latest.current.hint,
      mention: {
        people: (query) => searchPeople(artifactId, query),
        mentioned: (person) => {
          setOutsider(person.canOpen ? null : person);
        },
        words: () => latest.current.mention,
      },
      dropdown: { words: () => latest.current.dropdown },
      comments: commenting.marking,
    }, shown, editable ? undos.of(provider.doc, shown) : undefined);
    setInserting(false);
    view.current = live.view;
    setEditorState(live.view.state);
    // The caret goes into a tab just made, and into a doc that is still empty when it is opened (a new one).
    if (editable && (focusNext.current || (!opened.current && live.view.state.doc.textContent === ""))) {
      focusNext.current = false;
      live.view.focus();
    }
    opened.current = true;
    return () => {
      view.current = null;
      live.destroy();
    };
    // generation is the room being rebuilt: the same provider holds a new document.

  }, [provider, generation, editable, synced, label, shown, undos, commenting.marking, artifactId]);

  const ctx: ToolContext = { run, openLink: link.openLink, addImages: pictures.addImages };

  const [wide, toggleWide] = usePageWidth();
  const connection = provider.state;
  const saveState = editable ? saveStateOf(connection, provider.saved) : null;
  useEffect(() => {
    onSaveState?.(saveState);
  }, [onSaveState, saveState]);
  useEffect(
    () => () => {
      onSaveState?.(null);
    },
    [onSaveState],
  );
  const typed = useTyping(editorState);
  // The doc's tabs: a menu in the insert row for people who may edit, a list beside the page for readers (who have no row).
  const tabList = (close?: () => void) => (
    <TabList
      tabs={tabs}
      shown={shown}
      editable={editable}
      onShow={(id) => {
        focusNext.current = false;
        setChosen(id);
        close?.();
      }}
      onAdd={() => {
        const made = addTab(doc, t("tabs.untitled"), shown);
        if (made === null) return;
        focusNext.current = true;
        setChosen(made);
        close?.();
      }}
      onRename={(id, title) => renameTab(doc, id, title)}
      onMove={(id, by) => {
        // The tab that moves stays the one shown, wherever it goes.
        setChosen(id);
        moveTab(doc, id, by);
      }}
      onDelete={(id) => {
        const at = tabs.findIndex((x) => x.id === id);
        const next = tabs[at + 1] ?? tabs[at - 1];
        if (deleteTab(doc, id) && next) setChosen(next.id);
      }}
    />
  );

  return (
    <div className={wide ? "ed ed-wide" : "ed"} data-connection={connection}>
      {editable && (
        <InsertRow
          state={editorState}
          ctx={ctx}
          wide={wide}
          onWide={toggleWide}
          tabs={synced && <RowPopover label={t("tabs.label")}>{(close) => tabList(close)}</RowPopover>}
        />
      )}
      {link.overlay}
      <div className="ed-banner ed-connection-banner" hidden={connection === "live"}>
        <RoomPresence awareness={provider.awareness} self={provider.doc.clientID} state={connection} />
        {connection === "offline" && editable && <> · {t("editor.offlineHint")}</>}
      </div>
      {pictures.banners}
      {editable && outsider !== null && holds(editorState.doc, outsider.user) && (
        <p className="ed-banner ed-outsider" role="status">
          {format(t("mention.cannotOpen"), { name: outsider.name })}
          <button
            type="button"
            className="quiet"
            onClick={() => {
              setOutsider(null);
            }}
          >
            {t("mention.dismiss")}
          </button>
        </p>
      )}
      {!editable && <p className="ed-banner">{t("editor.readOnly")}</p>}
      <div className="ed-body">
        {synced && !editable && tabs.length > 1 && tabList()}
        <div className="ed-scroll" ref={scroller} onKeyDownCapture={typed.onKeyDown}>
          <div className="ed-paper" ref={parent} />
          {editable && inserting && <InsertionMenu state={editorState} ctx={ctx} anchor={view}
            onClose={() => { setInserting(false); view.current?.focus(); }}
            onCancel={(query) => { const v = view.current; if (v) v.dispatch(v.state.tr.insertText(`/${query}`)); }} />}
          <FormatBubble
            view={view}
            scroller={scroller}
            state={editorState}
            ctx={ctx}
            editable={editable}
            comment={comments === undefined ? null : { start: commenting.start, anchored: commenting.anchored }}
            typing={typed.typing}
          />
        </div>
      </div>
    </div>
  );
}
