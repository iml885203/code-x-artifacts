import { useEffect, useState } from "react";
import { useT } from "../i18n";
import { getSlide } from "./api";

interface Props {
  id: string;
  version: number;
  slide: number;
  onClose: () => void;
}

type Load = { status: "loading" } | { status: "ready"; notes: string } | { status: "failed" };

/** The side panel: the speaker notes of the slide on the stage, read from the server as text. */
export function NotesPanel({ id, version, slide, onClose }: Props) {
  const t = useT();
  const [load, setLoad] = useState<Load>({ status: "loading" });
  useEffect(() => {
    let current = true;
    getSlide(id, slide).then(
      (read) => {
        if (current) setLoad({ status: "ready", notes: typeof read.slide.notes === "string" ? read.slide.notes : "" });
      },
      (err: unknown) => {
        console.error("read the notes of a slide", err);
        if (current) setLoad({ status: "failed" });
      },
    );
    return () => {
      current = false;
    };
  }, [id, version, slide]);

  return (
    <aside
      className="versions deck-notes"
      aria-label={t("deck.notes")}
      onKeyDown={(e) => {
        if (e.key === "Escape") onClose();
      }}
    >
      <header className="versions-head">
        <h2>{t("deck.notes")}</h2>
        <button type="button" className="quiet" onClick={onClose}>
          {t("deck.closeNotes")}
        </button>
      </header>
      <div className="deck-notes-body" aria-live="polite">
        {load.status === "failed" && <p className="error">{t("deck.notesFailed")}</p>}
        {load.status === "ready" && (load.notes.trim() === "" ? <p className="deck-notes-none">{t("deck.noNotes")}</p> : <p>{load.notes}</p>)}
      </div>
    </aside>
  );
}
