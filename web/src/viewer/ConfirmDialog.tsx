import { useEffect, useId, useRef, useState } from "react";
import { useT } from "../i18n";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  hint: string;
  /** What the confirming button says. */
  confirm: string;
  /** What the dialog says when the action failed; it then stays open. */
  failure: string;
  /** The action destroys something: the button is danger-coloured. */
  danger?: boolean;
  /** Rejects when the action did not happen; the dialog then stays and says so. */
  onConfirm: () => Promise<void>;
}

/** Asks once before something the person cannot take back with one click. */
export function ConfirmDialog({ open, onOpenChange, title, hint, confirm, failure, danger = false, onConfirm }: Props) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();

  // The browser's modal dialog does the rest: focus trap, inert page behind
  // it, Escape, focus back to where it was. Radix's modal dialogs lock scroll
  // by injecting a <style>, which the site CSP blocks (#46).
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  function close() {
    if (busy) return;
    setFailed(false);
    onOpenChange(false);
  }

  return (
    <dialog
      ref={ref}
      className="dialog"
      role="alertdialog"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-hint`}
      onCancel={(e) => {
        e.preventDefault(); // the open prop closes it
        close();
      }}
    >
      <h2 id={`${id}-title`} className="dialog-title">{title}</h2>
      <p id={`${id}-hint`} className="dialog-text">{hint}</p>
      {failed && (
        <p className="error" role="alert">
          {failure}
        </p>
      )}
      <div className="dialog-actions">
        <button type="button" autoFocus onClick={close}>{t("viewer.cancel")}</button>
        <button
          type="button"
          className={danger ? "danger" : "primary"}
          disabled={busy}
          onClick={() => {
            setBusy(true);
            setFailed(false);
            onConfirm().then(
              () => {
                setBusy(false);
              },
              () => {
                setBusy(false);
                setFailed(true);
              },
            );
          }}
        >
          {confirm}
        </button>
      </div>
    </dialog>
  );
}
