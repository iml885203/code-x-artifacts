import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useRef, type ReactNode } from "react";
import { Chevron, TextIcon } from "../design/toolbar/icons";
import { format, useT, type Translate } from "../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../menuFocus";
import { useToast } from "../Toast";
import { Tooltip } from "../Tooltip";
import { canInsert, firstPlace, newElement, type Insertable, type Words } from "./fields";
import { BulletsIcon, RedoIcon, ShapeIcon, TableIcon, UndoIcon } from "./icons";
import { refusal } from "./useDeck";
import type { SlideEdit } from "./useSlideEdit";
import "./edit.css";

interface Props {
  edit: SlideEdit;
  /** The review says the selected slide is likely to overflow. */
  overflows: boolean;
  /** An element was added: the place to start typing in. */
  onInserted: (place: string) => void;
}

function words(t: Translate): Words {
  return {
    text: t("slideedit.newText"),
    point: t("slideedit.newPoint"),
    column: (n) => format(t("slideedit.column"), { n: String(n) }),
    card: (n) => format(t("slideedit.card"), { n: String(n) }),
    cardText: t("slideedit.cardText"),
    statValue: t("slideedit.statValue"),
    statLabel: t("slideedit.statLabel"),
  };
}

const names = { text: "slideedit.text", bullets: "slideedit.bullets", table: "slideedit.table", cards: "slideedit.cards", stats: "slideedit.stats" } as const;

function Tool({ label, disabled, onClick, children }: { label: string; disabled: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <Tooltip content={label}>
      <button type="button" className="deck-tool" aria-label={label} disabled={disabled} onClick={onClick}>
        {children}
      </button>
    </Tooltip>
  );
}

function ShapeMenu({ disabled, onPick }: { disabled: boolean; onPick: (kind: Insertable) => void }) {
  const t = useT();
  const button = useRef<HTMLButtonElement>(null);
  return (
    <DropdownMenu.Root modal={false}>
      <Tooltip content={t("slideedit.shape")}>
        <DropdownMenu.Trigger ref={button} className="deck-tool deck-tool-wide" aria-label={t("slideedit.shape")} disabled={disabled}>
          <ShapeIcon />
          <Chevron />
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, button.current); }} onCloseAutoFocus={keepFocusWhereItWent} className="menu bar-menu" align="start" sideOffset={8}>
          {(["cards", "stats"] as const).map((k) => (
            <DropdownMenu.Item key={k} className="menu-item" onSelect={() => { onPick(k); }}>
              {t(names[k])}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

const typing = (el: EventTarget | null) => el instanceof HTMLElement && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName));

/** Ctrl or Cmd+Z undoes, with Shift (or Ctrl+Y) redoes, unless the person is typing somewhere. */
function useUndoKeys(edit: SlideEdit) {
  useEffect(() => {
    const press = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.altKey || typing(e.target)) return;
      const key = e.key.toLowerCase();
      const back = key === "z" && !e.shiftKey;
      const ahead = (key === "z" && e.shiftKey) || key === "y";
      if (!back && !ahead) return;
      e.preventDefault();
      void (back ? edit.undo() : edit.redo());
    };
    window.addEventListener("keydown", press);
    return () => {
      window.removeEventListener("keydown", press);
    };
  }, [edit]);
}

/** The editor's floating tools at the top left of the stage, and what the stage has to say about the slide. */
export function SlideTools({ edit, overflows, onInserted }: Props) {
  const t = useT();
  const toast = useToast();
  useUndoKeys(edit);
  const slide = edit.slide;
  const full = slide !== null && !canInsert(slide);
  const off = slide === null || edit.busy || full;

  function insert(kind: Insertable) {
    if (!slide) return;
    const at = Array.isArray(slide.elements) ? slide.elements.length : 0;
    edit.save({ op: "insert", kind, element: newElement(kind, words(t)) }).then(
      (saved) => { if (saved === "saved") onInserted(firstPlace(kind, at)); },
      (err: unknown) => {
        console.error("insert an element on the slide", err);
        toast({ message: refusal(t, err), error: true });
      },
    );
  }

  function travel(run: () => Promise<void>) {
    run().catch((err: unknown) => {
      console.error("undo or redo a change to the slide", err);
      toast({ message: refusal(t, err), error: true });
    });
  }

  const c = edit.conflict;
  return (
    <>
      <div className="deck-bar deck-insert" role="toolbar" aria-label={t("slideedit.insert")}>
        <Tool label={full ? t("slideedit.full") : t(names.text)} disabled={off} onClick={() => { insert("text"); }}><TextIcon /></Tool>
        <Tool label={full ? t("slideedit.full") : t(names.bullets)} disabled={off} onClick={() => { insert("bullets"); }}><BulletsIcon /></Tool>
        <Tool label={full ? t("slideedit.full") : t(names.table)} disabled={off} onClick={() => { insert("table"); }}><TableIcon /></Tool>
        <ShapeMenu disabled={off} onPick={insert} />
        <span className="deck-bar-sep" aria-hidden="true" />
        <Tool label={t("slideedit.undo")} disabled={!edit.canUndo} onClick={() => { travel(edit.undo); }}><UndoIcon /></Tool>
        <Tool label={t("slideedit.redo")} disabled={!edit.canRedo} onClick={() => { travel(edit.redo); }}><RedoIcon /></Tool>
      </div>
      {(c !== null || overflows) && (
        <div className="slide-notes">
          {overflows && <p className="slide-note" role="status">{t("slideedit.overflow")}</p>}
          {c && (
            <div className="slide-note slide-conflict" role="alert">
              <p>{c.gone ? t("slideedit.gone") : t("slideedit.stale")}</p>
              <p className="slide-kept">{c.change.op === "text" ? c.change.text : t(names[c.change.kind])}</p>
              <div className="slide-note-actions">
                {!c.gone && (
                  <button type="button" className="btn" disabled={edit.busy} onClick={() => { travel(edit.applyConflict); }}>
                    {t("slideedit.apply")}
                  </button>
                )}
                <button type="button" className="btn" onClick={edit.dropConflict}>{t("slideedit.discard")}</button>
              </div>
            </div>
          )}
        </div>
      )}
    </>
  );
}
