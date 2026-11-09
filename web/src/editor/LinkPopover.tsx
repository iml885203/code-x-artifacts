// The Cmd+K popover (address and text, Save, Cancel) and the bar that shows
// the link the caret is in (open, change, remove). A refused address stays in
// the field with the reason beside it; nothing in the document changes until
// Save is accepted.
import { useState, type SyntheticEvent } from "react";
import { format, useT } from "../i18n";
import { checkAddress, type LinkRange } from "./link";

interface FormProps {
  draft: LinkRange;
  onSave: (text: string, href: string) => void;
  onCancel: () => void;
}

export function LinkForm({ draft, onSave, onCancel }: FormProps) {
  const t = useT();
  const [address, setAddress] = useState(draft.href);
  const [text, setText] = useState(draft.text);
  const [reason, setReason] = useState<string | null>(null);

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    const checked = checkAddress(address);
    if ("href" in checked) {
      onSave(text, checked.href);
    } else {
      setReason(checked.refused === "empty" ? t("editor.linkEmpty") : format(t("editor.linkRefused"), { address: checked.address }));
    }
  };

  return (
    <form className="ed-link-form" role="dialog" aria-label={t("editor.link")} onSubmit={submit}>
      <label>
        {t("editor.linkAddress")}
        <input
          type="text"
          value={address}
          autoFocus
          aria-invalid={reason !== null}
          aria-describedby={reason === null ? undefined : "ed-link-reason"}
          onChange={(e) => {
            setAddress(e.target.value);
            setReason(null);
          }}
        />
      </label>
      <label>
        {t("editor.linkText")}
        <input
          type="text"
          value={text}
          onChange={(e) => {
            setText(e.target.value);
          }}
        />
      </label>
      <button type="submit">{t("editor.linkSave")}</button>
      <button type="button" className="quiet" onClick={onCancel}>
        {t("editor.linkCancel")}
      </button>
      {reason !== null && (
        <p id="ed-link-reason" role="alert" className="ed-link-reason">
          {reason}
        </p>
      )}
    </form>
  );
}

interface BarProps {
  link: LinkRange;
  onChange: () => void;
  onRemove: () => void;
}

/** The link the caret is in. The address is shown as text and opened by its own button, so the editor's click never navigates. */
export function LinkBar({ link, onChange, onRemove }: BarProps) {
  const t = useT();
  return (
    <div className="ed-link-bar" role="group" aria-label={t("editor.link")}>
      <span className="ed-link-href">{link.href}</span>
      <a href={link.href} target="_blank" rel="noopener noreferrer nofollow">
        {t("editor.linkOpen")}
      </a>
      <button type="button" className="quiet" onClick={onChange}>
        {t("editor.linkChange")}
      </button>
      <button type="button" className="quiet" onClick={onRemove}>
        {t("editor.linkRemove")}
      </button>
    </div>
  );
}
