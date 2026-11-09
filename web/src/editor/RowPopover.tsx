// A button that opens a small panel under it, for the insert row's Tabs and More. A menu would not do: the panels hold
// tools that stay open while they are used (a tab being renamed, a mark being toggled). Escape and a press outside close it.
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { ChevronDown } from "../icons/editor";

interface Props {
  /** The button's name, and the panel's. */
  label: string;
  /** What the button shows next to its chevron; the label when absent. */
  face?: ReactNode;
  /** The panel's contents; `close` shuts it (choosing a tab does). */
  children: (close: () => void) => ReactNode;
}

export function RowPopover({ label, face, children }: Props) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const panel = useId();

  useEffect(() => {
    if (!open) return;
    const outside = (e: PointerEvent) => {
      if (e.target instanceof Node && !root.current?.contains(e.target)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
      button.current?.focus();
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);

  return (
    <div className="ed-pop" ref={root}>
      <button
        ref={button}
        type="button"
        className="quiet ed-pop-button"
        aria-expanded={open}
        aria-controls={open ? panel : undefined}
        aria-label={label}
        onClick={() => {
          setOpen(!open);
        }}
      >
        {face ?? label}
        <ChevronDown />
      </button>
      {open && (
        <div id={panel} className="ed-pop-panel" role="group" aria-label={label}>
          {children(() => {
            setOpen(false);
          })}
        </div>
      )}
    </div>
  );
}
