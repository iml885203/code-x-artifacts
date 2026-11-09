import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type RefObject } from "react";
import { format, useT } from "../i18n";
import { useToast } from "../Toast";
import { editable, multiline, textAt, type Slide } from "./fields";
import { placedFields, type Placed } from "./placement";
import { refusal } from "./useDeck";
import type { SlideEdit } from "./useSlideEdit";
import "./edit.css";

/** A place on slide `n` to start typing in once the page shows it: the first string of an element just inserted. Each insert is a new object. */
export interface Wanted {
  n: number;
  place: string;
}

interface Props {
  frame: RefObject<HTMLIFrameElement | null>;
  selected: number;
  edit: SlideEdit;
  /** The size the slide is drawn at on the stage. */
  size: { width: number; height: number };
  want: Wanted | null;
}

interface Open {
  n: number;
  path: string;
  text: string;
  from: string;
}

/** What the stage's frame last said about the places of its slide's strings, kept with the page that said it. */
function useReported(frame: RefObject<HTMLIFrameElement | null>) {
  const [said, setSaid] = useState<{ from: MessageEventSource; number: number; fields: Placed[] } | null>(null);
  useEffect(() => {
    const hear = (e: MessageEvent) => {
      const got = placedFields(e, frame.current);
      if (got && e.source) setSaid({ from: e.source, ...got });
    };
    window.addEventListener("message", hear);
    return () => {
      window.removeEventListener("message", hear);
    };
  }, [frame]);
  return said;
}

function box(f: Placed, size: Props["size"]): CSSProperties {
  return { left: f.x * size.width, top: f.y * size.height, width: f.w * size.width, height: f.h * size.height };
}

function typeOf(f: Placed, size: Props["size"]): CSSProperties {
  return { fontSize: f.size * size.width, lineHeight: `${String(f.line * size.width)}px`, fontWeight: f.weight, textAlign: f.align };
}

/**
 * An input method is composing (Chinese, Japanese): its Enter picks a candidate, not the text. Safari sends the
 * Enter that ends a composition with isComposing already false, and only its legacy key code tells.
 */
function composing(e: KeyboardEvent): boolean {
  // eslint-disable-next-line @typescript-eslint/no-deprecated -- keyCode 229 is the only sign Safari gives; SlideEditing.test.tsx (leaves Enter to an input method) guards it
  return e.nativeEvent.isComposing || e.keyCode === 229;
}

/**
 * The shell's own fields over the slide's strings, for an editor. The deck's page only says where each string is;
 * the text comes from the slide's JSON. Enter or clicking away saves, Escape puts it back, Shift+Enter breaks a line
 * of a text element; Enter while an input method is composing belongs to the input method.
 */
export function SlideFields({ frame, selected, edit, size, want }: Props) {
  const t = useT();
  const toast = useToast();
  const said = useReported(frame);
  const [editing, setEditing] = useState<Open | null>(null);
  const [took, setTook] = useState<Wanted | null>(null);
  const settled = useRef(false);
  const slide: Slide | null = edit.slide;
  const fields = said && slide && said.number === selected && said.from === frame.current?.contentWindow ? said.fields.filter((f) => editable(slide, f.path)) : [];
  const open = editing?.n === selected ? editing : null;
  // The field is a new element for each place (keyed by it), so this runs once, when it first opens.
  const opened = useCallback((el: HTMLTextAreaElement | null) => {
    el?.focus();
    el?.select();
  }, []);

  function begin(path: string) {
    const text = slide ? textAt(slide, path) : null;
    if (text === null) return;
    setEditing({ n: selected, path, text, from: text });
  }

  if (want && want !== took && want.n === selected && fields.some((f) => f.path === want.place)) {
    setTook(want);
    begin(want.place);
  }

  function cancel() {
    settled.current = true;
    setEditing(null);
  }

  function commit() {
    if (!open || settled.current) return;
    settled.current = true;
    if (open.text === open.from) {
      setEditing(null);
      return;
    }
    edit.save({ op: "text", path: open.path, text: open.text }).then(
      (saved) => {
        if (saved !== "skipped") {
          setEditing(null);
          return;
        }
        settled.current = false;
        toast({ message: t("deck.busy") });
      },
      (err: unknown) => {
        console.error("save the text of the slide", err);
        settled.current = false;
        toast({ message: refusal(t, err), error: true });
      },
    );
  }

  function keys(e: KeyboardEvent<HTMLTextAreaElement>, path: string) {
    if (composing(e)) return;
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      cancel();
    } else if (e.key === "Enter") {
      if (e.shiftKey && slide && multiline(slide, path)) return;
      e.preventDefault();
      if (!e.shiftKey) commit();
    }
  }

  function label(path: string): string {
    const text = textAt(slide, path);
    return format(t("slideedit.edit"), { text: text === null || text === "" ? t("slideedit.empty") : text });
  }

  const placed = open ? fields.find((f) => f.path === open.path) : undefined;
  return (
    <div className="slide-fields">
      {fields.map((f) =>
        f.path === open?.path ? null : (
          <button key={f.path} type="button" className="slide-field" style={box(f, size)} aria-label={label(f.path)} disabled={edit.busy} onClick={() => { begin(f.path); }} />
        ),
      )}
      {open && placed && (
        <textarea
          key={open.path}
          className="slide-input"
          style={{ ...box(placed, size), height: undefined, minHeight: placed.h * size.height, ...typeOf(placed, size) }}
          aria-label={t("slideedit.field")}
          value={open.text}
          readOnly={edit.busy}
          ref={opened}
          onChange={(e) => { setEditing({ ...open, text: e.target.value }); }}
          onKeyDown={(e) => { keys(e, open.path); }}
          onFocus={() => { settled.current = false; }}
          onBlur={commit}
        />
      )}
    </div>
  );
}
