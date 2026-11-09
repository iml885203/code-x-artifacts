import { useEffect, useRef, useState, type ReactNode } from "react";
import { format, useT } from "../../i18n";
import { formatColor, hexOf, parseColor, type Rgba } from "./css";

/** A titled group of controls. */
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details className="prop-section" aria-label={title} open>
      <summary>{title}</summary>
      {children}
    </details>
  );
}

interface FieldProps {
  label: string;
  /** What the element has now; the box shows it again whenever it changes, and when what was typed is refused. */
  value: string;
  /** What was typed, when it is not what the element has. Returns false when it was refused. */
  onCommit: (text: string) => boolean;
  /** Short text shown before the box, when the label is long. */
  short?: string;
  disabled?: boolean;
  placeholder?: string | undefined;
  wide?: boolean;
}

/** A box that commits on Enter or when it loses focus: typing is not a step of the history, the commit is. */
export function Field({ label, value, onCommit, short, disabled, placeholder, wide }: FieldProps) {
  const [draft, setDraft] = useState(value);
  const committed = useRef<string | null>(null);
  useEffect(() => { committed.current = null; }, [value]);
  // What the element has changed (or what was typed was refused): the box shows it again.
  const [seen, setSeen] = useState(value);
  if (seen !== value) {
    setSeen(value);
    setDraft(value);
  }
  const commit = () => {
    if (draft.trim() === value.trim() || draft.trim() === committed.current) return;
    // Enter already commits the intent; a later blur must not replay it over a new binding.
    if (onCommit(draft)) committed.current = draft.trim();
    else setDraft(value);
  };
  return (
    <label className={wide ? "prop-field wide" : "prop-field"}>
      {short !== undefined && <span aria-hidden="true">{short}</span>}
      <input
        type="text"
        aria-label={label}
        title={label}
        value={draft}
        disabled={disabled}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
        onChange={(e) => {
          committed.current = null;
          setDraft(e.target.value);
        }}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            commit();
          } else if (e.key === "Escape") {
            setDraft(value);
          }
        }}
      />
    </label>
  );
}

export interface Choice<T extends string> {
  value: T;
  label: string;
}

/** A row of buttons of which one is pressed (none when the element has a value the row does not offer). */
export function Segmented<T extends string>({ label, choices, value, onChoose }: { label: string; choices: Choice<T>[]; value: T | null; onChoose: (value: T) => void }) {
  return (
    <div className="prop-segmented" role="group" aria-label={label}>
      {choices.map((c) => (
        <button
          key={c.value}
          type="button"
          aria-pressed={c.value === value}
          onClick={() => {
            if (c.value !== value) onChoose(c.value);
          }}
        >
          {c.label}
        </button>
      ))}
    </div>
  );
}

export function Toggle({ label, on, onToggle }: { label: string; on: boolean; onToggle: (on: boolean) => void }) {
  return (
    <button
      type="button"
      className="prop-toggle"
      aria-pressed={on}
      onClick={() => {
        onToggle(!on);
      }}
    >
      {label}
    </button>
  );
}

/** A channel edit is applied to the current room colour, not a delayed frame report. */
export type ColorChange = (current: Rgba) => Rgba;

/**
 * A colour: its value as hex, a swatch that opens the browser's picker, and an
 * alpha in percent. Alpha below 100 is written as rgba(). The picker commits when
 * it is closed, not for each colour it passes.
 */
export function ColorField({ label, color, onCommit }: { label: string; color: Rgba; onCommit: (change: ColorChange) => void }) {
  const t = useT();
  const picker = useRef<HTMLInputElement>(null);
  const held = useRef(onCommit);
  useEffect(() => {
    held.current = onCommit;
  });
  useEffect(() => {
    const el = picker.current;
    if (!el) return;
    const done = () => {
      const picked = parseColor(el.value);
      if (picked) held.current((current) => ({ ...picked, a: current.a }));
    };
    el.addEventListener("change", done);
    return () => {
      el.removeEventListener("change", done);
    };
  }, []);
  const typed = (text: string) => {
    const parsed = parseColor(text);
    if (!parsed) return false;
    // A plain colour keeps the alpha the element has; one that says its own (rgba, #rrggbbaa) wins.
    onCommit((current) => ({ ...parsed, a: parsed.a < 1 ? parsed.a : current.a }));
    return true;
  };
  return (
    <div className="prop-color" role="group" aria-label={label}>
      <input ref={picker} type="color" className="prop-swatch" aria-label={format(t("props.picker"), { name: label })} value={hexOf(color)} onChange={() => undefined} style={{ background: formatColor(color) }} />
      <Field label={format(t("props.colorValue"), { name: label })} value={hexOf(color)} onCommit={typed} />
      <Field
        label={`${label}, ${t("props.alpha")}`}
        value={String(Math.round(color.a * 100))}
        short="%"
        onCommit={(text) => {
          const n = Number(text);
          if (text.trim() === "" || !Number.isFinite(n)) return false;
          onCommit((current) => ({ ...current, a: Math.min(100, Math.max(0, n)) / 100 }));
          return true;
        }}
      />
    </div>
  );
}
