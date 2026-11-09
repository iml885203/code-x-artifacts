import { useEffect, useId, useRef, useState } from "react";
import { ApiError, setEditors, type Artifact, type EditorsChange, type Visibility } from "../api";
import { initialOf, colorClassOf } from "../presence/people";
import { CopyButton } from "../Copy";
import { format, useT, type Key } from "../i18n";
import { audienceHint, audienceIcon, audienceLabel, audiences } from "./audience";
import "./share.css";

interface Props {
  artifact: Artifact;
  visibility: Visibility;
  /** Whether this deployment has public links at all. */
  publicLinks: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** A choice is saved at once; resolves to whether the server kept it. */
  onVisibility: (v: Visibility) => Promise<boolean>;
  onChanged: (saved: Artifact) => void;
}

/** What someone has on this artifact, for the list of people with access. */
function people(a: Artifact, visibility: Visibility, publicLinks: boolean): { name: string; role: Key; everyone?: Key }[] {
  const wide = visibility !== "private";
  return [
    { name: a.author, role: "share.owner" },
    ...a.editors.map((name) => ({ name, role: "share.editor" as const })),
    ...(wide ? [{ name: "", everyone: "share.everyone" as const, role: a.organisationEdit ? ("share.editor" as const) : ("share.reader" as const) }] : []),
    ...(visibility === "public" && publicLinks ? [{ name: "", everyone: "share.anyone" as const, role: "share.reader" as const }] : []),
  ];
}

/** Access changes are saved individually; dismissing never rolls back a saved change. */
export function ShareDialog({ artifact, visibility, publicLinks, open, onOpenChange, onVisibility, onChanged }: Props) {
  const t = useT();
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  const backdropPress = useRef(false);
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [account, setAccount] = useState("");
  const [problem, setProblem] = useState("");
  const [said, setSaid] = useState<{ was: Visibility; now: Visibility } | "saved" | null>(null);

  async function save(request: () => Promise<boolean>, failure: string): Promise<boolean> {
    if (pending.current || !artifact.canChange) return false;
    pending.current = true;
    setBusy(true);
    setProblem("");
    setSaid(null);
    try {
      if (await request()) return true;
      setProblem(failure);
    } catch (err) {
      setProblem(err instanceof ApiError && err.reason !== "" ? `${failure} ${err.reason}` : failure);
    } finally {
      pending.current = false;
      setBusy(false);
    }
    return false;
  }

  async function choose(next: Visibility, undoable: boolean) {
    const was = visibility;
    if (await save(() => onVisibility(next), t("viewer.changeFailed"))) {
      setSaid(undoable ? { was, now: next } : "saved");
    }
  }

  async function editors(change: EditorsChange) {
    const saved = await save(async () => {
      onChanged(await setEditors(artifact.id, change));
      return true;
    }, t("editors.failed"));
    if (saved) setSaid("saved");
    return saved;
  }
  // The browser's modal dialog does the rest, as in ConfirmDialog: focus trap, inert page, Escape, focus back.
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      setSaid(null);
      setProblem("");
      setAccount("");
      d.showModal();
    }
    if (!open && d.open) d.close();
  }, [open]);

  function isBackdrop(e: { target: EventTarget; currentTarget: HTMLDialogElement; clientX: number; clientY: number }) {
    if (e.target !== e.currentTarget) return false;
    const box = e.currentTarget.getBoundingClientRect();
    return e.clientX < box.left || e.clientX > box.right || e.clientY < box.top || e.clientY > box.bottom;
  }

  const hint = (a: Visibility) => (a !== "public" ? t(audienceHint[a]) : publicLinks ? t("visibility.public.hint") : visibility === "public" ? t("visibility.public.hintOff") : t("visibility.public.unavailable"));
  return (
    <dialog
      ref={ref}
      className="dialog share-dialog"
      aria-labelledby={`${id}-title`}
      onPointerDown={(e) => {
        backdropPress.current = e.isPrimary && e.button === 0 && isBackdrop(e);
      }}
      onPointerCancel={() => { backdropPress.current = false; }}
      onClick={(e) => {
        const startedOutside = backdropPress.current;
        backdropPress.current = false;
        // Native backdrop events target the dialog, as does its interior padding.
        if (startedOutside && e.button === 0 && e.detail > 0 && isBackdrop(e)) onOpenChange(false);
      }}
      onCancel={(e) => {
        e.preventDefault();
        onOpenChange(false);
      }}
    >
      <h2 id={`${id}-title`} className="dialog-title">
        {format(t("share.dialog"), { title: artifact.title })}
      </h2>
      <div className="share-said" aria-live="polite">
        {problem !== "" && <p className="error" role="alert">{problem}</p>}
        {busy ? <p role="status">{t("share.saving")}</p> : said !== null && (
          <p role="status">
            <span>{said === "saved" ? t("share.saved") : format(t("viewer.changedTo"), { visibility: t(audienceLabel[said.now]) })}</span>
            {said !== "saved" && <button type="button" className="quiet" onClick={() => { void choose(said.was, false); }}>{t("viewer.undo")}</button>}
          </p>
        )}
      </div>
      <div className="share-body">
        <h3 id={`${id}-audience-title`} className="share-people-title">{t("share.title")}</h3>
        {artifact.canChange ? (
          <div className="share-rows" role="radiogroup" aria-labelledby={`${id}-audience-title`}>
            {audiences.map((a) => {
              const Icon = audienceIcon[a];
              // A public choice stays usable once made, even after the server turned public links off.
              const off = a === "public" && !publicLinks && visibility !== "public";
              return (
                <label key={a} className="share-row" data-chosen={a === visibility}>
                  <input
                    type="radio"
                    name={`${id}-audience`}
                    checked={a === visibility}
                    disabled={off || busy}
                    aria-describedby={`${id}-${a}`}
                    onChange={() => {
                      void choose(a, true);
                    }}
                  />
                  <Icon />
                  <span className="share-row-text">
                    <span className="share-row-title">{t(audienceLabel[a])}</span>
                    <span id={`${id}-${a}`} className="share-row-hint">
                      {hint(a)}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
        ) : (
          <p className="share-now">
            {(() => {
              const Icon = audienceIcon[visibility];
              return <Icon />;
            })()}
            <span className="share-row-text">
              <span className="share-row-title">{t(audienceLabel[visibility])}</span>
              <span className="share-row-hint">{hint(visibility)}</span>
            </span>
          </p>
        )}
        <div className="share-link">
          <input readOnly aria-label={t("share.link")} value={`${location.origin}/a/${artifact.id}`} onFocus={(e) => { e.currentTarget.select(); }} />
          <CopyButton text={() => `${location.origin}/a/${artifact.id}`}>{t("viewer.copyLink")}</CopyButton>
        </div>
        <h3 className="share-people-title">{t("share.people")}</h3>
        <ul className="share-people">
          {people(artifact, visibility, publicLinks).map((p) => (
            <li key={p.everyone ?? p.name}>
              <span className="share-person">
                <span className={`pr-face ${p.everyone !== undefined ? "share-everyone" : colorClassOf({ id: p.name, name: p.name, agent: false, self: false })}`} aria-hidden="true">
                  {p.everyone !== undefined ? (() => { const Icon = audienceIcon[visibility]; return <Icon />; })() : initialOf(p.name)}
                </span>
                <span className="share-name">{p.everyone !== undefined ? t(p.everyone) : p.name}</span>
              </span>
              <span className="share-person-actions">
                <span className="share-role">{t(p.role)}</span>
                {artifact.canChange && p.everyone === undefined && p.name !== artifact.author && (
                  <button type="button" className="quiet" disabled={busy} aria-label={`${t("editors.remove")} ${p.name}`} onClick={() => { void editors({ remove: [p.name] }); }}>{t("editors.remove")}</button>
                )}
              </span>
            </li>
          ))}
        </ul>
        {artifact.canChange && <div className="share-editing">
          <form onSubmit={(e) => {
            e.preventDefault();
            const name = account.trim();
            if (name !== "") void editors({ add: [name] }).then((saved) => { if (saved) setAccount(""); });
          }}>
            <label htmlFor={`${id}-account`}>{t("editors.account")}</label>
            <div className="share-add">
              <input id={`${id}-account`} value={account} autoComplete="off" autoCapitalize="none" spellCheck={false} disabled={busy} onChange={(e) => { setAccount(e.target.value); }} aria-describedby={`${id}-account-hint`} />
              <button type="submit" disabled={busy || account.trim() === ""}>{t("editors.add")}</button>
            </div>
            <p id={`${id}-account-hint`} className="share-help">{t("share.accountHint")}</p>
          </form>
          {visibility !== "private" && <label className="share-edit-switch">
            <input type="checkbox" checked={artifact.organisationEdit} disabled={busy} onChange={(e) => { void editors({ organisationEdit: e.target.checked }); }} />
            <span>{t("editors.organisation")}</span>
          </label>}
          <p className="share-help">{t("editors.hint")}</p>
        </div>}
      </div>
      <div className="dialog-actions">
        {artifact.canChange && <p className="share-help">{t("share.immediate")}</p>}
        <button
          type="button"
          className="primary"
          autoFocus
          onClick={() => {
            onOpenChange(false);
          }}
        >
          {t("share.done")}
        </button>
      </div>
    </dialog>
  );
}
