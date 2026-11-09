import { useEffect, useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { format, useT } from "../i18n";
import type { SlideEntry } from "./api";
import { gapAt, moveTarget } from "./order";

/** What an editor can do from the thumbnails; absent for a reader, who only moves between slides. */
export interface ThumbnailEdits {
  onMove: (n: number, to: number) => void;
  onDelete: (n: number) => void;
}

interface Props {
  id: string;
  /** The version the thumbnails show: a new one reloads them. */
  version: number;
  slides: SlideEntry[];
  selected: number;
  onSelect: (n: number) => void;
  layout: "strip" | "grid";
  /** In the grid, Enter or a second click opens the slide on the stage. */
  onOpen?: (n: number) => void;
  edits: ThumbnailEdits | null;
}

const previous = new Set(["ArrowLeft", "ArrowUp"]);
const following = new Set(["ArrowRight", "ArrowDown"]);

/**
 * The slides of a deck as numbered thumbnails. Each is the deck's preview (`/raw/{id}?preview`, no script but the
 * platform's stub, the same sandbox as the page) opened at its slide by the address's fragment, in a frame that takes
 * no pointer, so nothing the deck holds runs outside the sandbox and a drag is the button's. One button is in the tab
 * order; the arrow keys move the selection.
 */
export function Thumbnails({ id, version, slides, selected, onSelect, layout, onOpen, edits }: Props) {
  const t = useT();
  const list = useRef<HTMLOListElement>(null);
  const keepFocus = useRef(false);
  const dragging = useRef<number | null>(null);
  const [gap, setGap] = useState<number | null>(null);
  const count = slides.length;

  useEffect(() => {
    if (!keepFocus.current) return;
    keepFocus.current = false;
    list.current?.querySelector<HTMLButtonElement>(`[data-slide="${String(selected)}"]`)?.focus();
  }, [selected, version]);

  function go(n: number) {
    keepFocus.current = true;
    onSelect(Math.min(Math.max(1, n), count));
  }

  function key(e: KeyboardEvent, n: number) {
    const step = previous.has(e.key) ? -1 : following.has(e.key) ? 1 : 0;
    if (step !== 0 && e.altKey && edits) {
      const to = n + step;
      if (to >= 1 && to <= count) {
        keepFocus.current = true;
        edits.onMove(n, to);
      }
    } else if (step !== 0) {
      go(n + step);
    } else if (e.key === "Home" || e.key === "End") {
      go(e.key === "Home" ? 1 : count);
    } else if ((e.key === "Delete" || e.key === "Backspace") && edits) {
      edits.onDelete(n);
    } else if (e.key === "Enter" && onOpen) {
      onOpen(n);
    } else {
      return;
    }
    e.preventDefault();
  }

  function over(e: DragEvent<HTMLButtonElement>, n: number) {
    if (dragging.current === null) return;
    e.preventDefault();
    const box = e.currentTarget.getBoundingClientRect();
    setGap(gapAt(n, e.clientX, box.left, box.width));
  }

  function drop(e: DragEvent) {
    e.preventDefault();
    const from = dragging.current;
    const to = from === null || gap === null ? null : moveTarget(from, gap, count);
    dragging.current = null;
    setGap(null);
    if (from !== null && to !== null) edits?.onMove(from, to);
  }

  return (
    <ol ref={list} className={`deck-thumbs deck-thumbs-${layout}`} aria-label={t("deck.filmstrip")}>
      {slides.map((s) => {
        const label = s.title.trim() === "" ? format(t("deck.slideUntitled"), { n: String(s.number) }) : format(t("deck.slide"), { n: String(s.number), title: s.title });
        const marks = [gap === s.number ? "drop-before" : "", gap === s.number + 1 ? "drop-after" : ""].join(" ").trim();
        return (
          <li key={s.number} className={marks}>
            <button
              type="button"
              className="deck-thumb"
              data-slide={s.number}
              aria-label={label}
              aria-current={s.number === selected ? "true" : undefined}
              tabIndex={s.number === selected ? 0 : -1}
              draggable={edits !== null}
              onClick={() => {
                if (s.number === selected && onOpen) onOpen(s.number);
                else onSelect(s.number);
              }}
              onKeyDown={(e) => { key(e, s.number); }}
              onDragStart={edits ? (e) => {
                dragging.current = s.number;
                e.dataTransfer.effectAllowed = "move";
                e.dataTransfer.setData("text/plain", String(s.number));
              } : undefined}
              onDragOver={edits ? (e) => { over(e, s.number); } : undefined}
              onDrop={edits ? drop : undefined}
              onDragEnd={edits ? () => { dragging.current = null; setGap(null); } : undefined}
            >
              <iframe
                key={`${String(version)}-${String(s.number)}`}
                src={`/raw/${id}?preview#slide-${String(s.number)}`}
                sandbox="allow-scripts"
                loading="lazy"
                tabIndex={-1}
                aria-hidden="true"
                title=""
              />
              <span className="deck-thumb-number" aria-hidden="true">{s.number}</span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
