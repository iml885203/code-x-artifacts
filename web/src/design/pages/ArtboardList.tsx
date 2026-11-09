import { useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { Plus } from "../../icons/chrome";
import { format, useT } from "../../i18n";
import type { DesignFrame, DesignPage } from "../manifest";
import { type Outcome } from "./rules";
import type { Drop, PageStructure } from "./structure";
import { refusalText } from "./words";

interface Props {
  page: DesignPage;
  /** Null for someone who may not change the design: the list is only read. */
  structure: PageStructure | null;
  /** Called after a frame was added, so the canvas can bring the page into view. */
  onAdded?: () => void;
  selected?: string | null;
  onSelect?: (id: string) => void;
}

/**
 * The frames of the page, row by row: select, add, rename (double-click or F2),
 * and drag a frame before another, to the end of a row or into a row of its own. Each is a step of
 * the person's own history.
 */
export function ArtboardList({ page, structure, onAdded, selected, onSelect }: Props) {
  const t = useT();
  const [open, setOpen] = useState<string | null>(null);
  const [lastSelected, setLastSelected] = useState(selected);
  if (lastSelected !== selected) {
    setLastSelected(selected);
    setOpen(selected ?? null);
  }
  const [renaming, setRenaming] = useState<string | null>(null);
  const [problem, setProblem] = useState("");
  const [drag, setDrag] = useState<string | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const cancelled = useRef(false);
  const editable = structure !== null;

  const done = (outcome: Outcome) => {
    setProblem(refusalText(t, outcome) ?? "");
    return outcome;
  };

  const add = () => {
    if (!structure) return;
    const out = done(structure.addFrame(page.id, (n) => format(t("structure.newFrame"), { n: String(n) }), open ?? undefined));
    if (out.kind === "changed") {
      if (out.frame) setOpen(out.frame);
      onAdded?.();
    }
  };

  const drop = (to: Drop) => {
    const id = drag;
    setDrag(null);
    setOver(null);
    if (structure && id) done(structure.moveFrame(id, to));
  };

  /** Over the top half of a frame drops before it; over the bottom half, after it (before the next one, or at the end of the row). */
  const dropOnFrame = (e: DragEvent<HTMLElement>, row: DesignFrame[], at: number, r: number): Drop => {
    const box = e.currentTarget.getBoundingClientRect();
    const top = box.height <= 0 || e.clientY - box.top < box.height / 2;
    if (top) return { before: row[at]?.id ?? "" };
    const next = row[at + 1];
    return next ? { before: next.id } : { endOf: r };
  };

  const onKey = (e: KeyboardEvent<HTMLButtonElement>, frame: DesignFrame, row: DesignFrame[], r: number) => {
    if (!structure) return;
    const at = row.findIndex((f) => f.id === frame.id);
    if (e.key === "F2") {
      e.preventDefault();
      setRenaming(frame.id);
    } else if (e.altKey && e.key === "ArrowUp" && at > 0) {
      e.preventDefault();
      done(structure.moveFrame(frame.id, { before: row[at - 1]?.id ?? "" }));
    } else if (e.altKey && e.key === "ArrowDown" && at >= 0 && at < row.length - 1) {
      e.preventDefault();
      const after = row[at + 2];
      done(structure.moveFrame(frame.id, after ? { before: after.id } : { endOf: r }));
    }
  };

  const zone = (key: string, to: Drop, label?: string) => (
    <li
      key={key}
      className={`fnewrow${over === key ? " is-over" : ""}`}
      data-newrow={key}
      aria-hidden="true"
      onDragOver={(e) => {
        if (!drag) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        if (over !== key) setOver(key);
      }}
      onDragLeave={() => {
        if (over === key) setOver(null);
      }}
      onDrop={(e) => {
        e.preventDefault();
        drop(to);
      }}
    >
      {drag ? label : null}
    </li>
  );

  return (
    <section className={`frames${drag ? " is-dragging" : ""}`} aria-label={t("structure.frames")}>
      <div className="frames-head">
        <h3 className="outline-frame">{t("structure.frames")}</h3>
        {editable && (
          <button type="button" className="frames-add" aria-label={t("structure.addFrame")} title={t("structure.addFrame")} onClick={add}>
            <Plus />
          </button>
        )}
      </div>
      <ul className="frames-rows">
        {page.rows.flatMap((row, r) => [
          editable ? zone(`before-${String(r)}`, { newRowBefore: r }, t("structure.newRow")) : null,
          <li
            key={`row-${String(r)}`}
            className={`frow${over === `row-${String(r)}` ? " is-over" : ""}`}
            data-row-index={r}
            onDragOver={(e) => {
              if (!drag) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
              if (over !== `row-${String(r)}`) setOver(`row-${String(r)}`);
            }}
            onDrop={(e) => {
              e.preventDefault();
              drop({ endOf: r });
            }}
          >
            <span className="frow-label">{format(t("structure.row"), { n: String(r + 1) })}</span>
            <ul className="frow-frames">
              {row.map((frame, at) => (
                <li
                  key={frame.id}
                  className={`fitem${open === frame.id ? " is-open" : ""}${drag === frame.id ? " is-dragged" : ""}`}
                  data-frame-item={frame.id}
                  draggable={editable && renaming !== frame.id}
                  onDragStart={(e) => {
                    e.dataTransfer.effectAllowed = "move";
                    e.dataTransfer.setData("text/plain", frame.id);
                    setDrag(frame.id);
                  }}
                  onDragEnd={() => {
                    setDrag(null);
                    setOver(null);
                  }}
                  onDragOver={(e) => {
                    if (!drag || drag === frame.id) return;
                    e.preventDefault();
                    e.stopPropagation();
                    e.dataTransfer.dropEffect = "move";
                  }}
                  onDrop={(e) => {
                    if (!drag) return;
                    e.preventDefault();
                    e.stopPropagation();
                    drop(dropOnFrame(e, row, at, r));
                  }}
                >
                  <div className="fitem-line">
                    {renaming === frame.id ? (
                      <input
                        className="orow-name"
                        aria-label={format(t("structure.renameFrame"), { name: frame.name })}
                        defaultValue={frame.name}
                        maxLength={200}
                        ref={(el) => el?.focus()}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") e.currentTarget.blur();
                          else if (e.key === "Escape") {
                            cancelled.current = true;
                            e.currentTarget.blur();
                          }
                        }}
                        onBlur={(e) => {
                          if (!cancelled.current && structure) done(structure.renameFrame(frame.id, e.currentTarget.value));
                          cancelled.current = false;
                          setRenaming(null);
                        }}
                      />
                    ) : (
                      <button
                        type="button"
                        className="fitem-main"
                        aria-pressed={onSelect ? selected === frame.id : undefined}
                        aria-expanded={structure ? open === frame.id : undefined}
                        onClick={() => {
                          setOpen(open === frame.id ? null : frame.id);
                          onSelect?.(frame.id);
                        }}
                        onDoubleClick={() => {
                          if (editable) setRenaming(frame.id);
                        }}
                        onKeyDown={(e) => {
                          onKey(e, frame, row, r);
                        }}
                      >
                        <span className="orow-label named">{frame.name}</span>

                      </button>
                    )}
                  </div>

                </li>
              ))}
            </ul>
          </li>,
        ])}
        {editable && zone("end", { newRowBefore: null }, t("structure.newRow"))}
      </ul>
      {problem && (
        <p className="pproblem" role="alert">
          {problem}
        </p>
      )}
    </section>
  );
}
