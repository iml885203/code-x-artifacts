import { useRef, useState, type RefObject } from "react";
import type { Artifact } from "../api";
import { format, useT } from "../i18n";
import { useToast } from "../Toast";
import { ConfirmDialog } from "../viewer/ConfirmDialog";
import { DeckBar, type Zoom } from "./DeckBar";
import { DeckFooter, type FooterEdits } from "./DeckFooter";
import { NotesPanel } from "./NotesPanel";
import { SlideFields, type Wanted } from "./SlideFields";
import { SlideTools } from "./SlideTools";
import { Thumbnails, type ThumbnailEdits } from "./Thumbnails";
import { refusal, useDeck, type DeckEdit } from "./useDeck";
import { useSlideEdit } from "./useSlideEdit";
import { useStage } from "./useStage";
import "./deck.css";

interface Props {
  artifact: Artifact;
  /** The viewer's frame: the bridge answers it and Export, PDF asks it to print. */
  frame: RefObject<HTMLIFrameElement | null>;
  sandbox: string;
  /** The slide the address names (?slide=N), else the first. */
  start: number;
  exports: string[];
  onExport: (format: string) => void;
  notes: { open: boolean; onToggle: () => void };
  /** A version this page saved, or a newer one it found: the viewer shows it. */
  onSaved: (version: number) => void;
}

/**
 * A deck in the viewer: the stage with the slide on it, the floating bar, and the filmstrip. The stage is the deck's
 * own page in the server's sandbox (`?embed`: its controls show only while presenting); the shell only tells it which
 * slide to show and hears which one is showing. Editors add, duplicate, move and delete slides through the slide
 * routes, each with the version the filmstrip shows, and change what is on the selected slide in place (SlideFields,
 * SlideTools); readers get the same filmstrip to move around in.
 */
export function SlidesStage({ artifact, frame, sandbox, start, exports, onExport, notes, onSaved }: Props) {
  const t = useT();
  const toast = useToast();
  const deck = useDeck(artifact.id, artifact.version, start, onSaved);
  const [view, setView] = useState<"single" | "grid">("single");
  const [zoom, setZoom] = useState<Zoom>("fit");
  // The slide the person is asked about, as the filmstrip showed it then.
  const [deleting, setDeleting] = useState<{ n: number; base: number; title: string } | null>(null);
  const area = useRef<HTMLDivElement>(null);
  const stage = useStage(area, frame, deck.selected, deck.select, zoom);
  const slides = deck.list?.slides ?? [];
  const editing = artifact.canEdit && view === "single";
  const content = useSlideEdit(artifact.id, deck, editing);
  const [want, setWant] = useState<Wanted | null>(null);
  const count = slides.length;

  function run(e: DeckEdit) {
    deck.edit(e).catch((err: unknown) => {
      console.error("change the slides of the deck", err);
      toast({ message: refusal(t, err), error: true });
    });
  }

  const n = deck.selected;
  function ask(at: number) {
    const list = deck.list;
    if (!list || list.slides.length <= 1) return;
    setDeleting({ n: at, base: list.version, title: list.slides.find((s) => s.number === at)?.title ?? "" });
  }
  const footerEdits: FooterEdits | null = artifact.canEdit ? {
    busy: deck.busy || deck.list === null,
    onAdd: () => { run({ op: "add", after: Math.min(n, count) }); },
    onDuplicate: () => { run({ op: "duplicate", n }); },
    onMove: (step) => { run({ op: "move", n, to: n + step }); },
    onDelete: () => { ask(n); },
  } : null;
  const thumbEdits: ThumbnailEdits | null = artifact.canEdit ? {
    onMove: (from, to) => { run({ op: "move", n: from, to }); },
    onDelete: ask,
  } : null;
  const thumbnails = (layout: "strip" | "grid") => (
    <Thumbnails
      id={artifact.id}
      version={deck.list?.version ?? artifact.version}
      slides={slides}
      selected={n}
      onSelect={deck.select}
      layout={layout}
      edits={thumbEdits}
      {...(layout === "grid" ? { onOpen: (at: number) => { deck.select(at); setView("single"); } } : {})}
    />
  );

  return (
    <>
      <div className="deck-view">
        <div className="deck-stage">
          <div ref={area} className="deck-scroll">
            <div className="deck-paper" style={{ width: `${String(stage.size.width)}px`, height: `${String(stage.size.height)}px` }}>
              <StageFrame key={artifact.version} id={artifact.id} title={artifact.title} frame={frame} sandbox={sandbox} startAt={n} />
              {editing && <SlideFields frame={frame} selected={n} edit={content} size={stage.size} want={want} />}
            </div>
          </div>
          {view === "grid" && <div className="deck-grid">{thumbnails("grid")}</div>}
          {editing && <SlideTools edit={content} overflows={slides[n - 1]?.overflows === true} onInserted={(place) => { setWant({ n, place }); }} />}
          <DeckBar exports={exports} onExport={onExport} onPresent={stage.present} panel={notes} percent={stage.percent} onZoom={setZoom} />
        </div>
        <DeckFooter selected={n} count={count} view={view} onView={setView} edits={footerEdits}>
          {deck.failed ? (
            <p className="deck-failed" role="alert">
              {t("deck.listFailed")}
              <button type="button" className="quiet" onClick={deck.retry}>{t("deck.retry")}</button>
            </p>
          ) : view === "single" && thumbnails("strip")}
        </DeckFooter>
      </div>
      {notes.open && <NotesPanel id={artifact.id} version={artifact.version} slide={n} onClose={notes.onToggle} />}
      {artifact.canEdit && (
        <ConfirmDialog
          open={deleting !== null}
          onOpenChange={(open) => { if (!open) setDeleting(null); }}
          title={format(t("deck.confirmDelete"), { n: String(deleting?.n ?? n) })}
          hint={format(t("deck.confirmDeleteHint"), { title: deleting?.title ?? "" })}
          confirm={t("deck.delete")}
          failure={t("deck.deleteFailed")}
          danger
          onConfirm={async () => {
            if (deleting !== null) await deck.edit({ op: "delete", n: deleting.n, base: deleting.base });
            setDeleting(null);
          }}
        />
      )}
    </>
  );
}

/** The deck's page for one version, opened at the slide selected when the version arrived. */
function StageFrame({ id, title, frame, sandbox, startAt }: { id: string; title: string; frame: RefObject<HTMLIFrameElement | null>; sandbox: string; startAt: number }) {
  const [src] = useState(() => `/raw/${id}?slide=${String(startAt)}&embed`);
  return <iframe ref={frame} data-artifact-frame="" src={src} allow="fullscreen" allowFullScreen title={title} sandbox={sandbox} />;
}
